package handlers

import (
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// merc.go — mercenaries, the handlers' half (entity/merc.go is the World's;
// the owner, 2026-11-10: "lets add mercenary pets similar to EQ 1. any class
// same level as player with heirloom gear, casts same abilities as player on
// cooldown. if healer heal team members, if tank they taunt", then "mercs will
// occupy a group slot" and "mercs can be any race the player picks").
//
// world.mercs (SetMercs: off unless set; the Heirloom Test Server sets it)
// turns all of it on. Off, a Mercenary Liaison has nothing to say and /merc
// says mercenaries are off on this server; a hire kept in table charmerc waits.
//
// A Mercenary Liaison (any NPC so named; tools/place_merc_liaisons.py stands
// one beside each city's announcements board, Freeport's beside Dabrish) hires
// one for nothing, asking a box at a time under mark 0x5F000000 (beside the
// Teleporter's 0x5E): a calling by role (a healer, a tank, a melee fighter, a
// caster), a class of it (all fifteen: the healers the cleric, druid and
// shaman; the tanks the warrior, paladin and shadow knight; the melee the
// ranger, monk, bard and rogue; the casters the magician, necromancer,
// enchanter, wizard and alchemist), a race (its owner's own, any of the game's
// ten, and with world.open_races the froglok and every racedefs race creation
// offers: entity.CreationRaces), and a sex where the race has two bodies. Any
// race takes any class. A new hire takes the place of one out; one a
// character, kept in table charmerc (its name, class, race, sex, face and hair,
// stance, the end of its rest after a death) until dismissed.
//
// Its kit (mercKit) is what a player of its class, race and level would have
// of the Heirloom Outfitters: the class's heirloom of each slot (the seed's
// own, its lowest record: tools/seed_heirlooms.py; the robe for a priest or a
// caster alone), in the hands the balance simulator gives a player of the
// class (balancekit.go: a tank or a priest a weapon and a shield, a melee
// class two weapons from level 20, a monk its bare hands, a caster its
// two-hander), each with its figures for the merc's level and race
// (Heirlooms.ViewFor); what they add and their looks as a player's worn items'
// (wornGear, wornLook); a player's base stats at that level with its training
// points spent (entity.SimBaseStats, its race's bonuses with them); and the
// class's book up to its level (entity.SimBookFromDB: the spells its levels
// give, no mastery's). The kit is made again when its owner's level changes
// (entity.MercNoteLevel) and at every coming.
//
// It comes back at its owner's login (OnPlayerEntered), unless it rests after
// a death (entity.MercRest, then /merc); through a keep restart it is carried
// as it was (keep.go's restoreMerc). /merc (chat.go: "/merc", "#merc", a PS2's
// "/t merc ...") is its commands: its state, stance, follow, stay, attack,
// group, revive and dismiss.

// MercLiaisonName is the liaison's name: an NPC of it is one.
const MercLiaisonName = "Mercenary Liaison"

// mercBoxMark marks a liaison's question's id.
const mercBoxMark = 0x5F000000

// IsMercBox reports a box a liaison asked.
func IsMercBox(id uint32) bool { return id&0xFF000000 == mercBoxMark }

var mercsOn atomic.Bool

// SetMercs sets world.mercs.
func SetMercs(on bool) { mercsOn.Store(on) }

// MercsOn reports world.mercs.
func MercsOn() bool { return mercsOn.Load() }

// The liaison's words and the merc's lines (ours). A question is 127
// characters at most (questAskChars).
const (
	mercAskRole   = "Mercenary Liaison: 'Which calling do you need? A mercenary fights at your level for nothing, one at a time.'"
	mercAskClass  = "Mercenary Liaison: 'Which kind of %s?'"
	mercAskRace   = "Mercenary Liaison: 'Of which race?'"
	mercAskSex    = "Mercenary Liaison: 'A man or a woman?'"
	mercHiredLine = "%s the %s %s is yours. Type /merc to see what it heeds."
	mercOffLine   = "Mercenaries are off on this server."
	mercNoneLine  = "You have no mercenary. A Mercenary Liaison, beside each city's announcements board, hires one for nothing."
	mercRestLine  = "Your mercenary %s rests for %d more seconds."
	mercBackLine  = "Your mercenary %s is back at your side."
	mercKitFailed = "The liaison's ledger is out of order. Try again."
	mercHelpLine  = "/merc stance passive|assist|aggressive, /merc follow, /merc stay, /merc attack (your target), /merc path, /merc pet, /merc charm [off], /merc group, /merc revive, /merc dismiss. /familiar dismiss sends a familiar away."
)

// mercRoles are the liaison's first answers, each role's classes in their
// box's order.
var mercRoles = []struct {
	label, word string
	classes     []int
}{
	{"A healer", "healer", []int{9, 7, 8}},
	{"A tank", "tank", []int{0, 2, 3}},
	{"A melee fighter", "melee fighter", []int{1, 4, 5, 6}},
	{"A caster", "caster", []int{10, 11, 12, 13, 14}},
}

// The liaison's race boxes: its owner's own, the game's first five and last
// five, and the races creation offers past them, five a box.
var mercRaceGroups = [][]int{{0, 1, 2, 3, 4}, {5, 6, 7, 8, 9}}

const mercRacesABox = 5

// The liaison's questions.
const (
	mercStageRole = iota
	mercStageClass
	mercStageRaceGroup
	mercStageRace
	mercStageSex
)

// mercAsk is a liaison's open question: its box, its stage, what was chosen
// so far, the other races' page shown.
type mercAsk struct {
	box                uint32
	stage, role, class int
	race, page         int
	others             bool
	npc                entity.EntityID
}

// Mercs is the handlers' half of the mercenaries.
type Mercs struct {
	gw   *entity.World
	db   *database.SQLiteDB
	heir *Heirlooms
	now  func() time.Time

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]mercAsk
}

