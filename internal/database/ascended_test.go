package database

import (
	"database/sql"
	"errors"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ascendedDB is importedDB's records (a sword and a helm of gear, the helm
// with a use effect, a potion and an Heirloom) and five more: a NO RENT ring
// (Lord Nagafen's twins were made so: norent.go), a spell scroll, a thrown
// potion worn as a ranged weapon, a stack of throwing knives and a record
// whose Ascended name is longer than CLIENT shows.
func ascendedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := importedDB(t)
	for _, q := range []string{
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc,
			model, color, wis, cha, intelligence, POWMAX, PoT, HoT, PR, DR, CR, LR, AR)
			VALUES (715011, 900, 0, 7, 0, 8, 0, 1, 0, 0, 0, 0, 0, 60, 1, 0, 0, 32767, 1023, 0, 1, 0, 0, 'Ring of Nagafen', 'Hot.',
				11, 2, 14, 4, 5, 25, 3, 1, 9, 10, 11, 12, 45)`,
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (60001, 1, 0, -607649328, 0, -1, 0, 0, 1, 0, 0, 0, 0, 60, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Largesse of Plague', '', 0, 0),
			       (60002, 1, 0, 8, 0, 17, 0, 0, 1, 0, 10, 0, 0, 60, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Volatile Draught', '', 0, 0),
			       (60003, 1, 0, 8, 0, 17, 0, 0, 1, 0, 9, 25, 0, 60, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Throwing Knife', '', 0, 0),
			       (60004, 1, 0, 8, 0, 5, 0, 0, 1, 0, 0, 0, 0, 60, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
	} {
		args := []any{}
		if strings.Contains(q, "60004") {
			args = append(args, strings.Repeat("n", MaxItemNameChars-len(AscendedPrefix)+1))
		}
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db
}

func ascendedCopy(t *testing.T, db *sql.DB, base int) (int, bool, error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, made, err := AscendedCopyTx(tx, base, "test", time.Unix(1_800_000_000, 0))
	if err != nil {
		return id, made, err
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id, made, nil
}

// The petition's "flat +10% to all of their stats": a figure above 0 times
// 110/100 in whole numbers, half up; a penalty and nothing as they are.
func TestAnAscendedFigureIsATenthMore(t *testing.T) {
	for v, want := range map[int]int{0: 0, 1: 1, 4: 4, 5: 6, 9: 10, 10: 11, 14: 15, 15: 17, 25: 28, 41: 45, 581: 639, 100000: 110000,
		-1: -1, -10: -10} {
		if got := AscendedFigure(v); got != want {
			t.Errorf("AscendedFigure(%d) = %d, want %d", v, got, want)
		}
	}
	if got := ascendedLine("Self, Increase AR by 75, Decrease Hitpoints by 1500, 30min"); got != "Self, Increase AR by 83, Decrease Hitpoints by 1650, 30min" {
		t.Errorf("an effect line: %q", got)
	}
	for in, want := range map[string]string{"Ascended Ring of Veeshan": "Ring of Veeshan", "ascended  Fang ": "Fang",
		"Ring of Veeshan": "Ring of Veeshan", "Ascendedness": "Ascendedness"} {
		if got := AscendedBaseName(in); got != want {
			t.Errorf("AscendedBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Petition 1446: an Ascended record is its base with every stat, AC, resist,
// hit point and power maximum and tick and a weapon's damage a tenth more,
// named "Ascended <name>"; its durability (maxhp) and price (itemcost), its
// look, slot, level, classes, NO TRADE and Lore flags its base's; never NO
// RENT; its use effect's "by N" a tenth more. Made once a record. The
// control: what is no gear (a potion, an Heirloom, a spell scroll, a thrown
// potion, a stack) and a copy are given back as they are.
func TestAnAscendedRecordIsItsBaseATenthMore(t *testing.T) {
	db := ascendedDB(t)
	id, made, err := ascendedCopy(t, db, 1026)
	if err != nil || !made || id != ascendedFirst {
		t.Fatalf("Steelwind's Ascended record: %d (made %v, %v), want %d made", id, made, err, ascendedFirst)
	}
	var name, desc string
	var dmg, str, ac, hp, fr, cost, model, maxhp, rent, slot, level, classes int
	var sta sql.NullInt64
	if err := db.QueryRow(`SELECT itemname, itemdesc, weapondamage, str, sta, AC, HPMAX, FR, itemcost, model, maxhp, rent, equipslot,
		levelreq, classuse FROM itempattern WHERE patternid = ?`, id).
		Scan(&name, &desc, &dmg, &str, &sta, &ac, &hp, &fr, &cost, &model, &maxhp, &rent, &slot, &level, &classes); err != nil {
		t.Fatal(err)
	}
	if name != "Ascended Steelwind" || desc != "A blade." || dmg != 639 || str != 17 || sta.Valid || ac != 0 || hp != 77 || fr != -10 ||
		cost != 4201 || model != 402229003 || maxhp != 200 || rent != 1 || slot != 14 || level != 49 || classes != 1 {
		t.Errorf("Steelwind's: %q %q dmg %d str %d sta %v ac %d hp %d fr %d cost %d model %d durability %d rent %d slot %d level %d classes %d",
			name, desc, dmg, str, sta, ac, hp, fr, cost, model, maxhp, rent, slot, level, classes)
	}
	// every scaled figure of a ring, and NO RENT made rent 1 with its NO TRADE and Lore kept
	ring, _, err := ascendedCopy(t, db, 715011)
	if err != nil {
		t.Fatal(err)
	}
	var wis, cha, intl, pow, pot, hot, pr, dr, cr, lr, ar, trade, lore int
	if err := db.QueryRow(`SELECT wis, cha, intelligence, POWMAX, PoT, HoT, PR, DR, CR, LR, AR, rent, trade, lore, itemcost, maxhp
		FROM itempattern WHERE patternid = ?`, ring).Scan(&wis, &cha, &intl, &pow, &pot, &hot, &pr, &dr, &cr, &lr, &ar, &rent, &trade, &lore,
		&cost, &maxhp); err != nil {
		t.Fatal(err)
	}
	if got := []int{wis, cha, intl, pow, pot, hot, pr, dr, cr, lr, ar, rent, trade, lore, cost, maxhp}; !equalInts(got,
		[]int{15, 4, 6, 28, 3, 1, 10, 11, 12, 13, 50, 1, 1, 1, 900, 0}) {
		t.Errorf("the ring's: %v", got)
	}
	helm, _, err := ascendedCopy(t, db, 6207)
	if err != nil {
		t.Fatal(err)
	}
	var line string
	var cooldown int
	if err := db.QueryRow(`SELECT line, cooldown FROM itemeffect WHERE patternid = ?`, helm).Scan(&line, &cooldown); err != nil {
		t.Fatal(err)
	}
	if line != "Self, Increase AR by 83, Increase STR by 28, 30min" || cooldown != 60 {
		t.Errorf("the helm's use effect: %q cooldown %d", line, cooldown)
	}
	if again, made, err := ascendedCopy(t, db, 1026); again != id || made || err != nil {
		t.Errorf("Steelwind again: %d (made %v, %v), want %d", again, made, err, id)
	}
	if self, made, err := ascendedCopy(t, db, id); self != id || made || err != nil {
		t.Errorf("an Ascended record given again: %d (made %v, %v), want itself", self, made, err)
	}
	imp, _ := copyOf(t, db, 1026) // an import's copy (imported.go)
	for _, base := range []int{50001, 714302, 60001, 60002, 60003, imp, 424242} {
		if got, made, err := ascendedCopy(t, db, base); got != base || made || err != nil {
			t.Errorf("record %d (no gear, a copy or none): %d (made %v, %v), want itself", base, got, made, err)
		}
	}
	if _, _, err := ascendedCopy(t, db, 60004); !errors.Is(err, ErrAscendedNameTooLong) {
		t.Errorf("a name past %d characters: %v", MaxItemNameChars, err)
	}
	m, err := AscendedMap(db)
	if err != nil || len(m) != 3 || m[id] != 1026 || m[ring] != 715011 || m[helm] != 6207 {
		t.Errorf("AscendedMap: %v %v", m, err)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A base changed after its Ascended record was made (a GM's figure, a new
// look) takes the record with it at the server's start, and a record a GM
// edited goes back to the rule (fix the base, never a copy); one as its base
// makes it is not written. The control: an untouched record stays.
func TestTheAscendedRecordsFollowTheirBasesAtTheStart(t *testing.T) {
	db := ascendedDB(t)
	sword, _, _ := ascendedCopy(t, db, 1026)
	helm, _, _ := ascendedCopy(t, db, 6207)
	if _, err := db.Exec(`UPDATE itempattern SET model = 612804823, str = 30, itemname = 'Steelwind Blade' WHERE patternid = 1026`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE itempattern SET AC = 999 WHERE patternid = ?`, helm); err != nil {
		t.Fatal(err)
	}
	n, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0))
	if err != nil || n != 2 {
		t.Fatalf("synced %d (%v), want the sword's and the helm's", n, err)
	}
	var model, str, ac int
	var name string
	if err := db.QueryRow(`SELECT model, str, itemname FROM itempattern WHERE patternid = ?`, sword).Scan(&model, &str, &name); err != nil {
		t.Fatal(err)
	}
	if model != 612804823 || str != 33 || name != "Ascended Steelwind Blade" {
		t.Errorf("the sword's record after its base changed: model %d str %d %q", model, str, name)
	}
	if err := db.QueryRow(`SELECT AC FROM itempattern WHERE patternid = ?`, helm).Scan(&ac); err != nil || ac != 143 {
		t.Errorf("the helm's record a GM edited: AC %d (%v), want its base's 130 a tenth more", ac, err)
	}
	if n, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil || n != 0 {
		t.Errorf("a second start synced %d (%v), want none", n, err)
	}
}

