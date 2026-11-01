package entity

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// balancekit.go — the kit the fight simulator (balancesim.go) gives a player
// of a class and level out of a server's own tables: its base stats with its
// training points spent, what it wears (itempattern, as CLIENT's CanUse lets
// it), and its book (classspells, each spell as the cast handler builds it).

// simWear is the wear slots an outfit is looked for in, by the equip slot
// each fills (templateGear's: handlers/template.go).
var simWear = []struct{ wear, equip, count int }{
	{1, 0, 1},   // head
	{19, 1, 1},  // hands
	{3, 2, 2},   // ears
	{4, 4, 1},   // neck
	{5, 5, 1},   // chest
	{7, 8, 1},   // forearms (bracers)
	{8, 9, 2},   // rings
	{9, 11, 1},  // waist
	{10, 12, 1}, // legs
	{11, 13, 1}, // feet
	{2, 21, 1},  // robe
}

// simTrainWeights is how an archetype spends its training points, STR..CHA:
// what each class builds up (tanks STA and STR, melee STR and DEX, priests
// WIS and STA, casters INT and STA).
var simTrainWeights = [...][7]float64{
	archTank:   {0.3, 0.4, 0.2, 0.1, 0, 0, 0},
	archMelee:  {0.3, 0.2, 0.2, 0.3, 0, 0, 0},
	archPriest: {0, 0.35, 0, 0.1, 0.45, 0, 0.1},
	archCaster: {0, 0.3, 0, 0.2, 0, 0.5, 0},
}

// SimBaseStats is a character's base stats at a level: the create screen's
// (LookupBaseStats and its 20 points) and every training point its level
// has given (TrainingPointsDue), spent as its archetype spends them, each
// within the level's cap.
func SimBaseStats(class, race, level int) [7]int {
	b := LookupBaseStats(race, class)
	base := [7]int{b.Str, b.Sta, b.Agi, b.Dex, b.Wis, b.Int, b.Cha}
	weights := simTrainWeights[archetypeOf(class)]
	points := float64(TrainingPointsDue(level))
	cap := StatCapBase(level)
	for i := range base {
		base[i] = min(base[i]+int(math.Round(points*weights[i])), cap)
	}
	return base
}

// SimKitFromDB is a player of the class, race and level kitted as the
// server's tables allow: for each slot the item at the tier's place (0 the
// least, 1 the best) among those of the last ten levels it may wear (all
// below when there are none), the best weapon of that place for its hands
// (a shield in the off hand for a tank or a priest that may hold one, a
// second weapon for a melee class), and every spell of its class up to its
// level in its book.
func SimKitFromDB(db *database.SQLiteDB, class, race, level int, tier float64) (SimPlayer, error) {
	pl := SimPlayer{Class: class, Race: race, Level: level, Base: SimBaseStats(class, race, level),
		Play: SimPlay{Melee: true, Buffs: true, Spells: true}}
	if v, ok := simPetsCache.Load(db); ok {
		pl.Pets = v.(*SimPets)
	} else {
		pets, err := SimPetsFromDB(db) // its pet as the server's (balancesim.go)
		if err != nil {
			return pl, err
		}
		simPetsCache.Store(db, pets)
		pl.Pets = pets
	}
	inv := database.NewInventoryStore(db)
	items, err := inv.WearablePatterns(class, race, level, []int{1, 2, 3, 4, 5, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 18, 19})
	if err != nil {
		return pl, err
	}
	book, err := SimBookFromDB(db, class, level, false)
	if err != nil {
		return pl, err
	}
	bySlot := map[int][]database.ItemPatternRecord{}
	for _, it := range items {
		bySlot[it.EquipSlot] = append(bySlot[it.EquipSlot], it)
	}
	g := Gear{WeaponCond: 1}
	wear := func(it *database.ItemPatternRecord, n int) {
		for range n {
			for id, v := range simItemStats(it) {
				g.Add(id, v)
			}
		}
	}
	for _, s := range simWear {
		if it := simPick(bySlot[s.wear], level, tier, simArmourScore); it != nil {
			wear(it, s.count)
		}
	}
	// the hands: what its arts want held, as a player of the class arms
	// itself (the workbook's required equipment, which a cast without it is
	// refused for: workbookmore.go): a weapon of the type most of them
	// name (a rogue's backstabs a one-handed piercer, a paladin's strikes a
	// blunt weapon, a bard's and a ranger's blades a slashing one), a second
	// weapon in the off hand for a melee class (a ranger dual wields), a
	// shield for a tank or a priest; its pick of that kind as any weapon's,
	// by the tier (simHands). A monk's are bare (its punches want them; its
	// hand to hand: offence.go), and it carries no weapon (until 2026-11-01
	// it held one and its 30 punches were never cast). What its other arts
	// want it carries beside (simOtherHands: a ranger's bow for its arrows)
	// and switches to for them, as the server switches a player's hands for
	// an art (handlers/handswitch.go)
	hands := simHandsOf(book)
	fight := simBareHands()
	if class != classMonk {
		fight = simFightingHands(class, level, tier, hands, bySlot)
	}
	sets := []SimHands{fight}
	if class != classMonk {
		sets = append(sets, simOtherHands(class, level, tier, book, hands, bySlot, sets)...)
	}
	pl.Body, pl.Hands = g, sets
	pl.Gear = g.with(fight.Gear)
	pl.Worn, pl.AttackType = fight.Worn, fight.AttackType
	switch {
	case fight.Worn[15]:
		pl.SecondaryType = int(fight.Secondary)
	case fight.Worn[20]:
		pl.SecondaryType = -1 // a held item: no weapon in that hand
	}
	pl.Spells = book
	return pl, nil
}

