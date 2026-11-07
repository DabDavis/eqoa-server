package handlers

import (
	"io"
	"log"
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A real server's Plane of Sky armour (EQOA_REAL_DB names its database, a copy
// of which is walked; skipped on one without tools/quests/data's
// plane-of-sky-armor.json imported): every piece's quest by every class it is
// for, as a player does it: holding the recipe, talked to, the offer taken,
// talked to again. What it asks is taken, nothing else; what it gives is one
// record, wearable by that class and by no other; the quest can be done again;
// and another role's giver takes nothing and gives nothing.
func TestRealSkyArmourGivesEachClassItsOwn(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	rows, err := w.db.Query(`SELECT questid FROM quests WHERE source LIKE 'EQOA-Data PlaneOfSkyArmor/%' ORDER BY questid`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		t.Skip("the Plane of Sky's armour is not imported here")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	raceFor := func(c int) int {
		for _, cr := range w.combo {
			if cr[0] == c {
				return cr[1]
			}
		}
		t.Fatalf("no race for class %d", c)
		return 0
	}
	held := func(pid int) int {
		var n int
		w.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, w.p.CharServerID, pid).Scan(&n)
		return n
	}
	made := 0
	for _, id := range ids {
		qu := w.quest(id)
		st := qu.First()
		if st == nil || st.Kind != database.StepGive || len(st.Items) != 3 || !qu.Repeatable || !qu.Enabled {
			t.Fatalf("%q is no repeatable hand-in of three items: %+v", qu.Name, st)
		}
		for _, c := range qu.Classes {
			w.become(c, raceFor(c))
			w.p.Level = 60
			// what no one holds: this quest finds it holds nothing else of it
			for _, it := range st.Items {
				w.db.Exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid = ?`, w.p.CharServerID, it.PatternID)
			}
			for _, it := range st.Items {
				if _, err := w.inv.AddCharItem(w.p.CharServerID, it.PatternID, it.Count, 0, 0, 0); err != nil {
					t.Fatal(err)
				}
			}
			items, err := w.inv.GetCharacterItems(w.p.CharServerID)
			if err != nil {
				t.Fatal(err)
			}
			w.sess.SetInventoryList(newInvList(items))
			rw := st.RewardFor(c)
			if len(rw.Items) != 1 || rw.Items[0].Count != 1 {
				t.Fatalf("%q for class %d rewards %+v", qu.Name, c, rw.Items)
			}
			piece := rw.Items[0].PatternID
			pat, err := w.qe.items.GetItemPattern(piece)
			if err != nil || pat == nil {
				t.Fatalf("%q's reward %d: %v", qu.Name, piece, err)
			}
			if pat.ClassUse != 1<<uint(c) {
				t.Errorf("%q for class %d gives %q of mask %d", qu.Name, c, pat.ItemName, pat.ClassUse)
			}
			if why := cannotWear(w.p, pat); why != "" {
				t.Errorf("class %d cannot wear %q: %s", c, pat.ItemName, why)
			}
			was := held(piece)
			for round := 1; round <= 2; round++ {
				if round == 2 { // a replacement: the recipe again
					for _, it := range st.Items {
						w.inv.AddCharItem(w.p.CharServerID, it.PatternID, it.Count, 0, 0, 0)
					}
					items, _ = w.inv.GetCharacterItems(w.p.CharServerID)
					w.sess.SetInventoryList(newInvList(items))
				}
				questMsgs(w.sess)
				giver := w.npc(qu.Giver, 0)
				w.qe.Talk(w.sess, w.p, giver)
				w.answer() // its offer, taken
				if s, _ := w.progress(id); s != st.Step {
					t.Fatalf("class %d, %q round %d: not taken (step %d): %s", c, qu.Name, round, s, w.lastWords())
				}
				w.qe.Talk(w.sess, w.p, giver)
				if s, d := w.progress(id); s != 0 || d != round {
					t.Fatalf("class %d, %q round %d: step %d done %d: %s", c, qu.Name, round, s, d, w.lastWords())
				}
				for _, it := range st.Items {
					if n := held(it.PatternID); n != 0 {
						t.Errorf("class %d, %q: %d of item %d left", c, qu.Name, n, it.PatternID)
					}
				}
				if n := held(piece); n != was+round {
					t.Errorf("class %d, %q round %d: holds %d of %q, had %d", c, qu.Name, round, n, pat.ItemName, was)
				}
			}
			made++
		}
	}
	if made != 15*6 {
		t.Errorf("%d pieces made, want 90", made)
	}
	// another role's giver: nothing offered, nothing taken, whatever is held
	byGiver := map[string][]int{}
	for _, id := range ids {
		qu := w.quest(id)
		byGiver[qu.Giver] = qu.Classes
	}
	for giver := range byGiver {
		for g2, classes := range byGiver {
			if g2 == giver {
				continue
			}
			c := classes[0]
			w.become(c, raceFor(c))
			w.p.Level = 60
			w.qe.mu.Lock()
			got := w.qe.readyHandInLocked(w.qe.lookAt(w.sess), w.p, w.qe.charLocked(w.p.CharServerID), giver)
			next := w.qe.nextOfferLocked(w.qe.lookAt(w.sess), w.p, w.qe.charLocked(w.p.CharServerID), giver, nil)
			w.qe.mu.Unlock()
			if got != nil {
				t.Errorf("class %d offered %q at another role's giver %s", c, got.Name, giver)
			}
			if next != nil && next.Giver == giver && containsInt(next.Classes, c) == false && len(next.Classes) > 0 {
				t.Errorf("class %d offered %q at another role's giver %s", c, next.Name, giver)
			}
		}
	}
}
