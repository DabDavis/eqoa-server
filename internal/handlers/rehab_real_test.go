package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The rehabilitators on a real server's data (EQOA_REAL_DB names its
// database, a copy of which is opened) once tools/seed_rehabilitation.py has
// written it: every rehabilitator (Anja, Frejo), spawned as the server spawns
// a row (NPCFromRecord), talks: its talk opens the menu and the World will not
// have it attacked; the Rose of Rehabilitation is the tool's record (a rose's
// art, no trade); and every class's level 60 epic, read by the quest engine,
// gives a rose at its last step. Before the tool Frejo's row makes a creature
// (npc_type 0) and there is no rose: this fails. Skipped without a database.
func TestRealRehabilitators(t *testing.T) {
	db := realRehabDB(t)
	inv := database.NewInventoryStore(db)

	rose, ok, err := inv.ItemPatternByID(database.RoseOfRehabilitation)
	if err != nil || !ok {
		t.Errorf("no Rose of Rehabilitation (%v): tools/seed_rehabilitation.py --apply first", err)
	} else if rose.ItemName != database.RoseOfRehabilitationName || int32(rose.ItemIcon) != 759960290 || rose.Trade != 1 ||
		rose.ItemCost != 10_000_000 || rose.EquipSlot != -1 {
		t.Errorf("the rose's record: %q icon %d trade %d value %d slot %d", rose.ItemName, rose.ItemIcon, rose.Trade,
			rose.ItemCost, rose.EquipSlot)
	}

	recs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	tw := newTalkWorld(t, 0, 0)
	tw.h.rehab = NewRehab(db, tw.w, inv, database.NewSpellbookStore(db), nil, database.NewSQLiteCharacterStore(db))
	tw.p.Level, tw.p.BaseStr = 60, 130
	tw.p.RecalcStats(true) // alive: a dead player engages nothing
	if tw.p.IsDead() {
		t.Fatal("the player is dead")
	}
	slot, names := 1, map[string]bool{}
	for _, rec := range recs {
		if !IsRehabilitator(rec.NPCName) {
			continue
		}
		npc := entity.NPCFromRecord(rec, 0, nil)
		names[npc.Name] = true
		tw.p.X, tw.p.Y, tw.p.Z, tw.p.World = npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.World
		id := tw.add(slot, npc)
		slot++
		tw.talk(id)
		if m := questMsgs(tw.s); !IsRehabBox(boxID(m)) {
			t.Errorf("%s (npc_type %d) opens no menu: %q", npc.Name, rec.NPCType, m)
		}
		if tw.w.EngageAttack(tw.s, id, entity.Swing{Reach: 50}, time.Now()) {
			t.Errorf("%s (npc_type %d) can be attacked", npc.Name, rec.NPCType)
		}
	}
	if !names["Rehabilitator Anja"] || !names["Rehabilitator Frejo"] {
		t.Errorf("the rehabilitators: %v", names)
	}

	qe, err := NewQuestEngine(db, nil, nil, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	epic := regexp.MustCompile(`^60/[^/]+/reward-[a-z]+$`)
	n := 0
	for _, qu := range qe.cat.all {
		if !epic.MatchString(importKey(qu.Source)) {
			continue
		}
		n++
		last := qu.Steps[len(qu.Steps)-1]
		given := false
		for _, it := range last.RewardItems {
			given = given || it.PatternID == database.RoseOfRehabilitation
		}
		if !given || !qu.Enabled {
			t.Errorf("%s (%d, enabled %v) gives no rose at its last step: %v", qu.Name, qu.ID, qu.Enabled, last.RewardItems)
		}
	}
	if n != 23 {
		t.Errorf("%d epic reward quests, want 23", n)
	}
}

// realRehabDB is a copy of the database EQOA_REAL_DB names, opened (the test
// skipped without one).
func realRehabDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "rehab.db")
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
	t.Cleanup(func() { db.Close() })
	return db
}