// Heirloom petition 3235: the owner's "just race all" set the Ascended
// records of narrower bases to 1023, and the next start's sync put the
// Ascended Breastplate of the Anointed back to its base's 1439 (no ogre). A
// copy whose race mask lets every race wear it that its base's does, and
// more, keeps it through the start (1023 reaches the froglok's bit 1024 as
// the server reads it); its other columns still follow the base. The
// controls: a copy narrowed below its base, or widened to other races while
// losing one of its base's, goes back to the base's mask; a base widened past
// its copy takes the copy with it.
func TestTheSyncKeepsAnAscendedRaceMaskWiderThanItsBase(t *testing.T) {
	db := ascendedDB(t)
	sword, _, _ := ascendedCopy(t, db, 1026)
	helm, _, _ := ascendedCopy(t, db, 6207)
	if _, err := db.Exec(`UPDATE itempattern SET raceuse = 1439 WHERE patternid IN (1026, 6207, ?, ?)`, sword, helm); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE itempattern SET raceuse = 1023 WHERE patternid = ?`, sword); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE itempattern SET raceuse = 1, str = 30 WHERE patternid = ?`, helm); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	race := func(id int) int {
		t.Helper()
		var r int
		if err := db.QueryRow(`SELECT raceuse FROM itempattern WHERE patternid = ?`, id).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := race(sword); r != 1023 {
		t.Errorf("the sword's Ascended record widened to every race: %d after the start, want 1023 kept", r)
	}
	if r := race(helm); r != 1439 {
		t.Errorf("the helm's Ascended record narrowed to one race: %d after the start, want its base's 1439", r)
	}
	if n, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil || n != 0 {
		t.Errorf("a second start synced %d (%v), want none: the kept mask is the rule's", n, err)
	}
	// the copy's other columns still follow its base
	if _, err := db.Exec(`UPDATE itempattern SET str = 30 WHERE patternid = 1026`); err != nil {
		t.Fatal(err)
	}
	if n, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil || n != 1 {
		t.Errorf("the base's STR changed: synced %d (%v), want the sword's", n, err)
	}
	var str int
	if err := db.QueryRow(`SELECT str FROM itempattern WHERE patternid = ?`, sword).Scan(&str); err != nil || str != 33 || race(sword) != 1023 {
		t.Errorf("the sword after its base's STR changed: str %d race %d (%v), want 33 and 1023", str, race(sword), err)
	}
	// another race for one of its base's: not wider, the base's again
	if _, err := db.Exec(`UPDATE itempattern SET raceuse = 1438 | 512 WHERE patternid = ?`, sword); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if r := race(sword); r != 1439 {
		t.Errorf("the sword's record trading the base's human for the ogre: %d, want its base's 1439", r)
	}
	// a base widened past its copy takes it along
	if _, err := db.Exec(`UPDATE itempattern SET raceuse = 0 WHERE patternid = 1026`); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if r := race(sword); r != 0 {
		t.Errorf("the sword's base opened to every race (0): its record %d, want 0", r)
	}
}

