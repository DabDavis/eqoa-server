package entity

import (
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// pet.go — summoned pets: the creature a spell makes, which follows its
// owner and fights for it (F3 20261025-044023: "pet spells do not summon a
// pet").
//
// What CLIENT knows of a pet is two fields and two messages
// (claude-notes/pets-charm-client-20261021.md): 0x007E sets m_petId, an id
// as the state channels carry it (the session's object id here), 0x007F
// clears it and the pet's name, and the MemoryDump clears both too, so
// 0x007E goes again after every dump while the pet lives (PetDumped). The
// pet bar latches the name once off the pet's channel (bytes 116-139) and
// shows its hit points (byte 27).
//
// What Sandstorm did (harvests/sandstorm-spells-20261021/pets/
// kimura-summon-fire-elemental.json): a level 45 magician's Fire Elemental,
// a spell of level 44, came 1.5 s after its 8 s cast as a new creature
// named as the spell, level 44, model 0x8A416241, scale 1.2; 0x007E with its
// id at the landing, and 2 s later 0x1003 {"Fire Elemental", 0x60000000}:
// the non-collide list (VIClientPart38300, the ignore list's), so the owner
// walks through its own pet. Its channel: byte 142 3 (player-side: a
// steel-blue name, no melee at it, Tab skips it), 143 1, npc flags 0, the
// hands' attack types 1 and 5.
//
// Here:
//   - a summoning spell (spelltext.go's EffectSummon) lands as a pet named
//     as the spell, at the spell's level but never above the caster's (the
//     one pet recorded fits either), its body the most common among this
//     server's own NPCs of the spell's name, else of the creature its line
//     names, else of the name's last word after a possessive ("Pendril's
//     Animation": animation), else a stand-in by the caster's class; its hit
//     points and melee a creature's of its level. A second pet replaces the
//     first (0x007F first, so the bar takes the new name).
//   - it follows its owner a few units off, comes along through a teleport
//     or into another world, and fights in rounds as creatures do. Its hits
//     are its owner's (the 0x00DB names the owner as the attacker: a figure
//     the player did not deal is drawn over the player), and so are its
//     kills' XP. What it hits fights back at it.
//   - 0x0041 (the start menu's Pet entries and the quick chat): 1 passive
//     (fights only when told to), 2 defensive (also what attacks it or its
//     owner; a new pet's), 3 neutral (also what attacks it), 4 aggressive
//     (also what its owner fights), 5 dismiss (CLIENT asked first), 7 back
//     off, 8 attack the target named. 0 and 6 are no command. CLIENT shows
//     nothing for any of them, so each is answered in a line of text.
//   - it goes (0x007F, and its non-collide entry off) when dismissed, when it
//     dies and when its owner dies; with no word to anyone when its owner
//     leaves the world.

// petStance is a 0x0041 stance.
type petStance byte

const (
	petPassive    petStance = 1
	petDefensive  petStance = 2
	petNeutral    petStance = 3
	petAggressive petStance = 4
)

// The 0x0041 commands that are not stances.
const (
	petCmdDismiss = 5
	petCmdBackOff = 7
	petCmdAttack  = 8
)

// petNoCollideID is the id of a pet's entry on its owner's non-collide
// list: Sandstorm's, where an ignored player's entry counts up from 1.
const petNoCollideID uint32 = 0x60000000

const (
	petFollow      float32 = 3   // how far off its owner it stands
	petFollowStart float32 = 5   // how far its owner gets before it follows
	petCatchUp     float32 = 15  // farther, it runs half as fast again
	petLost        float32 = 100 // farther (a teleport), it is put beside its owner
	petLeash       float32 = 60  // a target farther than this from its owner is let go
	petReach       float32 = 4   // how near its target it fights from
	petAssist      float32 = 40  // how far round its owner it looks for a fight
	petRunSpeed    float32 = 8.833333
	petCorpseTime          = 10 * time.Second
	petBackOffHold         = 6 * time.Second // after a back off it takes no fight of its own
)

// petState is one pet, kept by its owner's id.
type petState struct {
	owner     *Player
	npc       *NPC
	stance    petStance
	target    EntityID  // what it fights, 0 none
	told      uint32    // the object id the owner was last told (0x007E), 0 none
	noCollide bool      // its non-collide entry is on the owner's list
	heldUntil time.Time // a back off's: no fight of its own until then
	last      time.Time // its last step
}

// petOfLocked is the player's pet, alive, or nil.
func (w *World) petOfLocked(p *Player) *NPC {
	if ps := w.petStateLocked(p); ps != nil {
		return ps.npc
	}
	return nil
}

func (w *World) petStateLocked(p *Player) *petState {
	if p == nil {
		return nil
	}
	ps := w.pets[p.ServerID]
	if ps == nil || ps.owner != p || ps.npc.Dead {
		return nil
	}
	return ps
}

// PetOf is the player's pet's id, or 0.
func (w *World) PetOf(p *Player) EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if pet := w.petOfLocked(p); pet != nil {
		return pet.ServerID
	}
	return 0
}