// Every path pick of a real server's quests chosen again on the spot, through
// a rehabilitator's boxes: a new character of the quest's class, its row done
// with the first option picked at each of its choices and what that option
// gives held (its spells, its scrolls' spells, its items in the pack), changes
// each step for the step's next option. After each: the pick kept, the row
// still done, the new option's spells known and its items held, the old
// option's spells gone (but one a merchant sells as a scroll) and its items
// taken back, an item both give held once. Skipped without a database.
func TestRealPathsChosenAgain(t *testing.T) {
	db := realRehabDB(t)
	inv := database.NewInventoryStore(db)
	book := database.NewSpellbookStore(db)
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Rehabtester", AccountID: 1, Level: 60}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	sid := rec.ServerID
	tw := newTalkWorld(t, 0, 0)
	tw.p.CharServerID, tw.p.Level, tw.p.BaseStr = sid, 60, 130
	tw.p.RecalcStats(true)
	qe, err := NewQuestEngine(db, tw.w, tw.mgr, inv, book)
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	cm := NewClassMasteryHandler(tw.mgr)
	cm.SetGameWorld(tw.w)
	cm.SetStore(store)
	cm.SetSpellbook(book)
	r := NewRehab(db, tw.w, inv, book, cm, store)
	r.quests = func() *QuestEngine { return qe }
	tw.h.rehab = r
	rehab := tw.add(1, &entity.NPC{ServerID: 9_900_001, Name: "Rehabilitator Tester", NPCType: 0x80, SpawnY: 54})

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	scrollOf := func(pid int) (int, bool) { // a scroll's spell: the one spell of its name
		var name string
		var icon int64
		if db.QueryRow(`SELECT itemname, itemicon FROM itempattern WHERE patternid = ?`, pid).Scan(&name, &icon) != nil ||
			int32(icon) != scrollIcon {
			return 0, false
		}
		var ids []int
		rows, err := db.Query(`SELECT spellid FROM spellpattern WHERE lower(spellname) = lower(?)`, name)
		if err != nil {
			return 0, false
		}
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) != 1 {
			return 0, false
		}
		return ids[0], true
	}
	sold := map[int]bool{} // the spells a merchant sells as a scroll
	icon := int32(scrollIcon)
	if rows, err := db.Query(`SELECT s.spellid FROM spellpattern s JOIN itempattern p ON lower(p.itemname) = lower(s.spellname)
		JOIN npcitem n ON n.patternid = p.patternid WHERE p.itemicon IN (?, ?)`, int64(icon), int64(uint32(icon))); err == nil {
		for rows.Next() {
			var id int
			rows.Scan(&id)
			sold[id] = true
		}
		rows.Close()
	}
	knows := func(id int) bool {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM spells WHERE serverid = ? AND spellid = ?`, sid, id).Scan(&n)
		return n > 0
	}
	held := func(pid int) int {
		var n int
		db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, sid, pid).Scan(&n)
		return n
	}
	// gives is what an option gives: its spells (its scrolls' too) and its
	// items, scrolls among them
	gives := func(o database.StepOption) (spells []int, items map[int]int) {
		spells = append(spells, o.Reward.Spells...)
		items = map[int]int{}
		for _, it := range o.Reward.Items {
			items[it.PatternID] += it.Count
			if sp, ok := scrollOf(it.PatternID); ok {
				spells = append(spells, sp)
			}
		}
		return spells, items
	}
	classOf := map[string]int{"warrior": 0, "ranger": 1, "paladin": 2, "shadowknight": 3, "monk": 4, "bard": 5, "rogue": 6,
		"druid": 7, "shaman": 8, "cleric": 9, "magician": 10, "necromancer": 11, "enchanter": 12, "wizard": 13, "alchemist": 14}

	steps, changed, failures := 0, 0, 0
	fail := func(format string, args ...any) {
		t.Helper()
		if failures++; failures <= 25 {
			t.Errorf(format, args...)
		}
	}
	for _, qu := range qe.cat.all {
		level, _, ok := pathOf(qu)
		if !ok {
			continue
		}
		key := importKey(qu.Source)
		var class string
		if m := reL20Key.FindStringSubmatch(key); m != nil {
			class = m[1]
		} else if m := rePathKey.FindStringSubmatch(key); m != nil {
			class = m[3]
		}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			var live []int // the step's options that do something
			for k, o := range st.Choices.Options {
				if !o.Stay {
					live = append(live, k)
				}
			}
			if len(live) < 2 {
				continue
			}
			steps++
			from, to := live[0], live[1]

			// the character: this quest done, the first option picked at each
			// choice, what those give held
			for _, tbl := range []string{"charquests", "spells", "charinventory", "charresets"} {
				exec(`DELETE FROM `+tbl+` WHERE serverid = ?`, sid)
			}
			tw.p.Class = classOf[class]
			picks := ""
			order := 0
			for j := range qu.Steps {
				o := qu.Steps[j].Choices.Options
				if len(o) == 0 {
					continue
				}
				k := 0
				for k < len(o) && o[k].Stay {
					k++
				}
				if k == len(o) {
					continue
				}
				picks = database.WithPick(picks, qu.Steps[j].Step, k)
				spells, items := gives(o[k])
				for _, sp := range spells {
					if !knows(sp) {
						exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
							VALUES (?, ?, ?, 0, -1, 0, -1)`, sid, sp, order)
						order++
					}
				}
				for pid, n := range items {
					if _, scroll := scrollOf(pid); scroll {
						continue // used: its spell is in the book
					}
					exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
						VALUES (?, ?, 0, 0, ?, -1, 0, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))`,
						sid, n, pid, sid)
				}
			}
			exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, picks) VALUES (?, ?, 0, 0, 1, 0, ?)`, sid, qu.ID, picks)
			qe.ReloadCharacter(sid)
			rows, err := inv.GetCharacterItems(sid)
			if err != nil {
				t.Fatal(err)
			}
			tw.s.SetInventoryList(newInvList(rows))
			questMsgs(tw.s)

			// the boxes: the menu's path, the step's pick, the next option, yes
			opts := st.Choices.Options
			choose := func(m []string, label string) bool {
				t.Helper()
				_, answers := asked(m)
				for k, a := range answers {
					if a == clipRunes(label, questAnswerChars) {
						tw.pick(boxID(m), byte(k))
						return true
					}
				}
				fail("%s (%d) step %d: no answer %q in %q", qu.Name, qu.ID, st.Step, label, m)
				return false
			}
			tw.talk(rehab)
			m := questMsgs(tw.s)
			if !choose(m, "Choose a path again (free)") {
				continue
			}
			if m = questMsgs(tw.s); !choose(m, fmt.Sprintf("Level %d: %s", level, opts[from].Answer)) {
				continue
			}
			if m = questMsgs(tw.s); !choose(m, opts[to].Answer) {
				continue
			}
			if m = questMsgs(tw.s); !choose(m, rehabChangeYes) {
				continue
			}
			questMsgs(tw.s)

			// what changed
			var picksNow string
			var done int
			db.QueryRow(`SELECT picks, done FROM charquests WHERE serverid = ? AND questid = ?`, sid, qu.ID).Scan(&picksNow, &done)
			if got, ok := database.ParsePicks(picksNow)[st.Step]; !ok || got != to || done != 1 {
				fail("%s (%d) step %d: picks %q done %d, want %d kept, done", qu.Name, qu.ID, st.Step, picksNow, done, to)
				continue
			}
			oldSpells, oldItems := gives(opts[from])
			newSpells, newItems := gives(opts[to])
			newSet := map[int]bool{}
			for _, sp := range newSpells {
				newSet[sp] = true
			}
			for _, sp := range oldSpells {
				if !newSet[sp] && !sold[sp] && knows(sp) {
					fail("%s (%d) step %d: the old pick's spell %d stays", qu.Name, qu.ID, st.Step, sp)
				}
			}
			for pid, n := range newItems {
				if _, scroll := scrollOf(pid); !scroll && held(pid) != n {
					fail("%s (%d) step %d: the new pick's item %d held %d, want %d", qu.Name, qu.ID, st.Step, pid, held(pid), n)
				} else if scroll && held(pid) != n && oldItems[pid] == 0 {
					fail("%s (%d) step %d: the new pick's scroll %d held %d, want %d", qu.Name, qu.ID, st.Step, pid, held(pid), n)
				}
			}
			for _, sp := range opts[to].Reward.Spells {
				if !knows(sp) {
					fail("%s (%d) step %d: the new pick's spell %d not known", qu.Name, qu.ID, st.Step, sp)
				}
			}
			for pid := range oldItems {
				if _, scroll := scrollOf(pid); !scroll && newItems[pid] == 0 && held(pid) != 0 {
					fail("%s (%d) step %d: the old pick's item %d still held (%d)", qu.Name, qu.ID, st.Step, pid, held(pid))
				}
			}
			changed++
		}
	}
	t.Logf("%d path steps, %d changed on the spot", steps, changed)
	if steps == 0 || changed != steps {
		t.Errorf("%d of %d path steps changed", changed, steps)
	}
}
