package entity

import (
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Mr. Moogie's challenge on a real server's database with its data step applied (EQOA_REAL_DB names
// it; a copy is read; the step is the lane's metalmoogie_20261109.py, petitions 1379 and 1380): his
// metal twin's row stands out of the world, his template reads with no problem, a challenge from
// beside Mr. Moogie copies him with Jibcro's 50k hit points, and under the database's own balance his
// hits on a level 60 warrior of AC 2000 and AGI 400 land near 300, four a round; the Duelist's Favor
// reads as half again as big for an hour, the Moogie Tribute is NO TRADE and LORE, and his corpse holds
// one. Skipped without a database, or with one the step was not applied to.
func TestRealMetalMoogie(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "metal.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM npcs WHERE npc_name = ?`, MetalMoogieName).Scan(&n); err != nil || n == 0 {
		t.Skipf("no %s in this database (the data step not applied): %v", MetalMoogieName, err)
	}
	w := NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "metal moogie") {
			t.Errorf("his template's problem: %s", p)
		}
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	var moogie *NPC
	for _, npc := range w.NPCs {
		if npc.Name == MoogieName && !npc.Temporary {
			moogie = npc
		}
	}
	if moogie == nil {
		t.Fatal("no Mr. Moogie")
	}
	if !w.MetalMoogieHere() {
		t.Fatal("no spawn-only Metal Moogie")
	}
	sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 7, 1), Port: 7001}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 999001, CharServerID: 999001, Name: "Tester", Class: 0, Level: 60, HP: 5000, MaxHP: 5000,
		X: moogie.X + 3, Y: moogie.Y, Z: moogie.Z, World: moogie.World, Size: PlayerSize, GMTankProfile: 2}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	line, ok := w.MetalMoogieChallenge(sess, moogie.ServerID, time.Now())
	if !ok {
		t.Fatalf("no challenge: %q", line)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.NPCs[w.metal.npc]
	if c.MaxHP < 49000 || c.MaxHP > 51000 || c.Level != 60 || c.TemplateName() != "metal moogie" {
		t.Errorf("he rose with %d hit points at level %d on %q", c.MaxHP, c.Level, c.TemplateName())
	}
	if d := dist3(c.X, c.Y, c.Z, moogie.X, moogie.Y, moogie.Z); d > MetalRiseNear {
		t.Errorf("he rose %.1f from Mr. Moogie", d)
	}
	if _, _, _, hits := npcRound(c); len(hits) != 4 {
		t.Errorf("%d hits a round", len(hits))
	}
	sum, lo, hi := 0, math.MaxInt, 0
	const rolls = 1000
	for i := 0; i < rolls; i++ {
		k := i
		hit := w.npcHitPlayerLocked(c, p, npcMeleeOn(c, p, func(m int) int { return k * m / rolls }))
		sum += hit
		lo, hi = min(lo, hit), max(hi, hit)
	}
	if mean := float64(sum) / rolls; mean < 270 || mean > 330 {
		t.Errorf("his hits on the 2,000 AC 400 AGI warrior: mean %.1f (%d-%d), want about 300", mean, lo, hi)
	} else {
		t.Logf("%d hit points; on the 2,000 AC 400 AGI level 60 warrior %.1f a hit (%d-%d), four a round", c.MaxHP, mean, lo, hi)
	}
	t.Logf("%s", w.templateLineLocked(c, time.Now()))

	var line2 string
	var fx int64
	if err := db.QueryRow(`SELECT e.line, e.fxdict FROM spellpattern p JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellname = ?`,
		DuelistsFavorName).Scan(&line2, &fx); err != nil {
		t.Fatalf("the favor's record: %v", err)
	}
	if d := ParseSpellNamed(DuelistsFavorName, line2, ""); len(d.Effects) != 1 || d.Effects[0].Kind != EffectSize || d.Effects[0].Amount != 50 ||
		d.Duration != time.Hour {
		t.Errorf("the favor reads %+v", d)
	}
	var trade, lore, held int
	if err := db.QueryRow(`SELECT trade, lore FROM itempattern WHERE itemname = ?`, MoogieTributeName).Scan(&trade, &lore); err != nil || trade != 1 || lore != 1 {
		t.Errorf("the tribute: trade %d, lore %d, %v", trade, lore, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid WHERE i.npcid = ? AND p.itemname = ?`,
		c.NPCID, MoogieTributeName).Scan(&held); err != nil || held != 1 {
		t.Errorf("%d tributes on his corpse (%v), want one", held, err)
	}
}
