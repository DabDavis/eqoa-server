package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// merc.go — mercenaries, as EverQuest's (the owner, 2026-11-10: "lets add
// mercenary pets similar to EQ 1. any class same level as player with
// heirloom gear, casts same abilities as player on cooldown. if healer heal
// team members, if tank they taunt"; his choices that day: a Mercenary
// Liaison hires one for nothing, beside each city's announcements board and
// Dabrish; one a character; one beside the class pet; the Heirloom Test
// Server alone, world.mercs; no experience for it; then "mercs will occupy a
// group slot" and "mercs can be any race the player picks").
//
// A merc is an NPC with no row (Temporary) owned by its hirer (NPC.Owner),
// so the server keeps it on the players' side as a pet: no player's hit or
// harmful spell, no other pet's, no guard and no creature's sight takes it
// on, its corpse is no one's loot, a creature fights it as it fights a pet
// (npcFightPetLocked) and a player's heal or buff lands on it as on a pet
// (landOnPetLocked). It is not the pet: CLIENT keeps one pet id (m_petId)
// and its bar shows the class pet; nothing of the pet's slot (w.pets, 0x007E,
// 0x0041) is a merc's, which is commanded by /merc (handlers/merc.go).
//
// What the handlers give it is a MercKit: its class, race, sex and name, the
// heirlooms a player of its class wears at its level (their looks on its
// channel, what they add as a player's gear), a player's base stats at that
// level (SimBaseStats: training points spent as its archetype spends them)
// and the class's book up to it (SimBookFromDB). Its body is a Player that is
// never in the world (mercState.body): its stats, its hit points, its power,
// its armour, its melee, its defence and its spells' figures are a player's,
// through the player's own math (stats.go, gear.go, offence.go, defence.go,
// cast.go's spellAmount), and a creature's hit on it is a player's
// (npcHitOnPet). Its level is its owner's, followed at once (its figures by
// the handlers' new kit after it: OnMercNote's MercNoteLevel).
//
// It follows its owner as a pet does (pet.go's step, floors, swimming, and
// its coming along through a teleport or into another world, but never into
// capture the flag's arena: it waits outside, ctfaway.go), assists it
// (mercfight.go: what its owner fights, never a player, never a duel's) and
// casts as its role does (merccast.go). A kill of its is its owner's, as a
// pet's: the experience (no share of it its own: the split is the players'),
// the factions, the quests. It is a member of its owner's group (mercgroup.go)
// and it goes with its owner: at a logout (its hire kept: the handlers bring
// it back at the next login), a death of its own (a rest, then /merc), a
// dismissal, a keep restart carrying it (keepmerc.go).

// MercStance is how far a merc takes a fight on (/merc stance).
type MercStance uint8

const (
	// MercPassive fights only what /merc attack names; a healer still heals
	MercPassive MercStance = 1
	// MercAssist fights what its owner called it to (merccall.go): its
	// owner's auto-attack's target, a creature its owner did a hostile act on
	// or that fights one he healed or helped, and what fights the merc itself
	// (the default)
	MercAssist MercStance = 2
	// MercAggressive took on what fought or was fought by its owner's group;
	// since GM ticket 1959 (merccall.go) it fights as MercAssist, kept so a
	// saved stance and the word still read
	MercAggressive MercStance = 3
)

// MercStanceNames are the stances' words, as /merc takes them.
var MercStanceNames = map[MercStance]string{MercPassive: "passive", MercAssist: "assist", MercAggressive: "aggressive"}

// MercStanceOf is a stance's word's stance, false for none.
func MercStanceOf(word string) (MercStance, bool) {
	for s, w := range MercStanceNames {
		if w == word {
			return s, true
		}
	}
	return 0, false
}

// mercRole is what a merc's class does in a fight (merccast.go).
type mercRole uint8

const (
	roleTank   mercRole = iota // warrior, paladin, shadow knight: taunts, melee
	roleMelee                  // ranger, monk, bard, rogue: arts and melee
	roleHealer                 // druid, shaman, cleric: heals the team
	roleCaster                 // magician, necromancer, enchanter, wizard, alchemist: spells
)

// mercRoleOf is a class's role, by its archetype (defence.go's: the hit
// point factor).
func mercRoleOf(class int) mercRole {
	switch archetypeOf(class) {
	case archTank:
		return roleTank
	case archMelee:
		return roleMelee
	case archPriest:
		return roleHealer
	}
	return roleCaster
}

// MercRoleName is a class's role as the liaison and /merc say it.
func MercRoleName(class int) string {
	return [...]string{roleTank: "tank", roleMelee: "melee fighter", roleHealer: "healer", roleCaster: "caster"}[mercRoleOf(class)]
}

// MercKit is a merc as the handlers make it (handlers/merc.go): who it is,
// what it wears, its base stats and its book, at its level.
type MercKit struct {
	Class, Race, Sex, Level int
	Name                    string
	// Base is its base stats, STR..CHA, before what it wears
	Base [7]int
	// Gear is what its heirlooms add, as a player's worn items' (gear.go)
	Gear Gear
	Look MercLook
	// Worn is the equip slots its hands fill (14 main hand, 15 off hand, 16
	// both, 17 shield, 18 ranged), for its swing (GetSwing); AttackType the
	// swinging weapon's and Range its reach (SwingFor)
	Worn              [21]bool
	AttackType, Range int
	// Spells is its book: the class's up to its level
	Spells []CastSpell
	// Bow is the bow a tank or a melee fighter shoots at range (mercshoot.go)
	Bow MercBow
	// PetPick is the words of the pet kind its hirer picked (/merc pet:
	// mercpet.go's mercPetSpell), none for its best
	PetPick []string
	// SpellsOff is the spells its hirer turned off in its menu (handlers'
	// mercmenu.go), by id: its book keeps them out (mercBook)
	SpellsOff []int
	// Offensive: a healer's /merc offensive, its hits and damage over time
	// cast as well as its heals; false, defensive (the default), heals and
	// buffs alone (merccast.go's mercAttackTurnLocked)
	Offensive bool
}

