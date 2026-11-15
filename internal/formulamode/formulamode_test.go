package formulamode

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// a made-up formula: no real model
const toyFormula = "formulas:\n  npc_melee: base * 2 + 1\n"

type shard struct {
	db    *database.SQLiteDB
	path  string
	w     *entity.World
	c     *Control
	mu    sync.Mutex
	heard []string
}

func (s *shard) told() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.heard, "\n")
}

func newShard(t *testing.T, dir, name, peer, peerPath string, seed entity.FormulaMode) *shard {
	t.Helper()
	s := &shard{path: filepath.Join(dir, name+".db"), w: entity.NewWorld()}
	db, err := database.NewSQLite(s.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Exec(`INSERT OR IGNORE INTO accountinfo (accountid, username, acctlevel) VALUES (7, 'dab', 2)`)
	s.db = db
	s.c, err = New(db, s.w, name, map[string]string{peer: peerPath}, seed)
	if err != nil {
		t.Fatal(err)
	}
	s.c.SetTell(func(line string) { s.mu.Lock(); s.heard = append(s.heard, line); s.mu.Unlock() })
	return s
}

func loadToy(t *testing.T, s *shard) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "damage.formula")
	if err := os.WriteFile(p, []byte(toyFormula), 0o600); err != nil {
		t.Fatal(err)
	}
	s.w.SetDamageFormulaFile(p)
}

func audits(t *testing.T, db *database.SQLiteDB) string {
	t.Helper()
	rows, err := db.Query(`SELECT detail FROM gm_audit ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var d string
		rows.Scan(&d)
		b.WriteString(d + "\n")
	}
	return b.String()
}

var dab = database.GMEdit{AccountID: 7, Via: "game"}

// A switch here: refused with no formula loaded, at once with one, audited
// and told; kept over a restart, which the config's seed does not undo.
func TestSwitchHereAndPersistence(t *testing.T) {
	dir := t.TempDir()
	h := newShard(t, dir, "heirloom", "live", filepath.Join(dir, "live.db"), entity.FormulaOff)
	if h.w.DamageFormulaMode() != entity.FormulaOff {
		t.Fatalf("seeded off: %v", h.w.DamageFormulaMode())
	}
	if _, err := h.c.Set("heirloom", entity.FormulaLive, dab); err == nil || !strings.Contains(err.Error(), "no formula loaded") {
		t.Fatalf("live with no formula: %v", err)
	}
	loadToy(t, h)
	if _, err := h.c.Set("", entity.FormulaShadow, dab); err == nil || !strings.Contains(err.Error(), "no shadow log") {
		t.Errorf("shadow with no log: %v", err)
	}
	line, err := h.c.Set("heirloom", entity.FormulaLive, dab)
	if err != nil || h.w.DamageFormulaMode() != entity.FormulaLive {
		t.Fatalf("switched: %q %v, mode %v", line, err, h.w.DamageFormulaMode())
	}
	if !strings.Contains(h.told(), "Heirloom is now jibs") || !strings.Contains(audits(t, h.db), "set to live") {
		t.Errorf("told %q; audit %q", h.told(), audits(t, h.db))
	}
	// a restart: a new World and switch on the same database, the config seeding off
	w2 := entity.NewWorld()
	if _, err := New(h.db, w2, "heirloom", nil, entity.FormulaOff); err != nil {
		t.Fatal(err)
	}
	if w2.DamageFormulaMode() != entity.FormulaLive {
		t.Errorf("after a restart: %v, want the GM's live kept", w2.DamageFormulaMode())
	}
	if _, err := h.c.Set("heirloom", entity.FormulaOff, dab); err != nil || h.w.DamageFormulaMode() != entity.FormulaOff {
		t.Errorf("back to default: %v %v", err, h.w.DamageFormulaMode())
	}
	if _, err := h.c.Set("narnia", entity.FormulaOff, dab); err == nil || !strings.Contains(err.Error(), "unknown shard") {
		t.Errorf("an unknown shard: %v", err)
	}
}

// A switch of the other shard: refused while it has no formula loaded, then
// written to its database and taken up by its server at its poll, told to
// its GMs, audited on both; and either way round.
func TestSwitchAcrossShards(t *testing.T) {
	dir := t.TempDir()
	hp, lp := filepath.Join(dir, "heirloom.db"), filepath.Join(dir, "live.db")
	h := newShard(t, dir, "heirloom", "live", lp, entity.FormulaOff)
	l := newShard(t, dir, "live", "heirloom", hp, entity.FormulaOff)
	if _, err := h.c.Set("live", entity.FormulaLive, dab); err == nil || !strings.Contains(err.Error(), "no formula loaded on Live") {
		t.Fatalf("live with none loaded there: %v", err)
	}
	loadToy(t, l)
	l.c.Poll() // Live's server writes what it has loaded
	if _, err := h.c.Set("live", entity.FormulaLive, dab); err != nil {
		t.Fatalf("Heirloom switching Live: %v", err)
	}
	if l.w.DamageFormulaMode() != entity.FormulaOff {
		t.Error("Live switched before its poll")
	}
	l.c.Poll()
	if l.w.DamageFormulaMode() != entity.FormulaLive {
		t.Errorf("Live after its poll: %v", l.w.DamageFormulaMode())
	}
	if !strings.Contains(l.told(), "Live is now jibs") || !strings.Contains(l.told(), "on Heirloom") {
		t.Errorf("Live's GMs were told %q", l.told())
	}
	if !strings.Contains(audits(t, l.db), "from Heirloom") || !strings.Contains(audits(t, h.db), "mode of Live set to live") {
		t.Errorf("audits: live %q heirloom %q", audits(t, l.db), audits(t, h.db))
	}
	st := h.c.Status()
	if len(st) != 2 || st[0].Shard != "heirloom" || st[1].Mode != "live" || st[1].Loaded != 1 {
		t.Errorf("Heirloom's view: %+v", st)
	}
	// and back from Live to Heirloom, which needs no formula for default
	if _, err := l.c.Set("heirloom", entity.FormulaOff, dab); err != nil {
		t.Fatalf("Live switching Heirloom: %v", err)
	}
	h.c.Poll()
	if h.w.DamageFormulaMode() != entity.FormulaOff {
		t.Errorf("Heirloom: %v", h.w.DamageFormulaMode())
	}
}

// A shard whose database is not configured, or is this one's, is refused
// with a line saying so.
func TestSwitchRefusesAnUnconfiguredShard(t *testing.T) {
	dir := t.TempDir()
	h := newShard(t, dir, "heirloom", "live", "", entity.FormulaOff)
	if _, err := h.c.Set("live", entity.FormulaOff, dab); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("no database for live: %v", err)
	}
	h2 := newShard(t, t.TempDir(), "heirloom", "live", filepath.Join(dir, "nope.db"), entity.FormulaOff)
	if _, err := h2.c.Set("live", entity.FormulaOff, dab); err == nil {
		t.Error("a missing file was written")
	}
	if _, err := os.Stat(filepath.Join(dir, "nope.db")); err == nil {
		t.Error("a missing peer database was made")
	}
	h3 := newShard(t, dir, "x", "live", h.path, entity.FormulaOff)
	h3.c.db = h.db
	if _, err := h3.c.Set("live", entity.FormulaOff, dab); err == nil || !strings.Contains(err.Error(), "own database") {
		t.Errorf("its own database as the peer: %v", err)
	}
}
