package handlers

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The city class quests' rewards on a real server (EQOA_REAL_DB names its
// database, a copy of which is read), after tools/fill_class_quest_rewards.py:
// every scroll the tool put on a quest's last step, as its own reward or a
// level-20 choice's, is one scribe.go scribes (isScroll; scrollSpell over
// every spell, in its own order) into a spell of the quest's class (a
// classspells record of that name at that level), with the class in its mask
// and its level not above the quest's; every choice it wrote parses, at an NPC,
// with the guides' two paths. The tool reads scribe.go's rules in Python: this
// reads them in scribe.go's own code. Skipped without a database, and on one
// the tool has not filled: none is in this repository.
func TestRealClassQuestRewardsScribe(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "classquests.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'class_quest_fill_log'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/fill_class_quest_rewards.py has not filled this database")
	}
	// the class's own spells: spell key -> the levels of its records
	own := map[int]map[string]map[int]bool{}
	rows, err := db.Query(`SELECT c.class, p.spellname, p.abilitylvl FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c, lv int
		var name string
		if err := rows.Scan(&c, &name, &lv); err != nil {
			t.Fatal(err)
		}
		if own[c] == nil {
			own[c] = map[string]map[int]bool{}
		}
		k := spellKey(name)
		if own[c][k] == nil {
			own[c][k] = map[int]bool{}
		}
		own[c][k][lv] = true
	}
	rows.Close()
	type place struct{ quest, step int }
	filled := map[place]string{} // the column the tool wrote
	rows, err = db.Query(`SELECT rid, col FROM class_quest_fill_log WHERE tbl = 'queststeps'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var rid, col string
		if err := rows.Scan(&rid, &col); err != nil {
			t.Fatal(err)
		}
		q, s, _ := strings.Cut(rid, "/")
		qi, _ := strconv.Atoi(q)
		si, _ := strconv.Atoi(s)
		filled[place{qi, si}] = col
	}
	rows.Close()
	if len(filled) == 0 {
		t.Skip("the tool's log names no quest step")
	}

	quests, err := database.NewQuestStore(db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	names, err := database.NewSpellbookStore(db).SpellNames()
	if err != nil {
		t.Fatal(err)
	}
	items := database.NewItemPatternStore(db)
	scrolls, choices := 0, 0
	for _, q := range quests {
		for i := range q.Steps {
			st := &q.Steps[i]
			col, ok := filled[place{q.ID, st.Step}]
			if !ok {
				continue
			}
			if len(q.Classes) != 1 {
				t.Errorf("quest %d %q: a city class quest of %d classes", q.ID, q.Name, len(q.Classes))
				continue
			}
			class := q.Classes[0]
			rewards := append([]database.ItemCount(nil), st.RewardItems...)
			if col == "choices" {
				choices++
				if len(st.Choices.Options) != 2 || !database.AtNPC(st.Kind) {
					t.Errorf("quest %d %q step %d: a %s step's choice of %d options", q.ID, q.Name, st.Step, st.Kind, len(st.Choices.Options))
				}
				for _, o := range st.Choices.Options {
					if len(o.Reward.Items) != 1 {
						t.Errorf("quest %d %q: option %q gives %d items", q.ID, q.Name, o.Answer, len(o.Reward.Items))
					}
					rewards = append(rewards, o.Reward.Items...)
				}
			}
			for _, it := range rewards {
				p, err := items.GetItemPattern(it.PatternID)
				if err != nil || p == nil {
					t.Errorf("quest %d %q: reward %d is no item: %v", q.ID, q.Name, it.PatternID, err)
					continue
				}
				if !isScroll(p) {
					continue // an item as it is
				}
				scrolls++
				spell, ok := scrollSpell(names, p.ItemName, p.LevelReq)
				switch {
				case !ok:
					t.Errorf("quest %d %q: scroll %d %q scribes nothing", q.ID, q.Name, p.PatternID, p.ItemName)
				case !own[class][spellKey(spell.Name)][spell.Level]:
					t.Errorf("quest %d %q: scroll %d %q scribes %q (level %d), not a spell of class %d", q.ID, q.Name,
						p.PatternID, p.ItemName, spell.Name, spell.Level, class)
				case p.ClassUse&(1<<uint(class)) == 0:
					t.Errorf("quest %d %q: scroll %d %q is not class %d's", q.ID, q.Name, p.PatternID, p.ItemName, class)
				case p.LevelReq > q.MinLevel:
					t.Errorf("quest %d %q (level %d): scroll %d %q is of level %d", q.ID, q.Name, q.MinLevel, p.PatternID, p.ItemName, p.LevelReq)
				}
			}
		}
	}
	if scrolls == 0 {
		t.Fatal("no scroll among the tool's rewards")
	}
	t.Logf("%d quest steps filled, %d scrolls scribe their class's spells, %d level-20 choices", len(filled), scrolls, choices)
}