// petLevel is a summoned pet's level: the spell's, never above the caster's
// (Sandstorm's one pet, a level 44 spell's from a caster of 45, was 44).
func petLevel(spellLevel, casterLevel int) int {
	l := spellLevel
	if l <= 0 || l > casterLevel {
		l = casterLevel
	}
	return max(l, 1)
}

// petClassStandIn is the creature a class's pet is when nothing of its own
// name is in the world: a necromancer's skeleton, a magician's elementaling,
// an enchanter's animation.
var petClassStandIn = map[int]string{
	11: "skeleton",
	10: "earth elementaling",
	12: "animation",
}

// petName is a name as the pet records are matched: trimmed, any case.
func petName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// petTemplateLocked is the NPC a pet of spell is made after: among this
// server's own NPCs (not pets, not a GM's copies) of each name in turn, the
// spell's, the creature its line names, the spell name's last word after a
// possessive, the class's stand-in, the most common body of the first that
// has any (the lowest id of those wearing it). Must be called with the lock
// held.
func (w *World) petTemplateLocked(spellName, lineName string, class int) *NPC {
	var names []string
	add := func(s string) {
		if s = petName(s); s != "" {
			for _, n := range names {
				if n == s {
					return
				}
			}
			names = append(names, s)
		}
	}
	add(spellName)
	add(lineName)
	if i := strings.Index(spellName, "'s "); i >= 0 {
		add(spellName[i+3:])
	}
	add(petClassStandIn[class])

	byName := map[string][]*NPC{}
	for _, n := range names {
		byName[n] = nil
	}
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Owner != 0 {
			continue
		}
		n := petName(npc.Name)
		if _, want := byName[n]; want {
			byName[n] = append(byName[n], npc)
		}
	}
	for _, n := range names {
		npcs := byName[n]
		if len(npcs) == 0 {
			continue
		}
		count := map[int64]int{}
		for _, npc := range npcs {
			count[npc.ModelID]++
		}
		sort.Slice(npcs, func(i, j int) bool {
			a, b := npcs[i], npcs[j]
			if count[a.ModelID] != count[b.ModelID] {
				return count[a.ModelID] > count[b.ModelID]
			}
			if a.ModelID != b.ModelID {
				return a.ModelID < b.ModelID
			}
			return a.ServerID < b.ServerID
		})
		return npcs[0]
	}
	return nil
}

