package entity

import (
	"fmt"
	"log"
	"math"
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
	petAssist      float32 = 30  // how far round its owner it looks for a fight (petition 1134, Jibmage: "approximately 30 meters")
	petRunSpeed    float32 = 8.833333
	petCorpseTime          = 10 * time.Second
	petBackOffHold         = 6 * time.Second // after a back off it takes no fight of its own
)

// An enchanter's pet taunts its target every third round it fights it (F3
// 20261025-174403, jib: "Enchanter pets would taunt periodically. Maybe
// every 3 combat round. That taunt also did a small amount of damage, like
// a single melee hit relative to the pets level"): the target turns on the
// pet and takes one more of the pet's hits; Grand Animation's does 500
// (EQOA-Data's Animator: "Has 500 dmg taunt and dual hits"). The cadence is
// jib's recollection, not a recording.
const (
	petTauntEvery       = 3
	grandAnimationTaunt = 500
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
	// pvpTarget is the flagged player it goes for, 0 none (pvp.go)
	pvpTarget EntityID
	// taunts is whether it taunts every petTauntEvery rounds (an
	// enchanter's summoned pet), and rounds how many it has begun on its
	// target
	taunts bool
	rounds int
	// tapNext is when its life tap is up again (pettap.go)
	tapNext time.Time
	// kind is what it fights as (petrotation.go): ours, or one of the
	// necromancer's pets Sandstorm's were recorded as; tapIn the swings to
	// its next tap, specialIn to a spectre's next instant, specialAt when
	// the one lined up is due, fightStart when its fight's first swing came
	kind       petKind
	tapIn      int
	specialIn  int
	specialAt  time.Time
	fightStart time.Time
	// hitScale is what each melee hit of its is multiplied by (0 or 1:
	// nothing): a recorded kind's, so that Sandstorm's slower round deals
	// the damage a second its own round did (petrotation.go)
	hitScale float64
	// tmpl is the pettemplate row it fights by, nil for none; foe what its
	// abilities' clocks were last started on (pettemplate.go)
	tmpl *petTemplate
	foe  EntityID
	// spell is the summoning spell that made it, 0 for a charmed one: what a
	// restart summons it again by (keeppet.go)
	spell int
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

// petBodyAliases are the creatures a pet whose spell names none in the world
// wears instead: the enchanter's Earthen Golem a granite golem's body,
// Ferrous Golem an iron golem's (ours: no NPC of either name exists, and the
// class's stand-in, an animation, is no golem). Its level and swing are still
// the spell's.
var petBodyAliases = map[string]string{
	"earthen golem": "granite golem",
	"ferrous golem": "iron golem",
	// Vampire Form's body: no NPC here is named "vampire", and the fledgling
	// vampires wear the vampires' body (vampire.go)
	"vampire": "fledgling vampire",
}

// petClassStandIn is the creature a class's pet is when nothing of its own
// name is in the world: a necromancer's skeleton, a magician's elementaling,
// an enchanter's animation, a wizard's familiar (Allakhazam's records of
// every Familiar and Guardian: "Summon Pet", Self Only; the lines "Summon
// Pet" and "Summons a pet" name no creature, so Minor and Arch Familiar and
// Arch Guardian made nothing until then: the test lanes' report of
// 2026-10-29), a shaman's spirit (the hundreds of NPC rows named for its
// pet spells, Companion, Feral, Vigilant and Frenzied Spirit, wear the
// wolves' bodies; its Spiritual Guard, Hunter, Slayer and Warrior and its
// epic name none), a shadow knight's skeleton (ours, the necromancer's: no
// record says what its Death Knight's Summon Servant was).
var petClassStandIn = map[int]string{
	3:  "skeleton",
	8:  "companion spirit",
	11: "skeleton",
	10: "earth elementaling",
	12: enchanterAnimation,
	13: "familiar",
}

const (
	classWarrior       = 0
	classEnchanter     = 12
	enchanterAnimation = "animation"
	// grandAnimation is the Animator's pet (EQOA-Data's enchanter master
	// class, Animator 2: "Level 54 pet, ubah tank. Has 500 dmg taunt and dual
	// hits"); no record of it is on this server yet.
	grandAnimation = "grand animation"
)

// petSwing is a summoned pet's own swing (npcSwingKind), when it has one:
// an enchanter's pet swings once a round whatever its level, as a caster
// does (Attack1), and Grand Animation twice (Attack2), as live EQOA's did
// (F3 20261025-164209, jib: "enchanter pets should only single hit
// regardless of level with grand animation dual hitting"). Any other
// class's pet swings as a warrior of its level (newPet), four hits a round
// from level 35 with no shield (F3 pass 18, the user: "at level 35 and
// above melee/war classes quad attack, and many pets/mobs are missing
// that"): a pet took its template creature's npcclass row, and Return
// Home's made most of them casters (a skeleton, an elementaling).
func petSwing(class int, spell string) (fixed bool, kind int) {
	if class != classEnchanter {
		return false, 0
	}
	if petName(spell) == grandAnimation {
		return true, 4
	}
	return true, 0
}

// petName is a name as the pet records are matched: trimmed, any case.
func petName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// petSpellName is the name of the pet a summoning spell makes: the spell's,
// less a leading "Summon " ("Summon Stone Servant" makes a Stone Servant),
// which is the name its swing and taunt go by.
func petSpellName(spell string) string {
	s := strings.TrimSpace(spell)
	if len(s) > 7 && strings.EqualFold(s[:7], "summon ") {
		return strings.TrimSpace(s[7:])
	}
	return s
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
	add(petBodyAliases[petName(spellName)])
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
	pet.Class = classWarrior // its swing a warrior's (petSwing has the enchanter's)
	pet.MaxHP = NPCMaxHP(level)
	pet.HP = pet.MaxHP
	pet.effMaxHP = 0
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
	pet.SpawnOnly = false // a body out of the world (spawnonly.go) is a pet in it
	pet.Owner = owner.ServerID
	pet.endRound()
	pet.RoundTarget = 0
	pet.stopMoving()
	// a pet fights by no class template (classtemplate.go), its body's
	// creature's or not
	pet.tmpl, pet.tbase, pet.tfight = nil, templateBase{}, nil
	pet.sheet = nil // a workbook pet's row is put on after (petstats.go)
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
	name := petSpellName(spell.Name)
	sheet := w.templates.sheets[spell.SpellID] // the workbook's numbers for it, nil none (petstats.go)
	var t *NPC
	if sheet != nil {
		t = w.petBodyLocked(sheet, spell.Name, spell.Def.Pet, caster.Class)
		if sheet.Name != "" {
			name = sheet.Name
		}
	} else {
		t = w.petTemplateLocked(spell.Name, spell.Def.Pet, caster.Class)
	}
	if t == nil {
		log.Printf("Pet: %s's %q: no creature of its name, its line's %q or its class's stand-in in this world: no pet",
			caster.Name, spell.Name, spell.Def.Pet)
		return
	}
	if old := w.pets[caster.ServerID]; old != nil {
		w.dismissPetLocked(old, sess, "", now)
	}
	level := petLevel(spell.Level, caster.Level)
	if spell.Def.PetLevel > 0 {
		level = petLevel(spell.Def.PetLevel, caster.Level) // its line's "level N"
	}
	if sheet != nil && sheet.Level > 0 {
		level = petLevel(sheet.Level, caster.Level) // the workbook's
	}
	pet := newPet(t, w.allocFreeIDLocked(), caster, name, level)
	if sheet != nil {
		applyPetSheet(pet, sheet) // its body, weapons and numbers (petstats.go)
	}
	w.applyBalanceLocked(pet) // a pet's hit points of its level (balance.go)
	petPowerLocked(pet)       // and its power (npcregen.go)
	pet.SwingFixed, pet.SwingKind = petSwing(caster.Class, name)
	kind := petKindOf(caster.Class, spell.Name)
	hitScale := 1.0
	pt := w.petTemplateForLocked(spell)
	ownSwing := false
	if sheet != nil {
		// its own spells, its template row's class and swing (petstats.go);
		// none of the recorded necromancer pets' rotation
		kind, ownSwing = petKindOurs, pt == nil
		pt = petSheetTemplate(sheet, spell, pt)
	}
	switch k, round, ok := petKindSwing(kind); {
	case pt != nil:
		fixed, swing := pet.SwingFixed, pet.SwingKind
		hitScale = w.applyPetTemplateLocked(pet, pt) // its class (pettemplate.go)
		if ownSwing {
			// no template row: pet.go's swing (an enchanter's once a round)
			pet.SwingFixed, pet.SwingKind, pet.SwingRound = fixed, swing, 0
		}
	case ok:
		was := petHitRate(pet)
		pet.SwingFixed, pet.SwingKind, pet.SwingRound = true, k, round // Sandstorm's (petrotation.go)
		hitScale = was / petHitRate(pet)                               // its damage a second kept
	}
	w.putNPCLocked(pet)
	w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	if w.pets == nil {
		w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: caster, npc: pet, stance: petSheetStance(sheet), last: now,
		taunts: caster.Class == classEnchanter && pt == nil, kind: kind, hitScale: hitScale, tmpl: pt, spell: spell.SpellID}
	w.pets[caster.ServerID] = ps
	if sess != nil {
		w.announcePetLocked(ps, sess)
	}
	tmplName := "no template"
	if pt != nil {
		tmplName = fmt.Sprintf("template %q (class %d, swing %d)", pt.tmpl.Name, pet.Class, npcSwingKind(pet))
	}
	log.Printf("Pet: %s summons %q (%s's body, model %#x, size %.2f), level %d, %d HP, fights as %s (hits x%.2f), %s",
		caster.Name, pet.Name, t.Name, uint32(pet.ModelID), pet.Size, pet.Level, pet.MaxHP, kind, hitScale, tmplName)
	if sheet != nil {
		logPetSheet(pet, sheet)
	}
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
			SetEntityState(state, data)
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
// (a dead one's corpse lies its time; a living one dies where it stands and
// goes once its death has played: petDiesLocked), and its owner told (0x007F,
// its non-collide entry off, text when there is any). sess is the owner's, or
// nil when it has none to tell. Must be called with the lock held.
func (w *World) dismissPetLocked(ps *petState, sess *session.Session, text string, now time.Time) {
	if ps.npc.Charm != nil {
		// a charmed creature is let go, not taken away (charm.go)
		w.releaseCharmLocked(ps, sess, text, false)
		return
	}
	pet := ps.npc
	if w.pets[ps.owner.ServerID] == ps {
		delete(w.pets, ps.owner.ServerID)
	}
	w.dropAggroLocked(pet.ServerID)
	if !pet.Dead {
		w.petDiesLocked(pet, now) // seen dying, then gone (petition 1398)
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

// petDismissedLies is how long the body of a pet that went alive lies dead
// before it goes: the longest death of CHAR.ESF's 536 bodies (play list slot
// 14) runs 4.0 s (the median 1.3 s; the Swarming Death's 1.0 s, a Fire
// Elemental's 1.5 s), and a second more for the channel to take the death to
// every client. A pet killed in a fight lies petCorpseTime, as before.
const petDismissedLies = 5 * time.Second

// petDiesLocked lays a pet that goes alive (dismissed, replaced by a new
// summoning, its owner gone or dead, a charm taking its place) down dead where
// it stands, as a killed creature's channel shows its death (byte 58 Die, 26 a
// corpse, health 0: serializeEntityDiscLocked), so CLIENT (UpdateCreatures'
// SetDeath) and vanilla play its death on everyone's screen; its body goes once
// that has played (petDismissedLies), as a Temporary NPC's corpse goes
// (corpseSystem). Until petition 1398 (Contintious: "whenever you dismiss a
// pet or recast a pet, the pet should show dying it currently just vanishes")
// it was taken out of the world at once. No one's kill: no experience, no
// faction, nothing on its corpse and no one to loot it (handlers/loot.go: a
// pet's). Must be called with the lock held.
func (w *World) petDiesLocked(pet *NPC, now time.Time) {
	pet.HP = 0
	w.killNPC(pet)
	pet.DeathTime = now // the tick's, as its corpse's time is counted by
	pet.CorpseDecay = petDismissedLies
	log.Printf("Pet: %q dies as it goes (its body for %v)", pet.Name, petDismissedLies)
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
		if pet.sheet != nil && !pet.sheet.TakesStance(int(cmd)) {
			// a workbook pet takes its row's stances alone (petstats.go)
			w.sendChatToSession(sess, petStanceRefused[petStance(cmd)])
			return
		}
		ps.stance = petStance(cmd)
		w.sendChatToSession(sess, petStanceWords[ps.stance])
	case petCmdDismiss:
		if pet.Charm != nil {
			// let go, it turns on its master (charm.go)
			w.releaseCharmLocked(ps, sess, "Your pet has been dismissed.", true)
			return
		}
		w.dismissPetLocked(ps, sess, "Your pet has been dismissed.", now)
	case petCmdBackOff:
		ps.target, ps.pvpTarget = 0, 0
		ps.heldUntil = now.Add(petBackOffHold)
		pet.endRound()
		w.sendChatToSession(sess, "Your pet backs off.")
	case petCmdAttack:
		ent, ok := w.findEntityByObjectIDLocked(sess, target)
		if ok && (w.duelPetCommandLocked(ps, ent) || w.pvpPetCommandLocked(ps, ent)) { // its owner's duel opponent (duelfight.go), or a foe (pvp.go)
			w.sendChatToSession(sess, "Your pet attacks "+entityNameLocked(ent)+".")
			return
		}
		npc, isNPC := ent.(*NPC)
		if ok && isNPC && challengeBars(npc, p.ServerID) {
			w.sendChatToSession(sess, fmt.Sprintf(duelLine(MetalLinePet, npc), npc.challenge.name)) // another's duel (metalmoogie.go)
			return
		}
		if !ok || !isNPC || !pet.petFights() || !petMayFight(pet, npc) {
			w.sendChatToSession(sess, petCannotAttack)
			return
		}
		if distXZ(p.X, p.Z, npc.X, npc.Z) > petLeash {
			w.sendChatToSession(sess, petTargetTooFar) // one it could fight, too far off (petition 1399)
			return
		}
		ps.target = npc.ServerID
		ps.heldUntil = time.Time{}
		w.sendChatToSession(sess, "Your pet attacks "+npc.Name+".")
	}
}

// The attack command's refusals: a target the pet may not fight at all (none,
// a player, an unattackable creature, another's pet or duel, a pet that fights
// nothing), and one it may but too far from its owner to go for (petLeash). The
// magician Waldorf's petition 1399 had the second say the first's line ("Should
// read "Target is out of range." Or something like that"). Ours: CLIENT has no
// line for a pet's (its word 723 "Out of range." is a ranged weapon's, and
// Sandstorm's red "Out of range." a cast's, castOutOfRange), nor does a recording.
const (
	petCannotAttack = "Your pet cannot attack that."
	petTargetTooFar = "Your target is too far away."
)

// petStanceRefused is the line for a stance a workbook pet does not take.
var petStanceRefused = map[petStance]string{
	petPassive:    "Your pet cannot be passive.",
	petDefensive:  "Your pet cannot be defensive.",
	petNeutral:    "Your pet cannot be neutral.",
	petAggressive: "Your pet cannot be aggressive.",
}

var petStanceWords = map[petStance]string{
	petPassive:    "Your pet is passive: it fights only when told to.",
	petDefensive:  "Your pet is defensive: it fights what attacks you or it.",
	petNeutral:    "Your pet is neutral: it fights what attacks it.",
	petAggressive: "Your pet is aggressive: it fights what you fight, and what attacks you, it or your group.",
}

// petMayFight is whether a pet may take on the creature: alive, in its world,
// attackable (not npc flags 0x80, as CLIENT's own test) and nobody's pet.
func petMayFight(pet, npc *NPC) bool {
	return npc != nil && npc != pet && !npc.Dead && npc.World == pet.World && npc.NPCType&0x80 == 0 && npc.Owner == 0 &&
		!challengeBars(npc, pet.Owner) // another's duel (metalmoogie.go)
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
	w.petCastSystemLocked(now) // the templated pets' casts landing (pettemplate.go)
	for id, ps := range w.pets {
		owner, pet := ps.owner, ps.npc
		if w.Players[id] != owner {
			// logged out, or logged in again as a new player: nobody to tell
			w.dismissPetLocked(ps, nil, "", now)
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if pet.Dead {
			w.dismissPetLocked(ps, sess, "Your pet has died.", now)
			continue
		}
		if owner.IsDead() || owner.respawnPending() {
			w.dismissPetLocked(ps, sess, "", now)
			continue
		}
		if h := pet.Charm; h != nil && !h.until.IsZero() && !now.Before(h.until) {
			w.releaseCharmLocked(ps, sess, charmWornOff, true) // charm.go
			continue
		}
		if h := pet.Charm; h != nil && charmFar(pet, h.world, pet.World, pet.X, pet.Z) {
			// led past its grace on foot, its charm still running: let go
			// there, and home at once (charm.go's leash)
			w.releaseCharmLocked(ps, sess, charmTooFar, false)
			continue
		}
		dt := float32(now.Sub(ps.last).Seconds())
		ps.last = now
		dt = min(max(dt, 0), 0.5)

		w.petEffectsLocked(ps, now)
		if pet.World != owner.World || distXZ(pet.X, pet.Z, owner.X, owner.Z) > petLost {
			if h := pet.Charm; h != nil && charmFar(pet, h.world, owner.World, owner.X, owner.Z) {
				// its master taken past its grace: the charm is gone, and it
				// home (charm.go: "a one zone grace for being CoHed or rezzed")
				w.releaseCharmLocked(ps, sess, charmWornOff, false)
				continue
			}
			// a teleport, another world, a summoned pet's coach: it comes
			// along (a coach never takes a charmed one: CoachLeavesCharm)
			oldX, oldZ := pet.X, pet.Z
			pet.World = owner.World
			pet.X, pet.Y, pet.Z = besideOwner(owner)
			pet.stopMoving()
			ps.target = 0
			pet.endRound()
			w.SpatialIndex.Update(pet.ServerID, oldX, oldZ, pet.X, pet.Z)
		}
		w.petChooseTargetLocked(ps, now)
		if w.duelPetLocked(ps, dt, now) || w.pvpPetLocked(ps, dt, now) { // its owner's duel (duelfight.go), or a foe (pvp.go)
			if sess != nil {
				w.announcePetLocked(ps, sess)
			}
			continue
		}
		if t := w.NPCs[ps.target]; ps.target != 0 && t != nil {
			w.petFightLocked(ps, t, dt, now)
		} else {
			ps.target = 0
			if !w.petTemplateTurnLocked(ps, nil, now) { // its heals and buffs while it follows (pettemplate.go)
				w.petFollowLocked(ps, dt)
			} else {
				pet.stopMoving()
			}
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
	w.petStepLocked(pet, x, z, owner.Y, speed, dt)
}

// petStepLocked steps the pet toward (x, z), its floor looked for from
// toward's height (its owner's or its target's: a floor they went down to, a
// cave under the hill it stands on), else, where the ground at its place has
// none that low, from its own height. Looked for at its target's height
// alone, a pet on a hill sent at a creature below stood inside the hill at
// the creature's height until the slope came down to it (the user,
// 2026-10-31: "when i was sending jedds pet skeleton to attack mobs, it would
// glitch under the hill i was on then run at the mob"). A floor placed on
// the ground at toward's height (a bridge's deck, a house's floor, a stair:
// the walls files', placedFloor) is taken over the terrain under it: the
// terrain's alone, a pet following its owner over a bridge walked the river
// bed under it (the user, 2026-11-01: "Jedds pet skeleton collision made it
// stand under the bridge"). Must be called with the lock held.
func (w *World) petStepLocked(pet *NPC, x, z, toward, speed, dt float32) {
	own := pet.Y
	pet.Y = toward
	w.stepNPCToward(pet, x, z, speed, dt)
	// where it was summoned is no spawn: a pet summoned on a bridge or a
	// house's floor looks for its floor as one summoned on the terrain does
	// (groundNPCLocked would keep it at its summoning height 10 off it, then
	// put it on the terrain under it); one summoned in the water keeps the
	// water's rule
	if w.ground == nil || (!w.spawnOnGroundLocked(pet) && w.spawnInWaterLocked(pet)) {
		return
	}
	y, ok := w.ground.Floor(pet.World, pet.X, toward, pet.Z)
	if deck, placed := w.placedFloorLocked(pet.World, pet.X, toward, pet.Z); placed && (!ok || deck > y) {
		y, ok = deck, true
	}
	// and the deck it walks on goes on under it, though its owner stands
	// lower: on the land at a bridge's end, under the deck's crest, a pet
	// still on the deck looked only from its owner's feet, found the river's
	// bed under the deck and swam there (2026-11-01, the same report)
	if deck, on := w.deckUnderLocked(pet.World, pet.X, own, pet.Z); on && (!ok || deck > y) {
		y, ok = deck, true
	}
	if ok {
		pet.Y = y
	} else if y, ok := w.ground.Floor(pet.World, pet.X, own, pet.Z); ok {
		pet.Y = y
	} else {
		pet.Y = own
	}
	// in deep water it swims at the surface (water.go), and its animation is
	// the floor's it stands on now: stepNPCToward grounds a pet on the
	// terrain alone and plays a swim there, so over a river a pet on a
	// bridge's deck swam on the deck (the user, 2026-11-01: "the hulking
	// bones pet is on the bridge ,but swimming in mid air. seems to trigger
	// at where the bridge meets the land")
	pet.Loco = locoIn(pet.Loco, w.swimLocked(pet), speed)
}

// locoIn is a move's animation for a body in the water or out of it: a walk
// or a run in it a swim and standing in it treading water, a swim out of it
// the walk or the run by its speed and a tread standing.
func locoIn(l byte, swims bool, speed float32) byte {
	switch {
	case swims && (l == LocoWalk || l == LocoRun):
		return LocoSwim
	case swims && l == LocoStand:
		return LocoTread
	case !swims && l == LocoSwim && speed >= NPCRunFrom:
		return LocoRun
	case !swims && l == LocoSwim:
		return LocoWalk
	case !swims && l == LocoTread:
		return LocoStand
	}
	return l
}

// supportGround is a Ground that knows every surface, what is placed on the
// terrain too (internal/ground's Support: the highest floor looking up
// within a span of heights).
type supportGround interface {
	Support(world uint8, x, y, z, up, down float32) (float32, bool)
}

// How far over and under a height a pet looks for a floor placed there: a
// stair's step over its owner's feet (not a table's top or a bridge's rail),
// and a few units under them.
const (
	petDeckUp   float32 = 0.5
	petDeckDown float32 = 4
)

// placedFloorLocked is the highest floor at (x, z) from petDeckDown under y
// to petDeckUp over it, the terrain's or anything's placed on it; false with
// a ground that knows only the terrain, or none there. Must be called with
// the lock held.
func (w *World) placedFloorLocked(world uint8, x, y, z float32) (float32, bool) {
	s, ok := w.ground.(supportGround)
	if !ok {
		return 0, false
	}
	return s.Support(world, x, y, z, petDeckUp, petDeckDown)
}

// deckGround is a Ground that tells what is placed on the terrain apart
// from it (internal/ground's PlacedSupport).
type deckGround interface {
	PlacedSupport(world uint8, x, y, z, up, down float32) (float32, bool)
}

// deckUnderLocked is the highest floor placed on the terrain (never the
// terrain's own) at (x, z) from petDeckDown under y to petDeckUp over it, y
// the pet's own height: the deck it walks on, going on under it; false with
// a ground that cannot tell, or none there. Must be called with the lock
// held.
func (w *World) deckUnderLocked(world uint8, x, y, z float32) (float32, bool) {
	p, ok := w.ground.(deckGround)
	if !ok {
		return 0, false
	}
	return p.PlacedSupport(world, x, y, z, petDeckUp, petDeckDown)
}

// petChooseTargetLocked lets go of a target that is gone, dead or too far
// from the owner, and gives a pet without one what its stance takes on:
// the nearest creature fighting it (neutral, defensive, aggressive), or
// fighting or fought by its owner's group (defensive, aggressive: its owner,
// another player of the group or another's pet; the user, 2026-11-07: "group
// members pets should attack if on defensive or offensive if a group
// member(or their pets) attacks or is attacked"), or what its owner fights
// or has hurt (aggressive).
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
	if ps.stance == petPassive || now.Before(ps.heldUntil) || !pet.petFights() {
		return
	}
	if ps.stance == petAggressive {
		if t := w.NPCs[owner.AttackTarget]; owner.Attacking() && petMayFight(pet, t) {
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
		if ps.stance != petNeutral { // defensive or aggressive: the group's fights
			mine = mine || npc.CurrentEnemy == owner.ServerID || fightsGroupMate(w, owner, npc) ||
				groupMateAttacks(w, owner, npc) || groupPetsFight(w, ps, npc)
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

// fightsGroupMate reports a creature fighting a player of the owner's group
// (petition 1132, Jibmage: "When a pets stance is set to aggressive if any players
// in the players group take damage, the pet should attack the mob hitting them"; a
// defensive pet's too since 2026-11-07). The creature must be within petAssist of
// the owner, as any the pet looks for. Must be called with the lock held.
func fightsGroupMate(w *World, owner *Player, npc *NPC) bool {
	if owner.GroupID == 0 {
		return false
	}
	m := w.Players[npc.CurrentEnemy]
	return m != nil && m.GroupID == owner.GroupID
}

// groupPetsFight reports a creature fighting another pet of the owner's group, or one
// such a pet fights (petition 1227, Waldorf: "pets should attack when other pets in the
// group attack/get hit. Pets in a group should be treated the same way as players in the
// group"): an aggressive or defensive pet's, as fightsGroupMate is for the group's
// players. Must be called with the lock held.
func groupPetsFight(w *World, ps *petState, npc *NPC) bool {
	owner := ps.owner
	if owner.GroupID == 0 {
		return false
	}
	g := w.groups[owner.GroupID]
	if g == nil {
		return false
	}
	for _, id := range g.Members {
		mate := w.pets[id]
		if mate == nil || mate == ps || mate.npc == nil || mate.npc.Dead {
			continue
		}
		if npc.CurrentEnemy == mate.npc.ServerID || mate.target == npc.ServerID {
			return true
		}
	}
	return false
}

// groupMateAttacks reports a creature another living player of the owner's group
// attacks: its auto-attack's target, or one it has hurt (on the creature's threat
// list: a hit, a harmful spell, a heal's hate in that fight). A defensive or
// aggressive pet joins it (the user, 2026-11-07). Must be called with the lock held.
func groupMateAttacks(w *World, owner *Player, npc *NPC) bool {
	if owner.GroupID == 0 {
		return false
	}
	g := w.groups[owner.GroupID]
	if g == nil {
		return false
	}
	for _, id := range g.Members {
		m := w.Players[id]
		if m == nil || m == owner || m.IsDead() {
			continue
		}
		if m.Attacking() && m.AttackTarget == npc.ServerID {
			return true
		}
		if _, hurt := npc.Threats[m.ServerID]; hurt {
			return true
		}
	}
	return false
}

// petFightLocked takes the pet to its target and fights it in rounds there.
func (w *World) petFightLocked(ps *petState, t *NPC, dt float32, now time.Time) {
	pet := ps.npc
	d := distXZ(pet.X, pet.Z, t.X, t.Z)
	if d > petReach {
		x := t.X + (pet.X-t.X)/d*(petReach-1)
		z := t.Z + (pet.Z-t.Z)/d*(petReach-1)
		w.petStepLocked(pet, x, z, t.Y, max(ps.owner.SpeedNow(), petRunSpeed), dt)
		return
	}
	pet.stopMoving()
	pet.Facing = headingTo(t.X-pet.X, t.Z-pet.Z)
	if w.petTemplateTurnLocked(ps, t, now) { // its template's abilities: casting, no melee (pettemplate.go)
		return
	}
	if t.Dead || w.NPCs[t.ServerID] != t || ps.target != t.ServerID {
		return // its taunt killed it
	}
	if ps.kind == petKindOurs && pet.sheet == nil && w.petTapLocked(ps, t, now) { // a necromancer's pet's life tap (pettap.go)
		return
	}
	length, delay, fx, hits := npcRound(pet)
	if pet.RoundTarget != t.ServerID {
		pet.endRound()
		pet.RoundTarget = t.ServerID
		ps.rounds = 0
		ps.specialAt, ps.fightStart = time.Time{}, time.Time{}
	}
	if pet.RoundStart.IsZero() || !now.Before(pet.RoundStart.Add(length)) {
		next := pet.RoundStart.Add(length)
		if pet.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		pet.RoundStart, pet.RoundHits = next, 0
		breakNPCInvis(pet) // a pet that fights is seen (invis.go)
		// a recorded kind's spell of the round goes first, as Sandstorm's
		// did, in the swing's moment (petrotation.go)
		if ps.kind != petKindOurs && w.petRoundSpellLocked(ps, t, ps.rounds+1, now) {
			return
		}
		w.broadcastNPCSwingLocked(pet, t.ServerID, uint32(delay/time.Millisecond), fx)
		ps.rounds++
		w.petSpecialAfterSwingLocked(ps, now)
		if ps.taunts && ps.rounds%petTauntEvery == 0 && w.petTauntLocked(ps, t, now) {
			return
		}
	}
	if w.petSpecialDueLocked(ps, t, now) { // a spectre's instant (petrotation.go)
		return
	}
	if w.petTemplateSpecialLocked(ps, t, now) { // a charm template's crit (pettemplate.go)
		return
	}
	for pet.RoundHits < len(hits) && !now.Before(pet.RoundStart.Add(hits[pet.RoundHits])) {
		pet.RoundHits++
		if w.petHitLocked(ps, t, now) {
			return
		}
	}
}

// petTauntLocked is the pet's taunt: its target's hate for it put above
// everyone else's, so it turns on the pet, and one more of the pet's hits
// (Grand Animation's grandAnimationTaunt). It reports a kill.
func (w *World) petTauntLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	if t.Dummy == NotDummy && t.challenge == nil { // a Metal Moogie keeps to his challenger (metalmoogie.go)
		var top int64
		for id, v := range t.Threats {
			if id != pet.ServerID && v > top {
				top = v
			}
		}
		if t.Threats == nil {
			t.Threats = make(map[EntityID]int64)
		}
		if t.Threats[pet.ServerID] <= top {
			t.Threats[pet.ServerID] = top + 1
		}
		t.CurrentEnemy = w.tauntedLocked(t, pet.ServerID) // a preferred foe of its stays its enemy (prefer.go)
		w.wakeNPCLocked(t)                                // npcsets.go
	}
	n := petMeleeHit(pet, t, meleeIntn)
	if petName(pet.Name) == grandAnimation {
		n = grandAnimationTaunt
	}
	log.Printf("Pet: %s's %q taunts %s (round %d)", ps.owner.Name, pet.Name, t.Name, ps.rounds)
	w.showPetTauntLocked(pet, t.ServerID)
	pet.LastCombatTime = now
	return w.petDamageLocked(ps, t, n, now, numberSpell) // an ability's hit (numbertags.go)
}

// petTauntFX is the visual a pet's taunt shows on its target: the Taunt
// spells' (spelleffect.fxdict of Taunt, Provoke and their kin). A taunt of
// hate alone showed nothing at all (the user, 2026-11-08: "i am not seeing the
// taunt fire though it may be").
const petTauntFX uint32 = 3483244074

// showPetTauntLocked shows everyone who sees the pet its taunt on target
// (0x00B1, as a creature's spell). Must be called with the lock held.
func (w *World) showPetTauntLocked(pet *NPC, target EntityID) {
	w.broadcastNPCSwingLocked(pet, target, 0, petTauntFX)
}

// petHitLocked is one of the pet's hits on its target, then its template's
// proc (pettemplate.go) and its effects' procs (proc.go). It reports a kill.
func (w *World) petHitLocked(ps *petState, t *NPC, now time.Time) bool {
	ps.npc.LastCombatTime = now
	if ps.npc.Charm != nil {
		// a charmed creature hits at its own strength, as a creature hits a
		// creature (charmedHitLocked), its charm template's melee_pct on it
		if w.petLandHitLocked(ps, t, ps.meleeOut(w.charmedHitLocked(ps.npc, t, meleeIntn)), now, false, numberMelee) {
			return true
		}
		if w.petTemplateProcLocked(ps, t, now) || w.petProcsLocked(ps, t, now) {
			return true
		}
		return w.creatureShieldOnPetLocked(ps, t, now)
	}
	n := petMeleeHit(ps.npc, t, meleeIntn)
	if ps.hitScale > 0 && ps.hitScale != 1 {
		n = int(math.Round(float64(n) * ps.hitScale)) // a recorded kind's (petrotation.go)
	}
	n = ps.meleeOut(n) // its template's melee_pct (pettemplate.go)
	if w.petDamageLocked(ps, t, n, now, numberMelee) {
		return true
	}
	if w.petTemplateProcLocked(ps, t, now) || w.petProcsLocked(ps, t, now) {
		return true
	}
	return w.creatureShieldOnPetLocked(ps, t, now) // creatureshield.go
}

// charmedHitLocked is a charmed creature's melee hit on a creature at its own
// strength ("they can stay mob strength", the user, 2026-11-02), as a creature
// hits a creature (guards.go's npcHitNPCLocked): a roll of its level, with
// what its effects add to its STR and DEX (its master's buffs), on a tank of
// the foe's level (npcHitOnPet's AC ten a level and AGI its level, with what
// the foe's own effects add), its level's damage share and its con's on the
// foe (npcHitLocked), as the foe's template takes it. Not the foe's con on
// it: a creature's hit takes none.
func (w *World) charmedHitLocked(pet, t *NPC, intn func(int) int) int {
	r := creatureRoll(pet.Level).withMods(pet.effectMods())
	m := t.creatureMods()
	top := int(math.Round(r.Top - r.ACSlope*float64(10*t.Level+m.AC) - agiSlope[archTank]*float64(t.Level+m.Agi)))
	hit := top - r.Width + intn(r.Width+1)
	return t.meleeTaken(w.npcHitLocked(pet, t.Level, max(hit, r.Floor, 1)))
}

// petDamageLocked is n of the pet's damage on its target, its owner's: the
// figure to the owner as the owner's (0x00DB, what made it kind, told as its
// pet's: numbertags.go), the target's hate for the pet, and on a kill the
// owner's XP and faction hits. It reports a kill.
func (w *World) petDamageLocked(ps *petState, t *NPC, n int, now time.Time, kind numberKind) bool {
	return w.petLandHitLocked(ps, t, n, now, true, kind)
}

// petLandHitLocked is petDamageLocked, balanced or landing as it is (a
// templated pet's taunt of a fixed figure: pettemplate.go).
func (w *World) petLandHitLocked(ps *petState, t *NPC, n int, now time.Time, balanced bool, kind numberKind) bool {
	owner, pet := ps.owner, ps.npc
	sess := w.findSessionByEntityIDLocked(owner.ServerID)
	tell := func() {
		if sess == nil || ps.kind != petKindOurs {
			// Sandstorm told its necromancer's zombie's, mummy's and
			// spectre's owner no hit of theirs (petrotation.go); the damage
			// meters count them where the figure would be (dpsmeter.go)
			if sess != nil && w.objectIDForEntityLocked(sess, t.ServerID) != 0 {
				w.dpsCountLocked(owner, n, true, dpsMeterNow())
			}
			return
		}
		if tid := w.objectIDForEntityLocked(sess, t.ServerID); tid != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), -n, tid, kind.byPet())
		}
	}
	if t.Dummy != NotDummy {
		// told, taken off nothing; the dummy fights its one player (dummy.go)
		tell()
		w.dummyEngagedLocked(t, owner, now)
		return false
	}
	if balanced {
		n = w.petDealtLocked(pet, t, n) // balance.go
	}
	t.HP = max(t.HP-n, 0)
	tell()
	w.markHealPetLocked(pet, t, now) // markheal.go: a mark on it heals the pet
	w.updateAggro(t, pet.ServerID, n)
	w.rootBreakLocked(t, w.balanceLevelOf(pet)) // root.go
	log.Printf("Pet: %s's %q hit %s for %d (HP %d/%d)", owner.Name, pet.Name, t.Name, n, t.HP, t.MaxHP)
	if t.HP > 0 {
		w.hitHealLocked(t, n, sess) // its heal at every hit that lands (hitheal.go)
		return false
	}
	t.Effects = nil
	w.killNPC(t)
	w.awardKillXPLocked(sess, owner, t) // the owner's kill (killexp.go)
	// the owner's faction hits, as its own kill's (faction.go)
	w.factionKillLocked(sess, owner, t)
	if owner.AttackTarget == t.ServerID {
		owner.disengage()
	}
	if ps.target == t.ServerID {
		// its fight over; a kill of another (its damage over time left on
		// one it fought before, an area spell's others) leaves its fight on
		ps.target = 0
		pet.endRound()
	}
	return true
}

// petMeleeHit is a pet's hit on a creature: a creature's roll of the pet's
// level (defence.go), its top less the target's defence as a tank's is
// (offence.go), never under the floor; what its effects add to its STR and
// DEX move the roll as a tank's do (offence.go's lines: STR the top and the
// floor, DEX the bottom, by petDexShare of the roll's width). Not measured:
// no pet's hits were recorded.
func petMeleeHit(pet, t *NPC, intn func(int) int) int {
	if hit, ok := sheetMeleeHit(pet, t, intn); ok {
		return hit // a workbook pet's weapon (petstats.go)
	}
	r := creatureRoll(pet.Level)
	m := pet.effectMods()
	def := targetDefence(t) + t.creatureDefence(pet.Level) // its effects' AC and AGI
	top := int(math.Round(r.Top - def))
	low := top - r.Width
	floor := r.Floor
	if m.Str != 0 || m.Dex != 0 {
		str := float64(m.Str)
		top = int(math.Round(r.Top - def + offHighSTR*str))
		low = min(low+int(math.Round(petDexShare*float64(m.Dex)*float64(r.Width))), top)
		floor += int(math.Floor(offFloorSTR * str))
	}
	hit := low + intn(top-low+1)
	return t.meleeTaken(max(hit, floor, 1)) // as the target's class template takes it (classtemplate.go)
}

// petDexShare is what a point of DEX raises the bottom of a pet's roll by, a
// share of the roll's width: a tank's (offence.go: the bottom's DEX term over
// the width a weapon's D gives the roll, the top's share of D less the
// bottom's), as a pet's roll has no weapon. 0.16% a point: 50 DEX lifts the
// bottom 8% of the way to the top.
var petDexShare = offenceArchs[archTank].LowShare * offLowDEX / (offHighD - offenceArchs[archTank].LowShare*offLowD)

// npcHitOnPet is a creature's hit on a pet: its roll on a tank of the pet's
// level, its AC NPCDefMod's estimate (ten a level) and its AGI its level,
// each with what its effects add (its own buffs and its owner's: the earth
// pet's skin and AGI lines, pettemplate.go), as a player's AC and AGI take
// off a creature's top (defence.go).
func npcHitOnPet(npc, pet *NPC, intn func(int) int) int {
	if pet.sheet != nil {
		return sheetHitOn(npc, pet, intn) // a workbook pet's armour and archetype (petstats.go)
	}
	r := npcMeleeRoll(npc)
	m := pet.effectMods()
	top := int(math.Round(r.Top - r.ACSlope*float64(10*pet.Level+m.AC) - agiSlope[archTank]*float64(pet.Level+m.Agi)))
	hit := top - r.Width + intn(r.Width+1)
	return max(hit, r.Floor, 1)
}

// creatureMods is what a creature's effects add to its stats in its fights,
// as a pet's do: the debuffs players and their pets put on it (the user,
// 2026-11-01: "yes make player debuffs count too"; until then a creature
// fought by its level alone, and an AC, STR, DEX or AGI debuff on one did
// nothing) and the buffs a charmed creature kept when it went free ("make
// the released mob's buffs count in its fights too"), each until it runs
// out, only the best of a line (bestOfLine). STR and DEX move its rolls as
// a pet's effects move its own (meleeRoll.withMods: on players, pets and
// guards), AC and AGI the hits on it as a player's take off a creature's
// (creatureDefence), a maximum its
// maximum hit points (syncEffectMaxHP); its resists were counted already
// (workbook.go's npcResistKindPct), its STA, WIS, INT and CHA count for
// nothing (no creature's hit points or hits come from them). Nothing for a
// pet: its effects count through effectMods.
func (n *NPC) creatureMods() StatMods {
	var m StatMods
	if n.Owner != 0 {
		return m
	}
	for i, e := range n.Effects {
		if !n.bestOfLine(i) {
			continue
		}
		m.sum(e.Mods)
	}
	return m
}

// bestOfLine is whether the NPC's i-th effect is the best of its line on it
// (the highest rank, the first of equals), as only the best of a line counts
// on a player (buffline.go): two ranks of a debuff's line on a creature count
// as the higher. An effect of no line always counts.
func (n *NPC) bestOfLine(i int) bool {
	e := n.Effects[i]
	if e.Line == "" {
		return true
	}
	for j, o := range n.Effects {
		if j == i || !sameLine(o.Line, e.Line) {
			continue
		}
		if o.Rank > e.Rank || o.Rank == e.Rank && j < i {
			return false
		}
	}
	return true
}

// creatureDefence is what a creature's AC and AGI effects take off an
// attacker's roll on it (an AC debuff: a negative, the roll higher): as a
// player's take off a creature's top (defence.go: the creature line's AC
// slope at the attacker's level, a tank's per point of AGI). Ours: no record
// of a creature's armour against a player exists.
func (n *NPC) creatureDefence(attackerLevel int) float64 {
	m := n.creatureMods()
	if m.AC == 0 && m.Agi == 0 {
		return 0
	}
	return creatureRoll(attackerLevel).ACSlope*float64(m.AC) + agiSlope[archTank]*float64(m.Agi)
}

// effectMods is what the NPC's effects add to its stats: a pet's own buffs
// and those cast on it, which its fights take (npcHitOnPet, petMeleeHit); a
// creature's are creatureMods.
func (n *NPC) effectMods() StatMods {
	var m StatMods
	for _, e := range n.Effects {
		m.sum(e.Mods)
	}
	return m
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
// pet dies at 0 (petSystem takes it away). Out of its reach it runs at the
// pet as at a player (chaseLocked), so a pet backed off to its owner brings
// the creature along to the group (Threnody's petition 1280, 2026-11-08:
// "pets are dropping agro when you use pet back off command to bring mob to
// group": a pet more than 10 off was dropped at once); past its leash, or
// with the pet in another world, it gives up and goes home as from a player.
func (w *World) npcFightPetLocked(npc, pet *NPC, now time.Time) {
	npc.chase.forRoute(pet.ServerID)
	in, giveUp := w.chaseLocked(npc, pet.X, pet.Y, pet.Z, pet.VelX, pet.VelZ, now)
	if giveUp || pet.World != npc.World {
		w.giveUpLocked(npc)
		return
	}
	if !in {
		npc.loseDueHits(now) // closing in: no hit lands from out of reach
		if npc.tmpl != nil {
			w.templateFarTurnLocked(npc, pet.ServerID, now) // its casts from afar (classtemplate.go)
		}
		return
	}
	// in reach its moving and turning are the chase's, as at a player: it
	// closes in and then stands, its channel's run ended with its steps (the
	// rubberbanding of F3 20261031-061313, a run kept while it stood, cannot
	// come back: chaseLocked stops it when it stands)
	if npc.RoundTarget != pet.ServerID {
		npc.endRound()
		npc.RoundTarget = pet.ServerID
	}
	if npc.tmpl != nil && w.templateTurnLocked(npc, pet.ServerID, now) {
		npc.loseDueHits(now) // casting at the pet: no swing meanwhile (classtemplate.go)
		return
	}
	if npc.tmpl != nil {
		defer func() {
			if !pet.Dead && !npc.Dead && npc.CurrentEnemy == pet.ServerID {
				w.templateSpecialLocked(npc, pet.ServerID, now) // its kick
			}
		}()
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
		src := npc.figures()                                                                                        // an add's are its creature's (summon.go)
		n := npc.shared(w.npcHitLocked(src, w.balanceLevelOf(pet), src.meleeOut(npcHitOnPet(src, pet, meleeIntn)))) // balance.go, then its class template
		n = w.petTakenLocked(pet, n)                                                                                // the pet's template's taken_pct (pettemplate.go)
		pet.HP = max(pet.HP-n, 0)
		pet.LastCombatTime = now
		log.Printf("Pet: %s hit the pet %q for %d (HP %d/%d)", npc.Name, pet.Name, n, pet.HP, pet.MaxHP)
		w.rampageLocked(npc, pet.ServerID, n, now) // an enraged creature's share on the second of its list (enrage.go)
		if pet.HP == 0 {
			w.killNPC(pet)
			w.dropAggroLocked(pet.ServerID)
			log.Printf("Pet: %q killed by %s", pet.Name, npc.Name)
			return
		}
		if w.creatureProcOnPetLocked(npc, pet, now) { // a template's melee proc (creatureshield.go)
			return
		}
		if w.petShieldsLocked(pet, npc, now) { // its damage shield and defensive procs (dshield.go)
			return
		}
	}
}

// retargetLocked turns a creature to its highest threat left, or to none.
func (w *World) retargetLocked(npc *NPC) {
	npc.CurrentEnemy = w.pickEnemyLocked(npc) // a preferred foe first (prefer.go)
	w.wakeNPCLocked(npc)                      // its next enemy, or its fight to put away (npcsets.go)
}

// landOnPetLocked is a beneficial spell on a pet: a heal now, and what lasts
// kept on it as the best of its line (putOnPetLocked; its heals tick in
// petEffectsLocked, its AC, AGI, STR and DEX count in its fights: pet.go's
// npcHitOnPet and petMeleeHit; its other stats a pet does not use).
func (w *World) landOnPetLocked(cs *castState, caster *Player, casterSess *session.Session, pet *NPC, now time.Time) {
	// a spell that raises the maximum heals after the raise, as on a player
	// (landOnPlayerLocked), or a pet at full health would get none of it
	raises := cs.spell.Def.RaisesMaxHP()
	heal := func() {
		for _, e := range cs.spell.Def.Effects {
			if e.Kind != EffectHP {
				continue
			}
			switch n := spellAmount(e, caster); {
			case n > 0 && pet.HP < pet.MaxHP:
				pet.HP = min(pet.HP+n, pet.MaxHP)
				w.tellAmountLocked(caster, casterSess, pet.ServerID, n, cs.spell.healKind())
			case n < 0 && !e.Self && !pet.Dead:
				// its owner's blast on it (the alchemist's Destroy
				// Contraption, -20000: until 2026-11-01 a friendly spell's
				// hit on a pet was dropped and the contraption stood)
				pet.HP = max(pet.HP+n, 0)
				w.tellAmountLocked(caster, casterSess, pet.ServerID, n, cs.spell.hitKind())
				if pet.HP == 0 {
					log.Printf("Cast: %s's %q destroys the pet %q", caster.Name, cs.spell.Name, pet.Name)
					w.killNPC(pet)
				}
			}
		}
	}
	if !raises {
		heal()
	}
	// power given now (an enchanter's Power Boon, Jet, Surge: the wiki's
	// charmed tree that nukes again "as soon as it has a little bit more
	// power"), into the pet's own pool (charm.go, npcregen.go)
	for _, e := range cs.spell.Def.Effects {
		if e.Kind == EffectPower && !e.Self && pet.MaxPower > 0 {
			if n := spellAmount(e, caster); n > 0 && pet.Power < pet.MaxPower {
				pet.Power = min(pet.Power+n, pet.MaxPower)
			}
		}
	}
	if ae := lasting(cs, caster, now); ae != nil {
		if over := putOnPetLocked(pet, &cs.spell, ae); over != nil {
			log.Printf("Cast: %s's %q (rank %d) does not take on the pet %q: %q (rank %d) is on, line %s",
				caster.Name, cs.spell.Name, ae.Rank, pet.Name, over.Name, over.Rank, ae.Line)
			if casterSess != nil {
				w.sendColoredLocked(casterSess, fmt.Sprintf(lineStronger, pet.Name, over.Name), castRefusalRGB)
			}
		}
		pet.syncEffectMaxHP()
	}
	if raises {
		heal()
	}
	log.Printf("Cast: %s's %q lands on the pet %q (%d/%d HP, %d effects)", caster.Name, cs.spell.Name, pet.Name, pet.HP, pet.MaxHP, len(pet.Effects))
}

// A pet's maximum hit points take its effects (the user, 2026-11-01, of the
// shaman's Avatar: "a one time max hp buff and massive stat buff"; until
// then a pet's maximum took none, so Avatar on a pet only healed it): MaxHP
// is its own (the balance's, its template's) plus what its effects add,
// effMaxHP. A raise's heal comes after it (landOnPetLocked); an effect's end
// takes its raise back within the pet's tick (petEffectsLocked), whatever
// took the effect off, its hit points kept within the lower maximum; the
// balance setting a pet's hit points again keeps the raise
// (applyBalanceLocked). A charmed creature set free keeps its effects and
// their raise until each runs out (the user, 2026-11-01: "is charm
// breaks/release the mob retains all buffs until they expire": charm.go's
// releaseCharmLocked puts it on the creatures' effects, effectSystem, which
// takes the raise back as they end); a creature's maximum takes its effects
// as a pet's does, a player's debuff on it too (creatureMods); a respawn is
// a new life, without them.

// effectsMaxHP is what the NPC's effects add to its maximum hit points: each
// effect's maximum and its hit point factor at the NPC's level, as a
// player's (MaxHPNow).
func (n *NPC) effectsMaxHP() int {
	add := 0
	for _, ae := range n.Effects {
		add += ae.Mods.MaxHP + ae.Mods.HPFactor*n.Level
	}
	return add
}

// syncEffectMaxHP moves the NPC's maximum hit points by what its effects add
// now against what they added, its hit points kept within it.
func (n *NPC) syncEffectMaxHP() {
	add := n.effectsMaxHP()
	if add == n.effMaxHP {
		return
	}
	n.MaxHP = max(n.MaxHP+add-n.effMaxHP, 1)
	n.effMaxHP = add
	n.HP = min(n.HP, n.MaxHP)
}

// dropEffectMaxHP takes back what the NPC's effects added to its maximum (a
// respawn: its effects gone with its life).
func (n *NPC) dropEffectMaxHP() {
	if n.effMaxHP == 0 {
		return
	}
	n.MaxHP = max(n.MaxHP-n.effMaxHP, 1)
	n.effMaxHP = 0
	n.HP = min(n.HP, n.MaxHP)
}

// putOnPetLocked keeps a beneficial spell's lasting effect on a pet, only the
// best of its line counting as on a player (buffline.go), now that a pet's
// stats count: under an effect of its line of a higher rank it does not take
// (that effect given back); else it takes off every effect of its line of
// its rank or below, and an earlier cast of the same spell, and goes on.
func putOnPetLocked(pet *NPC, spell *CastSpell, ae *ActiveEffect) (over *ActiveEffect) {
	ae.Line, ae.Rank = BuffLine(spell), lineRank(spell)
	for _, e := range pet.Effects {
		if sameLine(e.Line, ae.Line) && e.SpellID != ae.SpellID && e.Rank > ae.Rank {
			return e
		}
	}
	left := pet.Effects[:0]
	for _, e := range pet.Effects {
		if e.SpellID != ae.SpellID && !sameLine(e.Line, ae.Line) {
			left = append(left, e)
		}
	}
	pet.Effects = append(left, ae)
	return nil
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
		if ae.HPTick <= 0 && ae.PowerTick <= 0 || ae.NextTick.IsZero() || now.Before(ae.NextTick) {
			continue
		}
		ae.NextTick = ae.NextTick.Add(effectTick)
		n, pw := ae.HPTick, ae.PowerTick
		if ae.Rules.Twice { // the workbook's first tick, twice (workbook.go)
			ae.Rules.Twice, n, pw = false, 2*n, 2*pw
		}
		if n > 0 && pet.HP < pet.MaxHP {
			pet.HP = min(pet.HP+n, pet.MaxHP)
		}
		if pw > 0 && pet.MaxPower > 0 && pet.Power < pet.MaxPower {
			pet.Power = min(pet.Power+pw, pet.MaxPower) // its power over time (charm.go: what its master gives it)
		}
	}
	pet.Effects = left
	pet.syncEffectMaxHP() // a raise that ended (or went another way) taken back
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

// ItemSummon is what a summoning item's use came to (SummonByItem).
type ItemSummon int

const (
	ItemSummonNone ItemSummon = iota // no player, a dead one, or no body for the pet
	ItemSummoned
	ItemPetOut     // the item's own pet is out already
	ItemPetResting // not before notBefore
)

// SummonByItem brings a summoning spell's pet as an item's use, with no cast
// (handlers/guenhwyvar.go): as the spell's landing does (summonPetLocked), a
// pet of another kind replaced. The item's own pet out, or the time before
// notBefore, brings nothing.
func (w *World) SummonByItem(sess *session.Session, spell CastSpell, now, notBefore time.Time) ItemSummon {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() {
		return ItemSummonNone
	}
	name := petSpellName(spell.Name)
	if s := w.templates.sheets[spell.SpellID]; s != nil && s.Name != "" {
		name = s.Name
	}
	if old := w.pets[p.ServerID]; old != nil && old.npc != nil && !old.npc.Dead && petName(old.npc.Name) == petName(name) {
		return ItemPetOut
	}
	if now.Before(notBefore) {
		return ItemPetResting
	}
	w.summonPetLocked(&castState{spell: spell, caster: p.ServerID}, p, sess, now)
	if ps := w.pets[p.ServerID]; ps == nil || ps.npc == nil || petName(ps.npc.Name) != petName(name) {
		return ItemSummonNone
	}
	return ItemSummoned
}