// MercLook is what a merc's channel shows of its body and its heirlooms.
type MercLook struct {
	Face, HairColor, HairLength, HairStyle   int
	Chest, Bracer, Gloves, Legs, Boots, Helm byte
	// Tint is its armour's disc tint, 0-14 (the channel's 99-104)
	Tint byte
	// Robe is the robe's style, -1 for none
	Robe                       int
	Primary, Secondary, Shield uint32
	PrimaryType, SecondaryType byte
	// WornPieces and WornMask: the item records on the pieces an aura can be
	// on (Player.WornPieces; pieceaura.go)
	WornPieces [AuraPieces]int
	WornMask   uint8 `json:",omitempty"`
}

// mercState is one merc, kept by its owner's id and its slot.
type mercState struct {
	owner *Player
	// slot is its hire's (1 to MercSlots: charmerc.slot, /merc 2): its follow
	// place (followslot.go) and its non-collide entry
	slot int
	npc  *NPC
	// body is the merc as a player of its class and level, never in the
	// world: its stats and figures (the file's doc); its id is the NPC's
	body *Player
	kit  MercKit
	role mercRole
	// life tells this hire's casts from an earlier one's (merccast.go)
	life   uint64
	stance MercStance
	// stay holds it where it stood (/merc stay), stayAt that place
	stay   bool
	stayAt [3]float32
	// target is what it fights, 0 none; ordered what /merc attack named,
	// fought whatever its stance until it dies or goes
	target, ordered EntityID
	last            time.Time // its last step
	// combatReady is the end of the wait at the start of automatic combat.
	// It stays set across targets until the merc and its owner's fight ends.
	combatReady time.Time
	// ready is when each spell may be cast again (its lockout: cast time,
	// recast and castLockoutExtra from its start), by id; artReady the arts'
	// global cooldown (balance.go); busy a cast's time and castBusy after it,
	// when it neither casts again nor swings
	ready    map[int]time.Time
	artReady time.Time
	busy     time.Time
	// noCollide: its entry is on its owner's non-collide list
	noCollide bool
	// asked: its owner was asked into a group with it since it came
	// (mercgroup.go); full: told the group had no room
	asked, full bool
	// levelAsked is the level its kit was last asked for again (MercNoteLevel)
	levelAsked int
	// regroup: a keep restart carried it in its owner's group, its owner's
	// client holding the group (keepmerc.go): the group upkeep puts it back
	regroup bool
	// book is its book as it casts it (merccast.go), made for bookOf, its
	// body when it was made
	book   []mercSpell
	bookOf *Player
	// reactTo is the foe it last turned its attacks on, reactAt when it may
	// begin them (mercoffbeat.go)
	reactTo EntityID
	reactAt time.Time
	// pet is its own pet, nil for none out (mercpet.go)
	pet *petState
	// idle is its ease while its owner stands (mercidle.go)
	idle mercIdle
	// petBook is its pet's heals and buffs (mercpetbuff.go), made for
	// petBookOf, its body, as book is
	petBook   *mercPetSpells
	petBookOf *Player
	// bowOut: its bow in its hands (mercshoot.go); shot its arrow on its way,
	// shotReady when it may shoot again
	bowOut    bool
	shot      *mercShot
	shotReady time.Time
}

// mercNoCollideID is a merc's entry on its owner's non-collide list (0x1003:
// CLIENT's ignore list, pet.go): past the pet's petNoCollideID, as an
// ignored player's count up from it; the first slot's, the familiar's next
// (familiarNoCollideID), the second and third slots' after it.
const mercNoCollideID uint32 = petNoCollideID + 1

// MercSlots is how many mercs a player may have at once (the owner,
// 2026-11-12: "if a player wants to have 3 mercenaries in their group it is
// fine"), as database.MercSlots.
const MercSlots = 3

// mercNoCollideIDOf is the merc of the slot's entry on its owner's
// non-collide list.
func mercNoCollideIDOf(slot int) uint32 {
	if slot <= 1 {
		return mercNoCollideID
	}
	return familiarNoCollideID + uint32(slot-1)
}

// MercRest is how long a dead merc rests before /merc brings it back (ours:
// the owner's "after a rest (60 s, your call)").
const MercRest = 60 * time.Second

// The lines a merc's owner is told (ours).
const (
	mercDiedLine  = "Your mercenary %s has died. It rests %d seconds, then comes back by itself."
	mercLevelLine = "Your mercenary %s is level %d."
)

// MercNoteKind is what became of a merc, for the handlers (OnMercNote).
type MercNoteKind uint8

const (
	// MercNoteDied: it died; its owner's row rests until Until
	MercNoteDied MercNoteKind = iota + 1
	// MercNoteLevel: its owner's level is no longer its kit's (a level up,
	// #setlevel, Dabrish): the handlers make its kit again at Level
	MercNoteLevel
	// MercNoteGone: it left the world with its owner (a logout); its hire
	// stays
	MercNoteGone
	// MercNoteDismissed: its hire ended other than by its owner's /merc
	// dismiss (a group leader's boot: mercgroup.go)
	MercNoteDismissed
	// MercNoteBack: its owner left the arena, where it waited outside
	// (ctfaway.go), or a place in its owner's group freed for it on standby
	// (mercbench.go): the handlers make it again from its row (Slot's; 0
	// every row's), as at a login
	MercNoteBack
)