// newPet is a pet of the template's body for owner, of the level and the
// spell's name, beside its owner. It is a GM's copy's kind of NPC (no row,
// no loot, its corpse gone after its time) owned by the player.
func newPet(t *NPC, id EntityID, owner *Player, name string, level int) *NPC {
	pet := *t
	pet.ServerID = id
	pet.Name = strings.TrimSpace(name)
	pet.NPCID = 0   // no merchant's wares, no corpse loot
	pet.NPCType = 0 // npc flags 0, as Sandstorm's summoned pet's
	pet.Faction = FactionFriendly
	pet.Level = level
	pet.MaxHP = NPCMaxHP(level)
	pet.HP = pet.MaxHP
	pet.OffMod, pet.DefMod, pet.WeaponDamage = NPCOffMod(level), NPCDefMod(level), NPCWeaponDamage(level)
	if pet.PrimaryWeapon == 0 && pet.SecondaryWeapon == 0 {
		pet.PrimaryType, pet.SecondaryType = 1, 5 // Sandstorm's pet's hands
	}
	pet.World = owner.World
	pet.X, pet.Y, pet.Z = besideOwner(owner)
	pet.Facing = headingTo(owner.X-pet.X, owner.Z-pet.Z)
	pet.SpawnX, pet.SpawnY, pet.SpawnZ, pet.SpawnFacing = pet.X, pet.Y, pet.Z, pet.Facing
	pet.Dead, pet.DeathTime = false, time.Time{}
	pet.Threats, pet.CurrentEnemy = nil, 0
	pet.LastCombat, pet.LastCombatTime = time.Time{}, time.Time{}
	pet.Effects = nil
	pet.Temporary = true
	pet.RespawnTime = petCorpseTime
	pet.CorpseDecay = petCorpseTime
	pet.Dummy, pet.DummyTarget = NotDummy, 0
	pet.HiddenUntil = time.Time{}
	pet.Owner = owner.ServerID
	pet.endRound()
	pet.RoundTarget = 0
	pet.stopMoving()
	return &pet
}

// besideOwner is a spot at the owner's side, petFollow off it.
func besideOwner(p *Player) (x, y, z float32) {
	h := float64(p.Facing)
	return p.X + petFollow*float32(math.Cos(h)), p.Y, p.Z - petFollow*float32(math.Sin(h))
}

// summonPetLocked is a summoning landing: the caster's new pet, the old one
// gone. Must be called with the lock held.
func (w *World) summonPetLocked(cs *castState, caster *Player, sess *session.Session, now time.Time) {
	spell := &cs.spell
	t := w.petTemplateLocked(spell.Name, spell.Def.Pet, caster.Class)
	if t == nil {
		log.Printf("Pet: %s's %q: no creature of its name, its line's %q or its class's stand-in in this world: no pet",
			caster.Name, spell.Name, spell.Def.Pet)
		return
	}
	if old := w.pets[caster.ServerID]; old != nil {
		w.dismissPetLocked(old, sess, "")
	}
	level := petLevel(spell.Level, caster.Level)
	pet := newPet(t, w.allocFreeIDLocked(), caster, spell.Name, level)
	w.NPCs[pet.ServerID] = pet
	w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	if w.pets == nil {
		w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: caster, npc: pet, stance: petDefensive, last: now}
	w.pets[caster.ServerID] = ps
	if sess != nil {
		w.announcePetLocked(ps, sess)
	}
	log.Printf("Pet: %s summons %q (%s's body, model %#x, size %.2f), level %d, %d HP",
		caster.Name, pet.Name, t.Name, uint32(pet.ModelID), pet.Size, pet.Level, pet.MaxHP)
}

// announcePetLocked tells the owner its pet's id (0x007E) when it has not
// been told this one, putting the pet on one of its channels first if it is
// not on one, and puts the pet on its non-collide list once.
func (w *World) announcePetLocked(ps *petState, sess *session.Session) {
	pet := ps.npc
	id := w.objectIDForEntityLocked(sess, pet.ServerID)
	if id == 0 {
		id = w.pinPetChannelLocked(sess, pet)
	}
	if id == 0 {
		return
	}
	if id != ps.told {
		sendSetPet(sess, id)
		ps.told = id
	}
	if !ps.noCollide {
		sendNoCollideAdd(sess, pet.Name, petNoCollideID)
		ps.noCollide = true
	}
}

