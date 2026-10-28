package entity

import (
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The world's half of the GM's TEST menu (handlers/gmchannel.go): what a
// GM's 0x0701 changes in the game. Each takes the World's lock; none is
// kept past the server's run but the rename, which the handler writes.

// ToggleGMHidden turns the player's GM invisibility over (Toggle Invis) and
// reports it now, and whether there is such a player. It is no effect, so
// no action breaks it; the channel's byte 154 goes with Invisible.
func (w *World) ToggleGMHidden(id EntityID) (on, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil {
		return false, false
	}
	p.GMHidden = !p.GMHidden
	return p.GMHidden, true
}

// SetGMForm shows the player in another body (Change Sprite): model is the
// DictID the channel carries, size its scale (0: the player's own). Model 0
// is Revert Sprite: its own body, or an illusion's, again.
func (w *World) SetGMForm(id EntityID, model uint32, size float32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil {
		return false
	}
	p.GMForm, p.GMFormSize = model, size
	if model == 0 {
		p.GMFormSize = 0
	}
	return true
}

// ToggleFrozen turns a GM's freeze on the player over: frozen, its speed is
// 0 (SpeedNow) until the GM lets it go. It reports the freeze now, the speed
// to tell its client, and whether there is such a player.
func (w *World) ToggleFrozen(id EntityID) (on bool, speed float32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil {
		return false, 0, false
	}
	p.Frozen = !p.Frozen
	return p.Frozen, p.SpeedNow(), true
}

// KillPlayerNow is a GM's KILL on a player: its death as any other (the
// experience debt, the respawn after PlayerRespawnDelay), told to sess, the
// player's own session. It reports false for one not here or dead already.
func (w *World) KillPlayerNow(sess *session.Session, id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil || p.IsDead() {
		return false
	}
	w.playerDiedLocked(sess, p, time.Now())
	p.StatsDirty = true
	return true
}

// HealNPC brings a living NPC to its full health (a GM's HEAL), reporting
// its name and whether it could.
func (w *World) HealNPC(id EntityID) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, found := w.NPCs[id]
	if !found || npc.Dead {
		return "", false
	}
	npc.HP = npc.MaxHP
	return npc.Name, true
}

// HoldNPC is Kill Spawner (hold: killed now if it lives, and kept from
// coming back) and Unkill Spawner (release: back on its spawn point now if
// it is dead). Not a GM's #spawn or a training dummy. It reports the NPC's
// name and whether it could.
func (w *World) HoldNPC(id EntityID, hold bool) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc, found := w.NPCs[id]
	if !found || npc.Temporary || npc.Dummy != NotDummy {
		return "", false
	}
	npc.GMHeld = hold
	switch {
	case hold && !npc.Dead:
		npc.HP = 0
		w.killNPC(npc)
	case !hold && npc.Dead:
		w.respawnNPCLocked(npc)
		npc.Effects = nil
		delete(w.affected, id)
	}
	return npc.Name, true
}

// NPCBySpawn is the NPC a spawner number names: its npcs row (NPCID), else
// an entity id; never a GM's #spawn.
func (w *World) NPCBySpawn(n int) (EntityID, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for id, npc := range w.NPCs {
		if npc.NPCID == n && !npc.Temporary {
			return id, true
		}
	}
	if n > 0 {
		if npc, ok := w.NPCs[EntityID(n)]; ok && !npc.Temporary {
			return npc.ServerID, true
		}
	}
	return 0, false
}

// RenamePlayer gives an online player a new name where the world holds it
// (its Player and its guild's record of it), reporting the old one. The
// character's row is the caller's to write.
func (w *World) RenamePlayer(id EntityID, name string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	old := p.Name
	p.Name = name
	if g := w.guildOfLocked(p); g != nil {
		if m := g.Members[GuildMemberID(p)]; m != nil {
			m.Name = name
		}
	}
	return old, true
}

// DisbandGuildOfMember ends the guild a character of that name belongs to
// (a GM's Disband Guild: no rank needed), online or not. It returns the
// guild and its members' Players that were online, all now out of it.
func (w *World) DisbandGuildOfMember(name string) (*Guild, []*Player, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var g *Guild
	for _, gg := range w.guilds {
		for _, m := range gg.Members {
			if strings.EqualFold(m.Name, strings.TrimSpace(name)) {
				g = gg
				break
			}
		}
		if g != nil {
			break
		}
	}
	if g == nil {
		return nil, nil, ErrNotInGuild
	}
	var online []*Player
	for _, p := range w.Players {
		if p.GuildID == g.ID {
			online = append(online, p)
			clearGuild(p)
		}
	}
	delete(w.guilds, g.ID)
	return g, online, nil
}

// RaceName and ClassName are a race's and a class's names as #stats writes
// them (a number for one out of range).
func RaceName(race int) string   { return nameIn(raceNames, race) }
func ClassName(class int) string { return nameIn(classNames, class) }