// MercNote is a merc's event, handed to the hook after the tick's lock.
type MercNote struct {
	Kind  MercNoteKind
	Owner *Player
	// Slot is the merc's (charmerc.slot), 0 for every one of its owner's
	Slot  int
	Sess  *session.Session
	Name  string
	Level int
	Until time.Time
}

// MercNoteFunc hears a merc's events (handlers/merc.go: its row, its kit).
type MercNoteFunc func(MercNote)

// OnMercNote sets the mercs' event hook.
func (w *World) OnMercNote(fn MercNoteFunc) {
	w.mu.Lock()
	w.onMercNote = fn
	w.mu.Unlock()
}

// noteMercLocked queues a merc's event for runMercNotes. Must be called with
// the lock held.
func (w *World) noteMercLocked(n MercNote) {
	w.mercNotes = append(w.mercNotes, n)
}

// runMercNotes hands the tick's merc events to the hook. Must be called
// WITHOUT the lock.
func (w *World) runMercNotes() {
	w.mu.Lock()
	notes, fn := w.mercNotes, w.onMercNote
	w.mercNotes = nil
	w.mu.Unlock()
	if fn == nil {
		return
	}
	for _, n := range notes {
		fn(n)
	}
}

// mercHiredByLocked reports id a merc whose owner is owner (dead or not: a
// heal over time it laid is still its own). Must be called with the lock held.
func (w *World) mercHiredByLocked(id, owner EntityID) bool {
	n := w.NPCs[id]
	return n.isMerc() && n.merc.owner != nil && n.merc.owner.ServerID == owner
}

// mercsOfLocked is the player's mercs out, alive, by slot. Must be called
// with the lock held.
func (w *World) mercsOfLocked(p *Player) []*mercState {
	if p == nil {
		return nil
	}
	var out []*mercState
	for _, ms := range w.mercs[p.ServerID] {
		if ms.owner == p && !ms.npc.Dead {
			out = append(out, ms)
		}
	}
	return out
}

// mercAtLocked is the player's merc of the slot out, alive, or nil. Must be
// called with the lock held.
func (w *World) mercAtLocked(p *Player, slot int) *mercState {
	for _, ms := range w.mercsOfLocked(p) {
		if ms.slot == slot {
			return ms
		}
	}
	return nil
}

// mercLiveLocked reports the merc still out (dead or not: its death is seen
// at the merc system's turn). Must be called with the lock held.
func (w *World) mercLiveLocked(ms *mercState) bool {
	if ms == nil || ms.owner == nil {
		return false
	}
	for _, o := range w.mercs[ms.owner.ServerID] {
		if o == ms {
			return true
		}
	}
	return false
}

// putMercLocked keeps the merc among its owner's, by slot. Must be called
// with the lock held.
func (w *World) putMercLocked(ms *mercState) {
	if w.mercs == nil {
		w.mercs = make(map[EntityID][]*mercState)
	}
	id := ms.owner.ServerID
	list := append(w.mercs[id], ms)
	sort.SliceStable(list, func(i, j int) bool { return list[i].slot < list[j].slot })
	w.mercs[id] = list
}

// dropMercLocked lets the merc go from its owner's. Must be called with the
// lock held.
func (w *World) dropMercLocked(ms *mercState) {
	id := ms.owner.ServerID
	list := w.mercs[id]
	for i, o := range list {
		if o == ms {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(w.mercs, id)
		return
	}
	w.mercs[id] = list
}

// allMercsLocked is every merc out, its owners' in turn. Must be called with
// the lock held.
func (w *World) allMercsLocked() []*mercState {
	var out []*mercState
	for _, list := range w.mercs {
		out = append(out, list...)
	}
	return out
}

// mercPickLocked is the player's mercs out an order names: the slot's (1 to
// MercSlots), or every one for 0. Must be called with the lock held.
func (w *World) mercPickLocked(p *Player, slot int) []*mercState {
	if slot == 0 {
		return w.mercsOfLocked(p)
	}
	if ms := w.mercAtLocked(p, slot); ms != nil {
		return []*mercState{ms}
	}
	return nil
}

// MercOf is the session's player's first merc's entity id, 0 for none.
func (w *World) MercOf(sess *session.Session) EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if list := w.mercsOfLocked(w.sessionPlayerLocked(sess)); len(list) > 0 {
		return list[0].npc.ServerID
	}
	return 0
}

// MercInfo is what /merc says of a merc.
type MercInfo struct {
	Slot                       int
	Name                       string
	Class, Race, Sex, Level    int
	HP, MaxHP, Power, MaxPower int
	Stance                     MercStance
	Stay                       bool
	Target                     string
	// Offensive: a healer's mode (MercKit.Offensive)
	Offensive bool
	// Grouped: in its owner's group
	Grouped bool
}

// MercInfosOf is the session's player's mercs out, by slot.
func (w *World) MercInfosOf(sess *session.Session) []MercInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []MercInfo
	for _, ms := range w.mercsOfLocked(w.sessionPlayerLocked(sess)) {
		n := ms.npc
		info := MercInfo{Slot: ms.slot, Name: n.Name, Class: ms.kit.Class, Race: ms.kit.Race, Sex: ms.kit.Sex, Level: n.Level, HP: n.HP,
			MaxHP: n.MaxHP, Power: n.Power, MaxPower: n.MaxPower, Stance: ms.stance, Stay: ms.stay, Grouped: w.mercInOwnGroupLocked(ms), Offensive: ms.kit.Offensive}
		if t := w.NPCs[ms.target]; t != nil {
			info.Target = t.Name
		}
		out = append(out, info)
	}
	return out
}

