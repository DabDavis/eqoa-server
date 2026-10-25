package handlers

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// visualDB is a database with three spells: Ward Death (a harmful line, no
// visual), Minor Healing (a heal, a visual of its own) and Summon: Undead
// Mammoth (no effect row at all), and the GM's account.
func visualDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/fx.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, acctstatus, acctlevel) VALUES (1, 'gm', 1, 1), (2, 'player', 1, 0)`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (333, 13, 0, 0, 30, 3, 40, 0, 0, 1, 6, -1, 'Ward Death', 'Magical energy assaults undead creatures.'),
		        (334, 16, 0, 0, 30, 3, 50, 0, 0, 1, 6, -1, 'Smite Death', 'Magical energy assaults undead creatures.'),
		        (100, 1, 0, 0, 30, 2, 5, 0, 0, 1, 1, -1, 'Minor Healing', ''),
		        (6569, 1, 0, 0, 5, 12, 0, 0, 0, 0, 1, 255, 'Summon: Undead Mammoth', 'Summons an undead mammoth companion.')`,
		`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES
		 (333, 'Target undead, Decrease Hitpoints by 62 ; Increase 30% based on WIS', 0, 'test'),
		 (334, 'Target undead, Decrease Hitpoints by 76 ; Increase 30% based on WIS', 0, 'test'),
		 (100, 'Target, Increase Hitpoints by 30', 3266773242, 'test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return db
}

// cast is one 0x00B1 as it went out.
type cast struct{ caster, target, ms, fx uint32 }

// castsTo is every 0x00B1 queued for s.
func castsTo(s *session.Session) []cast {
	var out []cast
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 19 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCastingCombat {
			le := binary.LittleEndian
			out = append(out, cast{le.Uint32(d[2:]), le.Uint32(d[6:]), le.Uint32(d[11:]), le.Uint32(d[15:])})
		}
	}
	return out
}

// fxWorld is gmWorld with the spells, an NPC both see as 100107, and the
// player seeing the GM as 100108.
func fxWorld(t *testing.T) (*chatWorld, *database.SQLiteDB, *session.Session, *entity.Player, *session.Session) {
	c, gs, gp, ps, _ := gmWorld(t)
	gs.SetPlayerObjectID(500000001)
	ps.SetPlayerObjectID(500000002)
	db := visualDB(t)
	c.h.SetSpellVisuals(database.NewSpellbookStore(db))
	n := &entity.NPC{ServerID: 691, Name: "a decaying skeleton", Level: 3}
	n.X, n.Y, n.Z = 25005, 50, 15005
	c.gw.NPCs[n.ServerID] = n
	for _, s := range []*session.Session{gs, ps} {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ServerID, 100107
	}
	vs := entity.GetOrCreateVisibility(ps)
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = gp.ServerID, 100108
	return c, db, gs, gp, ps
}

func auditCount(t *testing.T, db *database.SQLiteDB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit`).Scan(&n); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0
		}
		t.Fatal(err)
	}
	return n
}

func fxdictOf(t *testing.T, db *database.SQLiteDB, id int) (uint32, bool) {
	t.Helper()
	var v int64
	if err := db.QueryRow(`SELECT fxdict FROM spelleffect WHERE spellid = ?`, id).Scan(&v); err != nil {
		return 0, false
	}
	return uint32(v), true
}

