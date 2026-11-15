package entity

import (
	"bytes"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Heirloom petitions 2189, 2191 and 2192 on a real server's database with
// tools/impale_gore_20261115.py and tools/iod_named_kits_20261115.py applied
// (EQOA_REAL_DB; a copy is read): each of the Isle of Dread's creatures below
// fights a level 60 player standing beside it for 60 s of the server's own
// fight (aiSystem, templateSystem), and its kit lands as the tools mean it:
// Festering Jack's AoE about every 20 s at 1,000 before the player's resist,
// Contagion's 1,000 poison AoE within its first 30 s, a breath stealer's and a
// virulent blade's mana tap taking power every 20 s, and Impale's Impaling Gore
// taking a player of AC 2,520 to 1,020 (not 20). Skipped without a database,
// or one the tools were not applied to.
func TestRealIsleOfDreadKits(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "iod.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, logTable := range []string{"impale_gore_1115_log", "iod_named_kits_1115_log"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = ?`, logTable).Scan(&n); err != nil || n == 0 {
			t.Skipf("%s not in this database (its tool not applied)", logTable)
		}
	}
	w := NewWorld()
	w.SessionManager = session.NewManager()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "iod ") {
			t.Errorf("a template's problem: %s", p)
		}
	}

	// the first standing creature of the name in the island's box, at its level
	find := func(name string, level int) *NPC {
		var best *NPC
		for _, n := range w.NPCs {
			if strings.EqualFold(n.Name, name) && n.World == 0 && n.Level == level && n.X > 24000 && n.X < 26000 &&
				n.Z > 2000 && n.Z < 4000 && !w.npcSpawnOnlyLocked(n) && !n.Temporary && (best == nil || n.NPCID < best.NPCID) {
				best = n
			}
		}
		return best
	}
	hitRe := regexp.MustCompile(`'s "([^"]+)" hits \S+ for (\d+) of (\d+)`)
	fightFor := func(npc *NPC, class int, d time.Duration) (p *Player, lines []string) {
		sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, byte(npc.ServerID%250)), Port: 7001}, 1, 1, 1, 2, true)
		p = &Player{ServerID: EntityID(990000 + npc.ServerID%1000), CharServerID: 999001, Name: "Tester", Class: class, Level: 60,
			HP: 1_000_000, MaxHP: 1_000_000, Power: 8000, MaxPower: 8000, X: npc.X + 2, Y: npc.Y, Z: npc.Z, World: npc.World,
			Size: PlayerSize, AC: 2520, Agi: 520}
		w.mu.Lock()
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		w.addThreatLocked(npc, p.ServerID, 1)
		w.mu.Unlock()
		buf.Reset()
		now := time.Unix(1_800_000_000, 0)
		for end := now.Add(d); now.Before(end); now = now.Add(100 * time.Millisecond) {
			w.mu.Lock()
			w.TickCount++
			p.HP = p.MaxHP // a healer at its back: never down
			w.aiSystem(now)
			w.templateSystem(now)
			w.mu.Unlock()
		}
		w.mu.Lock()
		delete(w.Players, p.ServerID)
		w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
		w.giveUpLocked(npc)
		w.mu.Unlock()
		return p, strings.Split(buf.String(), "\n")
	}
	casts := func(lines []string, spell string) (n int, figures []int) {
		for _, l := range lines {
			if m := hitRe.FindStringSubmatch(l); m != nil && m[1] == spell {
				n++
				f, _ := strconv.Atoi(m[3])
				figures = append(figures, f)
			}
		}
		return n, figures
	}

	if jack := find("Festering Jack", 55); jack == nil {
		t.Error("no Festering Jack at 55")
	} else {
		_, lines := fightFor(jack, 0, 60*time.Second)
		n, figs := casts(lines, "Shadow Burst")
		t.Logf("Festering Jack: %d Shadow Bursts in 60 s, figures %v", n, figs)
		if n < 2 || figs[0] < 950 || figs[0] > 1050 {
			t.Errorf("Festering Jack's AoE: %d in 60 s, figures %v; want 3, about 1,000 each", n, figs)
		}
	}
	if cont := find("Contagion", 55); cont == nil {
		t.Error("no Contagion at 55")
	} else {
		_, lines := fightFor(cont, 0, 30*time.Second)
		n, figs := casts(lines, "Pestilent Breath")
		t.Logf("Contagion: %d Pestilent Breaths in its first 30 s, figures %v", n, figs)
		if n < 1 || figs[0] < 950 || figs[0] > 1050 {
			t.Errorf("Contagion's AoE in its first 30 s: %d, figures %v; want one of about 1,000", n, figs)
		}
	}
	for _, c := range []struct{ name, spell string }{{"breath stealer", "Stolen Breath"}, {"virulent blade", "Virulent Siphon"}} {
		npc := find(c.name, 53)
		if npc == nil {
			t.Errorf("no %s at 53", c.name)
			continue
		}
		p, lines := fightFor(npc, 13, 45*time.Second)
		taps := 0
		for _, l := range lines {
			if strings.Contains(l, c.spell) && strings.Contains(l, "begins") {
				taps++
			}
		}
		t.Logf("%s (%s): %d casts in 45 s, the wizard's power %d of %d", c.name, npc.TemplateName(), taps, p.Power, p.MaxPower)
		if taps < 2 || p.Power >= p.MaxPower {
			t.Errorf("%s's %s: %d casts in 45 s, power %d of %d; want 2-3 taking power", c.name, c.spell, taps, p.Power, p.MaxPower)
		}
	}
	if impale := find("Impale", 55); impale == nil {
		t.Error("no Impale at 55")
	} else {
		var lowest int = 1 << 30
		sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, 251), Port: 7001}, 1, 1, 1, 2, true)
		p := &Player{ServerID: 998001, CharServerID: 999002, Name: "Zerk", Class: 1, Level: 60, HP: 1_000_000, MaxHP: 1_000_000,
			X: impale.X + 2, Y: impale.Y, Z: impale.Z, World: impale.World, Size: PlayerSize, AC: 2520, Agi: 520}
		w.mu.Lock()
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		w.addThreatLocked(impale, p.ServerID, 1)
		w.mu.Unlock()
		now := time.Unix(1_800_000_000, 0)
		for end := now.Add(40 * time.Second); now.Before(end); now = now.Add(100 * time.Millisecond) {
			w.mu.Lock()
			w.TickCount++
			p.HP = p.MaxHP
			w.aiSystem(now)
			w.templateSystem(now)
			lowest = min(lowest, p.StatNow(StatAC))
			w.mu.Unlock()
		}
		t.Logf("Impale: Zerkziee's AC at its lowest %d under Impaling Gore", lowest)
		if lowest != 1020 {
			t.Errorf("under Impaling Gore Zerkziee's AC went to %d, want 1,020", lowest)
		}
	}
}