// MercInfoOf is the session's player's first merc out, false for none.
func (w *World) MercInfoOf(sess *session.Session) (MercInfo, bool) {
	if list := w.MercInfosOf(sess); len(list) > 0 {
		return list[0], true
	}
	return MercInfo{}, false
}

// mercModelLocked is the body a merc of race and sex wears: a race of
// creation's past the game's own its row's (CreationBody: the froglok's, a
// racedefs row's), a race of the game's its players' (charactermodel, its
// other sex's when the race has one body), else its owner's own. Must be
// called with the lock held.
func (w *World) mercModelLocked(race, sex int, owner *Player) uint32 {
	if b := CreationBody(race, sex); b != 0 {
		return uint32(b)
	}
	if m := w.raceModels[raceSex{race, sex}]; m != 0 {
		return m
	}
	if m := w.raceModels[raceSex{race, 1 - sex}]; m != 0 {
		return m
	}
	return uint32(owner.ModelID)
}

// RaceModel is the body a merc of race and sex wears (mercModelLocked), false
// when the server knows none: the handlers' look at its body's faces.
func (w *World) RaceModel(race, sex int) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	m := w.mercModelLocked(race, sex, &Player{})
	return m, m != 0
}

// newMercBody is the merc as a player of its kit (the file's doc), under the
// NPC's id.
func newMercBody(id EntityID, k *MercKit) *Player {
	p := &Player{ServerID: id, Name: k.Name, Class: k.Class, Race: k.Race, Sex: k.Sex, Level: max(k.Level, 1),
		Speed: RunSpeed, Faction: FactionFriendly}
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex = k.Base[0], k.Base[1], k.Base[2], k.Base[3]
	p.BaseWis, p.BaseInt, p.BaseCha = k.Base[4], k.Base[5], k.Base[6]
	p.Gear = k.Gear
	p.PrimaryType, p.SecondaryType = k.Look.PrimaryType, k.Look.SecondaryType
	p.Primary, p.Secondary = k.Look.Primary, k.Look.Secondary
	if k.Worn[17] {
		p.Shield = max(k.Look.Shield, 1) // a shield in hand, for a spell's required equipment (equipFits)
	}
	p.Swing = SwingFor(k.Class, p.Level, k.Worn, k.AttackType != 0, k.AttackType, k.Range)
	p.RecalcStats(true)
	return p
}

// dressMercLocked puts the kit's body and looks on the merc's NPC: its body
// and size by its race and sex, its race on the wire (WireRace: a race of
// racedefs passes as its wire race, as its players do), its hands, armour,
// robe and hair, its swing a player's of its class and level. Must be called
// with the lock held.
func (w *World) dressMercLocked(n *NPC, k *MercKit, owner *Player) {
	l := &k.Look
	n.ModelID = int64(w.mercModelLocked(k.Race, k.Sex, owner))
	// a race whose own body takes no armour (the hill giant's) in the body of
	// the armour on its chest, as its players are (armorbody.go; the owner,
	// 2026-11-11: "mercenaries arent always wearing armor visually, like uwu's
	// merc ithiel", a hill giant necromancer in its robe)
	if b := w.armorBodies[k.Race][k.Gear.Armor]; b != 0 && takesArmorBody(k.Race, uint32(n.ModelID)) {
		n.ModelID = int64(b)
	}
	n.Size = w.raceSizeLocked(k.Race)
	n.Race = strconv.Itoa(WireRace(k.Race))
	n.Face, n.HairColor, n.HairLength, n.HairStyle = l.Face, l.HairColor, l.HairLength, l.HairStyle
	n.Torso, n.Forearm, n.NPCGloves = int(l.Chest), int(l.Bracer), int(l.Gloves)
	n.NPCLegs, n.Feet, n.Head = int(l.Legs), int(l.Boots), int(l.Helm)
	n.Tint = int(l.Tint)
	n.RobeStyle = ""
	if l.Robe >= 0 {
		n.RobeStyle = strconv.Itoa(l.Robe)
	}
	n.PrimaryWeapon, n.SecondaryWeapon, n.Shield = int(l.Primary), int(l.Secondary), int(l.Shield)
	n.PrimaryType, n.SecondaryType = l.PrimaryType, l.SecondaryType
	n.WornPieces, n.WornMask = l.WornPieces, l.WornMask
	n.Class = k.Class
}