func TestAscendedKeepsRace(t *testing.T) {
	for _, c := range []struct {
		copy, base int64
		keep       bool
	}{
		{1023, 1439, true}, {1439, 1439, true}, {1471, 1439, true}, {0, 1439, true}, {1023, 0, true},
		{1, 1439, false}, {1950, 1439, false}, {2047, 1023, true}, {1023, 2047, true},
	} {
		if got := AscendedKeepsRace(c.copy, c.base); got != c.keep {
			t.Errorf("AscendedKeepsRace(%d, %d) = %v, want %v", c.copy, c.base, got, c.keep)
		}
	}
}

// The heirlooms' figures and the balance's kits are made of the regular
// items (WearablePatterns): an Ascended record is none of them. The control:
// its base is.
func TestWearablePatternsLeaveTheAscendedRecordsOut(t *testing.T) {
	db := ascendedDB(t)
	sword, _, _ := ascendedCopy(t, db, 1026)
	inv := NewInventoryStore(&SQLiteDB{DB: db})
	items, err := inv.WearablePatterns(0, 0, 60, []int{14})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, it := range items {
		ids = append(ids, it.PatternID)
	}
	if !equalInts(ids, []int{1026, 714302}) {
		t.Errorf("wearable in the hands: %v, want Steelwind and the Heirloom, not Ascended record %d", ids, sword)
	}
}

