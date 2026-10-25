package entity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// What the GM panel (internal/gmweb) does to the running world. A row of npcs
// edited, added or deleted there takes effect at once rather than at the next
// restart, and a player's level and purse change where the game holds them,
// in the Player, as well as in its row.

// NPCRefreshHold is how long an NPC the panel changed stays out of every view:
// more than two visibility rounds (VisibilityTickInterval ticks each), so every
// client is told to delete it before it is made again from its new record.
const NPCRefreshHold = 1200 * time.Millisecond

// UpsertNPC puts the NPC a row describes into the world, or makes the one there
// match it: alive, at full health, on its spawn point, out of any fight, with
// the class given. One already in the world keeps its pointer, so nothing that
// holds it (the spell effects) is left with a stale copy. It reports whether
// the NPC is new. An id held by a player or a GM's spawn cannot be placed: the
// spatial index knows one entity an id.
func (w *World) UpsertNPC(rec *database.NPCRecord, class int) (created bool, err error) {
	fresh := NPCFromRecord(rec, class, w.rangedModels)
	fresh.HiddenUntil = time.Now().Add(NPCRefreshHold)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.applyNPCSettingsLocked(fresh)
	if _, taken := w.Players[fresh.ServerID]; taken {
		return false, fmt.Errorf("entity id %d is a player's in the world now; the NPC comes at the next restart", fresh.ServerID)
	}
	if existing, ok := w.NPCs[fresh.ServerID]; ok {
		if existing.Temporary {
			return false, fmt.Errorf("entity id %d is a GM spawn's; despawn it first", fresh.ServerID)
		}
		oldX, oldZ := existing.X, existing.Z
		*existing = *fresh
		w.SpatialIndex.Update(existing.ServerID, oldX, oldZ, existing.X, existing.Z)
		delete(w.affected, existing.ServerID)
		return false, nil
	}
	w.NPCs[fresh.ServerID] = fresh
	w.SpatialIndex.Insert(fresh.ServerID, fresh.X, fresh.Z)
	return true, nil
}

// RemoveNPC takes an NPC out of the world at once, whether a row or a GM's
// spawn made it (DespawnNPC refuses a row's: this is for a row deleted). Each
// client deletes it at its next visibility round.
func (w *World) RemoveNPC(id EntityID) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, ok := w.NPCs[id]
	if !ok {
		return "", false
	}
	w.removeNPCLocked(npc)
	delete(w.affected, id)
	return npc.Name, true
}

// RespawnNPCNow brings an NPC back at once on its spawn point, alive and at
// full health, out of any fight (a corpse included).
func (w *World) RespawnNPCNow(id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, ok := w.NPCs[id]
	if !ok {
		return false
	}
	w.respawnNPCLocked(npc)
	npc.Effects = nil
	npc.endDummyFight()
	delete(w.affected, id)
	return true
}

// KillNPCNow kills an NPC outright (no swing, no experience): it lies as a
// corpse for its corpse time and comes back after its respawn time, as one a
// player killed. It reports false for an NPC not in the world or dead already.
func (w *World) KillNPCNow(id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, ok := w.NPCs[id]
	if !ok || npc.Dead || npc.Dummy != NotDummy {
		return false
	}
	npc.HP = 0
	w.killNPC(npc)
	return true
}

// NPCState is an NPC as the world holds it now: where it stands (which is
// not its row's place while it fights or after a GM moved its row), whether it
// is alive and whom it fights.
type NPCState struct {
	ID         EntityID `json:"id"`
	Name       string   `json:"name"`
	World      uint8    `json:"world"`
	X          float32  `json:"x"`
	Y          float32  `json:"y"`
	Z          float32  `json:"z"`
	Facing     float32  `json:"facing"`
	SpawnX     float32  `json:"spawnX"`
	SpawnY     float32  `json:"spawnY"`
	SpawnZ     float32  `json:"spawnZ"`
	Level      int      `json:"level"`
	HP         int      `json:"hp"`
	MaxHP      int      `json:"maxHP"`
	Dead       bool     `json:"dead"`
	Class      int      `json:"class"`
	Faction    byte     `json:"faction"`
	Temporary  bool     `json:"temporary"`
	Fighting   string   `json:"fighting,omitempty"`
	ModelID    int64    `json:"modelID"`
	NPCType    int      `json:"npcType"`
	RespawnSec float64  `json:"respawnSec"`
}