// spawnMercLocked makes the merc of the kit beside its owner: its NPC dressed
// and at a player's hit points and power, in the world, its owner's. Must be
// called with the lock held.
func (w *World) spawnMercLocked(owner *Player, slot int, kit MercKit, stance MercStance, now time.Time) *mercState {
	if kit.Level <= 0 || kit.Level != owner.Level {
		kit.Level = owner.Level
	}
	id := w.allocFreeIDLocked()
	n := &NPC{ServerID: id, Name: kit.Name, World: owner.World, Level: kit.Level, Faction: FactionFriendly, Temporary: true,
		RespawnTime: petCorpseTime, CorpseDecay: petCorpseTime, Owner: owner.ServerID}
	w.dressMercLocked(n, &kit, owner)
	n.X, n.Y, n.Z = w.followPlaceLocked(n, owner, mercFollowSlot(slot))
	n.Facing = headingTo(owner.X-n.X, owner.Z-n.Z)
	n.SpawnX, n.SpawnY, n.SpawnZ, n.SpawnFacing = n.X, n.Y, n.Z, n.Facing
	body := newMercBody(id, &kit)
	n.SwingFixed, n.SwingKind = true, body.Swing.Kind // a player's round of its class and level (rounds.go)
	n.MaxHP, n.MaxPower = body.MaxHPNow(), body.MaxPowerNow()
	n.HP, n.Power = n.MaxHP, n.MaxPower
	if stance == 0 {
		stance = MercAssist
	}
	w.mercLife++
	ms := &mercState{owner: owner, slot: max(slot, 1), npc: n, body: body, kit: kit, role: mercRoleOf(kit.Class), life: w.mercLife, stance: stance,
		last: now, ready: map[int]time.Time{}, levelAsked: kit.Level, idle: mercIdle{quiet: now}}
	n.merc = ms
	syncMercBody(ms)
	w.putNPCLocked(n)
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	w.putMercLocked(ms)
	log.Printf("Merc: %s's %q (slot %d) comes, a level %d %s (%s), race %d sex %d, body %#x: %d HP, %d power, AC %d, STR %d DEX %d AGI %d STA %d WIS %d INT %d, weapon %d, swing %d, %d spells",
		owner.Name, n.Name, ms.slot, n.Level, className(kit.Class), MercRoleName(kit.Class), kit.Race, kit.Sex, uint32(n.ModelID), n.MaxHP, n.MaxPower,
		body.StatNow(StatAC), body.StatNow(StatStr), body.StatNow(StatDex), body.StatNow(StatAgi), body.StatNow(StatSta),
		body.StatNow(StatWis), body.StatNow(StatInt), body.Gear.WeaponDamage, body.Swing.Kind, len(kit.Spells))
	return ms
}

// className is a class's name for the log.
func className(class int) string {
	if class >= 0 && class < len(classNames) {
		return classNames[class]
	}
	return fmt.Sprintf("class %d", class)
}

// HireMerc is HireMercAt's first slot.
func (w *World) HireMerc(sess *session.Session, kit MercKit, stance MercStance, now time.Time) (name, refusal string) {
	name, refusal, _ = w.HireMercAt(sess, 1, kit, stance, now)
	return name, refusal
}

// HireMercAt puts the kit's merc beside the session's player in the slot (1
// to MercSlots), in place of any it has there (gone at once, its group place
// given to the new one). A group of four with no place for it (its leader
// raised no raid flag: mercgroup.go) takes it on standby instead
// (mercbench.go): standby true, its owner told, it comes when a place frees.
// The refusal is a line: the dead, one in no world.
func (w *World) HireMercAt(sess *session.Session, slot int, kit MercKit, stance MercStance, now time.Time) (name, refusal string, standby bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	slot = min(max(slot, 1), MercSlots)
	p := w.sessionPlayerLocked(sess)
	if p == nil || !sess.CharacterInWorld() {
		return "", "You must be in the world to hire a mercenary.", false
	}
	if p.IsDead() || p.respawnPending() {
		return "", "The dead hire no one.", false
	}
	if w.ctfKeepsOutLocked(p) {
		// a match is its players' alone: it waits outside the arena and comes
		// when its owner leaves it (ctfaway.go)
		w.ctfMercWaitsLocked(p, nil, slot, kit.Name)
		return "", fmt.Sprintf(ctfLineMercAway, kit.Name), false
	}
	var old *mercState
	for _, ms := range w.mercs[p.ServerID] {
		if ms.owner == p && ms.slot == slot {
			old = ms
		}
	}
	if !w.mercRoomLocked(p, old) {
		if old != nil {
			w.mercGoesLocked(old, sess, now)
		}
		w.benchSlotLocked(p, sess, slot, kit.Name, mercStandbyFullLine, now)
		return kit.Name, "", true
	}
	if old != nil {
		w.mercGoesLocked(old, sess, now)
	}
	w.unbenchLocked(p, slot)
	ms := w.spawnMercLocked(p, slot, kit, stance, now)
	w.mercGroupJoinLocked(ms, sess)
	w.mercTellLocked(ms, sess)
	return ms.npc.Name, "", false
}

// mercGoesLocked takes the merc out of the world at once, out of its group,
// off its owner's non-collide list (sess, the owner's, or nil), and out of
// every fight. Must be called with the lock held.
func (w *World) mercGoesLocked(ms *mercState, sess *session.Session, now time.Time) {
	n := ms.npc
	w.dropMercLocked(ms)
	w.mercLeaveGroupLocked(ms)
	w.mercPetGoesLocked(ms, now) // its pet with it (mercpet.go)
	w.dropAggroLocked(n.ServerID)
	if sess != nil && ms.noCollide {
		sendNoCollideRemove(sess, mercNoCollideIDOf(ms.slot))
	}
	if w.NPCs[n.ServerID] == n {
		w.removeNPCLocked(n)
	}
	n.Dead = true // its casts and ticks find it gone (merccast.go)
	log.Printf("Merc: %s's %q goes", ms.owner.Name, n.Name)
}

