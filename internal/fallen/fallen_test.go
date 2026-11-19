package fallen

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// The page: both sections, the living ranked and the fallen with their fall,
// nothing of any account; /api/hardcore the same as JSON.
func TestThePage(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/page.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	mk := func(name string, level int) uint32 {
		rec := database.CharacterRecord{CharName: name, AccountID: 9, Level: level, TClass: 3}
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		db.ChooseHardcore(rec.ServerID, true)
		return rec.ServerID
	}
	mk("Alivia", 30)
	dead := mk("Fallow", 52)
	at := time.Date(2026, 11, 18, 8, 45, 0, 0, time.UTC)
	db.Fall(database.FallRecord{ServerID: dead, At: at.Unix(), Cause: "creature", Killer: "a Zek sentry", Place: "Freeport", Level: 52, Class: 3, CMs: 140})
	p := New(db, config.HardcoreConfig{Shard: "Heirloom"})
	rec := httptest.NewRecorder()
	p.ServePage(rec, httptest.NewRequest("GET", "/hardcore", nil))
	body, _ := io.ReadAll(rec.Body)
	page := string(body)
	for _, want := range []string{"Living Hardcore Leaderboard", "Memorial of the Fallen", "Heirloom Server", "<b>Alivia</b>",
		`<b class="dead">Fallow</b>`, "2026-11-18 08:45", "a Zek sentry", "Freeport", "Shadow Knight"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(page, "accountid") || strings.Contains(page, ">9<") {
		t.Error("the page shows account data")
	}
	rec = httptest.NewRecorder()
	p.ServeAPI(rec, httptest.NewRequest("GET", "/api/hardcore", nil))
	if b := rec.Body.String(); !strings.Contains(b, `"living":[{"rank":1,"shard":"Heirloom","name":"Alivia"`) || !strings.Contains(b, `"killer":"a Zek sentry"`) {
		t.Errorf("api %s", b)
	}
}

// The page names no site unless world.hardcore.site_name does.
func TestPageSiteName(t *testing.T) {
	p := &Page{}
	if h, f := p.head(), p.foot(); strings.Contains(h, "{{") || strings.Contains(f, "{{") ||
		!strings.Contains(h, "<title>Hardcore Heroes</title>") || !strings.Contains(f, "A fan-run EQOA server") {
		t.Fatalf("no site:\n%s\n%s", h[:300], f)
	}
	p.site = "example.com"
	if h, f := p.head(), p.foot(); !strings.Contains(h, "<title>Hardcore Heroes - example.com</title>") ||
		!strings.Contains(h, "example.com's EQOA servers") || !strings.Contains(f, "example.com: a fan-run") {
		t.Fatalf("site:\n%s\n%s", h[:300], f)
	}
}