// NewMercs is the mercenaries' handler over the World and the database; heir
// gives the heirlooms' figures (nil: none, the merc in plain records).
func NewMercs(gw *entity.World, db *database.SQLiteDB, heir *Heirlooms) *Mercs {
	return &Mercs{gw: gw, db: db, heir: heir, now: time.Now, asked: map[uint32]mercAsk{}}
}

// mercsH is the handler RegisterDefaultHandlers made (a keep restart's).
var mercsH atomic.Pointer[Mercs]

// Start hears the World: a login brings its merc back, the World's merc
// events keep their rows and kits.
func (m *Mercs) Start() {
	if m == nil || m.gw == nil {
		return
	}
	mercsH.Store(m)
	m.gw.OnPlayerEntered(func(sess *session.Session, p *entity.Player) { go m.comeBack(sess, p) })
	m.gw.OnMercNote(m.note)
}

// mercRace is a race the liaison offers: its id, its name, whether it has
// two bodies (asked a sex).
type mercRace struct {
	id    int
	name  string
	sexes bool
}

// mercOtherRaces is the races creation offers past the game's ten (the
// froglok and the racedefs rows, world.open_races: entity.CreationRaces).
func mercOtherRaces() []mercRace {
	var out []mercRace
	for _, c := range entity.CreationRaces() {
		out = append(out, mercRace{id: c.ID, name: c.Name, sexes: c.Body[1] != 0 && c.Body[1] != c.Body[0]})
	}
	return out
}

// mercRaceOf is a race the liaison may hire as, false for none: the game's
// ten (two bodies each), its owner's own, or one of mercOtherRaces.
func mercRaceOf(id, own int) (mercRace, bool) {
	if id >= 0 && id <= 9 {
		return mercRace{id: id, name: entity.RaceName(id), sexes: true}, true
	}
	for _, r := range mercOtherRaces() {
		if r.id == id {
			return r, true
		}
	}
	if id == own {
		return mercRace{id: id, name: entity.RaceName(id)}, true
	}
	return mercRace{}, false
}

// askLocked sends p the liaison's question under a new box, kept. Called
// with m.mu held.
func (m *Mercs) askLocked(sess *session.Session, p *entity.Player, a mercAsk, question string, answers []string) {
	m.seq++
	a.box = mercBoxMark | (m.seq & 0xFFFFFF)
	m.asked[p.CharServerID] = a
	SendQuestPopupMulti(sess, a.box, question, answers)
}

// say is the liaison's line in a dialogue box.
func mercSay(sess *session.Session, line string) {
	for _, part := range splitSay(MercLiaisonName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// Talk is a talk to npc: the liaison's first question when npc is one and
// mercenaries are on, else false (a liaison then says nothing: no dialog).
func (m *Mercs) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if m == nil || npc == nil || npc.Dead || !strings.EqualFold(strings.TrimSpace(npc.Name), MercLiaisonName) || !MercsOn() {
		return false
	}
	answers := make([]string, len(mercRoles))
	for i, r := range mercRoles {
		answers[i] = r.label
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageRole, npc: npc.ServerID}, mercAskRole, answers)
	m.mu.Unlock()
	return true
}