// DismissMerc sends the session's player's merc of the slot away, out or on
// standby (mercbench.go) or waiting outside the arena (ctfaway.go), its hire
// ended (the handlers drop its row). It reports the merc's name, false for
// none there.
func (w *World) DismissMerc(sess *session.Session, slot int, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", false
	}
	if ms := w.mercAtLocked(p, slot); ms != nil {
		name := ms.npc.Name
		w.mercGoesLocked(ms, sess, now)
		return name, true
	}
	if b := w.benchedAtLocked(p, slot); b != nil {
		w.unbenchLocked(p, slot)
		return b.name, true
	}
	return w.ctfDismissAwayLocked(p, slot, true) // one waiting outside the arena (ctfaway.go)
}

// idOrZero is the player's id, 0 for none.
func (p *Player) idOrZero() EntityID {
	if p == nil {
		return 0
	}
	return p.ServerID
}

// SetMercStance sets the stance of the session's player's merc of the slot
// (0: every one out). It reports how many took it.
func (w *World) SetMercStance(sess *session.Session, slot int, s MercStance) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	list := w.mercPickLocked(w.sessionPlayerLocked(sess), slot)
	for _, ms := range list {
		ms.stance = s
		if s == MercPassive && ms.ordered == 0 {
			ms.target = 0
			ms.npc.endRound()
		}
		log.Printf("Merc: %s's %q is %s", ms.owner.Name, ms.npc.Name, MercStanceNames[s])
	}
	return len(list)
}

// SetMercOffensive puts the session's player's healer mercs of the slot (0:
// every one out) offensive or defensive (MercKit.Offensive). It reports the
// slots that took it and how many picked are no healers (left as they were).
func (w *World) SetMercOffensive(sess *session.Session, slot int, on bool) (slots []int, others int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ms := range w.mercPickLocked(w.sessionPlayerLocked(sess), slot) {
		if !MercIsHealer(ms.kit.Class) {
			others++
			continue
		}
		ms.kit.Offensive = on
		slots = append(slots, ms.slot)
		log.Printf("Merc: %s's %q is %s", ms.owner.Name, ms.npc.Name, MercModeName(ms.kit.Class, on))
	}
	return slots, others
}

// MercIsHealer is whether a class's merc is a healer (a cleric, druid or
// shaman: mercRoleOf), which /merc defensive and offensive are for.
func MercIsHealer(class int) bool { return mercRoleOf(class) == roleHealer }

// MercModeName is a healer's mode in a word ("" for any other class).
func MercModeName(class int, offensive bool) string {
	switch {
	case !MercIsHealer(class):
		return ""
	case offensive:
		return "offensive"
	}
	return "defensive"
}

// SetMercStay holds the session's player's merc of the slot (0: every one
// out) where it stands (stay) or has it follow again. It reports how many
// took it.
func (w *World) SetMercStay(sess *session.Session, slot int, stay bool) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	list := w.mercPickLocked(w.sessionPlayerLocked(sess), slot)
	for _, ms := range list {
		ms.stay = stay
		if stay {
			n := ms.npc
			ms.stayAt = [3]float32{n.X, n.Y, n.Z}
			n.stopMoving()
		}
	}
	return len(list)
}

// MercAttack sets the session's player's merc of the slot (0: every one out)
// on its target (the creature its client has targeted), whatever its stance,
// until it dies or goes. It reports the creature's name, or the line refusing
// it.
func (w *World) MercAttack(sess *session.Session, slot int) (name, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	list := w.mercPickLocked(p, slot)
	if len(list) == 0 {
		return "", "You have no mercenary out."
	}
	if w.duelEngagedLocked(p.ServerID) {
		return "", MercLineDueling // its owner's duel is its own (ownfight.go, petition 1901)
	}
	ent, ok := w.findEntityByObjectIDLocked(sess, p.TargetObjectID)
	t, isNPC := ent.(*NPC)
	if !ok || !isNPC {
		return "", "Your mercenary cannot attack that."
	}
	n := 0
	for _, ms := range list {
		if mercMayFight(ms.npc, t) {
			n++
		}
	}
	if n == 0 {
		return "", "Your mercenary cannot attack that."
	}
	if distXZ(p.X, p.Z, t.X, t.Z) > petLeash {
		return "", petTargetTooFar
	}
	for _, ms := range list {
		if mercMayFight(ms.npc, t) {
			ms.ordered, ms.target = t.ServerID, t.ServerID
		}
	}
	return t.Name, ""
}

// SetMercKit gives the character's merc of the slot a new kit (its level's:
// the handlers' answer to MercNoteLevel), its hit points and power keeping
// their shares.
func (w *World) SetMercKit(charID uint32, slot int, kit MercKit) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	ms := w.mercAtLocked(p, slot)
	if ms == nil || kit.Class != ms.kit.Class {
		return false
	}
	kit.Name = ms.kit.Name
	w.rekitMercLocked(ms, kit)
	return true
}

// rekitMercLocked puts a new kit on the merc: its body made again, its looks,
// its hit points and power keeping their shares. Must be called with the lock
// held.
func (w *World) rekitMercLocked(ms *mercState, kit MercKit) {
	n := ms.npc
	hp, pw := share(n.HP, n.MaxHP), share(n.Power, n.MaxPower)
	picked := strings.Join(ms.kit.PetPick, ",") != strings.Join(kit.PetPick, ",")
	ms.kit = kit
	ms.bowOut, ms.shot = false, nil // its hands the new kit's (mercshoot.go)
	// a new pick of its pet's kind (/merc pet): a pet out of another
	// summoning goes, and the picked one is summoned (mercpet.go)
	if ps := w.mercPetOutLocked(ms); picked && ps != nil {
		if want := ms.mercPetSpell(); want != nil && want.SpellID != ps.spell {
			w.mercPetGoesLocked(ms, time.Now())
		}
	}
	w.dressMercLocked(n, &kit, ms.owner)
	ms.body = newMercBody(n.ServerID, &kit)
	n.Level = ms.body.Level
	n.SwingFixed, n.SwingKind = true, ms.body.Swing.Kind
	syncMercBody(ms)
	n.HP = max(1, int(math.Round(hp*float64(n.MaxHP))))
	n.Power = int(math.Round(pw * float64(n.MaxPower)))
	log.Printf("Merc: %s's %q is kitted at level %d: %d HP, %d power, AC %d, weapon %d, %d spells", ms.owner.Name, n.Name, n.Level,
		n.MaxHP, n.MaxPower, ms.body.StatNow(StatAC), ms.body.Gear.WeaponDamage, len(kit.Spells))
}