// SimHands is one way a kit holds its hands (SimKitFromDB): the equip slots
// worn (14 main hand, 15 off hand, 16 both hands, 17 shield, 18 ranged, 19
// and 20 held), the swinging item's attack type and reach for its swing
// (SwingFor: 1-6 melee, 7, 9 or 10 ranged with its range; 0 bare hands), the
// hands' attack types as the server's worn look gives them (the main hand's;
// the off hand's or the ranged weapon's), for a spell's required equipment,
// and what its items add (their stats, and the swinging weapon's DMG as
// handlers' wornGear takes it).
type SimHands struct {
	Worn               [21]bool
	AttackType, Range  int
	Primary, Secondary byte
	Gear               Gear
}

// fits is whether the hands hold what a spell's required equipment names.
func (h *SimHands) fits(req string) bool {
	p := Player{PrimaryType: h.Primary, SecondaryType: h.Secondary}
	if h.Worn[17] {
		p.Shield = 1
	}
	return equipFits(&p, req)
}

// simBareHands is hands holding nothing.
func simBareHands() SimHands { return SimHands{Gear: Gear{WeaponCond: 1, BareHands: true}} }

// simMeleeHands is hands holding a weapon in the main hand (both, for a
// two-hander) and off in the other hand's offSlot (15 a second weapon, 17 a
// shield, 20 an item held; nil none, and none beside a two-hander).
func simMeleeHands(main *database.ItemPatternRecord, two bool, off *database.ItemPatternRecord, offSlot int) SimHands {
	h := SimHands{Gear: Gear{WeaponCond: 1}}
	add := func(it *database.ItemPatternRecord) {
		for id, v := range simItemStats(it) {
			h.Gear.Add(id, v)
		}
	}
	add(main)
	slot := 14
	if two {
		slot = 16
	}
	h.Worn[slot] = true
	h.Gear.WeaponDamage, h.Gear.WeaponTwoHanded, h.Gear.WeaponSlot, h.Gear.WeaponType = main.WeaponDamage, two, slot, main.AttackType
	h.AttackType, h.Primary = main.AttackType, byte(main.AttackType)
	if h.AttackType > 6 {
		h.AttackType = 1 // a melee swing, whatever the record calls it
	}
	if off != nil && !two {
		add(off)
		h.Worn[offSlot] = true
		if offSlot == 15 || offSlot == 20 {
			h.Secondary = byte(off.AttackType)
		}
	}
	return h
}

// simRangedHands is hands holding a ranged weapon alone, as its equip leaves
// them (equip.go's equipDisplaces: a bow takes the hands off).
func simRangedHands(bow *database.ItemPatternRecord) SimHands {
	h := SimHands{Gear: Gear{WeaponCond: 1, WeaponDamage: bow.WeaponDamage, WeaponSlot: 18, WeaponType: bow.AttackType}}
	for id, v := range simItemStats(bow) {
		h.Gear.Add(id, v)
	}
	h.Worn[18] = true
	h.AttackType, h.Range, h.Secondary = bow.AttackType, bow.Unk5, byte(bow.AttackType)
	return h
}