// Answer is HandleDialogueOption's question for a liaison's box: each answer
// asks the next, the last hires. It reports whether the box was a liaison's
// (an old one, or a close, does nothing).
func (m *Mercs) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsMercBox(box) {
		return false
	}
	if m == nil || p == nil || option == 255 || !MercsOn() {
		return true
	}
	m.mu.Lock()
	a, ok := m.asked[p.CharServerID]
	if ok && a.box == box {
		delete(m.asked, p.CharServerID)
	}
	m.mu.Unlock()
	if !ok || a.box != box {
		return true
	}
	n := int(option)
	switch a.stage {
	case mercStageRole:
		if n >= len(mercRoles) {
			return true
		}
		a.role = n
		var answers []string
		for _, c := range mercRoles[n].classes {
			answers = append(answers, entity.ClassName(c))
		}
		a.stage = mercStageClass
		m.mu.Lock()
		m.askLocked(sess, p, a, fmt.Sprintf(mercAskClass, mercRoles[n].word), answers)
		m.mu.Unlock()
	case mercStageClass:
		classes := mercRoles[a.role].classes
		if n >= len(classes) {
			return true
		}
		a.class = classes[n]
		m.askRaceGroup(sess, p, a)
	case mercStageRaceGroup:
		switch {
		case n == 0: // its owner's own race and sex
			m.hire(sess, p, a.class, p.Race, p.Sex)
		case n-1 < len(mercRaceGroups):
			a.stage, a.others, a.page = mercStageRace, false, n-1
			m.askRaces(sess, p, a)
		case n-1 == len(mercRaceGroups) && len(mercOtherRaces()) > 0:
			a.stage, a.others, a.page = mercStageRace, true, 0
			m.askRaces(sess, p, a)
		}
	case mercStageRace:
		races := m.racesOf(a)
		more := a.others && (a.page+1)*mercRacesABox < len(mercOtherRaces())
		switch {
		case n < len(races):
			r := races[n]
			if !r.sexes {
				m.hire(sess, p, a.class, r.id, 0)
				return true
			}
			a.stage, a.race = mercStageSex, r.id
			m.mu.Lock()
			m.askLocked(sess, p, a, mercAskSex, []string{"A man", "A woman"})
			m.mu.Unlock()
		case n == len(races) && more:
			a.page++
			m.askRaces(sess, p, a)
		}
	case mercStageSex:
		if n <= 1 {
			m.hire(sess, p, a.class, a.race, n)
		}
	}
	return true
}

// askRaceGroup is the race's first question: its owner's own, the game's
// first five, its last five, and (creation offering them) the others.
func (m *Mercs) askRaceGroup(sess *session.Session, p *entity.Player, a mercAsk) {
	own := entity.RaceName(p.Race)
	answers := []string{"Like me: " + own}
	for _, g := range mercRaceGroups {
		answers = append(answers, entity.RaceName(g[0])+" to "+entity.RaceName(g[len(g)-1]))
	}
	if len(mercOtherRaces()) > 0 {
		answers = append(answers, "Other races")
	}
	a.stage = mercStageRaceGroup
	m.mu.Lock()
	m.askLocked(sess, p, a, mercAskRace, answers)
	m.mu.Unlock()
}

// racesOf is the races a race question shows: a game group's five, or a page
// of the others.
func (m *Mercs) racesOf(a mercAsk) []mercRace {
	if !a.others {
		if a.page < 0 || a.page >= len(mercRaceGroups) {
			return nil
		}
		var out []mercRace
		for _, id := range mercRaceGroups[a.page] {
			out = append(out, mercRace{id: id, name: entity.RaceName(id), sexes: true})
		}
		return out
	}
	all := mercOtherRaces()
	from := a.page * mercRacesABox
	if from >= len(all) {
		return nil
	}
	return all[from:min(from+mercRacesABox, len(all))]
}

// askRaces asks which race of a group or page, "More races" last when the
// others go on.
func (m *Mercs) askRaces(sess *session.Session, p *entity.Player, a mercAsk) {
	var answers []string
	for _, r := range m.racesOf(a) {
		answers = append(answers, r.name)
	}
	if a.others && (a.page+1)*mercRacesABox < len(mercOtherRaces()) {
		answers = append(answers, "More races")
	}
	if len(answers) == 0 {
		return
	}
	m.mu.Lock()
	m.askLocked(sess, p, a, mercAskRace, answers)
	m.mu.Unlock()
}