// #fx plays a visual from the GM onto the GM's target, to the GM and to
// whoever sees the GM, with its cast time; a spell's own, or the stand-in of
// its kind; the GM itself with no target. A player's is an unknown command
// and nothing is played or changed (the control).
func TestFXPlaysAVisualOnTheTarget(t *testing.T) {
	c, db, gs, gp, ps := fxWorld(t)
	gp.TargetObjectID = 100107

	c.say(ps, "/fx 0x1DAEB5B6")
	c.say(ps, "#spellfx 333 0x12AB34CD")
	if got := heard(t, ps); len(got) != 2 || got[0].text != `Unrecognized command "/fx".` || got[1].text != `Unrecognized command "#spellfx".` {
		t.Fatalf("a player's #fx and #spellfx: %+v", got)
	}
	if len(castsTo(ps)) != 0 || len(castsTo(gs)) != 0 {
		t.Fatal("a player's #fx played a visual")
	}
	if fx, _ := fxdictOf(t, db, 333); fx != 0 || auditCount(t, db) != 0 {
		t.Fatalf("a player's #spellfx changed Ward Death to %#x, %d audit lines", fx, auditCount(t, db))
	}
	ps.Queue().GetPending()

	c.say(gs, "/fx 0x1DAEB5B6 3")
	if got := castsTo(gs); len(got) != 1 || got[0] != (cast{gs.PlayerObjectID(), 100107, 3000, 0x1DAEB5B6}) {
		t.Fatalf("the GM's /fx, to the GM: %+v", got)
	}
	if got := castsTo(ps); len(got) != 1 || got[0] != (cast{100108, 100107, 3000, 0x1DAEB5B6}) {
		t.Fatalf("the GM's /fx, to the player seeing it: %+v", got)
	}
	if got := heard(t, gs); len(got) == 0 || !strings.Contains(got[len(got)-1].text, "0x1DAEB5B6 on a decaying skeleton, a 3.0 s cast") {
		t.Fatalf("the GM's /fx reply: %+v", got)
	}
	gs.Queue().GetPending()
	ps.Queue().GetPending()

	// a spell with no visual plays its kind's stand-in, with its cast time
	c.say(gs, "#fx ward death")
	if got := castsTo(gs); len(got) != 1 || got[0].fx != 0xE78024D9 || got[0].ms != 3000 || got[0].target != 100107 {
		t.Fatalf("#fx ward death: %+v", got)
	}
	gs.Queue().GetPending()
	// one with its own plays that
	c.say(gs, "#fx 100")
	if got := castsTo(gs); len(got) != 1 || got[0].fx != 3266773242 {
		t.Fatalf("#fx 100: %+v", got)
	}
	gs.Queue().GetPending()

	// no target: on the GM itself
	gp.TargetObjectID = 0xFFFFFFFF
	c.say(gs, "/fx 1DAEB5B6")
	if got := castsTo(gs); len(got) != 1 || got[0].target != gs.PlayerObjectID() {
		t.Fatalf("/fx with no target: %+v", got)
	}
	gs.Queue().GetPending()

	// a word that only looks like hex is a spell's name, not a visual
	c.say(gs, "/fx Fade")
	if got := castsTo(gs); len(got) != 0 {
		t.Fatalf("/fx Fade played %+v", got)
	}
	if got := heard(t, gs); len(got) != 1 || got[0].text != `[GM] No spell "Fade".` {
		t.Fatalf("/fx Fade: %+v", got)
	}
}