// simFightingHands is the hands a kit fights with: the weapon of the type
// most of its arts name (or any), a second one for a melee class, a shield
// for a tank or a priest that may hold one, else an item held (not a melee
// class); a two-hander where it deals more a second as GetSwing swings it
// (a rogue or a bard swings one once a round), never in place of a shield,
// nor for a ranger, which dual wields (the user, 2026-11-01: "they can dual
// wield and switch to a bow").
func simFightingHands(class, level int, tier float64, hands simHands, bySlot map[int][]database.ItemPatternRecord) SimHands {
	arch := archetypeOf(class)
	mains, twos := hands.weapons(append(append([]database.ItemPatternRecord(nil), bySlot[12]...), bySlot[14]...)), hands.weapons(bySlot[15])
	if len(mains) == 0 && len(twos) == 0 { // none of the kind to be had: any
		mains, twos = append(append([]database.ItemPatternRecord(nil), bySlot[12]...), bySlot[14]...), bySlot[15]
	}
	main := simPick(mains, level, tier, simWeaponScore)
	two := simPick(twos, level, tier, simWeaponScore)
	var off *database.ItemPatternRecord
	offSlot := 0
	switch {
	case arch == archTank || arch == archPriest:
		if off = simPick(bySlot[13], level, tier, simArmourScore); off != nil {
			offSlot = 17
		}
	case arch == archMelee:
		if off = simPick(hands.weapons(bySlot[14]), level, tier, simWeaponScore); off == nil {
			off = simPick(bySlot[14], level, tier, simWeaponScore)
		}
		if off != nil {
			offSlot = 15
		}
	}
	if off == nil && arch != archMelee {
		if off = simPick(bySlot[18], level, tier, simArmourScore); off != nil {
			offSlot = 20
		}
	}
	rate := func(it *database.ItemPatternRecord, worn [21]bool, two bool) float64 {
		d := float64(it.WeaponDamage)
		if two {
			d *= offTwoHand
		}
		kind := GetSwing(class, level, worn)
		return d * float64(len(swingHits(kind))) / SwingDelay(kind).Seconds()
	}
	useTwo := two != nil && main == nil
	if two != nil && main != nil && offSlot != 17 && class != classRanger {
		var w1, w2 [21]bool
		w1[14], w1[offSlot] = true, off != nil
		w2[16] = true
		useTwo = rate(two, w2, true) > rate(main, w1, false)
	}
	switch {
	case useTwo:
		return simMeleeHands(two, true, nil, 0)
	case main != nil:
		return simMeleeHands(main, false, off, offSlot)
	}
	return simBareHands()
}

// simOtherHands is what a kit carries for the arts its fighting hands do not
// fit (have, the first its fighting hands), each the hands the server's
// switch would put on for them: a bow where its arrows are a line of its arts
// (simHands.bowed: a ranger's 24 to its 20 blades), a weapon of the kind its
// other blows name (a second one for a melee class but a knight, as the
// switch gives one), bare hands; an art no kind of which it may wear (or a
// shield's: a tank or a priest fights with its own) is refused, as a
// player's is with nothing carried.
func simOtherHands(class, level int, tier float64, book []CastSpell, hands simHands, bySlot map[int][]database.ItemPatternRecord, have []SimHands) []SimHands {
	var out []SimHands
	fitted := func(req string) bool {
		for i := range have {
			if have[i].fits(req) {
				return true
			}
		}
		for i := range out {
			if out[i].fits(req) {
				return true
			}
		}
		return false
	}
	for i := range book {
		req := book[i].Def.ReqEquip
		if req == "" || fitted(req) {
			continue
		}
		var types []int
		var ranged, none bool
		for _, code := range ReqEquipCodes(req) {
			switch code {
			case "RANGE", "RANGED":
				ranged = true
			case "NONE":
				none = true
			default:
				for _, t := range EquipCodeTypes(code) {
					types = append(types, int(t))
				}
			}
		}
		of := func(items []database.ItemPatternRecord, keep func(*database.ItemPatternRecord) bool) []database.ItemPatternRecord {
			var o []database.ItemPatternRecord
			for i := range items {
				if keep(&items[i]) {
					o = append(o, items[i])
				}
			}
			return o
		}
		typed := func(it *database.ItemPatternRecord) bool { return slices.Contains(types, it.AttackType) }
		switch {
		case ranged && hands.bowed():
			bows := of(bySlot[16], func(it *database.ItemPatternRecord) bool {
				return (it.AttackType == 7 || it.AttackType == 9 || it.AttackType == 10) && it.WeaponDamage > 0 // never a thrown potion
			})
			if bow := simPick(bows, level, tier, simWeaponScore); bow != nil {
				out = append(out, simRangedHands(bow))
			}
		case len(types) > 0:
			main := simPick(of(append(append([]database.ItemPatternRecord(nil), bySlot[12]...), bySlot[14]...), typed), level, tier, simWeaponScore)
			two := main == nil
			if two {
				main = simPick(of(bySlot[15], typed), level, tier, simWeaponScore)
			}
			if main == nil {
				continue
			}
			var off *database.ItemPatternRecord
			knight := class == 2 || class == 3 // holds no second weapon (handlers' offHandWeaponBarred)
			if !two && archetypeOf(class) == archMelee && !knight {
				off = simPick(of(bySlot[14], typed), level, tier, simWeaponScore)
			}
			out = append(out, simMeleeHands(main, two, off, 15))
		case none:
			out = append(out, simBareHands())
		}
	}
	return out
}