// pinPetChannelLocked puts the pet on one of the session's entity slots now,
// as the visibility system would, a free one or the farthest entity's: the
// 0x007E that follows names the id it gets. It returns the id, 0 when the
// session has no slots.
func (w *World) pinPetChannelLocked(sess *session.Session, pet *NPC) uint32 {
	conn := sess.Connection()
	if conn == nil {
		return 0
	}
	vis := GetOrCreateVisibility(sess)
	vis.fit(conn)
	slot := -1
	for i, id := range vis.ChannelEntity {
		if id == 0 {
			slot = i
			break
		}
	}
	if slot < 0 {
		owner := w.sessionPlayerLocked(sess)
		var far float32 = -1
		for i, id := range vis.ChannelEntity {
			if d := w.distSqToLocked(owner, id); d > far {
				slot, far = i, d
			}
		}
		if slot < 0 {
			return 0
		}
		if state := conn.GetServerObject(slot + 1); state != nil {
			state.MarkRemoved()
		}
	}
	vis.nextObjectID++
	id := vis.nextObjectID
	vis.ChannelEntity[slot] = pet.ServerID
	vis.ChannelObjectID[slot] = id
	if state := conn.GetServerObject(slot + 1); state != nil {
		if data := w.serializeEntityForChannel(sess, pet.ServerID, id); data != nil {
			state.SetData(data)
		}
	}
	return id
}

// distSqToLocked is an entity's squared distance on the ground from the
// player, the most a float holds when it is gone.
func (w *World) distSqToLocked(p *Player, id EntityID) float32 {
	var x, z float32
	if npc, ok := w.NPCs[id]; ok {
		x, z = npc.X, npc.Z
	} else if o, ok := w.Players[id]; ok {
		x, z = o.X, o.Z
	} else {
		return math.MaxFloat32
	}
	if p == nil {
		return 0
	}
	dx, dz := x-p.X, z-p.Z
	return dx*dx + dz*dz
}

// pinPetLocked puts the player's pet first among what its session is to see
// (visibilitySystem), wherever it is in the player's world: a pet the
// owner's view lets go comes back with a new id, and its bar would lose it
// until the next 0x007E.
func (w *World) pinPetLocked(p *Player, cands []entityDist) []entityDist {
	pet := w.petOfLocked(p)
	if pet == nil || pet.World != p.World {
		return cands
	}
	for i := range cands {
		if cands[i].ID == pet.ServerID {
			cands[i].DistSq = -1
			return cands
		}
	}
	return append(cands, entityDist{ID: pet.ServerID, DistSq: -1})
}

// dismissPetLocked takes the pet away: off every fight, out of the world
// (a dead one's corpse lies its time), and its owner told (0x007F, its
// non-collide entry off, text when there is any). sess is the owner's, or
// nil when it has none to tell. Must be called with the lock held.
func (w *World) dismissPetLocked(ps *petState, sess *session.Session, text string) {
	pet := ps.npc
	if w.pets[ps.owner.ServerID] == ps {
		delete(w.pets, ps.owner.ServerID)
	}
	w.dropAggroLocked(pet.ServerID)
	if !pet.Dead {
		w.removeNPCLocked(pet)
	}
	if sess != nil {
		sendClearPet(sess)
		if ps.noCollide {
			sendNoCollideRemove(sess, petNoCollideID)
		}
		if text != "" {
			w.sendChatToSession(sess, text)
		}
	}
	log.Printf("Pet: %s's %q goes", ps.owner.Name, pet.Name)
}

// PetDumped is a MemoryDump sent to the session: it cleared the client's pet,
// so the next tick tells it again.
func (w *World) PetDumped(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ps := w.petStateLocked(w.sessionPlayerLocked(sess)); ps != nil {
		ps.told = 0
	}
}

