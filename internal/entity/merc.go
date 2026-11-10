package entity

import (
	"fmt"
	"log"
	"math"
	"strconv"
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
// its coming along through a teleport or into another world), assists it
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
	// MercAssist fights what its owner fights: its owner's auto-attack's
	// target, a creature fighting its owner or itself, one its owner hurt or
	// its owner's pet fights (the default)
	MercAssist MercStance = 2
	// MercAggressive also takes on what fights or is fought by its owner's
	// group
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
}

// mercState is one merc, kept by its owner's id.
type mercState struct {
	owner *Player
	npc   *NPC
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
}

// mercNoCollideID is a merc's entry on its owner's non-collide list (0x1003:
// CLIENT's ignore list, pet.go): past the pet's petNoCollideID, as an
// ignored player's count up from it.
const mercNoCollideID uint32 = petNoCollideID + 1

// MercRest is how long a dead merc rests before /merc brings it back (ours:
// the owner's "after a rest (60 s, your call)").
const MercRest = 60 * time.Second

// The lines a merc's owner is told (ours).
const (
	mercDiedLine  = "Your mercenary %s has died. Rest %d seconds, then /merc brings it back."
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
)

// MercNote is a merc's event, handed to the hook after the tick's lock.
type MercNote struct {
	Kind  MercNoteKind
	Owner *Player
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

// mercOfLocked is the player's merc, alive, or nil. Must be called with the
// lock held.
func (w *World) mercOfLocked(p *Player) *mercState {
	if p == nil {
		return nil
	}
	ms := w.mercs[p.ServerID]
	if ms == nil || ms.owner != p || ms.npc.Dead {
		return nil
	}
	return ms
}

// MercOf is the session's player's merc's entity id, 0 for none.
func (w *World) MercOf(sess *session.Session) EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if ms := w.mercOfLocked(w.sessionPlayerLocked(sess)); ms != nil {
		return ms.npc.ServerID
	}
	return 0
}

// MercInfo is what /merc says of a merc.
type MercInfo struct {
	Name                       string
	Class, Race, Sex, Level    int
	HP, MaxHP, Power, MaxPower int
	Stance                     MercStance
	Stay                       bool
	Target                     string
}

// MercInfoOf is the session's player's merc, false for none out.
func (w *World) MercInfoOf(sess *session.Session) (MercInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ms := w.mercOfLocked(w.sessionPlayerLocked(sess))
	if ms == nil {
		return MercInfo{}, false
	}
	n := ms.npc
	info := MercInfo{Name: n.Name, Class: ms.kit.Class, Race: ms.kit.Race, Sex: ms.kit.Sex, Level: n.Level, HP: n.HP, MaxHP: n.MaxHP,
		Power: n.Power, MaxPower: n.MaxPower, Stance: ms.stance, Stay: ms.stay}
	if t := w.NPCs[ms.target]; t != nil {
		info.Target = t.Name
	}
	return info, true
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
	n.Class = k.Class
}

// mercSpot is a place at its owner's left side, petFollow off it (the pet
// takes the right: besideOwner), so the two come apart.
func mercSpot(p *Player) (x, y, z float32) {
	h := float64(p.Facing)
	return p.X - petFollow*float32(math.Cos(h)), p.Y, p.Z + petFollow*float32(math.Sin(h))
}