// share is v of top, 1 with no top.
func share(v, top int) float64 {
	if top <= 0 {
		return 1
	}
	return min(max(float64(v)/float64(top), 0), 1)
}

// syncMercBody brings the merc's body in line with its NPC: its place, what
// its effects add (the NPC's effects are its buffs: a player's or its own,
// landOnPetLocked), its hit points and power within a player's maxima, and
// the NPC's maxima those (effMaxHP kept at what its effects add, so a pet's
// rule of a raise on top, syncEffectMaxHP, adds nothing a second time).
func syncMercBody(ms *mercState) {
	n, b := ms.npc, ms.body
	b.X, b.Y, b.Z, b.Facing, b.World = n.X, n.Y, n.Z, n.Facing, n.World
	b.Mods = sumEffectMods(n.Effects)
	n.MaxHP, n.MaxPower = b.MaxHPNow(), b.MaxPowerNow()
	n.effMaxHP = n.effectsMaxHP()
	n.HP, n.Power = min(n.HP, n.MaxHP), min(max(n.Power, 0), n.MaxPower)
	b.HP, b.Power = n.HP, n.Power
	b.LastCombatTime = n.LastCombatTime
}

// mercLevelLocked follows the owner's level: the body at the new level at
// once (its stats, its round, its hit points' share), its heirlooms' figures
// and its book by the handlers' new kit after it. Must be called with the
// lock held.
func (w *World) mercLevelLocked(ms *mercState, sess *session.Session) {
	o := ms.owner
	if ms.body.Level == o.Level {
		return
	}
	kit := ms.kit
	kit.Level = o.Level
	w.rekitMercLocked(ms, kit)
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(mercLevelLine, ms.npc.Name, o.Level))
	}
	if ms.levelAsked != o.Level {
		ms.levelAsked = o.Level
		w.noteMercLocked(MercNote{Kind: MercNoteLevel, Owner: o, Slot: ms.slot, Sess: sess, Name: ms.npc.Name, Level: o.Level})
	}
}

// mercDiedLocked is the merc's death: out of its group and its owner's view
// of it, its owner told how long it rests, and the handlers told to keep that
// (its corpse lies a pet's time: corpse.go). Must be called with the lock held.
func (w *World) mercDiedLocked(ms *mercState, sess *session.Session, now time.Time) {
	n := ms.npc
	w.dropMercLocked(ms)
	w.mercLeaveGroupLocked(ms)
	w.mercPetGoesLocked(ms, now) // its pet with it (mercpet.go)
	w.dropAggroLocked(n.ServerID)
	if sess != nil {
		if ms.noCollide {
			sendNoCollideRemove(sess, mercNoCollideIDOf(ms.slot))
		}
		w.sendChatToSession(sess, fmt.Sprintf(mercDiedLine, n.Name, int(MercRest/time.Second)))
	}
	until := now.Add(MercRest)
	w.noteMercLocked(MercNote{Kind: MercNoteDied, Owner: ms.owner, Slot: ms.slot, Sess: sess, Name: n.Name, Level: n.Level, Until: until})
	log.Printf("Merc: %s's %q died; it rests until %s", ms.owner.Name, n.Name, until.Format(time.TimeOnly))
}

// mercLeavingLocked is the owner leaving the world (leavingLocked): its mercs
// go with it, their hires kept for the next login, and its standby with them
// (mercbench.go: every hire comes at the login). Must be called with the
// lock held.
func (w *World) mercLeavingLocked(p *Player) {
	for _, ms := range append([]*mercState(nil), w.mercs[p.ServerID]...) {
		if ms.owner != p {
			continue
		}
		w.mercGoesLocked(ms, nil, time.Now())
		w.noteMercLocked(MercNote{Kind: MercNoteGone, Owner: p, Slot: ms.slot, Name: ms.npc.Name, Level: ms.npc.Level})
	}
	w.benchLeavingLocked(p)
}

// mercTellLocked puts the merc on its owner's non-collide list once (pet.go's
// 0x1003), so its owner walks through it as through its pet.
func (w *World) mercTellLocked(ms *mercState, sess *session.Session) {
	if sess == nil || ms.noCollide {
		return
	}
	sendNoCollideAdd(sess, ms.npc.Name, mercNoCollideIDOf(ms.slot))
	ms.noCollide = true
}