// with is what a kit wears all told: g its body's, h its hands' (their stats
// added, the weapon's fields theirs).
func (g Gear) with(h Gear) Gear {
	out := g
	out.Str, out.Sta, out.Agi, out.Dex = g.Str+h.Str, g.Sta+h.Sta, g.Agi+h.Agi, g.Dex+h.Dex
	out.Wis, out.Int, out.Cha = g.Wis+h.Wis, g.Int+h.Int, g.Cha+h.Cha
	out.MaxHP, out.MaxPower, out.AC = g.MaxHP+h.MaxHP, g.MaxPower+h.MaxPower, g.AC+h.AC
	out.PoTItems = append(append([]int(nil), g.PoTItems...), h.PoTItems...)
	out.HoTItems = append(append([]int(nil), g.HoTItems...), h.HoTItems...)
	out.PR, out.DR, out.FR, out.CR, out.LR, out.AR = g.PR+h.PR, g.DR+h.DR, g.FR+h.FR, g.CR+h.CR, g.LR+h.LR, g.AR+h.AR
	out.WeaponDamage, out.WeaponCond, out.WeaponTwoHanded = h.WeaponDamage, h.WeaponCond, h.WeaponTwoHanded
	out.WeaponSlot, out.WeaponType, out.BareHands = h.WeaponSlot, h.WeaponType, h.BareHands
	return out
}

// simHands is what a book's spells want held (their ReqEquip, the
// workbook's required equipment): of each melee attack type 1-6 how many
// of them it lets cast, and how many want a bow; a spell that takes any
// weapon (or names none) counts for none of them.
type simHands struct {
	byType [7]int
	bow    int
}

func simHandsOf(book []CastSpell) simHands {
	var h simHands
	for i := range book {
		req := book[i].Def.ReqEquip
		if req == "" {
			continue
		}
		var types [7]bool
		for _, tok := range strings.Split(strings.ToUpper(req), ",") {
			switch tok = strings.TrimSpace(tok); tok {
			case "RANGE", "RANGED":
				h.bow++
			default:
				for _, t := range equipCodes[tok] {
					if t >= 1 && t <= 6 {
						types[t] = true
					}
				}
			}
		}
		for t := 1; t <= 6; t++ {
			if types[t] {
				h.byType[t]++
			}
		}
	}
	return h
}

// simHandsLine is the fewest arts that choose a kit's weapon: a line of them.
const simHandsLine = 3

// most is the most spells one weapon type lets cast.
func (h simHands) most() int {
	m := 0
	for t := 1; t <= 6; t++ {
		m = max(m, h.byType[t])
	}
	return m
}

// bowed is whether its arrows want a bow: half as many of them as the most
// a weapon type lets cast, or more (a ranger's 24 arrows to its 17 blades).
func (h simHands) bowed() bool { return h.bow > 0 && 2*h.bow >= h.most() }

// weapons is the items of the attack types that let the most of its arts be
// cast (every item when the types tie, a warrior's arts taking any, or when
// fewer than simHandsLine arts name one: a monk's lone two-handed Fierce
// Strike among its punches does not choose its weapon).
func (h simHands) weapons(items []database.ItemPatternRecord) []database.ItemPatternRecord {
	m := h.most()
	if m < simHandsLine {
		return items
	}
	var out []database.ItemPatternRecord
	for _, it := range items {
		if it.AttackType >= 1 && it.AttackType <= 6 && h.byType[it.AttackType] == m {
			out = append(out, it)
		}
	}
	return out
}