func (w *World) npcStateLocked(npc *NPC) NPCState {
	s := NPCState{
		ID: npc.ServerID, Name: npc.Name, World: npc.World,
		X: npc.X, Y: npc.Y, Z: npc.Z, Facing: npc.Facing,
		SpawnX: npc.SpawnX, SpawnY: npc.SpawnY, SpawnZ: npc.SpawnZ,
		Level: npc.Level, HP: npc.HP, MaxHP: npc.MaxHP, Dead: npc.Dead,
		Class: npc.Class, Faction: npc.Faction, Temporary: npc.Temporary,
		ModelID: npc.ModelID, NPCType: npc.NPCType,
		RespawnSec: npc.RespawnTime.Seconds(),
	}
	if p, ok := w.Players[npc.CurrentEnemy]; ok && npc.CurrentEnemy != 0 {
		s.Fighting = p.Name
	} else if p, ok := w.Players[npc.DummyTarget]; ok && npc.DummyTarget != 0 {
		s.Fighting = p.Name
	}
	return s
}

// NPCStateOf is the NPC of that id as the world holds it, and whether there
// is one.
func (w *World) NPCStateOf(id EntityID) (NPCState, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc, ok := w.NPCs[id]
	if !ok {
		return NPCState{}, false
	}
	return w.npcStateLocked(npc), true
}

