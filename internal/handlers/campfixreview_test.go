package handlers

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// cfFlatGround is terrain at 49.5 everywhere: Floor the highest floor no more
// than the ground's step (2) above the height asked, as internal/ground's.
type cfFlatGround struct{}

func (cfFlatGround) Floor(_ uint8, _, y, _ float32) (float32, bool) {
	if 49.5 <= y+2 {
		return 49.5, true
	}
	return y, false
}

// cfReview is the report's review as the panel works it out now.
func (w *cfWorld) cfReview(r database.CampfixReport) CampfixReview {
	w.t.Helper()
	rv, err := NewCampfixReviewer(w.db, w.gw).Review(r)
	if err != nil {
		w.t.Fatal(err)
	}
	return rv
}

func hasReason(list []string, part string) bool {
	for _, l := range list {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// A player's report under review: each mark with what applying it does now
// and the review rules' colour. A move of 30 of a common creature in its camp
// is green, its height on the ground's floor; an add of a creature of one
// spawn is red (rare) and unchecked; a removal of the last of a name is red; a
// turn is green. The plans name the rows as they are and write the mark's
// facing byte.
func TestCampfixReviewColoursAndPlans(t *testing.T) {
	w := campfixWorld(t)
	w.gw.SetGround(cfFlatGround{})
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	w.face(w.pp, 0x40)
	w.ok(w.ps, "/mark bandit") // an open move: bandit 401, 30 east
	w.at(w.pp, 25020, 15030)
	w.ok(w.ps, "/markadd bandit captain")
	w.target(w.ps, w.pp, cfLord)
	w.at(w.pp, 25030, 15020)
	w.ok(w.ps, "/markremove")
	w.at(w.pp, 25000, 15060) // standing on bandit 402: a turn
	w.face(w.pp, 0x10)
	w.ok(w.ps, "/mark bandit")
	r := w.sent(w.ok(w.ps, "/campfix"))

	rv := w.cfReview(r)
	if len(rv.Marks) != 4 || rv.Petition != 0 || len(rv.Notes) != 0 {
		t.Fatalf("the review: %d marks, petition %d, notes %q", len(rv.Marks), rv.Petition, rv.Notes)
	}
	move, add, remove, turn := rv.Marks[0], rv.Marks[1], rv.Marks[2], rv.Marks[3]
	if move.Kind != "move" || move.Row != cfBanditE || move.Stale != "" || move.Colour != CampfixGreen || !move.Checked ||
		!hasReason(move.Reasons, "under 150, of a common creature within its camp") {
		t.Fatalf("the move: %+v", move)
	}
	if p := move.Plan; p.Action != CampfixUpdate || p.Row != cfBanditE || p.X != 25010 || p.Y != 49.5 || p.Z != 15000 ||
		p.Facing != move.FacingByte || p.Was == nil || p.Was.X != 25040 || p.Was.Facing != 64 || !strings.Contains(p.Ground, "0.50 below the mark") {
		t.Fatalf("the move's plan: %+v (was %+v)", p, p.Was)
	}
	if add.Kind != "add" || add.Colour != CampfixRed || add.Checked || !hasReason(add.Reasons, "an add of a rare or valuable creature: 1 spawn in Tunaria") {
		t.Fatalf("the add: %+v", add)
	}
	if p := add.Plan; p.Action != CampfixInsert || p.Template != cfCaptain || p.Y != 49.5 || p.Facing != add.FacingByte {
		t.Fatalf("the add's plan: %+v", p)
	}
	if remove.Row != cfLord || remove.Colour != CampfixRed || remove.Checked ||
		!hasReason(remove.Red, "it removes the last spawn of Lord Brutus in Tunaria") || !hasReason(remove.Yellow, "it leaves no Lord Brutus in the camp") {
		t.Fatalf("the removal: %+v", remove)
	}
	if p := remove.Plan; p.Action != CampfixDelete || p.Row != cfLord || p.Camp != 0 || p.Path {
		t.Fatalf("the removal's plan: %+v", p)
	}
	if turn.Kind != "turn" || turn.Row != cfBanditN || turn.Colour != CampfixGreen || !turn.Checked {
		t.Fatalf("the turn: %+v", turn)
	}
	if p := turn.Plan; p.Action != CampfixUpdate || p.Facing != turn.FacingByte || p.X != 25000 || p.Z != 15060 || p.Was.Facing != 192 {
		t.Fatalf("the turn's plan: %+v", p)
	}
	if rv.Counts[CampfixGreen] != 2 || rv.Counts[CampfixRed] != 2 {
		t.Fatalf("counts: %v", rv.Counts)
	}

	// no ground: a move keeps the mark's height, and says so in yellow
	w.gw.SetGround(nil)
	if m := w.cfReview(r).Marks[0]; m.Plan.Y != 50 || m.Colour != CampfixYellow || !hasReason(m.Reasons, "no floor within 2.5") {
		t.Fatalf("no ground: %+v", m)
	}
}

// A mark whose row moved, turned, was renamed, deleted or changed by anyone
// since the report was sent is stale, and says so; a change before it was sent
// is not (the control).
func TestCampfixReviewFindsStaleRows(t *testing.T) {
	w := campfixWorld(t)
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	w.ok(w.ps, "/mark bandit") // 401
	w.at(w.pp, 25000, 15060)
	w.ok(w.ps, "/mark bandit") // a turn of 402
	w.target(w.ps, w.pp, cfLord)
	w.at(w.pp, 25030, 15020)
	w.ok(w.ps, "/markremove") // 406
	w.target(w.ps, w.pp, cfCaptain)
	w.at(w.pp, 25080, 15080)
	w.ok(w.ps, "/mark") // the captain, 403
	r := w.sent(w.ok(w.ps, "/campfix"))
	for _, m := range w.cfReview(r).Marks {
		if m.Stale != "" {
			t.Fatalf("mark %d stale before anything changed: %s", m.N, m.Stale)
		}
	}
	audit := func(row int, at string) {
		w.t.Helper()
		if _, err := w.db.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
			VALUES (?, 1, 'jib', 'web', '', 'update', 'npcs', ?, '{}', '{}', '')`, at, `{"id": `+strconv.Itoa(row)+`}`); err != nil {
			w.t.Fatal(err)
		}
	}
	audit(cfBanditN, "2026-11-10T11:59:00Z") // before the report: the control
	if m := w.cfReview(r).Marks[1]; m.Stale != "" {
		t.Fatalf("a change before the report made it stale: %s", m.Stale)
	}
	if _, err := w.db.Exec(`UPDATE npcs SET x_coord = 25041 WHERE id = ?`, cfBanditE); err != nil {
		t.Fatal(err)
	}
	audit(cfBanditN, "2026-11-10 12:30:00") // a tool's time and key with a space
	if _, err := w.db.Exec(`DELETE FROM npcs WHERE id = ?`, cfLord); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.Exec(`UPDATE npcs SET facing = 9 WHERE id = ?`, cfCaptain); err != nil {
		t.Fatal(err)
	}
	rv := w.cfReview(r)
	for i, want := range []string{"row 401 moved since the report was sent", "row 402 was changed since the report was sent: update by jib",
		"row 406 is gone: deleted since the report was sent", "row 403 turned since the report was sent: it faces 9, the report found 0"} {
		if m := rv.Marks[i]; !strings.Contains(m.Stale, want) || m.Checked || m.Colour != "" || m.Plan.Action != "" {
			t.Errorf("mark %d: %+v, want stale %q", i+1, m, want)
		}
	}
	if rv.Counts["stale"] != 4 {
		t.Fatalf("counts: %v", rv.Counts)
	}
}

// The review rules' other colours: a removal of a stacked duplicate is green;
// of a quest's giver red; more than five adds in one report each red, and adds
// raising a name's spawns in the camp by more than half red; every mark of an
// account with an earlier report declined as abuse red.
func TestCampfixReviewRules(t *testing.T) {
	w := campfixWorld(t)
	if _, err := w.db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (700, 'goblin raider', 0, 20001, 50, 20001, 0, ?, 1.0, 30, 0, '0', 7)`, cfCreature); err != nil {
		t.Fatal(err)
	}
	w.gw.LoadNPCs(database.NewNPCStore(w.db))

	// a stacked duplicate's removal, and six goblin adds
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 20001.5, 20001.5)
	w.ok(w.ps, "/markremove") // 700, standing on it: 500 is 1.4 off
	for i := 0; i < 6; i++ {
		w.at(w.pp, 20030+3*float32(i), 20090)
		w.ok(w.ps, "/markadd goblin raider")
	}
	r := w.sent(w.ok(w.ps, "/campfix"))
	rv := w.cfReview(r)
	if m := rv.Marks[0]; m.Row != 700 || m.Colour != CampfixGreen || !hasReason(m.Green, "a stacked duplicate: row 500") {
		t.Fatalf("the duplicate's removal: %+v", m)
	}
	for _, m := range rv.Marks[1:] {
		if m.Colour != CampfixRed || m.Checked || !hasReason(m.Red, "6 adds in one report, more than 5") {
			t.Fatalf("add %d: %+v", m.N, m)
		}
	}

	// two adds of a name the camp has two of; a removal of a quest's giver
	w.now = w.now.Add(11 * time.Minute)
	if _, err := w.db.Exec(`INSERT INTO quests (questid, name, giver) VALUES (100001, 'The Bandits', 'bandit scout')`); err != nil {
		t.Fatal(err)
	}
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	w.ok(w.ps, "/markadd bandit")
	w.at(w.pp, 25015, 15000)
	w.ok(w.ps, "/markadd bandit")
	w.target(w.ps, w.pp, cfScout)
	w.at(w.pp, 24950, 14950) // 71 from the scout
	w.ok(w.ps, "/markremove")
	r2 := w.sent(w.ok(w.ps, "/campfix"))
	rv = w.cfReview(r2)
	for _, m := range rv.Marks[:2] {
		if m.Colour != CampfixRed || !hasReason(m.Red, "2 adds to the camp's 2: more than half") {
			t.Fatalf("add %d: %+v", m.N, m)
		}
	}
	if m := rv.Marks[2]; m.Colour != CampfixRed || !hasReason(m.Red, "bandit scout is the giver of quest 100001 (The Bandits)") {
		t.Fatalf("the giver's removal: %+v", m)
	}

	// declined as abuse: the account's later reports are red whole
	if _, err := w.db.Exec(`UPDATE campfix_reports SET status = 'declined', review = ? WHERE id = ?`,
		CampfixDeclinedAbuse+" by gm on 2026-11-10 12:20 UTC: no mark applied.", r.ID); err != nil {
		t.Fatal(err)
	}
	rv = w.cfReview(r2)
	if !hasReason(rv.Notes, "had an earlier report declined as abuse") {
		t.Fatalf("notes: %q", rv.Notes)
	}
	for _, m := range rv.Marks {
		if m.Colour != CampfixRed || !hasReason(m.Red, "declined as abuse (report ") {
			t.Fatalf("mark %d after the abuse: %+v", m.N, m)
		}
	}
	// the control: another account's report is not
	if got := w.cfReview(database.CampfixReport{ID: r2.ID, AccountID: 99, World: 0, Status: "waiting", Marks: r2.Marks,
		Created: r2.Created}); hasReason(got.Notes, "abuse") {
		t.Fatalf("another account: %q", got.Notes)
	}
}

// A petition's spawn request filed as a camp report: the marks where the
// petition's character stood, facing its way, an add of three on rings none
// within 2 of another, the report the petition's player's, its note naming the
// petition, a waiting row and a board entry posted by its filer; a move takes
// the petition's target of the name before the nearest. Refused: a kind no
// report has, a count on a move, a name no creature has, a petition with no
// server view, a removal with none of the name at hand.
func TestCampfixFromAPetition(t *testing.T) {
	w := campfixWorld(t)
	view := func(target string) string {
		return `{"filed":"2026-11-09T10:00:00Z","via":"support","character":{"name":"Player","serverid":2,"level":12,"class":"Ranger",` +
			`"race":"Human","world":0,"x":25010,"y":50,"z":15000,"facing":1.5708}` + target + `}`
	}
	file := func(sv string) int64 {
		w.t.Helper()
		p, err := w.db.FilePetition(database.Petition{ServerID: 2, CharName: "Player", AccountID: 2, Category: 1, Subcategory: 10,
			Text: "bandit camp not populated", ServerView: sv}, w.now)
		if err != nil {
			w.t.Fatal(err)
		}
		return p.ID
	}
	id := file(view(""))
	plan, err := PlanCampfixPetition(w.db, CampfixPetition{Petition: id, Kind: "add", Name: "a bandit", Count: 3,
		Note: "bandit camp not populated"}, w.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Marks) != 3 {
		t.Fatalf("marks: %+v", plan.Marks)
	}
	for i, m := range plan.Marks {
		if m.Kind != "add" || m.Name != "bandit" || m.Typed != "a bandit" || m.World != 0 || m.Y != 50 || m.Facing != 1.5708 ||
			m.FacingByte != facingByte(1.5708) || m.Template == 0 || m.At != "2026-11-09T10:00:00Z" {
			t.Fatalf("mark %d: %+v", i+1, m)
		}
		for j, o := range plan.Marks[:i] {
			if d := math.Hypot(m.X-o.X, m.Z-o.Z); d <= float64(campfixApart) {
				t.Fatalf("marks %d and %d are %.2f apart", j+1, i+1, d)
			}
		}
	}
	if m := plan.Marks[0]; m.X != 25010 || m.Z != 15000 {
		t.Fatalf("the first add is not where the character stood: %+v", m)
	}
	gm := database.GMEdit{AccountID: 1, Username: "gm", Via: "web"}
	r, entry, err := plan.File(w.db, gm, w.now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != database.CampfixWaiting || r.AccountID != 2 || r.CharName != "Player" || r.Level != 12 || r.Class != 1 ||
		r.Note != "From petition "+strconv.FormatInt(id, 10)+": bandit camp not populated" || r.X != 25010 || r.Z != 15000 || r.Board != entry.ID {
		t.Fatalf("the report: %+v", r)
	}
	if entry.CreatedBy != "gm" || entry.Owner != CampfixBoardOwner || !strings.Contains(entry.Details, "**From:** petition "+strconv.FormatInt(id, 10)+
		", filed 2026-11-09 10:00:00 UTC; made a camp report by gm.") || !strings.HasSuffix(entry.Title, ": 3 add") {
		t.Fatalf("the entry: %+v", entry)
	}
	if rv := w.cfReview(r); rv.Petition != id || len(rv.Marks) != 3 || rv.Marks[0].Plan.Action != CampfixInsert {
		t.Fatalf("its review: %+v", rv)
	}
	if n := w.near(r); len(n) == 0 || n[0].Alive != nil || n[0].Row == 0 {
		t.Fatalf("near: %+v", n)
	}

	// a move takes the petition's target of the name, 402, before the nearest, 401
	id = file(view(`,"target":{"kind":"npc","name":"bandit","npcid":402}`))
	plan, err = PlanCampfixPetition(w.db, CampfixPetition{Petition: id, Kind: "move", Name: "bandit"}, w.now)
	if err != nil || len(plan.Marks) != 1 || plan.Marks[0].Row != cfBanditN || plan.Marks[0].Kind != "move" {
		t.Fatalf("the move: %+v %v", plan.Marks, err)
	}
	plan, err = PlanCampfixPetition(w.db, CampfixPetition{Petition: id, Kind: "move", Name: "bandit captain"}, w.now)
	if err != nil || plan.Marks[0].Row != cfCaptain {
		t.Fatalf("the control, a target of another name: %+v %v", plan.Marks, err)
	}

	for _, tc := range []struct {
		req  CampfixPetition
		want string
	}{
		{CampfixPetition{Petition: id, Kind: "spawn", Name: "bandit"}, "kind is add, move, remove or turn"},
		{CampfixPetition{Petition: id, Kind: "move", Name: "bandit", Count: 2}, "a count is an add's"},
		{CampfixPetition{Petition: id, Kind: "add", Name: "zebra"}, "No creature named zebra here"},
		{CampfixPetition{Petition: id, Kind: "remove", Name: "bandit scout"}, "there is none to remove"},
		{CampfixPetition{Petition: file(""), Kind: "add", Name: "bandit"}, "has no server view"},
		{CampfixPetition{Petition: 9999, Kind: "add", Name: "bandit"}, "no petition 9999"},
	} {
		if _, err := PlanCampfixPetition(w.db, tc.req, w.now); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v, want %q", tc.req, err, tc.want)
		}
	}
	if _, err := PlanCampfixPetition(w.db, CampfixPetition{Petition: id, Kind: "remove", Name: "bandit"}, w.now); err != nil {
		t.Errorf("the control, the target's removal: %v", err)
	}
}

// A name of letters past ASCII is matched without case as /campfix matches
// one (SQLite's lower folds ASCII alone): a petition's add of "ÖGRE BRUTE"
// copies the row named "Ögre brute"; the control, a name no row has, is refused.
func TestCampfixPetitionMatchesANameOfNonASCIILetters(t *testing.T) {
	w := campfixWorld(t)
	if _, err := w.db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (710, 'Ögre brute', 0, 25020, 50, 15020, 0, ?, 1.0, 25, 0, '0', 7)`, cfCreature); err != nil {
		t.Fatal(err)
	}
	p, err := w.db.FilePetition(database.Petition{ServerID: 2, CharName: "Player", AccountID: 2, Category: 1, Subcategory: 10, Text: "x",
		ServerView: `{"filed":"2026-11-09T10:00:00Z","character":{"name":"Player","serverid":2,"level":12,"class":"Ranger","world":0,` +
			`"x":25010,"y":50,"z":15000,"facing":0}}`}, w.now)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanCampfixPetition(w.db, CampfixPetition{Petition: p.ID, Kind: "add", Name: "ÖGRE BRUTE"}, w.now)
	if err != nil || len(plan.Marks) != 1 || plan.Marks[0].Name != "Ögre brute" || plan.Marks[0].Template != 710 {
		t.Fatalf("the add: %+v %v", plan.Marks, err)
	}
	if _, err := PlanCampfixPetition(w.db, CampfixPetition{Petition: p.ID, Kind: "add", Name: "Ügre brute"}, w.now); err == nil {
		t.Fatal("the control, a name no row has, was taken")
	}
}
