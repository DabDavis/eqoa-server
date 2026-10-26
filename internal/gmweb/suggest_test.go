package gmweb

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// suggestFrom keeps a suggestion as the public form does, from the sender
// whose hash is given.
func suggestFrom(t *testing.T, f *fixture, spell int, fx uint32, hash, name, note string) {
	t.Helper()
	if _, err := f.db.AddSpellFXSuggestion(database.SpellFXSuggestion{SpellID: spell, FX: fx, Name: name, Note: note, AddrHash: hash}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func statusCount(t *testing.T, f *fixture, spell int, fx uint32, status string) int {
	t.Helper()
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM spellfx_suggestions WHERE spellid = ? AND fxdict = ? AND status = ?`, spell, int64(fx), status).Scan(&n)
	return n
}

func TestCommunityVisualsAreComparedAcceptedAndRevertible(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	const a, b = 0x1DAEB5B6, 0x2BB0C001
	suggestFrom(t, f, 100, a, "aaaaaaaa11111111", "Bob", "the green glow")
	suggestFrom(t, f, 100, a, "bbbbbbbb22222222", "", "")
	suggestFrom(t, f, 100, a, "cccccccc33333333", "<script>x</script>", "kept as typed")
	suggestFrom(t, f, 100, b, "dddddddd44444444", "Eve", "")
	suggestFrom(t, f, 101, b, "aaaaaaaa11111111", "Bob", "")

	code, out := c.op("visual_suggestions", nil)
	if code != 200 {
		t.Fatalf("visual_suggestions: %d %v", code, out)
	}
	spells := out["spells"].([]any)
	if len(spells) != 2 || out["waiting"].(float64) != 5 {
		t.Fatalf("listed %d spells, %v waiting: %v", len(spells), out["waiting"], out)
	}
	var heal map[string]any
	for _, s := range spells {
		if m := s.(map[string]any); m["spellid"].(float64) == 100 {
			heal = m
		}
	}
	if heal == nil || heal["total"].(float64) != 4 || heal["current"].(map[string]any)["standIn"] == nil {
		t.Fatalf("Minor Healing: %v", heal)
	}
	vs := heal["visuals"].([]any)
	first := vs[0].(map[string]any)
	if len(vs) != 2 || first["visual"] != "0x1DAEB5B6" || first["count"].(float64) != 3 || first["share"].(float64) != 75 {
		t.Fatalf("Minor Healing's visuals, most first: %v", vs)
	}
	if names := fmt.Sprint(first["names"]); !strings.Contains(names, "Bob") || !strings.Contains(names, "<script>x</script>") {
		t.Errorf("the names, as typed: %v", names)
	}
	if senders := fmt.Sprint(first["senders"]); !strings.Contains(senders, "aaaaaaaa") || strings.Contains(senders, "11111111") {
		t.Errorf("the senders as their hashes' first 8 characters: %v", senders)
	}

	// filters: a class none of them have, then one they do
	if code, out = c.op("visual_suggestions", map[string]any{"class": 5}); code != 200 || len(out["spells"].([]any)) != 0 {
		t.Fatalf("class 5: %d %v", code, out)
	}
	if code, out = c.op("visual_suggestions", map[string]any{"class": 0, "spell": "Minor Healing"}); code != 200 || len(out["spells"].([]any)) != 1 {
		t.Fatalf("class 0, Minor Healing: %d %v", code, out)
	}

	// a read token compares, never accepts
	_, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "ro", "scope": "read", "days": 1})
	ro := f.bearer(tok["token"].(string))
	if code, _ := ro.op("visual_suggestions", nil); code != 200 {
		t.Fatalf("a read token's visual_suggestions: %d", code)
	}
	if code, _ := ro.op("accept_visual_suggestion", map[string]any{"spell": 100, "visual": "0x1DAEB5B6"}); code == 200 {
		t.Fatalf("a read token accepted a suggestion")
	}

	// nobody suggested this one: accept refuses (set_spell_visual is the way)
	if code, _ := c.op("accept_visual_suggestion", map[string]any{"spell": 100, "visual": "0x00ABCDEF"}); code != 400 {
		t.Fatalf("accepting a visual nobody suggested: %d", code)
	}
	code, out = c.op("accept_visual_suggestion", map[string]any{"spell": "Minor Healing", "visual": "0x1DAEB5B6"})
	if code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	ch := out["change"].(map[string]any)
	auditID := ch["audit"]
	if auditID == nil || out["visual"].(map[string]any)["own"] != "0x1DAEB5B6" || out["decided"].(float64) != 4 {
		t.Fatalf("accept's answer: %v", out)
	}
	if statusCount(t, f, 100, a, database.SpellFXAccepted) != 3 || statusCount(t, f, 100, b, database.SpellFXDismissed) != 1 {
		t.Errorf("after accepting: the chosen visual's suggestions accepted, the other's dismissed")
	}
	if statusCount(t, f, 101, b, database.SpellFXNew) != 1 {
		t.Errorf("another spell's suggestion was decided too")
	}
	// the spell now has its own visual: off the stand-in-only list
	if code, out = c.op("visual_suggestions", map[string]any{"status": "all", "standin_only": true}); code != 200 || strings.Contains(fmt.Sprint(out), "Minor Healing") {
		t.Errorf("Minor Healing still on a stand-in after the accept: %v", out)
	}
	// the game's change is a row edit: revert_change undoes it
	if code, out = c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if code, out = c.op("spell_visual", map[string]any{"spell": 100}); code != 200 || out["own"] != nil || out["standIn"] == nil {
		t.Errorf("after the revert, Minor Healing plays: %v", out)
	}

	// dismiss, once
	if code, out = c.op("dismiss_visual_suggestion", map[string]any{"spell": 101, "visual": "0x2BB0C001"}); code != 200 || out["dismissed"].(float64) != 1 {
		t.Fatalf("dismiss: %d %v", code, out)
	}
	if code, _ = c.op("dismiss_visual_suggestion", map[string]any{"spell": 101, "visual": "0x2BB0C001"}); code != 400 {
		t.Errorf("dismissing what is no longer waiting: %d", code)
	}

	// a spammer's rows, by the hash's first 8 characters; never fewer
	if code, _ = c.op("clear_visual_suggestions", map[string]any{"sender": "aaaa"}); code != 400 {
		t.Errorf("a 4-character sender: %d", code)
	}
	if code, _ = c.op("clear_visual_suggestions", map[string]any{"sender": "aaaaaaa%"}); code != 400 {
		t.Errorf("a wildcard sender: %d", code)
	}
	if code, out = c.op("clear_visual_suggestions", map[string]any{"sender": "AAAAAAAA"}); code != 200 || out["deleted"].(float64) != 2 {
		t.Fatalf("clear: %d %v", code, out)
	}
	var left int
	f.db.QueryRow(`SELECT COUNT(*) FROM spellfx_suggestions WHERE addr_hash LIKE 'aaaaaaaa%'`).Scan(&left)
	if left != 0 {
		t.Errorf("%d of the sender's rows left", left)
	}
}

// The salt stays out of every page: the table is hidden from rows.
func TestTheSendersSaltIsNotATable(t *testing.T) {
	f := newFixture(t)
	if _, err := f.db.SpellFXSalt(); err != nil {
		t.Fatal(err)
	}
	c := f.browser()
	c.login("boss")
	if code, out := c.op("query_rows", map[string]any{"table": "spellfx_meta"}); code == 200 {
		t.Fatalf("spellfx_meta read through rows: %v", out)
	}
	if code, out := c.op("list_tables", nil); code != 200 || strings.Contains(fmt.Sprint(out), "spellfx_meta") {
		t.Fatalf("list_tables names spellfx_meta: %d", code)
	}
}

// Names and notes are what a player typed: the page puts them in as text,
// never as markup.
func TestCommunityVisualsShowsTextAsText(t *testing.T) {
	src, err := fs.ReadFile(staticFS, "static/views/community.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(string(src), bad) {
			t.Errorf("community.js uses %s", bad)
		}
	}
}

// The same spell recorded for another class (Return Home's three records) takes
// the accepted visual too while it is on a stand-in; a namesake with a visual of
// its own keeps it (the control), and so does a spell of another name.
func TestAcceptGivesTheVisualToNamesakesOnAStandIn(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	mustExec(t, f.db, `INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon,
		scope, recast, eqprequire, spellname, spelldesc) VALUES
		(9100, 1, 0, 0, 30, 2000, 5, 0, 0, 0, 6000, 0, 'Minor Healing', ''),
		(9101, 1, 0, 0, 30, 2000, 5, 0, 0, 0, 6000, 0, 'minor healing ', ''),
		(9102, 1, 0, 0, 30, 2000, 5, 0, 0, 0, 6000, 0, 'Minor Healing', '')`)
	mustExec(t, f.db, `INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (9101, '', 0, 'eqoa-data'), (9102, '', 0x3C001234, 'gm')`)
	const a = 0x1DAEB5B6
	suggestFrom(t, f, 100, a, "aaaaaaaa11111111", "Bob", "")
	code, out := c.op("accept_visual_suggestion", map[string]any{"spell": 100, "visual": "0x1DAEB5B6"})
	if code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	if got := len(out["namesakes"].([]any)); got != 2 {
		t.Fatalf("namesakes given the visual: %d (%v)", got, out["namesakes"])
	}
	fxOf := func(id int) int64 {
		var fx int64
		f.db.QueryRow(`SELECT COALESCE(fxdict, 0) FROM spelleffect WHERE spellid = ?`, id).Scan(&fx)
		return fx
	}
	if fxOf(9100) != a || fxOf(9101) != a {
		t.Errorf("the namesakes on a stand-in: %#x %#x", fxOf(9100), fxOf(9101))
	}
	if fxOf(9102) != 0x3C001234 {
		t.Errorf("a namesake with a visual of its own was changed: %#x", fxOf(9102))
	}
	if fxOf(101) != 0 {
		t.Errorf("a spell of another name was changed: %#x", fxOf(101))
	}
}