// mercName is a merc's name: one of the server's random names (character.go's
// parts), the same for the same owner, class, race and hour.
func mercName(serverID uint32, class, race int, at time.Time) string {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d/%d/%d/%d", serverID, class, race, at.Unix()/3600)
	v := h.Sum32()
	return namePrefixes[v%uint32(len(namePrefixes))] + nameSuffixes[(v/uint32(len(namePrefixes)))%uint32(len(nameSuffixes))]
}

// hire makes the merc of class, race and sex the player's: its row (its look
// chosen now), its kit at the player's level, beside it in the World, a line.
// A refusal (a full group, the dead) is said and keeps the old hire.
func (m *Mercs) hire(sess *session.Session, p *entity.Player, class, race, sex int) {
	if _, ok := mercRaceOf(race, p.Race); !ok || class < 0 || class > 14 {
		return
	}
	now := m.now()
	row := database.MercRow{ServerID: p.CharServerID, Name: mercName(p.CharServerID, class, race, now), Class: class, Race: race,
		Sex: min(max(sex, 0), 1), Stance: int(entity.MercAssist), Hired: now, Path: -1}
	row.Face, row.HairColor, row.HairLength, row.HairStyle = m.look(race, row.Sex, row.Name)
	kit, err := m.kit(row, p.Level)
	if err != nil {
		log.Printf("Merc: %s's hire of a %s: %v", p.Name, entity.ClassName(class), err)
		mercSay(sess, mercKitFailed)
		return
	}
	name, why := m.gw.HireMerc(sess, kit, entity.MercStance(row.Stance), now)
	if why != "" {
		mercSay(sess, why)
		return
	}
	if m.db != nil {
		if err := m.db.SetMerc(row); err != nil {
			log.Printf("Merc: %s's hire of %q not kept: %v", p.Name, name, err)
		}
	}
	log.Printf("Merc: %s hires %q (%s %s, sex %d)", p.Name, name, entity.RaceName(race), entity.ClassName(class), row.Sex)
	mercSay(sess, fmt.Sprintf(mercHiredLine, name, entity.RaceName(race), entity.ClassName(class)))
	if k := mercPathOf(class, row.Path); k >= 0 {
		SendClientMessage(sess, fmt.Sprintf(mercPathIsLine, name, mercPathName(class, k)))
	}
}