// tools/ascend_boss_loot.py makes the bosses' Ascended records itself, in
// this file's SQL: its constants must be the server's.
func TestAscendBossLootToolKeepsTheRule(t *testing.T) {
	src, err := os.ReadFile("../../tools/ascend_boss_loot.py")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	str := func(name string) string {
		t.Helper()
		m := regexp.MustCompile(`(?m)^` + name + ` = "((?:[^"\\]|\\.)*)"`).FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("the tool has no %s", name)
		}
		return m[1]
	}
	num := func(name string) int {
		t.Helper()
		m := regexp.MustCompile(`(?m)^` + name + ` = (-?\d+)`).FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("the tool has no %s", name)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if got := str("PREFIX"); got != AscendedPrefix {
		t.Errorf("PREFIX %q, the server's %q", got, AscendedPrefix)
	}
	if got := num("PERCENT"); got != AscendedPercent {
		t.Errorf("PERCENT %d, the server's %d", got, AscendedPercent)
	}
	if got := num("MAX_NAME"); got != MaxItemNameChars {
		t.Errorf("MAX_NAME %d, the server's %d", got, MaxItemNameChars)
	}
	if got := num("THROWN_POTION"); got != ascendedThrownPotion {
		t.Errorf("THROWN_POTION %d, the server's %d", got, ascendedThrownPotion)
	}
	if got := num("SCROLL_ART"); got != ScrollArt {
		t.Errorf("SCROLL_ART %d, the server's %d", got, ScrollArt)
	}
	if m := regexp.MustCompile(`(?m)^FIRST_ID, LAST_ID = (\d+), (\d+)`).FindStringSubmatch(text); m == nil ||
		m[1] != strconv.Itoa(ascendedFirst) || m[2] != strconv.Itoa(ascendedLast) {
		t.Errorf("FIRST_ID, LAST_ID %v, the server's %d, %d", m, ascendedFirst, ascendedLast)
	}
	if m := regexp.MustCompile(`(?m)^GEAR_SLOTS = \((\d+), (\d+)\)`).FindStringSubmatch(text); m == nil ||
		m[1] != strconv.Itoa(ascendedGearFirstSlot) || m[2] != strconv.Itoa(ascendedGearLastSlot) {
		t.Errorf("GEAR_SLOTS %v, the server's %d-%d", m, ascendedGearFirstSlot, ascendedGearLastSlot)
	}
	m := regexp.MustCompile(`(?s)\nSCALED = \((.*?)\)\n`).FindStringSubmatch(text)
	if m == nil {
		t.Fatal("the tool has no SCALED")
	}
	var scaled, want []string
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(m[1], -1) {
		scaled = append(scaled, q[1])
	}
	for c := range importedScaled {
		want = append(want, c)
	}
	sort.Strings(scaled)
	sort.Strings(want)
	if strings.Join(scaled, " ") != strings.Join(want, " ") {
		t.Errorf("SCALED %v, the server's %v", scaled, want)
	}
	// the SQL, placeholders aside
	m = regexp.MustCompile(`(?s)\nFIGURE_SQL = \((.*?)\)\n`).FindStringSubmatch(text)
	if m == nil {
		t.Fatal("the tool has no FIGURE_SQL")
	}
	var figure strings.Builder
	for _, part := range regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(m[1], -1) {
		figure.WriteString(part[1])
	}
	goSQL := strings.NewReplacer("%[1]s", "{q}", "%[2]d", "{m}").Replace
	for _, c := range []struct{ name, tool, server string }{
		{"FIGURE_SQL", figure.String(), goSQL(ascendedFigureSQL)},
		{"NAME_SQL", str("NAME_SQL"), goSQL(ascendedNameSQL)},
		{"RENT_SQL", str("RENT_SQL"), goSQL(ascendedRentSQL)},
	} {
		if c.tool != c.server {
			t.Errorf("%s:\n%s\nthe server's:\n%s", c.name, c.tool, c.server)
		}
	}
	sm := regexp.MustCompile(`(?s)\nSCHEMA = """(.*?)"""`).FindStringSubmatch(text)
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if sm == nil || norm(sm[1]) != norm(AscendedSchema) {
		t.Errorf("the tool's SCHEMA is not the server's itemascended")
	}
	im := regexp.MustCompile(`(?s)\nIMPORTED_SCHEMA = """(.*?)"""`).FindStringSubmatch(text)
	if im == nil || norm(im[1]) != norm(ImportedSchema) {
		t.Errorf("the tool's IMPORTED_SCHEMA is not the server's itemimported")
	}
}

