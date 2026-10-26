package spellfx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

// fixture: three spells classes have (a heal on a stand-in in two classes, a
// nuke with its own visual, a buff with no line and a cast too long for the
// preview) and one no class has.
func fixture(t *testing.T) (*Service, *database.SQLiteDB, *time.Time) {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc) VALUES
		(100, 1, 0, 0, 30, 2, 5, 0, 0, 0, 6, 0, 'Minor Healing', ''),
		(101, 5, 0, 0, 30, 3, 5, 0, 0, 0, 6, 0, 'Fiery Burst', ''),
		(102, 8, 0, 0, 30, 2000, 5, 0, 0, 0, 6, 0, 'Gift of Speed', ''),
		(103, 9, 0, 0, 30, 2, 5, 0, 0, 0, 6, 0, 'A Monster Spell', '')`)
	exec(`INSERT INTO classspells (class, spellid, source) VALUES (9, 100, 't'), (7, 100, 't'), (13, 101, 't'), (0, 102, 't')`)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES
		(100, 'Target, Increase HP per tic by 20, 24sec', 0, 't'),
		(101, 'Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX', ?, 't')`, int64(0xE78024D9))
	s := New(db)
	now := time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	return s, db, &now
}

func list(t *testing.T, s *Service) map[int]Spell {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeSpells(w, httptest.NewRequest("GET", "/api/spellfx/spells", nil))
	if w.Code != 200 {
		t.Fatalf("spells: %d %s", w.Code, w.Body)
	}
	var out struct{ Spells []Spell }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	m := map[int]Spell{}
	for _, sp := range out.Spells {
		m[sp.ID] = sp
	}
	return m
}

