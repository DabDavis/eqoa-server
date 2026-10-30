package entity

import (
	"math"
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
	inv := database.NewInventoryStore(db)
	items, err := inv.WearablePatterns(class, race, level, []int{1, 2, 3, 4, 5, 7, 8, 9, 10, 11, 12, 13, 14, 15, 18, 19})
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
	// the hands
	arch := archetypeOf(class)
	main := simPick(append(append([]database.ItemPatternRecord(nil), bySlot[12]...), bySlot[14]...), level, tier, simWeaponScore)
	two := simPick(bySlot[15], level, tier, simWeaponScore)
	var off *database.ItemPatternRecord
	offSlot := 0
	switch {
	case arch == archTank || arch == archPriest:
		if off = simPick(bySlot[13], level, tier, simArmourScore); off != nil {
			offSlot = 17
		}
	case arch == archMelee:
		if off = simPick(bySlot[14], level, tier, simWeaponScore); off != nil {
			offSlot = 15
		}
	}
	if off == nil && arch != archMelee {
		if off = simPick(bySlot[18], level, tier, simArmourScore); off != nil {
			offSlot = 20
		}
	}
	// a tank or a priest keeps its shield; anyone else takes the hands that
	// deal more a second as GetSwing swings them (a rogue or a bard swings
	// a two-hander once a round)
	rate := func(it *database.ItemPatternRecord, worn [21]bool, two bool) float64 {
		d := float64(it.WeaponDamage)
		if two {
			d *= offTwoHand
		}
		kind := GetSwing(class, level, worn)
		return d * float64(len(swingHits(kind))) / SwingDelay(kind).Seconds()
	}
	useTwo := two != nil && main == nil
	if two != nil && main != nil && offSlot != 17 {
		var w1, w2 [21]bool
		w1[14], w1[offSlot] = true, off != nil
		w2[16] = true
		useTwo = rate(two, w2, true) > rate(main, w1, false)
	}
	switch {
	case useTwo:
		wear(two, 1)
		g.WeaponDamage, g.WeaponTwoHanded = two.WeaponDamage, true
		pl.Worn[16], pl.AttackType = true, two.AttackType
	case main != nil:
		wear(main, 1)
		g.WeaponDamage = main.WeaponDamage
		pl.Worn[14], pl.AttackType = true, main.AttackType
		if off != nil {
			wear(off, 1)
			pl.Worn[offSlot] = true
		}
	}
	if pl.AttackType > 6 {
		pl.AttackType = 1 // a melee swing, whatever the record calls it
	}
	pl.Gear = g
	pl.Spells, err = SimBookFromDB(db, class, level, false)
	return pl, err
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
		g.WeaponDamage, pl.AttackType = rec.WeaponDamage, rec.AttackType
		if rec.EquipSlot == 15 {
			g.WeaponTwoHanded, pl.Worn[16] = true, true
		} else {
			pl.Worn[14] = true
		}
	}
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
		line, fx, _, _ := store.SpellEffect(id)
		def := ParseSpellNamed(sp.SpellName, line, sp.SpellDesc)
		book = append(book, CastSpell{SpellID: sp.SpellID, DisplayIndex: i + 1, Level: sp.AbilityLvl, Scope: sp.Scope,
			Name: sp.SpellName, Icon: uint32(sp.Icon), CastTime: time.Duration(sp.CastTime) * time.Second,
			Recast: time.Duration(sp.Recast) * time.Second, PowerCost: sp.Power,
			Range: math.Float32frombits(uint32(uint16(sp.Range)) << 16), Def: def, FX: fx,
			Area: ReadSpellArea(sp.SpellName, line, def)})
	}
	return book, nil
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