// spawnMercLocked makes the merc of the kit beside its owner: its NPC dressed
// and at a player's hit points and power, in the world, its owner's. Must be
// called with the lock held.
func (w *World) spawnMercLocked(owner *Player, kit MercKit, stance MercStance, now time.Time) *mercState {
	if kit.Level <= 0 || kit.Level != owner.Level {
		kit.Level = owner.Level
	}
	id := w.allocFreeIDLocked()
	n := &NPC{ServerID: id, Name: kit.Name, World: owner.World, Level: kit.Level, Faction: FactionFriendly, Temporary: true,
		RespawnTime: petCorpseTime, CorpseDecay: petCorpseTime, Owner: owner.ServerID}
	w.dressMercLocked(n, &kit, owner)
	n.X, n.Y, n.Z = mercSpot(owner)
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
	ms := &mercState{owner: owner, npc: n, body: body, kit: kit, role: mercRoleOf(kit.Class), life: w.mercLife, stance: stance,
		last: now, ready: map[int]time.Time{}, levelAsked: kit.Level}
	n.merc = ms
	syncMercBody(ms)
	w.putNPCLocked(n)
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	if w.mercs == nil {
		w.mercs = make(map[EntityID]*mercState)
	}
	w.mercs[owner.ServerID] = ms
	log.Printf("Merc: %s's %q comes, a level %d %s (%s), race %d sex %d, body %#x: %d HP, %d power, AC %d, STR %d DEX %d AGI %d STA %d WIS %d INT %d, weapon %d, swing %d, %d spells",
		owner.Name, n.Name, n.Level, className(kit.Class), MercRoleName(kit.Class), kit.Race, kit.Sex, uint32(n.ModelID), n.MaxHP, n.MaxPower,
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

// HireMerc puts the kit's merc beside the session's player, in place of any
// it has (gone at once, its group place given to the new one). The refusal
// is a line: the dead, one in no world, a group with no room for it
// (mercgroup.go: four, unless its leader raised a raid flag).
func (w *World) HireMerc(sess *session.Session, kit MercKit, stance MercStance, now time.Time) (name, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !sess.CharacterInWorld() {
		return "", "You must be in the world to hire a mercenary."
	}
	if p.IsDead() || p.respawnPending() {
		return "", "The dead hire no one."
	}
	old := w.mercs[p.ServerID]
	if why := w.mercGroupRoomLocked(p, old); why != "" {
		return "", why
	}
	if old != nil {
		w.mercGoesLocked(old, sess, now)
	}
	ms := w.spawnMercLocked(p, kit, stance, now)
	w.mercGroupJoinLocked(ms, sess)
	w.mercTellLocked(ms, sess)
	return ms.npc.Name, ""
}

// mercGoesLocked takes the merc out of the world at once, out of its group,
// off its owner's non-collide list (sess, the owner's, or nil), and out of
// every fight. Must be called with the lock held.
func (w *World) mercGoesLocked(ms *mercState, sess *session.Session, now time.Time) {
	n := ms.npc
	if w.mercs[ms.owner.ServerID] == ms {
		delete(w.mercs, ms.owner.ServerID)
	}
	w.mercLeaveGroupLocked(ms)
	w.dropAggroLocked(n.ServerID)
	if sess != nil && ms.noCollide {
		sendNoCollideRemove(sess, mercNoCollideID)
	}
	if w.NPCs[n.ServerID] == n {
		w.removeNPCLocked(n)
	}
	n.Dead = true // its casts and ticks find it gone (merccast.go)
	log.Printf("Merc: %s's %q goes", ms.owner.Name, n.Name)
}

// DismissMerc sends the session's player's merc away, its hire ended (the
// handlers drop its row). It reports the merc's name, false for none out.
func (w *World) DismissMerc(sess *session.Session, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	ms := w.mercs[p.idOrZero()]
	if p == nil || ms == nil || ms.owner != p {
		return "", false
	}
	name := ms.npc.Name
	w.mercGoesLocked(ms, sess, now)
	return name, true
}

// idOrZero is the player's id, 0 for none.
func (p *Player) idOrZero() EntityID {
	if p == nil {
		return 0
	}
	return p.ServerID
}

// SetMercStance sets the session's player's merc's stance. It reports false
// with no merc out.
func (w *World) SetMercStance(sess *session.Session, s MercStance) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercOfLocked(w.sessionPlayerLocked(sess))
	if ms == nil {
		return false
	}
	ms.stance = s
	if s == MercPassive && ms.ordered == 0 {
		ms.target = 0
		ms.npc.endRound()
	}
	log.Printf("Merc: %s's %q is %s", ms.owner.Name, ms.npc.Name, MercStanceNames[s])
	return true
}

// SetMercStay holds the session's player's merc where it stands (stay) or
// has it follow again. It reports false with no merc out.
func (w *World) SetMercStay(sess *session.Session, stay bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercOfLocked(w.sessionPlayerLocked(sess))
	if ms == nil {
		return false
	}
	ms.stay = stay
	if stay {
		n := ms.npc
		ms.stayAt = [3]float32{n.X, n.Y, n.Z}
		n.stopMoving()
	}
	return true
}

// MercAttack sets the session's player's merc on its target (the creature
// its client has targeted), whatever its stance, until it dies or goes. It
// reports the creature's name, or the line refusing it.
func (w *World) MercAttack(sess *session.Session) (name, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	ms := w.mercOfLocked(p)
	if ms == nil {
		return "", "You have no mercenary out."
	}
	ent, ok := w.findEntityByObjectIDLocked(sess, p.TargetObjectID)
	t, isNPC := ent.(*NPC)
	if !ok || !isNPC || !mercMayFight(ms.npc, t) {
		return "", "Your mercenary cannot attack that."
	}
	if distXZ(p.X, p.Z, t.X, t.Z) > petLeash {
		return "", petTargetTooFar
	}
	ms.ordered, ms.target = t.ServerID, t.ServerID
	return t.Name, ""
}

// SetMercKit gives the character's merc a new kit (its level's: the handlers'
// answer to MercNoteLevel), its hit points and power keeping their shares.
func (w *World) SetMercKit(charID uint32, kit MercKit) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	ms := w.mercOfLocked(p)
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
	ms.kit = kit
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
		w.noteMercLocked(MercNote{Kind: MercNoteLevel, Owner: o, Sess: sess, Name: ms.npc.Name, Level: o.Level})
	}
}