// A snapshot for a GM's machine refuses a table or column named like a
// secret it has no rule for (snapshot.go): the Ascended tables hold none and
// must not read as if they did.
func TestTheAscendedTablesReadAsNoSecret(t *testing.T) {
	for _, n := range []string{"itemascended", "patternid", "base", "made", "source",
		"ascended_loot_log", "seq", "run", "boss", "tbl", "ref", "before", "after", "audit"} {
		if SecretLikeName(n) {
			t.Errorf("%q reads like a secret", n)
		}
	}
}

// The owner, 2026-11-28: Skywalker set the icons and models of Ascended weapons, and every start put their bases'
// back. A look a GM gives the copy is kept through the start (model, icon, colour), while its figures still follow
// its base. The controls: a base's new look still reaches a copy that has none of its own (the first test's sword
// too), and a copy given its own keeps it even when its base's look changes later.
func TestTheSyncKeepsAnAscendedRecordsOwnLook(t *testing.T) {
	db := ascendedDB(t)
	sword, _, _ := ascendedCopy(t, db, 1026)
	helm, _, _ := ascendedCopy(t, db, 6207)
	sync := func(when string) {
		t.Helper()
		if _, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil {
			t.Fatalf("%s: %v", when, err)
		}
	}
	look := func(id int) (model, icon int) {
		t.Helper()
		if err := db.QueryRow(`SELECT model, itemicon FROM itempattern WHERE patternid = ?`, id).Scan(&model, &icon); err != nil {
			t.Fatal(err)
		}
		return
	}
	sync("the first start")
	if _, err := db.Exec(`UPDATE itempattern SET model = 1675423659, itemicon = 1473918600 WHERE patternid = ?`, sword); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE itempattern SET str = 30 WHERE patternid = 1026`); err != nil {
		t.Fatal(err)
	}
	sync("the start after the GM's look")
	if m, i := look(sword); m != 1675423659 || i != 1473918600 {
		t.Errorf("the sword's Ascended record's own look after the start: model %d icon %d, want the GM's kept", m, i)
	}
	var str int
	if err := db.QueryRow(`SELECT str FROM itempattern WHERE patternid = ?`, sword).Scan(&str); err != nil || str != 33 {
		t.Errorf("the sword's STR after its base's changed: %d (%v), want 33 (the figures still follow)", str, err)
	}
	// the base's new look: the helm (no look of its own) takes it, the sword keeps the GM's
	if _, err := db.Exec(`UPDATE itempattern SET model = 612804823, itemicon = 5 WHERE patternid IN (1026, 6207)`); err != nil {
		t.Fatal(err)
	}
	sync("the start after the bases' new look")
	if m, i := look(helm); m != 612804823 || i != 5 {
		t.Errorf("the helm's record after its base's new look: model %d icon %d, want the base's", m, i)
	}
	if m, i := look(sword); m != 1675423659 || i != 1473918600 {
		t.Errorf("the sword's record after its base's new look: model %d icon %d, want the GM's still", m, i)
	}
	if n, err := SyncAscendedCopies(db, time.Unix(1_800_000_000, 0)); err != nil || n != 0 {
		t.Errorf("a further start synced %d (%v), want none", n, err)
	}
}