// PetCommand is a 0x0041: the command byte, and for an attack the target as
// the client names it. Each is answered with a line; CLIENT itself refuses
// one with no pet (word 1021), which is answered the same here.
func (w *World) PetCommand(sess *session.Session, cmd byte, target uint32, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	ps := w.petStateLocked(p)
	if ps == nil {
		if cmd != 0 && cmd != 6 {
			w.sendChatToSession(sess, castNoPet)
		}
		return
	}
	pet := ps.npc
	switch cmd {
	case byte(petPassive), byte(petDefensive), byte(petNeutral), byte(petAggressive):
		ps.stance = petStance(cmd)
		w.sendChatToSession(sess, petStanceWords[ps.stance])
	case petCmdDismiss:
		w.dismissPetLocked(ps, sess, "Your pet has been dismissed.")
	case petCmdBackOff:
		ps.target = 0
		ps.heldUntil = now.Add(petBackOffHold)
		pet.endRound()
		w.sendChatToSession(sess, "Your pet backs off.")
	case petCmdAttack:
		ent, ok := w.findEntityByObjectIDLocked(sess, target)
		npc, isNPC := ent.(*NPC)
		if !ok || !isNPC || !petMayFight(pet, npc) || distXZ(p.X, p.Z, npc.X, npc.Z) > petLeash {
			w.sendChatToSession(sess, "Your pet cannot attack that.")
			return
		}
		ps.target = npc.ServerID
		ps.heldUntil = time.Time{}
		w.sendChatToSession(sess, "Your pet attacks "+npc.Name+".")
	}
}

var petStanceWords = map[petStance]string{
	petPassive:    "Your pet is passive: it fights only when told to.",
	petDefensive:  "Your pet is defensive: it fights what attacks you or it.",
	petNeutral:    "Your pet is neutral: it fights what attacks it.",
	petAggressive: "Your pet is aggressive: it fights what you fight, and what attacks you or it.",
}

// petMayFight is whether a pet may take on the creature: alive, in its world,
// attackable (not npc flags 0x80, as CLIENT's own test) and nobody's pet.
func petMayFight(pet, npc *NPC) bool {
	return npc != nil && npc != pet && !npc.Dead && npc.World == pet.World && npc.NPCType&0x80 == 0 && npc.Owner == 0
}