// NPCStates is every NPC of a world as it stands now (world < 0: all of them),
// in id order. Tens of thousands: the map's markers take it whole.
func (w *World) NPCStates(world int) []NPCState {
	w.mu.RLock()
	out := make([]NPCState, 0, len(w.NPCs))
	for _, npc := range w.NPCs {
		if world >= 0 && int(npc.World) != world {
			continue
		}
		out = append(out, w.npcStateLocked(npc))
	}
	w.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TemporaryNPCs is every GM spawn in the world (devspawn.go), in id order.
func (w *World) TemporaryNPCs() []NPCState {
	w.mu.RLock()
	var out []NPCState
	for _, npc := range w.NPCs {
		if npc.Temporary {
			out = append(out, w.npcStateLocked(npc))
		}
	}
	w.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetNPCClass gives every NPC whose npcclass key (database.NPCClassKey) is key
// the class: an npcclass row written or deleted (class 0, a warrior's, for a
// deleted one). It returns how many NPCs it changed.
func (w *World) SetNPCClass(key string, class int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, npc := range w.NPCs {
		if database.NPCClassKey(npc.Name) == key && npc.Class != class {
			npc.Class = class
			npc.endRound()
			n++
		}
	}
	return n
}

// ReloadNPCs puts the npcs table into the world again, whole: every NPC a row
// made is replaced by its row as it is now (one whose row is gone leaves, one
// whose row is new comes), each alive on its spawn point. A GM's spawns stay.
// Each changed NPC is made again in every view, as UpsertNPC's are.
func (w *World) ReloadNPCs(store *database.NPCStore) (loaded, removed int, err error) {
	recs, err := store.GetAllNPCs()
	if err != nil {
		return 0, 0, err
	}
	classes, err := store.NPCClasses()
	if err != nil {
		return 0, 0, err
	}
	hold := time.Now().Add(NPCRefreshHold)

	w.mu.Lock()
	defer w.mu.Unlock()
	keep := make(map[EntityID]bool, len(recs))
	for _, rec := range recs {
		fresh := NPCFromRecord(rec, classes[database.NPCClassKey(rec.NPCName)], w.rangedModels)
		if _, taken := w.Players[fresh.ServerID]; taken {
			continue
		}
		if existing, ok := w.NPCs[fresh.ServerID]; ok {
			if existing.Temporary {
				continue
			}
			keep[fresh.ServerID] = true
			if npcSameRecord(existing, fresh) {
				continue // unchanged: left as it is, fight and all
			}
			fresh.HiddenUntil = hold
			oldX, oldZ := existing.X, existing.Z
			*existing = *fresh
			w.SpatialIndex.Update(existing.ServerID, oldX, oldZ, existing.X, existing.Z)
			delete(w.affected, existing.ServerID)
			loaded++
			continue
		}
		keep[fresh.ServerID] = true
		fresh.HiddenUntil = hold
		w.NPCs[fresh.ServerID] = fresh
		w.SpatialIndex.Insert(fresh.ServerID, fresh.X, fresh.Z)
		loaded++
	}
	for id, npc := range w.NPCs {
		if !npc.Temporary && !keep[id] {
			w.removeNPCLocked(npc)
			delete(w.affected, id)
			removed++
		}
	}
	return loaded, removed, nil
}

// npcSameRecord is whether two NPCs were made from the same row as it was:
// every field NPCFromRecord sets from it.
func npcSameRecord(a, b *NPC) bool {
	return a.Name == b.Name && a.SpawnX == b.SpawnX && a.SpawnY == b.SpawnY && a.SpawnZ == b.SpawnZ &&
		a.SpawnFacing == b.SpawnFacing && a.World == b.World && a.ModelID == b.ModelID && a.Size == b.Size &&
		a.NPCID == b.NPCID && a.NPCType == b.NPCType && a.PrimaryWeapon == b.PrimaryWeapon &&
		a.SecondaryWeapon == b.SecondaryWeapon && a.PrimaryType == b.PrimaryType &&
		a.SecondaryType == b.SecondaryType && a.Shield == b.Shield && a.Torso == b.Torso &&
		a.Forearm == b.Forearm && a.NPCGloves == b.NPCGloves && a.NPCLegs == b.NPCLegs && a.Feet == b.Feet &&
		a.Head == b.Head && a.HairColor == b.HairColor && a.HairLength == b.HairLength &&
		a.HairStyle == b.HairStyle && a.Face == b.Face && a.RobeStyle == b.RobeStyle && a.Race == b.Race &&
		a.MaxHP == b.MaxHP && a.Level == b.Level && a.Class == b.Class
}

// PlayerState is a player in the world as the GM panel lists it.
type PlayerState struct {
	ID        EntityID `json:"id"`
	ServerID  uint32   `json:"serverid"`
	AccountID int      `json:"accountid"`
	Name      string   `json:"name"`
	Level     int      `json:"level"`
	Class     int      `json:"class"`
	Race      int      `json:"race"`
	World     uint8    `json:"world"`
	X         float32  `json:"x"`
	Y         float32  `json:"y"`
	Z         float32  `json:"z"`
	Facing    float32  `json:"facing"`
	HP        int      `json:"hp"`
	MaxHP     int      `json:"maxHP"`
	Power     int      `json:"power"`
	MaxPower  int      `json:"maxPower"`
	Tunar     int      `json:"tunar"`
	Dead      bool     `json:"dead"`
	Target    string   `json:"target,omitempty"`
	Fighting  string   `json:"fighting,omitempty"`
	GroupID   uint32   `json:"groupID,omitempty"`
	GuildName string   `json:"guild,omitempty"`
	Speed     float32  `json:"speed"`
}

// PlayerStates is every player in the world, by name.
func (w *World) PlayerStates() []PlayerState {
	w.mu.RLock()
	out := make([]PlayerState, 0, len(w.Players))
	for _, p := range w.Players {
		s := PlayerState{
			ID: p.ServerID, ServerID: p.CharServerID, AccountID: p.AccountID, Name: p.Name,
			Level: p.Level, Class: p.Class, Race: p.Race, World: p.World,
			X: p.X, Y: p.Y, Z: p.Z, Facing: p.Facing,
			HP: p.HP, MaxHP: p.MaxHP, Power: p.Power, MaxPower: p.MaxPower,
			Tunar: p.Tunar, Dead: p.IsDead(), GroupID: p.GroupID, GuildName: p.GuildName,
			Speed: p.Speed,
		}
		if npc, ok := w.NPCs[p.AttackTarget]; ok && p.AttackTarget != 0 {
			s.Fighting = npc.Name
		}
		out = append(out, s)
	}
	w.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// PlayerByCharacter is the world's player for a character (its serverid), if
// one is in the world.
func (w *World) PlayerByCharacter(serverID uint32) (PlayerState, bool) {
	for _, s := range w.PlayerStates() {
		if s.ServerID == serverID {
			return s, true
		}
	}
	return PlayerState{}, false
}

// SetPlayerLevel sets an online player's level where the game holds it: its
// stats recalculated for it (hit points and power kept within the new
// maxima) and the stat channel sent at the next tick. It reports false for a
// character not in the world.
func (w *World) SetPlayerLevel(serverID uint32, level int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil {
		return false
	}
	p.Level = max(1, min(60, level))
	p.XPInLevel = 0
	p.TotalXP = TotalXP(p.Level, 0)
	p.TopUpTrainingPoints()
	p.RecalcStats(false)
	p.StatsDirty = true
	return true
}

// SetPlayerTunar sets an online player's purse and bank where the game holds
// them (a merchant's sale writes the Player's purse back to the row, so a row
// changed alone would be undone by the next one).
func (w *World) SetPlayerTunar(serverID uint32, tunar, bank int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil {
		return false
	}
	p.Tunar, p.BankTunar = tunar, bank
	return true
}

// HealPlayer brings an online, living player's hit points and power to their
// maxima.
func (w *World) HealPlayer(serverID uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil || p.IsDead() {
		return false
	}
	p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
	p.StatsDirty = true
	return true
}

func (w *World) playerByCharacterLocked(serverID uint32) *Player {
	for _, p := range w.Players {
		if p.CharServerID == serverID {
			return p
		}
	}
	return nil
}