// look is a merc's face and hair: one of the looks this server's NPCs of its
// body wear (each a look the game draws), chosen by its name; none known,
// the plainest.
func (m *Mercs) look(race, sex int, name string) (face, color, length, style int) {
	if m.db == nil || m.gw == nil {
		return 0, 0, 0, 0
	}
	model, ok := m.gw.RaceModel(race, sex)
	if !ok {
		return 0, 0, 0, 0
	}
	rows, err := m.db.Query(`SELECT DISTINCT face, hair_color, hair_length, hair_style FROM npcs WHERE modelid = ? OR modelid = ?
		ORDER BY face, hair_color, hair_length, hair_style`, int64(model), int64(int32(model)))
	if err != nil {
		return 0, 0, 0, 0
	}
	defer rows.Close()
	var looks [][4]int
	for rows.Next() {
		var l [4]int
		if rows.Scan(&l[0], &l[1], &l[2], &l[3]) == nil {
			looks = append(looks, l)
		}
	}
	if len(looks) == 0 {
		return 0, 0, 0, 0
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	l := looks[h.Sum32()%uint32(len(looks))]
	return l[0], l[1], l[2], l[3]
}

// The equip slots a merc's heirlooms are worn in (the balance simulator's,
// balancekit.go's simWear; the hands' are a player's: 14 the main hand, 15
// the off hand, 16 both, 17 a shield).
var mercWear = []struct {
	role  string
	equip int
}{
	{"head", 0}, {"hands", 1}, {"ear", 2}, {"neck", 4}, {"chest", 5}, {"bracelet", 6}, {"bracers", 8},
	{"ring", 9}, {"waist", 11}, {"legs", 12}, {"feet", 13}, {"robe", 21},
}

// mercDualFrom is the level from which a melee class holds a weapon in each
// hand (CLIENT's EquipSlotFor: a warrior, ranger, monk, bard or rogue of 20).
const mercDualFrom = 20

// kit is the merc of the row at level as it fights (the file's doc).
func (m *Mercs) kit(row database.MercRow, level int) (entity.MercKit, error) {
	k := entity.MercKit{Class: row.Class, Race: row.Race, Sex: row.Sex, Level: max(level, 1), Name: row.Name}
	k.Look = entity.MercLook{Face: row.Face, HairColor: row.HairColor, HairLength: row.HairLength, HairStyle: row.HairStyle, Robe: -1}
	k.Base = entity.MercBaseStats(row.Class, row.Race, k.Level) // its role's (entity/mercroles.go)
	if m.db == nil {
		return k, nil
	}
	items, err := m.heirlooms(row.Class, row.Race, k.Level)
	if err != nil {
		return k, err
	}
	k.Gear = wornGear(items)
	if entity.MercIsTank(row.Class) {
		// a tank's resists at the cap and its armour the best of its level's
		// (entity/mercroles.go)
		entity.MercTankResists(&k.Gear, k.Base[4]+k.Gear.Wis)
		k.Gear.AC = max(k.Gear.AC, entity.MercTankAC(k.Level))
	}
	var p entity.Player
	wornLook(&p, items)
	l := &k.Look
	l.Chest, l.Bracer, l.Gloves, l.Legs, l.Boots, l.Helm = p.Chest, p.Bracer, p.Gloves, p.Legs, p.Boots, p.Helm
	l.Tint = entity.ArmorTint(p.ArmorColors[database.DyeChest])
	if p.RobeWorn {
		l.Robe = int(p.Robe)
	}
	l.Primary, l.Secondary, l.Shield = p.Primary, p.Secondary, p.Shield
	l.PrimaryType, l.SecondaryType = p.PrimaryType, p.SecondaryType
	if row.Class == mercAlchemist {
		if bottle := m.potionBottle(); bottle != 0 {
			// in the hand the staff was, as the alchemists' NPCs hold one (the
			// Chemists of Freeport, Aemilia); its staff's figures kept
			l.Primary, l.PrimaryType = bottle, thrownAttackType
		}
	}
	for _, it := range items {
		if it.EquipLoc >= 14 && it.EquipLoc <= 18 {
			k.Worn[it.EquipLoc] = true
		}
		if it.EquipLoc == 14 || it.EquipLoc == 16 {
			k.AttackType, k.Range = it.Pattern.AttackType, it.Pattern.Unk5
		}
	}
	if role := entity.MercRoleName(row.Class); role == "tank" || role == "melee fighter" {
		k.Bow = m.bow(row.Class, row.Race, k.Level) // its bow at range (entity/mercshoot.go)
	}
	k.Spells, err = entity.SimBookFromDB(m.db, row.Class, k.Level, false)
	k.Spells = mercPathBook(row.Class, row.Path, k.Spells) // its path's, no other's (mercpath.go)
	k.PetPick = mercPetPick(row.Class, row.Pet)            // the pet kind its hirer picked (mercpetkind.go)
	return k, err
}

// An alchemist's merc holds a potion bottle (petition 1585, Hogbear: "alc
// option feels off with alc not holding a potion bottle, they got a staff
// instead"): the look of the thrown potions (attack type 10, thrown) the
// alchemists throw, in its main hand where the staff was, as the alchemists'
// NPCs hold it (Chemist L`Kron, R`Exyl and R`Vorr, Aemilia: npcs.primary_weapon,
// the same model); its heirloom staff's figures stay its gear, and its swing's
// animation is a throw's (the hand's attack type). The model is read off the potions' records
// (potionBottle), never chosen by hand; none found, the staff is shown.
const (
	mercAlchemist    = 14
	thrownAttackType = 10
)

// potionBottle is the thrown potions' look: the model most thrown potion
// records (attack type 10, "Potion" in the name) share, 0 none.
func (m *Mercs) potionBottle() uint32 {
	var model int64
	err := m.db.QueryRow(`SELECT model FROM itempattern WHERE attacktype = ? AND itemname LIKE '%Potion%' AND model NOT IN (0, -1)
		GROUP BY model ORDER BY count(*) DESC, model LIMIT 1`, thrownAttackType).Scan(&model)
	if err != nil {
		return 0
	}
	return uint32(model)
}

// bow is the bow a tank's or a melee fighter's merc shoots at range (the
// owner, 2026-11-11: "lets make melee/tank mercs have a bow also"): its
// class's heirloom bow (a ranger's: its lowest record, the seed's own, as its
// other heirlooms), else the plain heirloom bow of the ranger's (no epic's),
// at the level's figures (Heirlooms.ViewFor). None in the database: none.
func (m *Mercs) bow(class, race, level int) entity.MercBow {
	var id int
	err := m.db.QueryRow(`SELECT patternid FROM heirlooms WHERE role = 'ranged' AND class = ? ORDER BY patternid LIMIT 1`, class).Scan(&id)
	if err != nil {
		err = m.db.QueryRow(`SELECT patternid FROM heirlooms WHERE role = 'ranged' AND epic = 0 ORDER BY patternid LIMIT 1`).Scan(&id)
	}
	if err != nil {
		return entity.MercBow{}
	}
	rec, err := database.NewItemPatternStore(m.db).GetItemPattern(id)
	if err != nil || rec == nil {
		return entity.MercBow{}
	}
	if m.heir != nil {
		m.heir.ViewFor(race, level, rec)
	}
	return entity.MercBow{Model: uint32(rec.Model), Type: byte(rec.AttackType), Damage: rec.WeaponDamage, Range: rec.Unk5, Name: rec.ItemName}
}

// heirlooms is what a merc of class and race wears at level: the class's
// heirloom of each slot (its lowest record: the seed's own), a robe for a
// priest or a caster, and its hands (the file's doc), each with its figures
// for the level (Heirlooms.ViewFor), as worn items.
func (m *Mercs) heirlooms(class, race, level int) ([]database.CharInventoryEntry, error) {
	rows, err := m.db.Query(`SELECT patternid, role FROM heirlooms WHERE class = ? ORDER BY patternid`, class)
	if err != nil {
		return nil, err
	}
	byRole := map[string][]int{}
	for rows.Next() {
		var id int
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			rows.Close()
			return nil, err
		}
		byRole[role] = append(byRole[role], id)
	}
	rows.Close()
	store := database.NewItemPatternStore(m.db)
	var out []database.CharInventoryEntry
	wear := func(id, equip int) *database.ItemPatternRecord {
		rec, err := store.GetItemPattern(id)
		if err != nil || rec == nil {
			return nil
		}
		if m.heir != nil {
			m.heir.ViewFor(race, level, rec)
		}
		out = append(out, database.CharInventoryEntry{Pattern: *rec, EquipLoc: equip, Location: 0, RemainHP: rec.MaxHP, StackLeft: 1})
		return rec
	}
	arch := entity.MercRoleName(class)
	for _, w := range mercWear {
		if w.role == "robe" && arch != "healer" && arch != "caster" {
			continue // a robe over a tank's or a melee fighter's armour would hide it
		}
		if ids := byRole[w.role]; len(ids) > 0 {
			wear(ids[0], w.equip)
		}
	}
	mains, two, shields := byRole["main"], byRole["twohand"], byRole["shield"]
	switch arch {
	case "caster":
		if len(two) > 0 {
			wear(two[0], 16)
		} else if len(mains) > 0 {
			wear(mains[0], 14)
		}
	case "melee fighter":
		if class == 4 {
			break // a monk's bare hands: its hand to hand and its punches (offence.go)
		}
		if len(mains) == 0 {
			break
		}
		main := wear(mains[0], 14)
		if level < mercDualFrom || main == nil {
			break
		}
		// its off hand a second weapon of the main hand's kind (a rogue's
		// piercer for its backstabs), else the next one
		off := 0
		for _, id := range mains[1:] {
			if rec, err := store.GetItemPattern(id); err == nil && rec != nil && rec.AttackType == main.AttackType {
				off = id
				break
			}
		}
		if off == 0 && len(mains) > 1 {
			off = mains[1]
		}
		if off != 0 {
			wear(off, 15)
		}
	default: // a tank or a priest: a weapon and a shield
		switch {
		case len(mains) > 0:
			wear(mains[0], 14)
		case len(two) > 0:
			wear(two[0], 16)
		}
		if len(mains) > 0 && len(shields) > 0 {
			wear(shields[0], 17)
		}
	}
	return out, nil
}