// mercSystem moves, heals, casts and fights for every merc (merccast.go,
// mercfight.go), follows each owner's level, and takes away a merc whose
// owner is gone or which died. Every tick; must be called with the lock held.
func (w *World) mercSystem(now time.Time) {
	w.mercBenchSystemLocked(now) // standby mercs back to free places (mercbench.go)
	if len(w.mercs) == 0 && len(w.mercCasts) == 0 {
		return
	}
	w.mercCastSystemLocked(now)
	engaged := w.dpsEngagedLocked()
	for _, ms := range w.allMercsLocked() {
		if !w.mercLiveLocked(ms) {
			continue // gone in an earlier one's turn (its group's standby)
		}
		owner, n := ms.owner, ms.npc
		id := owner.ServerID
		if w.Players[id] != owner || w.NPCs[n.ServerID] != n {
			// its owner logged out or in again (a new player), or it was
			// taken out of the world (a GM's #despawn): gone, its hire kept
			w.mercGoesLocked(ms, nil, now)
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if n.Dead {
			w.mercDiedLocked(ms, sess, now)
			continue
		}
		if w.ctfKeepsOutLocked(owner) {
			// its owner in a match or the arena's world: it waits outside,
			// out of the world, until its owner leaves it (ctfaway.go)
			w.ctfSendAwayLocked(owner, now)
			continue
		}
		dt := float32(now.Sub(ms.last).Seconds())
		ms.last = now
		dt = min(max(dt, 0), 0.5)
		w.mercEffectsLocked(ms, now)
		w.mercLevelLocked(ms, sess)
		syncMercBody(ms)
		if sess != nil {
			w.mercTellLocked(ms, sess)
		}
		w.mercGroupUpkeepLocked(ms, sess)
		if !w.mercLiveLocked(ms) {
			continue // dismissed out of its owner's group, or on standby (mercgroup.go)
		}
		if !w.mercInOwnGroupLocked(ms) {
			// out of its owner's group, its box (or another's) up: it stands
			// where it is and fights nothing until the answer (mercgroup.go)
			ms.target, ms.ordered = 0, 0
			ms.combatReady = time.Time{}
			n.endRound()
			n.stopMoving()
			w.mercPetTurnLocked(ms, sess, false, dt, now)
			continue
		}
		if owner.IsDead() || owner.respawnPending() {
			// its owner down: it stands where it is, out of every fight of its
			// own (a creature fighting it still does), until its owner is back
			ms.target, ms.ordered = 0, 0
			ms.combatReady = time.Time{}
			n.endRound()
			n.stopMoving()
			w.mercPetTurnLocked(ms, sess, false, dt, now)
			continue
		}
		if n.World != owner.World || distXZ(n.X, n.Z, owner.X, owner.Z) > petLost {
			// a teleport, another world, a coach: it comes along, as a pet does
			oldX, oldZ := n.X, n.Z
			n.World = owner.World
			n.X, n.Y, n.Z = w.followPlaceLocked(n, owner, mercFollowSlot(ms.slot))
			n.stopMoving()
			ms.target, ms.ordered = 0, 0
			ms.stay = false
			n.endRound()
			w.SpatialIndex.Update(n.ServerID, oldX, oldZ, n.X, n.Z)
			syncMercBody(ms)
		}
		w.mercTurnLocked(ms, dt, now, engaged[id])
		syncMercBody(ms)
		fight := ms.ordered != 0 || !now.Before(ms.combatReady)
		w.mercPetTurnLocked(ms, sess, fight, dt, now) // its own pet (mercpet.go)
	}
}

// mercEffectsLocked takes off the merc's effects that are over and runs their
// heals' and power's ticks, as a pet's (petEffectsLocked).
func (w *World) mercEffectsLocked(ms *mercState, now time.Time) {
	n := ms.npc
	if len(n.Effects) == 0 {
		return
	}
	left := n.Effects[:0]
	for _, ae := range n.Effects {
		if !now.Before(ae.Until) {
			continue
		}
		left = append(left, ae)
		if ae.HPTick <= 0 && ae.PowerTick <= 0 || ae.NextTick.IsZero() || now.Before(ae.NextTick) {
			continue
		}
		ae.NextTick = ae.NextTick.Add(effectTick)
		if ae.HPTick > 0 && n.HP < n.MaxHP {
			n.HP = min(n.HP+ae.HPTick, n.MaxHP)
		}
		if ae.PowerTick > 0 && n.Power < n.MaxPower {
			n.Power = min(n.Power+ae.PowerTick, n.MaxPower)
		}
	}
	for i := len(left); i < len(n.Effects); i++ {
		n.Effects[i] = nil
	}
	n.Effects = left
}

// mercRegen is the merc's regeneration tick (npcregen.go's petRegen): a
// player's HoT and PoT of its body (regen.go: a 50th of its maxima, its
// heirlooms' HoT and PoT, its effects'), within its maxima.
func (n *NPC) mercRegen() {
	b := n.merc.body
	if n.HP > 0 && n.HP < n.MaxHP {
		n.HP = min(n.HP+max(b.HoTNow(), 1), n.MaxHP)
	}
	if n.MaxPower > 0 && n.Power < n.MaxPower {
		n.Power = min(n.Power+max(b.PoTNow(), 1), n.MaxPower)
	}
}

// mercHomeLocked is where the merc stands out of a fight: beside its owner,
// or the place it holds (/merc stay). Must be called with the lock held.
func (w *World) mercHomeLocked(ms *mercState, dt float32, now time.Time) {
	n := ms.npc
	if !ms.stay {
		if w.mercIdleLocked(ms, dt, now) { // at ease, its owner standing (mercidle.go)
			return
		}
		w.followLocked(n, ms.owner, mercFollowSlot(ms.slot), dt, now)
		return
	}
	if distXZ(n.X, n.Z, ms.stayAt[0], ms.stayAt[2]) <= 1 {
		n.stopMoving()
		return
	}
	w.petStepLocked(n, ms.stayAt[0], ms.stayAt[2], ms.stayAt[1], max(ms.owner.SpeedNow(), 3), dt)
}