func distXZ(ax, az, bx, bz float32) float32 {
	dx, dz := ax-bx, az-bz
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

// petSystem moves and fights every pet, tells each owner its pet's id when
// that changed, ticks the pet's spells, and takes away a pet whose owner is
// gone or dead, or which died. Every tick; must be called with the lock
// held.
func (w *World) petSystem(now time.Time) {
	for id, ps := range w.pets {
		owner, pet := ps.owner, ps.npc
		if w.Players[id] != owner {
			// logged out, or logged in again as a new player: nobody to tell
			w.dismissPetLocked(ps, nil, "")
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if pet.Dead {
			w.dismissPetLocked(ps, sess, "Your pet has died.")
			continue
		}
		if owner.IsDead() || owner.respawnPending() {
			w.dismissPetLocked(ps, sess, "")
			continue
		}
		dt := float32(now.Sub(ps.last).Seconds())
		ps.last = now
		dt = min(max(dt, 0), 0.5)

		w.petEffectsLocked(ps, now)
		if pet.World != owner.World || distXZ(pet.X, pet.Z, owner.X, owner.Z) > petLost {
			// a teleport, a coach, another world: it comes along
			oldX, oldZ := pet.X, pet.Z
			pet.World = owner.World
			pet.X, pet.Y, pet.Z = besideOwner(owner)
			pet.stopMoving()
			ps.target = 0
			pet.endRound()
			w.SpatialIndex.Update(pet.ServerID, oldX, oldZ, pet.X, pet.Z)
		}
		w.petChooseTargetLocked(ps, now)
		if t := w.NPCs[ps.target]; ps.target != 0 && t != nil {
			w.petFightLocked(ps, t, dt, now)
		} else {
			ps.target = 0
			w.petFollowLocked(ps, dt)
		}
		if sess != nil {
			w.announcePetLocked(ps, sess)
		}
	}
}

// petFollowLocked keeps the pet petFollow off its owner: it follows once the
// owner is petFollowStart away, at the owner's speed (half as fast again
// once petCatchUp behind), and stands when there.
func (w *World) petFollowLocked(ps *petState, dt float32) {
	owner, pet := ps.owner, ps.npc
	d := distXZ(pet.X, pet.Z, owner.X, owner.Z)
	if d <= petFollow || (pet.Loco == LocoStand && d <= petFollowStart) {
		pet.stopMoving()
		return
	}
	speed := max(owner.SpeedNow(), 3)
	if d > petCatchUp {
		speed *= 1.5
	}
	x := owner.X + (pet.X-owner.X)/d*petFollow
	z := owner.Z + (pet.Z-owner.Z)/d*petFollow
	pet.Y = owner.Y
	w.stepNPCToward(pet, x, z, speed, dt)
}

// petChooseTargetLocked lets go of a target that is gone, dead or too far
// from the owner, and gives a pet without one what its stance takes on:
// the nearest creature fighting it (neutral, defensive, aggressive) or its
// owner (defensive, aggressive), or what its owner fights (aggressive).
func (w *World) petChooseTargetLocked(ps *petState, now time.Time) {
	owner, pet := ps.owner, ps.npc
	if ps.target != 0 {
		t := w.NPCs[ps.target]
		if t == nil || !petMayFight(pet, t) || distXZ(owner.X, owner.Z, t.X, t.Z) > petLeash {
			ps.target = 0
			pet.endRound()
		} else {
			return
		}
	}
	if ps.stance == petPassive || now.Before(ps.heldUntil) {
		return
	}
	if ps.stance == petAggressive {
		if t := w.NPCs[owner.AttackTarget]; owner.AttackTarget != 0 && petMayFight(pet, t) {
			ps.target = t.ServerID
			return
		}
	}
	var best *NPC
	var bestD float32
	for _, id := range w.SpatialIndex.QueryRadius(owner.X, owner.Z, petAssist) {
		npc := w.NPCs[id]
		if !petMayFight(pet, npc) {
			continue
		}
		mine := npc.CurrentEnemy == pet.ServerID
		if ps.stance != petNeutral {
			mine = mine || npc.CurrentEnemy == owner.ServerID
		}
		if ps.stance == petAggressive {
			_, hurt := npc.Threats[owner.ServerID]
			mine = mine || hurt
		}
		if !mine {
			continue
		}
		if d := distXZ(pet.X, pet.Z, npc.X, npc.Z); best == nil || d < bestD {
			best, bestD = npc, d
		}
	}
	if best != nil {
		ps.target = best.ServerID
	}
}

// petFightLocked takes the pet to its target and fights it in rounds there.
func (w *World) petFightLocked(ps *petState, t *NPC, dt float32, now time.Time) {
	pet := ps.npc
	d := distXZ(pet.X, pet.Z, t.X, t.Z)
	if d > petReach {
		x := t.X + (pet.X-t.X)/d*(petReach-1)
		z := t.Z + (pet.Z-t.Z)/d*(petReach-1)
		pet.Y = t.Y
		w.stepNPCToward(pet, x, z, max(ps.owner.SpeedNow(), petRunSpeed), dt)
		return
	}
	pet.stopMoving()
	pet.Facing = headingTo(t.X-pet.X, t.Z-pet.Z)
	length, delay, fx, hits := npcRound(pet)
	if pet.RoundTarget != t.ServerID {
		pet.endRound()
		pet.RoundTarget = t.ServerID
	}
	if pet.RoundStart.IsZero() || !now.Before(pet.RoundStart.Add(length)) {
		next := pet.RoundStart.Add(length)
		if pet.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		pet.RoundStart, pet.RoundHits = next, 0
		w.broadcastNPCSwingLocked(pet, t.ServerID, uint32(delay/time.Millisecond), fx)
	}
	for pet.RoundHits < len(hits) && !now.Before(pet.RoundStart.Add(hits[pet.RoundHits])) {
		pet.RoundHits++
		if w.petHitLocked(ps, t, now) {
			return
		}
	}
}

// petHitLocked is one of the pet's hits on its target, its owner's: the
// figure to the owner as the owner's (0x00DB), the target's hate for the
// pet, and on a kill the owner's XP and faction hits. It reports a kill.
func (w *World) petHitLocked(ps *petState, t *NPC, now time.Time) bool {
	owner, pet := ps.owner, ps.npc
	sess := w.findSessionByEntityIDLocked(owner.ServerID)
	n := petMeleeHit(pet, t, rand.Intn)
	pet.LastCombatTime = now
	tell := func() {
		if sess == nil {
			return
		}
		if tid := w.objectIDForEntityLocked(sess, t.ServerID); tid != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), -n, tid)
		}
	}
	if t.Dummy != NotDummy {
		// told, taken off nothing; the dummy fights its one player (dummy.go)
		tell()
		w.dummyEngagedLocked(t, owner, now)
		return false
	}
	t.HP = max(t.HP-n, 0)
	tell()
	w.updateAggro(t, pet.ServerID, n)
	log.Printf("Pet: %s's %q hit %s for %d (HP %d/%d)", owner.Name, pet.Name, t.Name, n, t.HP, t.MaxHP)
	if t.HP > 0 {
		return false
	}
	t.Effects = nil
	w.killNPC(t)
	if sess != nil {
		w.awardXP(sess, owner, t.Level*100)
	}
	// the owner's faction hits, as its own kill's (faction.go)
	w.factionKillLocked(sess, owner, t)
	if owner.AttackTarget == t.ServerID {
		owner.disengage()
	}
	ps.target = 0
	pet.endRound()
	return true
}