// mercDiedLocked is the merc's death: out of its group and its owner's view
// of it, its owner told how long it rests, and the handlers told to keep that
// (its corpse lies a pet's time: corpse.go). Must be called with the lock held.
func (w *World) mercDiedLocked(ms *mercState, sess *session.Session, now time.Time) {
	n := ms.npc
	delete(w.mercs, ms.owner.ServerID)
	w.mercLeaveGroupLocked(ms)
	w.dropAggroLocked(n.ServerID)
	if sess != nil {
		if ms.noCollide {
			sendNoCollideRemove(sess, mercNoCollideID)
		}
		w.sendChatToSession(sess, fmt.Sprintf(mercDiedLine, n.Name, int(MercRest/time.Second)))
	}
	until := now.Add(MercRest)
	w.noteMercLocked(MercNote{Kind: MercNoteDied, Owner: ms.owner, Sess: sess, Name: n.Name, Level: n.Level, Until: until})
	log.Printf("Merc: %s's %q died; it rests until %s", ms.owner.Name, n.Name, until.Format(time.TimeOnly))
}

// mercLeavingLocked is the owner leaving the world (leavingLocked): its merc
// goes with it, its hire kept for the next login. Must be called with the
// lock held.
func (w *World) mercLeavingLocked(p *Player) {
	ms := w.mercs[p.ServerID]
	if ms == nil || ms.owner != p {
		return
	}
	w.mercGoesLocked(ms, nil, time.Now())
	w.noteMercLocked(MercNote{Kind: MercNoteGone, Owner: p, Name: ms.npc.Name, Level: ms.npc.Level})
}

// mercTellLocked puts the merc on its owner's non-collide list once (pet.go's
// 0x1003), so its owner walks through it as through its pet.
func (w *World) mercTellLocked(ms *mercState, sess *session.Session) {
	if sess == nil || ms.noCollide {
		return
	}
	sendNoCollideAdd(sess, ms.npc.Name, mercNoCollideID)
	ms.noCollide = true
}

// mercSystem moves, heals, casts and fights for every merc (merccast.go,
// mercfight.go), follows each owner's level, and takes away a merc whose
// owner is gone or which died. Every tick; must be called with the lock held.
func (w *World) mercSystem(now time.Time) {
	if len(w.mercs) == 0 && len(w.mercCasts) == 0 {
		return
	}
	w.mercCastSystemLocked(now)
	for id, ms := range w.mercs {
		owner, n := ms.owner, ms.npc
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
		if owner.IsDead() || owner.respawnPending() {
			// its owner down: it stands where it is, out of every fight of its
			// own (a creature fighting it still does), until its owner is back
			ms.target, ms.ordered = 0, 0
			n.endRound()
			n.stopMoving()
			continue
		}
		if n.World != owner.World || distXZ(n.X, n.Z, owner.X, owner.Z) > petLost {
			// a teleport, another world, a coach: it comes along, as a pet does
			oldX, oldZ := n.X, n.Z
			n.World = owner.World
			n.X, n.Y, n.Z = mercSpot(owner)
			n.stopMoving()
			ms.target, ms.ordered = 0, 0
			ms.stay = false
			n.endRound()
			w.SpatialIndex.Update(n.ServerID, oldX, oldZ, n.X, n.Z)
			syncMercBody(ms)
		}
		w.mercTurnLocked(ms, dt, now)
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

// followLocked keeps npc petFollow off its owner as a pet follows
// (petFollowLocked): from petFollowStart away, at its owner's speed, half as
// fast again past petCatchUp. Must be called with the lock held.
func (w *World) followLocked(npc *NPC, owner *Player, dt float32) {
	d := distXZ(npc.X, npc.Z, owner.X, owner.Z)
	if d <= petFollow || (npc.Loco == LocoStand && d <= petFollowStart) {
		npc.stopMoving()
		return
	}
	speed := max(owner.SpeedNow(), 3)
	if d > petCatchUp {
		speed *= 1.5
	}
	x := owner.X + (npc.X-owner.X)/d*petFollow
	z := owner.Z + (npc.Z-owner.Z)/d*petFollow
	w.petStepLocked(npc, x, z, owner.Y, speed, dt)
}

// mercHomeLocked is where the merc stands out of a fight: beside its owner,
// or the place it holds (/merc stay). Must be called with the lock held.
func (w *World) mercHomeLocked(ms *mercState, dt float32) {
	n := ms.npc
	if !ms.stay {
		w.followLocked(n, ms.owner, dt)
		return
	}
	if distXZ(n.X, n.Z, ms.stayAt[0], ms.stayAt[2]) <= 1 {
		n.stopMoving()
		return
	}
	w.petStepLocked(n, ms.stayAt[0], ms.stayAt[2], ms.stayAt[1], max(ms.owner.SpeedNow(), 3), dt)
}