// comeBack brings a player's merc back at its login (OnPlayerEntered): its
// row's, at the player's level, unless it rests after its death (said) or
// mercenaries are off.
func (m *Mercs) comeBack(sess *session.Session, p *entity.Player) {
	if !MercsOn() || m.db == nil {
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID)
	if err != nil || !ok {
		if err != nil {
			log.Printf("Merc: %s's hire: %v", p.Name, err)
		}
		return
	}
	now := m.now()
	if now.Before(row.DeadUntil) {
		SendClientMessage(sess, fmt.Sprintf(mercRestLine, row.Name, int(row.DeadUntil.Sub(now).Seconds()+0.5)))
		return
	}
	m.bring(sess, p, row, now)
}

// bring puts the row's merc beside the player, made at its level, its rest
// ended in the row. It reports whether it came.
func (m *Mercs) bring(sess *session.Session, p *entity.Player, row database.MercRow, now time.Time) bool {
	kit, err := m.kit(row, p.Level)
	if err != nil {
		log.Printf("Merc: %s's %q: %v", p.Name, row.Name, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	name, why := m.gw.HireMerc(sess, kit, entity.MercStance(row.Stance), now)
	if why != "" {
		SendErrorMessage(sess, why)
		return false
	}
	if !row.DeadUntil.IsZero() {
		if err := m.db.SetMercRest(row.ServerID, time.Time{}); err != nil {
			log.Printf("Merc: %s's %q's rest not ended: %v", p.Name, name, err)
		}
	}
	SendClientMessage(sess, fmt.Sprintf(mercBackLine, name))
	return true
}

// note keeps a merc's event (entity.OnMercNote, off the World's lock): a
// death's rest, a dismissal by another's hand, its owner's new level's kit,
// its owner out of the arena it waited outside of.
func (m *Mercs) note(n entity.MercNote) {
	if m.db == nil || n.Owner == nil {
		return
	}
	switch n.Kind {
	case entity.MercNoteDied:
		if err := m.db.SetMercRest(n.Owner.CharServerID, n.Until); err != nil {
			log.Printf("Merc: %s's %q's rest not kept: %v", n.Owner.Name, n.Name, err)
		}
	case entity.MercNoteDismissed:
		if err := m.db.DeleteMerc(n.Owner.CharServerID); err != nil {
			log.Printf("Merc: %s's %q's hire not ended: %v", n.Owner.Name, n.Name, err)
		}
	case entity.MercNoteLevel:
		go m.rekit(n.Owner.CharServerID, n.Level)
	case entity.MercNoteBack:
		// its owner out of capture the flag's arena, where it waited
		// (entity/ctfaway.go): back from its row as at a login
		if n.Sess != nil {
			go m.comeBack(n.Sess, n.Owner)
		}
	}
}

// rekit makes the character's merc's kit at its owner's new level and gives
// it to the World.
func (m *Mercs) rekit(serverID uint32, level int) {
	row, ok, err := m.db.Merc(serverID)
	if err != nil || !ok {
		return
	}
	kit, err := m.kit(row, level)
	if err != nil {
		log.Printf("Merc: %q's kit at level %d: %v", row.Name, level, err)
		return
	}
	m.gw.SetMercKit(serverID, kit)
}

// Command is /merc and its words (chat.go's routing).
func (m *Mercs) Command(sess *session.Session, p *entity.Player, args string) {
	if m == nil || m.gw == nil || p == nil {
		return
	}
	if !MercsOn() {
		SendErrorMessage(sess, mercOffLine)
		return
	}
	words := strings.Fields(strings.ToLower(args))
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "", "status", "revive":
		m.status(sess, p, verb == "revive")
	case "dismiss":
		m.dismiss(sess, p)
	case "stance":
		if len(words) < 2 {
			SendErrorMessage(sess, "/merc stance passive, assist or aggressive.")
			return
		}
		s, ok := entity.MercStanceOf(words[1])
		if !ok {
			SendErrorMessage(sess, "/merc stance passive, assist or aggressive.")
			return
		}
		if !m.gw.SetMercStance(sess, s) {
			SendErrorMessage(sess, "You have no mercenary out.")
			return
		}
		if m.db != nil {
			if err := m.db.SetMercStance(p.CharServerID, int(s)); err != nil {
				log.Printf("Merc: %s's stance not kept: %v", p.Name, err)
			}
		}
		SendClientMessage(sess, mercStanceWords[s])
	case "follow", "stay":
		if !m.gw.SetMercStay(sess, verb == "stay") {
			SendErrorMessage(sess, "You have no mercenary out.")
			return
		}
		if verb == "stay" {
			SendClientMessage(sess, "Your mercenary holds its place.")
		} else {
			SendClientMessage(sess, "Your mercenary follows you.")
		}
	case "attack":
		name, why := m.gw.MercAttack(sess)
		if why != "" {
			SendErrorMessage(sess, why)
			return
		}
		SendClientMessage(sess, "Your mercenary attacks "+name+".")
	case "path":
		m.path(sess, p, strings.Join(words[1:], " "))
	case "pet":
		m.pet(sess, p, strings.Join(words[1:], " ")) // mercpetkind.go
	case "charm": // entity/mercharm.go
		var line, why string
		if len(words) > 1 && (words[1] == "off" || words[1] == "release") {
			line, why = m.gw.MercCharmOff(sess)
		} else {
			line, why = m.gw.MercCharm(sess, m.now())
		}
		if why != "" {
			SendErrorMessage(sess, why)
			return
		}
		SendClientMessage(sess, line)
	case "group":
		if line := m.gw.AskMercGroup(sess); line != "" {
			SendClientMessage(sess, line)
		}
	default:
		SendClientMessage(sess, mercHelpLine)
	}
}

// dismiss is /merc dismiss, and vanilla's Group > Dismiss Mercenary
// (mercui.go): the merc out (or waiting outside the arena) sent away and its
// hire ended.
func (m *Mercs) dismiss(sess *session.Session, p *entity.Player) {
	name, ok := m.gw.DismissMerc(sess, m.now())
	if !ok {
		SendErrorMessage(sess, "You have no mercenary out.")
		return
	}
	if m.db != nil {
		if err := m.db.DeleteMerc(p.CharServerID); err != nil {
			log.Printf("Merc: %s's dismissal of %q not kept: %v", p.Name, name, err)
		}
	}
	SendClientMessage(sess, fmt.Sprintf("You dismiss %s. A Mercenary Liaison will hire you another.", name))
}

// mercVerbs are /merc's words, which "/t merc <word>" takes as the command
// (chat.go: anything else to a "merc" is a tell).
var mercVerbs = map[string]bool{"status": true, "dismiss": true, "stance": true, "follow": true, "stay": true, "attack": true,
	"group": true, "revive": true, "help": true, "path": true, "pet": true, "charm": true}

// mercWords reports a tell's body that is /merc's: none, or one of its words
// first.
func mercWords(body string) bool {
	f := strings.Fields(strings.ToLower(body))
	return len(f) == 0 || mercVerbs[f[0]]
}

// mercStanceWords are the stances' lines.
var mercStanceWords = map[entity.MercStance]string{
	entity.MercPassive:    "Your mercenary is passive: it fights only what /merc attack names. A healer still heals.",
	entity.MercAssist:     "Your mercenary assists you: it fights what you fight, and what fights you or it.",
	entity.MercAggressive: "Your mercenary is aggressive: it fights what you fight, and what fights your group or is fought by it.",
}

// status is /merc alone: the merc out, said; a merc resting, its time left; a
// merc rested (or /merc revive), brought back; none, how to hire one.
func (m *Mercs) status(sess *session.Session, p *entity.Player, revive bool) {
	if info, ok := m.gw.MercInfoOf(sess); ok {
		line := fmt.Sprintf("Your mercenary %s, a level %d %s %s (%s): %d/%d hit points, %d/%d power, %s",
			info.Name, info.Level, entity.RaceName(info.Race), entity.ClassName(info.Class), entity.MercRoleName(info.Class),
			info.HP, info.MaxHP, info.Power, info.MaxPower, entity.MercStanceNames[info.Stance])
		if info.Stay {
			line += ", holding its place"
		}
		if info.Target != "" {
			line += ", fighting " + info.Target
		}
		SendClientMessage(sess, line+".")
		if !revive {
			SendClientMessage(sess, mercHelpLine)
		}
		return
	}
	if m.db == nil {
		SendClientMessage(sess, mercNoneLine)
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID)
	if err != nil || !ok {
		SendClientMessage(sess, mercNoneLine)
		return
	}
	now := m.now()
	if now.Before(row.DeadUntil) {
		SendClientMessage(sess, fmt.Sprintf(mercRestLine, row.Name, int(row.DeadUntil.Sub(now).Seconds()+0.5)))
		return
	}
	if p.IsDead() {
		SendErrorMessage(sess, "The dead call no one.")
		return
	}
	m.bring(sess, p, row, now)
}

// Restore gives a player carried through a keep restart its merc back where
// it stood (entity/keepmerc.go), made from its row at its level.
func (m *Mercs) Restore(sess *session.Session, p *entity.Player, k entity.KeptMerc, now time.Time) {
	if m == nil || m.db == nil || !MercsOn() {
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID)
	if err != nil || !ok {
		return
	}
	kit, err := m.kit(row, p.Level)
	if err != nil {
		log.Printf("Keep: %s's mercenary %q: %v", p.Name, row.Name, err)
		return
	}
	m.gw.RestoreMerc(sess, kit, k, now)
}