// petMeleeHit is a pet's hit on a creature: a creature's roll of the pet's
// level (defence.go), its top less the target's defence as a tank's is
// (offence.go), never under the floor. Not measured: no pet's hits were
// recorded.
func petMeleeHit(pet, t *NPC, intn func(int) int) int {
	r := creatureRoll(pet.Level)
	top := int(math.Round(r.Top - targetDefence(t)))
	hit := top - r.Width + intn(r.Width+1)
	return max(hit, r.Floor, 1)
}

// npcHitOnPet is a creature's hit on a pet: its roll on a tank of the pet's
// level, its AC NPCDefMod's estimate (ten a level), its AGI its level.
func npcHitOnPet(npc, pet *NPC, intn func(int) int) int {
	r := npcMeleeRoll(npc)
	top := int(math.Round(r.Top - r.ACSlope*float64(10*pet.Level) - agiSlope[archTank]*float64(pet.Level)))
	hit := top - r.Width + intn(r.Width+1)
	return max(hit, r.Floor, 1)
}

// petEnemyLocked is the pet a creature's current enemy is, when it is a pet
// it can still fight; nil otherwise.
func (w *World) petEnemyLocked(npc *NPC) *NPC {
	pet := w.NPCs[npc.CurrentEnemy]
	if pet == nil || pet.Owner == 0 || pet.Dead || pet.World != npc.World {
		return nil
	}
	return pet
}

// npcFightPetLocked is a creature's round on a pet, as on a player
// (npcRoundLocked): its swing to everyone who sees it, then its hits; the
// pet dies at 0 (petSystem takes it away). The leash is a player's, 10.
func (w *World) npcFightPetLocked(npc, pet *NPC, now time.Time) {
	if distXZ(npc.X, npc.Z, pet.X, pet.Z) > 10 {
		delete(npc.Threats, pet.ServerID)
		w.retargetLocked(npc)
		npc.endRound()
		return
	}
	if npc.RoundTarget != pet.ServerID {
		npc.endRound()
		npc.RoundTarget = pet.ServerID
	}
	length, delay, fx, hits := npcRound(npc)
	if npc.RoundStart.IsZero() || !now.Before(npc.RoundStart.Add(length)) {
		next := npc.RoundStart.Add(length)
		if npc.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		npc.RoundStart, npc.RoundHits = next, 0
		w.broadcastNPCSwingLocked(npc, pet.ServerID, uint32(delay/time.Millisecond), fx)
	}
	for npc.RoundHits < len(hits) && !now.Before(npc.RoundStart.Add(hits[npc.RoundHits])) {
		npc.RoundHits++
		n := npcHitOnPet(npc, pet, rand.Intn)
		pet.HP = max(pet.HP-n, 0)
		pet.LastCombatTime = now
		log.Printf("Pet: %s hit the pet %q for %d (HP %d/%d)", npc.Name, pet.Name, n, pet.HP, pet.MaxHP)
		if pet.HP == 0 {
			w.killNPC(pet)
			w.dropAggroLocked(pet.ServerID)
			log.Printf("Pet: %q killed by %s", pet.Name, npc.Name)
			return
		}
	}
}