// SimStartKitFromDB is a new character of the class and race at the level as
// its creation outfits one (database.StartItems: Sandstorm's starting weapon,
// an Apprentice Sword, Dagger, Club or Staff, in hand; its rations carried;
// no armour at all, a monk not even a weapon), its base stats and book as
// SimKitFromDB's: what a player fights with at 1-5 before it buys or finds
// anything (the coordinator's check of 2026-10-30: a level 3 alchemist "beat
// up by a level 1"). A race with no row for the class (a human shaman) takes
// the first race's that has one; a human's city the east's, then the west's.
func SimStartKitFromDB(db *database.SQLiteDB, class, race, level int) (SimPlayer, error) {
	pl := SimPlayer{Class: class, Race: race, Level: level, Base: SimBaseStats(class, race, level),
		Play: SimPlay{Melee: true, Buffs: true, Spells: true}}
	items, err := simStartItems(db, class, race)
	if err != nil {
		return pl, err
	}
	inv := database.NewInventoryStore(db)
	g := Gear{WeaponCond: 1}
	for _, it := range items {
		rec, ok, err := inv.ItemPatternByID(it.PatternID)
		if err != nil {
			return pl, err
		}
		if !ok || (rec.EquipSlot != 12 && rec.EquipSlot != 14 && rec.EquipSlot != 15) || g.WeaponDamage > 0 {
			continue // carried, not worn (rations; a second weapon)
		}
		for id, v := range simItemStats(&rec) {
			g.Add(id, v)
		}
		g.WeaponDamage, g.WeaponType, pl.AttackType = rec.WeaponDamage, rec.AttackType, rec.AttackType
		if rec.EquipSlot == 15 {
			g.WeaponTwoHanded, g.WeaponSlot, pl.Worn[16] = true, 16, true
		} else {
			g.WeaponSlot, pl.Worn[14] = 14, true
		}
	}
	g.BareHands = !pl.Worn[14] && !pl.Worn[16] // a monk's: its hand to hand (offence.go)
	if pl.AttackType > 6 {
		pl.AttackType = 1
	}
	pl.Gear = g
	pl.Spells, err = SimBookFromDB(db, class, level, false)
	return pl, err
}

// simStartItems is what a new character of the class carries: its race's
// row, a human's of the east then of the west, else the first race's that
// has the class (by race).
func simStartItems(db *database.SQLiteDB, class, race int) ([]database.StartItem, error) {
	for _, h := range []int{1, 2, 0} {
		items, err := db.StartItems(race, class, h)
		if err != nil || len(items) > 0 {
			return items, err
		}
	}
	var other int
	err := db.QueryRow(`SELECT race FROM startitems WHERE class = ? ORDER BY race, humtype LIMIT 1`, class).Scan(&other)
	if err != nil {
		return nil, nil // no row anywhere: nothing carried
	}
	for _, h := range []int{1, 2, 0} {
		items, err := db.StartItems(other, class, h)
		if err != nil || len(items) > 0 {
			return items, err
		}
	}
	return nil, nil
}

