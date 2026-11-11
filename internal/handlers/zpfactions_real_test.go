package handlers

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// TestRealZaoranPlateauFactions plays Zaoran Plateau as questmasters1.com's
// guide has it (tools/zp_guides_20261111.py's set zp-factions) on a copy of
// a server database that has the set: the passage at the portal guardian for
// the five runes and 5,000 tunar, then one new level 60 character for each
// side (a warrior for the Soldiers of Rallos Zek, a cleric for Mithaniel
// Marr's Crusaders, a wizard for the Akessan Collective) from its first quest
// to its Master Spellsmith's ability: the introduction, the alignment (+499
// or +500 with its side, -2000 with the other two), the weapon turn-ins to
// 2,000, and the scroll of its archetype for a mystical parchment. Aligned,
// the other sides' soldiers attack it on sight and a newcomer's they do not;
// the other sides' quests are no longer offered to it.
//
//	EQOA_REAL_DB=<db with zp-factions> go test ./internal/handlers/ -run TestRealZaoranPlateauFactions -v
func TestRealZaoranPlateauFactions(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	if os.Getenv("ZP_LOG") == "" {
		log.SetOutput(io.Discard) // the World's and the engine's lines are many thousands
		defer log.SetOutput(os.Stderr)
	}
	r := newWereRig(t, src, 1)
	defer r.db.Close()
	keys := map[string]int{}
	rows, err := r.db.Query(`SELECT key, questid FROM quest_import_quests WHERE key LIKE 'ZP/factions/%' OR key LIKE 'Portal/zaoran-plateau/%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		var id int
		rows.Scan(&k, &id)
		keys[k] = id
	}
	rows.Close()
	if keys["ZP/factions/zek/align"] == 0 {
		t.Skip("the database has no zp-factions set")
	}
	quest := func(k string) *database.Quest {
		r.qe.mu.Lock()
		defer r.qe.mu.Unlock()
		qu := r.qe.cat.byID[keys[k]]
		if qu == nil {
			t.Fatalf("no quest %s", k)
		}
		return qu
	}
	item := func(name string) int {
		var id int
		if err := r.db.QueryRow(`SELECT patternid FROM itempattern WHERE lower(itemname) = lower(?) ORDER BY patternid LIMIT 1`, name).Scan(&id); err != nil {
			t.Fatalf("no item %q", name)
		}
		return id
	}
	give := func(t *testing.T, c *wereChar, name string, n int) {
		pat, err := r.qe.items.GetItemPattern(item(name))
		if err != nil || pat == nil {
			t.Fatalf("item %q: %v", name, err)
		}
		if got, err := GiveItemNow(c.sess, r.gw, r.inv, *pat, n); err != nil || got != n {
			t.Fatalf("giving %d %q: %d (%v)", n, name, got, err)
		}
	}
	held := func(c *wereChar, name string) int {
		var n int
		r.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, c.p.CharServerID, item(name)).Scan(&n)
		return n
	}
	do := func(t *testing.T, c *wereChar, k string) {
		t.Helper()
		qu := quest(k)
		if why := r.take(c, qu); why != "" {
			t.Fatalf("%s: %s", k, why)
		}
		if _, why := r.play(c, qu); why != "" {
			t.Fatalf("%s: %s", k, why)
		}
	}
	const zek, marr, ake = "Soldiers of Rallos Zek", "Mithaniel Marr's Crusaders", "Akessan Collective"
	stand := func(c *wereChar) string {
		return fmt.Sprintf("Zek %d, Marr %d, Akessan %d", r.standing(c, zek), r.standing(c, marr), r.standing(c, ake))
	}

	// #494's armour lieutenants stand out of the world (a headstone raises them): talked to as they are
	lieutenant := map[string]string{zek: "Krognar", marr: "Merkym", ake: "Akessan Collector"}
	armour := map[string]string{zek: "ZP/armour/zek/helm", marr: "ZP/armour/honor/helm", ake: "ZP/armour/akessan/helm"}
	for _, k := range armour {
		var id int
		r.db.QueryRow(`SELECT questid FROM quest_import_quests WHERE key = ?`, k).Scan(&id)
		keys[k] = id
	}
	original := func(name string) *entity.NPC {
		for _, n := range r.gw.NPCs {
			if n.SpawnOnly && strings.EqualFold(n.Name, name) {
				return n
			}
		}
		t.Fatalf("no %s kept out of the world", name)
		return nil
	}
	// helmAt gives the character the helm's two gems and talks to the side's lieutenant: what it said, and
	// whether the helm's quest was finished once more (offered, taken and handed in)
	helmAt := func(t *testing.T, c *wereChar, fac string) (string, bool) {
		qu := quest(armour[fac])
		for _, it := range qu.First().Items {
			pat, err := r.qe.items.GetItemPattern(it.PatternID)
			if err != nil || pat == nil {
				t.Fatalf("gem %d: %v", it.PatternID, err)
			}
			if got, err := GiveItemNow(c.sess, r.gw, r.inv, *pat, it.Count); err != nil || got != it.Count {
				t.Fatalf("gem %q: %d (%v)", pat.ItemName, got, err)
			}
		}
		_, before := c.w.progress(qu.ID)
		lt := original(lieutenant[fac])
		r.standBeside(c, lt)
		questMsgs(c.sess)
		r.qe.Talk(c.sess, c.p, lt)
		said := strings.Join(questMsgs(c.sess), " | ")
		c.w.settle()
		for i := 0; i < 3; i++ { // the hand-in's step, at the same lieutenant
			if _, d := c.w.progress(qu.ID); d > before {
				break
			}
			r.qe.Talk(c.sess, c.p, lt)
			c.w.settle()
		}
		_, after := c.w.progress(qu.ID)
		return said, after > before
	}

	t.Run("passage", func(t *testing.T) {
		c := r.newChar(wereArchetype{"zp", 0, 6}, 60)
		for _, a := range []string{"tranquility", "spite", "honor", "war", "growth"} {
			if _, err := r.qe.SetCharacterQuest(c.p.CharServerID, keys["Portal/zaoran-plateau/"+a], QuestComplete, 0); err != nil {
				t.Fatal(err)
			}
		}
		for _, rune := range []string{"Agent of Tranquility's rune", "Agent of Spite's rune", "Agent of Honor's rune", "Agent of War's rune", "Agent of Growth's rune"} {
			give(t, c, rune, 1)
		}
		c.p.Tunar = 4999
		qu := quest("Portal/zaoran-plateau/portal")
		if why := r.take(c, qu); why != "" {
			t.Fatal(why)
		}
		if _, why := r.play(c, qu); why == "" {
			t.Fatalf("passed with 4,999 tunar")
		}
		c.p.Tunar = 6000
		if _, why := r.play(c, qu); why != "" {
			t.Fatal(why)
		}
		if c.p.Tunar != 1000 {
			t.Errorf("tunar %d after the passage, want 1000 (5,000 paid)", c.p.Tunar)
		}
		if n := held(c, "Agent of War's rune"); n != 1 {
			t.Errorf("the runes are shown, not taken: %d of the Agent of War's left", n)
		}
	})

	type side struct {
		name, intro, align, giver, faction string
		class, race                        int
		turnin, weapon, ability, scroll    string
		pre                                []string
	}
	sides := []side{
		{"zek", "ZP/factions/zek/intro", "ZP/factions/zek/align", "Lieutenant Ugmar", zek, 0, 9, "ZP/factions/zek/weapons-honor", "Honor Guard's blade",
			"ZP/factions/zek/ability", "Rage of Zek", nil},
		{"marr", "ZP/factions/marr/intro", "ZP/factions/marr/align", "Lieutenant Jenkavs", marr, 9, 0, "ZP/factions/marr/weapons-zaoran", "Zaoran war axe",
			"ZP/factions/marr/ability", "Divinity", nil},
		{"akessan", "ZP/factions/akessan/align", "ZP/factions/akessan/captains", "Akessan Agent", ake, 13, 8, "ZP/factions/akessan/weapons-zaoran", "Zaoran war axe",
			"ZP/factions/akessan/ability", "Fingers of Thunder",
			[]string{"ZP/factions/akessan/envoy", "ZP/factions/akessan/parchments", "ZP/factions/akessan/lenses", "ZP/factions/akessan/escort", "ZP/factions/akessan/jubrak"}},
	}
	for _, s := range sides {
		t.Run(s.name, func(t *testing.T) {
			c := r.newChar(wereArchetype{s.name, s.class, s.race}, 60)
			newcomer := r.newChar(wereArchetype{s.name + "new", s.class, s.race}, 60)
			// not aligned yet: the side's lieutenant will not make its armour
			if said, done := helmAt(t, c, s.faction); done || !strings.Contains(said, "I work only for those who have stood with "+s.faction) {
				t.Errorf("before the alignment, %s: done %v, said %q", lieutenant[s.faction], done, said)
			}
			do(t, c, s.intro)
			t.Logf("after %s: %s", s.intro, stand(c))
			if s.name == "akessan" {
				// the Liason's weapons to -320, as the guide: four Honor Guard blades a stack
				for i := 0; r.standing(c, ake) < -320; i++ {
					if i > 20 {
						t.Fatalf("the Liason's turn-ins stall at %d", r.standing(c, ake))
					}
					give(t, c, "Honor Guard's blade", 4)
					do(t, c, "ZP/factions/akessan/liason-weapons")
				}
				for _, k := range s.pre {
					do(t, c, k)
				}
			}
			do(t, c, s.align)
			t.Logf("aligned: %s", stand(c))
			want := map[string]int{zek: -2000, marr: -2000, ake: -2000}
			want[s.faction] = r.standing(c, s.faction)
			if want[s.faction] < 150 {
				t.Errorf("aligned with %s at only %d", s.faction, want[s.faction])
			}
			for f, v := range want {
				if got := r.standing(c, f); got != v {
					t.Errorf("%s: %d, want %d", f, got, v)
				}
			}
			// the other sides: their soldiers attack it, a newcomer's do not; their alignment is not offered
			for _, other := range []struct{ soldier, align, faction string }{
				{"Zaoran guard", "ZP/factions/zek/align", zek}, {"Honor Guard veteran", "ZP/factions/marr/align", marr},
				{"Akessan Nihilist", "ZP/factions/akessan/align", ake}} {
				ns := r.named(c, other.soldier)
				if len(ns) == 0 {
					t.Fatalf("no %s", other.soldier)
				}
				hostile := r.gw.TargetFace(c.p, ns[0]) == entity.FaceHostile
				if other.faction == s.faction && hostile {
					t.Errorf("its own side's %s attacks it", other.soldier)
				}
				if other.faction != s.faction && !hostile {
					t.Errorf("the other side's %s does not attack it", other.soldier)
				}
				if r.gw.TargetFace(newcomer.p, ns[0]) == entity.FaceHostile {
					t.Errorf("%s attacks a newcomer", other.soldier)
				}
				if other.align != s.align && other.faction != s.faction {
					if why := r.why(c, quest(other.align)); why == "" {
						t.Errorf("%s is still offered to it", other.align)
					}
				}
			}
			// aligned: its side's lieutenant makes the helm; the other sides' refuse it
			if said, done := helmAt(t, c, s.faction); !done {
				t.Errorf("aligned, %s did not make the helm: %q", lieutenant[s.faction], said)
			}
			for _, f := range []string{zek, marr, ake} {
				if f == s.faction {
					continue
				}
				if said, done := helmAt(t, c, f); done || !strings.Contains(said, lieutenant[f]) || !strings.Contains(said, "I work only for") {
					t.Errorf("aligned with %s, %s: done %v, said %q", s.faction, lieutenant[f], done, said)
				}
			}
			if why := r.why(c, quest(s.ability)); why == "" {
				t.Errorf("the ability is offered at %d", r.standing(c, s.faction))
			}
			for i := 0; r.standing(c, s.faction) < 2000; i++ {
				if i > 130 {
					t.Fatalf("the turn-ins stall at %d", r.standing(c, s.faction))
				}
				give(t, c, s.weapon, 4)
				do(t, c, s.turnin)
			}
			give(t, c, "mystical parchment", 1)
			do(t, c, s.ability)
			if held(c, s.scroll) != 1 || held(c, "mystical parchment") != 0 {
				t.Errorf("the Master Spellsmith's gift: %d %q, %d parchments left", held(c, s.scroll), s.scroll, held(c, "mystical parchment"))
			}
			t.Logf("%s: %s, %q held", s.name, stand(c), s.scroll)
			if strings.TrimSpace(s.giver) == "" {
				t.Fatal("no giver")
			}
		})
	}
}