func suggest(t *testing.T, s *Service, ip string, body any) (int, map[string]any) {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		raw, _ = json.Marshal(b)
	}
	w := httptest.NewRecorder()
	s.ServeSuggest(w, httptest.NewRequest("POST", "/api/spellfx/suggest", bytes.NewReader(raw)), ip)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestTheListIsEveryClassSpellWithWhatItPlays(t *testing.T) {
	s, _, _ := fixture(t)
	m := list(t, s)
	if len(m) != 3 {
		t.Fatalf("listed %d spells, want the three classes have: %v", len(m), m)
	}
	if _, ok := m[103]; ok {
		t.Errorf("a spell no class has is listed")
	}
	heal := m[100]
	if !heal.StandIn || heal.Kind != "heal" || heal.Visual != handlers.Hex32(entity.StandInVisuals()["heal"]) || heal.Harmful {
		t.Errorf("Minor Healing: %+v", heal)
	}
	if len(heal.Classes) != 2 || heal.Classes[0] != 7 || heal.Classes[1] != 9 {
		t.Errorf("Minor Healing's classes: %v, want [7 9] once", heal.Classes)
	}
	nuke := m[101]
	if nuke.StandIn || nuke.Visual != "0xE78024D9" || !nuke.Harmful || nuke.Target != "Target" || nuke.Cast != 3 || nuke.Level != 5 {
		t.Errorf("Fiery Burst: %+v", nuke)
	}
	if !strings.HasPrefix(nuke.Line, "Target, Decrease Hitpoints") {
		t.Errorf("Fiery Burst's line: %q", nuke.Line)
	}
	if m[102].Cast != entity.MaxVisualCast.Seconds() {
		t.Errorf("a 2000 s cast is listed %v s, want the preview's most %v", m[102].Cast, entity.MaxVisualCast.Seconds())
	}

	// gzip when the browser takes it, the same list
	r := httptest.NewRequest("GET", "/api/spellfx/spells", nil)
	r.Header.Set("Accept-Encoding", "gzip, deflate, br")
	w := httptest.NewRecorder()
	s.ServeSpells(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("no gzip: %v", w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	unz, _ := io.ReadAll(zr)
	plain := httptest.NewRecorder()
	s.ServeSpells(plain, httptest.NewRequest("GET", "/api/spellfx/spells", nil))
	if !bytes.Equal(unz, plain.Body.Bytes()) {
		t.Errorf("the gzipped list differs from the plain one")
	}
	// only GET
	w = httptest.NewRecorder()
	s.ServeSpells(w, httptest.NewRequest("POST", "/api/spellfx/spells", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to the list: %d", w.Code)
	}
}

// A GM's change (set_spell_visual, #spellfx: database.SetSpellVisual) shows
// at once; a change behind the server's back within the minute.
func TestAGMChangeShowsAtOnceOthersWithinAMinute(t *testing.T) {
	s, db, now := fixture(t)
	if !list(t, s)[100].StandIn {
		t.Fatal("Minor Healing should start on a stand-in")
	}
	if _, _, err := database.NewSpellbookStore(db).SetSpellVisual(100, 0x12AB34CD, database.GMEdit{AccountID: 0, Username: "gm", Via: "game"}, "test"); err != nil {
		t.Fatal(err)
	}
	if got := list(t, s)[100]; got.StandIn || got.Visual != "0x12AB34CD" {
		t.Errorf("after a GM's change: %+v", got)
	}
	// behind the server's back (an importer): the old list until the minute
	if _, err := db.Exec(`UPDATE spelleffect SET fxdict = ? WHERE spellid = 100`, int64(0x0000BEEF)); err != nil {
		t.Fatal(err)
	}
	if got := list(t, s)[100]; got.Visual != "0x12AB34CD" {
		t.Errorf("rebuilt before the minute: %+v", got)
	}
	*now = now.Add(61 * time.Second)
	if got := list(t, s)[100]; got.Visual != "0x0000BEEF" {
		t.Errorf("not rebuilt after the minute: %+v", got)
	}
}

func liveRows(t *testing.T, db *database.SQLiteDB, spell int) []database.SpellFXSuggestion {
	t.Helper()
	rows, err := db.SpellFXSuggestions(database.SpellFXNew, spell)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestASuggestionIsKeptReplacedPerAddressAndCounted(t *testing.T) {
	s, db, _ := fixture(t)
	code, out := suggest(t, s, "203.0.113.5", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": "Bob", "note": "the fire one"})
	if code != 200 || out["ok"] != true || out["count"] != 1.0 {
		t.Fatalf("first: %d %v", code, out)
	}
	// the same address changes its mind: replaced, not added
	code, out = suggest(t, s, "203.0.113.5", map[string]any{"spell": 101, "visual": "1DAEB5B7"})
	if code != 200 || out["count"] != 1.0 {
		t.Fatalf("replaced: %d %v", code, out)
	}
	rows := liveRows(t, db, 101)
	if len(rows) != 1 || rows[0].FX != 0x1DAEB5B7 || rows[0].Name != "" {
		t.Fatalf("after the same address again: %+v", rows)
	}
	// another address adds
	if code, out = suggest(t, s, "198.51.100.9", map[string]any{"spell": 101, "visual": "0x1DAEB5B6"}); code != 200 || out["count"] != 2.0 {
		t.Fatalf("another address: %d %v", code, out)
	}
	// the same address on another spell is its own
	if code, out = suggest(t, s, "203.0.113.5", map[string]any{"spell": 100, "visual": "0x1DAEB5B6"}); code != 200 || out["count"] != 1.0 {
		t.Fatalf("another spell: %d %v", code, out)
	}
	if n := len(liveRows(t, db, 101)); n != 2 {
		t.Errorf("Fiery Burst holds %d live suggestions, want 2", n)
	}
	// a decided one stays as history: a new one from the same address is added
	if _, err := db.DecideSpellFXSuggestions(101, 0x1DAEB5B6, database.SpellFXDismissed, "", "gm", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code, out = suggest(t, s, "198.51.100.9", map[string]any{"spell": 101, "visual": "0x1DAEB5B8"}); code != 200 || out["count"] != 3.0 {
		t.Fatalf("after a GM's dismissal: %d %v", code, out)
	}
}

func TestTheSenderIsKeptOnlyAsASaltedHash(t *testing.T) {
	s, db, _ := fixture(t)
	const ip = "203.0.113.77"
	if code, _ := suggest(t, s, ip, map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": "n", "note": "x"}); code != 200 {
		t.Fatal(code)
	}
	var rowText string
	db.QueryRow(`SELECT spellid || '|' || fxdict || '|' || name || '|' || note || '|' || addr_hash || '|' || created FROM spellfx_suggestions`).Scan(&rowText)
	if strings.Contains(rowText, ip) || strings.Contains(rowText, "203.0.113") {
		t.Fatalf("the address is kept: %s", rowText)
	}
	plain := sha256.Sum256([]byte(AddrKey(ip)))
	rows := liveRows(t, db, 101)
	if len(rows) != 1 || rows[0].AddrHash == "" || strings.HasPrefix(hex.EncodeToString(plain[:]), rows[0].AddrHash) {
		t.Fatalf("the hash is the address's unsalted one, or none: %+v", rows)
	}
	// another database salts it otherwise
	s2, db2, _ := fixture(t)
	suggest(t, s2, ip, map[string]any{"spell": 101, "visual": "0x1DAEB5B6"})
	if other := liveRows(t, db2, 101); len(other) != 1 || other[0].AddrHash == rows[0].AddrHash {
		t.Errorf("two databases hash an address alike: %+v %+v", other, rows)
	}
	// one salt for good
	a, _ := db.SpellFXSalt()
	b, _ := db.SpellFXSalt()
	if a == "" || a != b {
		t.Errorf("the salt changes: %q %q", a, b)
	}
}

// By default an address sends as many as it likes (one live suggestion a
// spell bounds what it holds).
func TestNoHourlyLimitByDefault(t *testing.T) {
	s, _, _ := fixture(t)
	if s.PerHour != 0 {
		t.Fatalf("the default hourly limit is %d, want none", s.PerHour)
	}
	for i := 0; i < 300; i++ {
		spell := 100 + i%2
		if code, out := suggest(t, s, "203.0.113.7", map[string]any{"spell": spell, "visual": fmt.Sprintf("0x%08X", i+1)}); code != 200 {
			t.Fatalf("send %d: %d %v", i, code, out)
		}
	}
}

func TestAnAddressSendsAtMostPerHour(t *testing.T) {
	s, _, now := fixture(t)
	s.PerHour = 3
	for i, v := range []string{"0x00000001", "0x00000002", "0x00000003"} {
		if code, out := suggest(t, s, "203.0.113.5", map[string]any{"spell": 101, "visual": v}); code != 200 {
			t.Fatalf("send %d: %d %v", i, code, out)
		}
	}
	if code, _ := suggest(t, s, "203.0.113.5", map[string]any{"spell": 100, "visual": "0x00000004"}); code != http.StatusTooManyRequests {
		t.Fatalf("the fourth in an hour: %d, want 429", code)
	}
	if code, _ := suggest(t, s, "198.51.100.1", map[string]any{"spell": 101, "visual": "0x00000004"}); code != 200 {
		t.Fatalf("another address: %d", code)
	}
	// an IPv6 /64 is one sender
	for _, ip := range []string{"2001:db8:1:2::1", "2001:db8:1:2::2", "2001:db8:1:2:ffff::9"} {
		if code, _ := suggest(t, s, ip, map[string]any{"spell": 101, "visual": "0x00000005"}); code != 200 {
			t.Fatalf("%s: %d", ip, code)
		}
	}
	if code, _ := suggest(t, s, "2001:db8:1:2::abcd", map[string]any{"spell": 101, "visual": "0x00000005"}); code != http.StatusTooManyRequests {
		t.Fatalf("a fourth address of the same /64: %d, want 429", code)
	}
	if code, _ := suggest(t, s, "2001:db8:1:3::1", map[string]any{"spell": 101, "visual": "0x00000005"}); code != 200 {
		t.Fatalf("the next /64: %d", code)
	}
	*now = now.Add(61 * time.Minute)
	if code, _ := suggest(t, s, "203.0.113.5", map[string]any{"spell": 100, "visual": "0x00000004"}); code != 200 {
		t.Fatalf("an hour later: %d", code)
	}
}

func TestTheFormRefusesWhatIsNotASuggestion(t *testing.T) {
	s, db, _ := fixture(t)
	long := strings.Repeat("a", MaxName+1)
	cases := []struct {
		why  string
		body any
	}{
		{"not JSON", "spell=101&visual=0x1"},
		{"no spell", map[string]any{"visual": "0x1DAEB5B6"}},
		{"a spell no class has", map[string]any{"spell": 103, "visual": "0x1DAEB5B6"}},
		{"no such spell", map[string]any{"spell": 9999, "visual": "0x1DAEB5B6"}},
		{"a word for a visual", map[string]any{"spell": 101, "visual": "Fade"}},
		{"not hex", map[string]any{"spell": 101, "visual": "0xZZ"}},
		{"none", map[string]any{"spell": 101, "visual": "0x0"}},
		{"too long a DictID", map[string]any{"spell": 101, "visual": "0x123456789"}},
		{"a long name", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": long}},
		{"a long note", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "note": strings.Repeat("b", MaxNote+1)}},
		{"a bell in the name", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": "a\abc"}},
		{"a newline in the name", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": "a\nb"}},
		{"an escape in the note", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "note": "a\x1b[2Jb"}},
		{"too big", `{"spell":101,"visual":"0x1DAEB5B6","note":"` + strings.Repeat("c", 3000) + `"}`},
	}
	for _, c := range cases {
		if code, out := suggest(t, s, "203.0.113.5", c.body); code != 400 || out["error"] == nil {
			t.Errorf("%s: %d %v, want 400 with an error", c.why, code, out)
		}
	}
	if n := len(liveRows(t, db, 0)); n != 0 {
		t.Errorf("%d refused suggestions were kept", n)
	}
	// a note may break lines; a name and a note are kept as given (the panel
	// shows them as text)
	if code, out := suggest(t, s, "203.0.113.5", map[string]any{"spell": 101, "visual": "0x1DAEB5B6", "name": "<b>Bob</b>", "note": "line one\nline two"}); code != 200 {
		t.Fatalf("a note of two lines: %d %v", code, out)
	}
	rows := liveRows(t, db, 101)
	if len(rows) != 1 || rows[0].Name != "<b>Bob</b>" || rows[0].Note != "line one\nline two" {
		t.Errorf("kept: %+v", rows)
	}
	w := httptest.NewRecorder()
	s.ServeSuggest(w, httptest.NewRequest("GET", "/api/spellfx/suggest", nil), "203.0.113.5")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET to the form: %d", w.Code)
	}
}

func TestAddrKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":            "203.0.113.5",
		"::ffff:203.0.113.5":     "203.0.113.5",
		"2001:db8:1:2:3:4:5:6":   "2001:db8:1:2::/64",
		" 2001:db8:1:2::9 ":      "2001:db8:1:2::/64",
		"not an address at all!": "not an address at all!",
	}
	for in, want := range cases {
		if got := AddrKey(in); got != want {
			t.Errorf("AddrKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// A spell no class has by level (a mastery, an epic, a master class's) is
// listed with how it is got, so the page can say so; one any class gets by
// level carries no "got", whatever another class's row says.
func TestTheListSaysHowASpellNoLevelGivesIsGot(t *testing.T) {
	s, db, now := fixture(t)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc) VALUES
			(110, 55, 0, 0, 30, 3, 50, 0, 0, 0, 6, 0, 'Group Prodigiousness', ''),
			(111, 60, 0, 0, 30, 3, 50, 0, 0, 0, 6, 0, 'Severupidus'' Cocktail of Death', ''),
			(112, 60, 0, 0, 30, 3, 50, 0, 0, 0, 6, 0, 'Lich', ''),
			(113, 30, 0, 0, 30, 3, 50, 0, 0, 0, 6, 0, 'Tide of Life', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES
			(14, 110, 'eqoa-data cm'), (14, 111, 'eqoa-data epic'), (11, 112, 'eqoa-data master Lich'),
			(9, 113, 'eqoa-data cm'), (7, 113, 'eqoa-data')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.Add(2 * time.Minute) // the list is built again
	m := list(t, s)
	for id, want := range map[int][2]string{110: {"mastery", ""}, 111: {"epic", ""}, 112: {"master class", "Lich"}, 113: {"", ""}, 100: {"", ""}} {
		if sp := m[id]; sp.Got != want[0] || sp.Master != want[1] {
			t.Errorf("%d %s: got %q master %q, want %q %q", id, sp.Name, sp.Got, sp.Master, want[0], want[1])
		}
	}
	if fmt.Sprint(m[113].Classes) != "[7 9]" {
		t.Errorf("Tide of Life's classes %v, want both", m[113].Classes)
	}
}