// SimBookFromDB is every spell of the class up to the level that its levels
// give (database.Granted: not a mastery's, an epic's or a master class's,
// unless all), each as the cast handler builds it (handlers/cast.go
// castSpell): its record, its line and its visual.
func SimBookFromDB(db *database.SQLiteDB, class, level int, all bool) ([]CastSpell, error) {
	rows, err := db.Query(`SELECT p.spellid, c.source FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid
		WHERE c.class = ? AND p.abilitylvl <= ? ORDER BY p.abilitylvl, p.spellid`, class, level)
	if err != nil {
		return nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		var source string
		if err := rows.Scan(&id, &source); err != nil {
			rows.Close()
			return nil, err
		}
		if all || database.Granted(source) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	store := database.NewSpellbookStore(db)
	var book []CastSpell
	for i, id := range ids {
		sp, err := store.SpellPattern(id)
		if err != nil || sp == nil {
			continue
		}
		row, _, _ := store.SpellEffectRow(id)
		line, fx := row.Line, row.FX
		def := SpellDefFrom(sp.SpellName, line, sp.SpellDesc, row.Effects) // the workbook's when it has them
		book = append(book, CastSpell{SpellID: sp.SpellID, DisplayIndex: i + 1, Level: sp.AbilityLvl, Scope: sp.Scope,
			Name: sp.SpellName, Icon: uint32(sp.Icon), CastTime: time.Duration(sp.CastTime) * time.Second,
			Recast: time.Duration(sp.Recast) * time.Second, PowerCost: sp.Power,
			Range: math.Float32frombits(uint32(uint16(sp.Range)) << 16), Def: def, FX: fx,
			Area: ReadSpellArea(sp.SpellName, line, def)})
	}
	return book, nil
}

// SimWithSpell is the kit with the spell of that id in its book, from every
// spell of its class at its level (a mastery's, a quest's too) when its own
// lacks it: a sweep's pet bought through the masteries (balancesim
// -petspell: Summon Badi, Gymon's Animation).
func SimWithSpell(db *database.SQLiteDB, pl SimPlayer, id int) (SimPlayer, error) {
	for _, s := range pl.Spells {
		if s.SpellID == id {
			return pl, nil
		}
	}
	all, err := SimBookFromDB(db, pl.Class, pl.Level, true)
	if err != nil {
		return pl, err
	}
	for _, s := range all {
		if s.SpellID == id {
			s.DisplayIndex = len(pl.Spells) + 1
			pl.Spells = append(append([]CastSpell(nil), pl.Spells...), s)
			return pl, nil
		}
	}
	return pl, fmt.Errorf("no spell %d of class %d at or below level %d", id, pl.Class, pl.Level)
}

// simPick is the item at the tier's place by score among those of the last
// ten levels (all of them when none is), nil for none. An item of no level
// (levelreq 0: a drop whose record lacks one, most of them a high level's)
// and a GM's test item are no one's outfit.
func simPick(items []database.ItemPatternRecord, level int, tier float64, score func(*database.ItemPatternRecord) int) *database.ItemPatternRecord {
	var near, all []database.ItemPatternRecord
	for _, it := range items {
		if it.LevelReq < 1 || strings.Contains(strings.ToLower(it.ItemName), "test") {
			continue
		}
		all = append(all, it)
		if it.LevelReq >= level-9 {
			near = append(near, it)
		}
	}
	if len(near) == 0 {
		near = all
	}
	if len(near) == 0 {
		return nil
	}
	sort.SliceStable(near, func(i, j int) bool { return score(&near[i]) < score(&near[j]) })
	i := int(math.Round(tier * float64(len(near)-1)))
	it := near[min(max(i, 0), len(near)-1)]
	return &it
}

func simV(x *int) int {
	if x == nil {
		return 0
	}
	return *x
}

// simArmourScore is an item's worth as the #template judges it
// (handlers/template.go templateScore).
func simArmourScore(p *database.ItemPatternRecord) int {
	stats := simV(p.Str) + simV(p.Sta) + simV(p.Agi) + simV(p.Dex) + simV(p.Wis) + simV(p.Intelligence) + simV(p.Cha)
	resists := simV(p.PR) + simV(p.DR) + simV(p.FR) + simV(p.CR) + simV(p.LR) + simV(p.AR)
	return simV(p.AC) + 2*(stats+simV(p.PoT)+simV(p.HoT)) + (simV(p.HPMAX)+simV(p.POWMAX))/2 + resists/4
}

// simWeaponScore is a weapon's: the DMG the melee rolls on, then its stats.
func simWeaponScore(p *database.ItemPatternRecord) int {
	return p.WeaponDamage*1000 + simArmourScore(p)
}

// simItemStats is an item's stats by the ids a dump gives them
// (handlers/character.go itemPatternStats).
func simItemStats(p *database.ItemPatternRecord) map[int]int {
	out := map[int]int{}
	for id, v := range map[int]*int{0: p.Str, 1: p.Sta, 2: p.Agi, 3: p.Dex, 4: p.Wis, 5: p.Intelligence, 6: p.Cha,
		8: p.HPMAX, 10: p.POWMAX, 12: p.PoT, 13: p.HoT, 14: p.AC, 22: p.PR, 23: p.DR, 24: p.FR, 25: p.CR, 26: p.LR, 27: p.AR} {
		if v != nil && *v != 0 {
			out[id] = *v
		}
	}
	return out
}