// retargetLocked turns a creature to its highest threat left, or to none.
func (w *World) retargetLocked(npc *NPC) {
	npc.CurrentEnemy = 0
	var top int64
	for id, t := range npc.Threats {
		if t > top {
			top = t
			npc.CurrentEnemy = id
		}
	}
}

// landOnPetLocked is a beneficial spell on a pet: a heal now, and what lasts
// kept on it (its heals tick in petEffectsLocked; what it does to stats a
// pet does not use yet).
func (w *World) landOnPetLocked(cs *castState, caster *Player, casterSess *session.Session, pet *NPC, now time.Time) {
	for _, e := range cs.spell.Def.Effects {
		if e.Kind != EffectHP {
			continue
		}
		if n := spellAmount(e, caster); n > 0 && pet.HP < pet.MaxHP {
			pet.HP = min(pet.HP+n, pet.MaxHP)
			w.tellAmountLocked(caster, casterSess, pet.ServerID, n)
		}
	}
	if ae := lasting(cs, caster, now); ae != nil {
		for i, old := range pet.Effects {
			if old.SpellID == ae.SpellID {
				pet.Effects = append(pet.Effects[:i], pet.Effects[i+1:]...)
				break
			}
		}
		pet.Effects = append(pet.Effects, ae)
	}
	log.Printf("Cast: %s's %q lands on the pet %q (%d/%d HP, %d effects)", caster.Name, cs.spell.Name, pet.Name, pet.HP, pet.MaxHP, len(pet.Effects))
}

// petEffectsLocked takes off a pet's effects that are over and runs their
// heals' ticks.
func (w *World) petEffectsLocked(ps *petState, now time.Time) {
	pet := ps.npc
	left := pet.Effects[:0]
	for _, ae := range pet.Effects {
		if !now.Before(ae.Until) {
			continue
		}
		left = append(left, ae)
		if ae.HPTick <= 0 || ae.NextTick.IsZero() || now.Before(ae.NextTick) {
			continue
		}
		ae.NextTick = ae.NextTick.Add(effectTick)
		if pet.HP < pet.MaxHP {
			pet.HP = min(pet.HP+ae.HPTick, pet.MaxHP)
		}
	}
	pet.Effects = left
}

// sendSetPet is 0x007E: the pet's id, one raw u32 into m_petId.
func sendSetPet(sess *session.Session, id uint32) {
	sendGroupMsg(sess, protocol.OpcodeSetPet, func(b *buffer.Writer) { b.WriteUint32(id) })
}

// sendClearPet is 0x007F, no payload: m_petId -1, the pet's name cleared.
func sendClearPet(sess *session.Session) {
	sendGroupMsg(sess, protocol.OpcodeClearPet, func(*buffer.Writer) {})
}

// sendNoCollideAdd is 0x1003: one {name, id} onto the non-collide list.
func sendNoCollideAdd(sess *session.Session, name string, id uint32) {
	sendGroupMsg(sess, protocol.OpcodeNoCollideAdd, func(b *buffer.Writer) {
		writeName8(b, name)
		b.WriteUint32(id)
	})
}

// sendNoCollideRemove is 0x1004: the entry of that id off it.
func sendNoCollideRemove(sess *session.Session, id uint32) {
	sendGroupMsg(sess, protocol.OpcodeNoCollideRemove, func(b *buffer.Writer) { b.WriteUint32(id) })
}