// #spellfx sets a spell's visual from its next cast, logged in gm_audit as
// the panel logs a row edit; the same again changes and logs nothing; 0 puts
// it back on its stand-in; a spell with no effect row gets one; a name
// several spells match sets none.
func TestSpellFXSetsAVisualAndLogsIt(t *testing.T) {
	c, db, gs, _, _ := fxWorld(t)

	c.say(gs, "/spellfx Ward Death")
	if got := heard(t, gs); len(got) != 1 || !strings.Contains(got[0].text, "Ward Death (333): no visual of its own; it plays the damage stand-in 0xE78024D9") {
		t.Fatalf("/spellfx Ward Death: %+v", got)
	}
	gs.Queue().GetPending()

	c.say(gs, "/spellfx Ward Death 0x12AB34CD")
	if fx, _ := fxdictOf(t, db, 333); fx != 0x12AB34CD {
		t.Fatalf("Ward Death's visual is %#x", fx)
	}
	var action, tbl, key, via, user, before, after string
	if err := db.QueryRow(`SELECT action, tbl, rowkey, via, username, before, after FROM gm_audit`).Scan(&action, &tbl, &key, &via, &user, &before, &after); err != nil {
		t.Fatal(err)
	}
	if action != "update" || tbl != "spelleffect" || key != `{"spellid":333}` || via != "game" || user != "gm" {
		t.Fatalf("the audit line: %s %s %s %s %s", action, tbl, key, via, user)
	}
	var b, a map[string]any
	json.Unmarshal([]byte(before), &b)
	json.Unmarshal([]byte(after), &a)
	if b["fxdict"] != float64(0) || a["fxdict"] != float64(0x12AB34CD) || a["line"] != b["line"] || a["source"] != "test" {
		t.Fatalf("the audit's rows: before %v, after %v", b, a)
	}
	if got := heard(t, gs); len(got) != 1 || !strings.Contains(got[0].text, "visual 0x12AB34CD from its next cast (it was the damage stand-in)") {
		t.Fatalf("the reply: %+v", got)
	}
	gs.Queue().GetPending()

	// from its next cast: #fx plays it now
	c.say(gs, "#fx 333")
	if got := castsTo(gs); len(got) != 1 || got[0].fx != 0x12AB34CD {
		t.Fatalf("#fx 333 after: %+v", got)
	}
	gs.Queue().GetPending()

	// the same again: nothing changed, nothing logged
	c.say(gs, "/spellfx 333 12AB34CD")
	if n := auditCount(t, db); n != 1 {
		t.Fatalf("setting the same visual logged: %d lines", n)
	}
	gs.Queue().GetPending()

	// a spell with no effect row gets one
	c.say(gs, "/spellfx undead mammoth 0xABCD")
	if fx, ok := fxdictOf(t, db, 6569); !ok || fx != 0xABCD {
		t.Fatalf("the mammoth's visual: %#x %v", fx, ok)
	}
	var ins string
	db.QueryRow(`SELECT action FROM gm_audit ORDER BY id DESC LIMIT 1`).Scan(&ins)
	if ins != "insert" {
		t.Fatalf("a new row's audit action: %q", ins)
	}
	gs.Queue().GetPending()

	// 0: the stand-in again
	c.say(gs, "/spellfx 333 0")
	if fx, _ := fxdictOf(t, db, 333); fx != 0 {
		t.Fatalf("/spellfx 333 0 left %#x", fx)
	}
	if got := heard(t, gs); len(got) != 1 || !strings.Contains(got[0].text, "back on the damage stand-in 0xE78024D9") {
		t.Fatalf("/spellfx 333 0: %+v", got)
	}
	gs.Queue().GetPending()

	// several spells: listed by id, none set
	c.say(gs, "/spellfx death 0x1")
	if got := heard(t, gs); len(got) != 3 || !strings.Contains(got[1].text, "333 Ward Death") || !strings.Contains(got[2].text, "334 Smite Death") {
		t.Fatalf("an ambiguous /spellfx: %+v", got)
	}
	if fx, _ := fxdictOf(t, db, 334); fx != 0 {
		t.Fatalf("an ambiguous /spellfx set Smite Death to %#x", fx)
	}
	// an unknown spell id: nothing
	n := auditCount(t, db)
	c.say(gs, "/spellfx 9999 0x1")
	if auditCount(t, db) != n {
		t.Fatal("an unknown spell was logged")
	}
}

// A visual is 0x and hex, or exactly eight hex digits; a word that could be
// hex is not one.
func TestParseVisual(t *testing.T) {
	for s, want := range map[string]uint32{"0x1DAEB5B6": 0x1DAEB5B6, "0X1": 1, "1daeb5b6": 0x1DAEB5B6, "0xABCD": 0xABCD} {
		if v, ok := ParseVisual(s); !ok || v != want {
			t.Errorf("%q: %#x %v", s, v, ok)
		}
	}
	for _, s := range []string{"Fade", "ABCD", "333", "0x", "0x123456789", "1DAEB5B6A", "0xGG", ""} {
		if v, ok := ParseVisual(s); ok {
			t.Errorf("%q read as %#x", s, v)
		}
	}
}

// FindSpells: an id, a name several ranks share, the names a word begins.
func TestFindSpells(t *testing.T) {
	all := []database.SpellName{{SpellID: 955, Name: "Howling Death"}, {SpellID: 956, Name: "Howling Death"}, {SpellID: 333, Name: "Ward Death"}, {SpellID: 6569, Name: "Summon: Undead Mammoth"}}
	ids := func(s []database.SpellName) []int {
		var out []int
		for _, x := range s {
			out = append(out, x.SpellID)
		}
		return out
	}
	if got := ids(FindSpells(all, "333")); len(got) != 1 || got[0] != 333 {
		t.Errorf("333: %v", got)
	}
	if got := ids(FindSpells(all, "howling death")); len(got) != 2 {
		t.Errorf("howling death: %v", got)
	}
	if got := ids(FindSpells(all, "death")); len(got) != 3 {
		t.Errorf("death (a later word): %v", got)
	}
	if got := ids(FindSpells(all, "undead")); len(got) != 1 || got[0] != 6569 {
		t.Errorf("undead: %v", got)
	}
	if got := FindSpells(all, "12"); got != nil {
		t.Errorf("an unknown id: %v", got)
	}
}
