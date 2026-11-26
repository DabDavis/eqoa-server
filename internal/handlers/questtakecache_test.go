package handlers

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questtakecache_test.go — the marks kept from pass to pass
// (questtakecache.go) are the marks a pass that keeps nothing makes.

// passBodiesLocked is the character's marks body (encodeQuestMarks) and head
// marks' line, as a pass with what was kept (kept) or one keeping nothing and
// reading everything again. Must be called with q.mu held.
func (f *questFixture) passBodiesLocked(kept bool) (string, string) {
	q := f.qe
	q.noTakeCache, q.noPackCache = !kept, !kept
	defer func() { q.noTakeCache, q.noPackCache = false, false }()
	c := q.namedLocked(f.sid, f.tw.p.Name)
	end := q.beginPassLocked(f.tw.p)
	defer end()
	marks, ok := q.marksLocked(q.lookAt(f.tw.s), f.tw.p, c, time.Now())
	if !ok {
		f.t.Fatal("the player is not in the world")
	}
	return string(encodeQuestMarks(marks)), headMarksText(q.headMarksLocked(q.lookAt(f.tw.s), f.tw.p, c))
}

// takeCacheScenario walks a character through every kind of change the kept
// marks hang on, with q.cacheIgnore = ignore, and returns the steps after
// which a kept pass differed from a fresh one ("" none). Each step's change
// moves the fresh set (checked: the scenario would show nothing otherwise).
func takeCacheScenario(t *testing.T, ignore uint8) []string {
	defer database.SetInventorySettle(database.SetInventorySettle(0)) // a kept read stands on its version alone
	headMode := questHeadMode.Load()
	questHeadMode.Store(headAll)
	defer questHeadMode.Store(headMode)
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.qe.cacheIgnore = ignore
	at := func(slot int, id entity.EntityID, name string, dx float32) {
		f.tw.add(slot, &entity.NPC{ServerID: id, Name: name, SpawnX: 25403 + dx, SpawnY: 54, SpawnZ: 15725})
	}
	at(1, 701, "Malsis", 1)
	at(2, 702, "Kellina", 2)
	at(3, 703, "Ander", 30)
	at(4, 704, "Brin", 60)
	at(5, 705, "Cora", 90)
	token := f.pattern("guild token", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, requires, classes, factionreq, offer) VALUES
		(9101, 'Level seven', 1, 'Ander', 7, 20, '', '', '', 'Ander: At seven.'),
		(9102, 'First errand', 1, 'Ander', 1, 20, '', '', '', 'Ander: Run along.'),
		(9103, 'Second errand', 1, 'Ander', 1, 20, '9102', '', '', 'Ander: Again.'),
		(9104, 'Hand it over', 1, 'Brin', 1, 20, '', '', '', 'Brin: Have you one?'),
		(9105, 'Standing', 1, 'Brin', 1, 20, '', '', 'Test Guild:500', 'Brin: Friends only.'),
		(9107, 'Knights only', 1, 'Cora', 1, 20, '', '3', '', 'Cora: Knight.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(9101, 1, 'talk', 'Kellina', 'Seven.'), (9102, 1, 'talk', 'Kellina', 'Errand.'), (9103, 1, 'talk', 'Kellina', 'Errand two.'),
		(9105, 1, 'talk', 'Kellina', 'Friends.'), (9107, 1, 'talk', 'Kellina', 'Knights.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, events)
		VALUES (9104, 1, 'give', 'Brin', '%d*1', 'A token.', '[{"on":"start","do":"dostep"}]')`, token))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	var bad []string
	lastFresh := ""
	step := func(name string, change func(), moves bool) {
		t.Helper()
		change()
		q := f.qe
		q.mu.Lock()
		kb, kh := f.passBodiesLocked(true)
		fb, fh := f.passBodiesLocked(false)
		kb2, kh2 := f.passBodiesLocked(true) // the kept again, now warm
		q.mu.Unlock()
		if kb != fb || kh != fh || kb2 != fb || kh2 != fh {
			bad = append(bad, name)
		}
		if moves && fb+fh == lastFresh {
			t.Fatalf("%s: the fresh set did not change: the step shows nothing", name)
		}
		lastFresh = fb + fh
	}
	set := func(id int, action string) func() {
		return func() {
			if _, err := f.qe.SetCharacterQuest(f.sid, id, action, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	step("start", func() {}, true)
	step("level up", func() { f.tw.p.Level = 7 }, true)
	step("quest taken", set(9102, QuestGive), true)
	step("quest finished", set(9102, QuestComplete), true)
	step("next quest taken", set(9103, QuestGive), true)
	step("quest dropped", set(9103, QuestReset), true)
	step("item gained", func() { f.own(token, 1) }, true)
	step("item equipped", func() {
		var id int
		f.db.QueryRow(`SELECT itemid FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, token).Scan(&id)
		if err := f.inv.SetEquipLoc(f.sid, id, 4); err != nil {
			t.Fatal(err)
		}
	}, false)
	step("item lost", func() { f.exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, token) }, true)
	step("item gained by SQL", func() {
		f.exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, 1, 0, 0, ?, -1, 0, 0)`, f.sid, token)
	}, true)
	step("standing raised", func() {
		f.tw.p.Factions = map[string]int{"Test Guild": 600}
	}, true)
	step("class changed", func() { f.tw.p.Class = 3 }, true)
	step("race changed", func() { f.tw.p.Race = 4 }, false)
	step("catalog reloaded", func() {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, offer) VALUES (9106, 'Late addition', 1, 'Ander', 1, 20, 'Ander: New.')`)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (9106, 1, 'talk', 'Kellina', 'New.')`)
		if err := f.qe.Reload(); err != nil {
			t.Fatal(err)
		}
	}, true)
	step("quest held", func() {
		f.qe.mu.Lock()
		f.qe.held = map[int]bool{9106: true}
		f.qe.mu.Unlock()
	}, true)
	step("level down", func() { f.tw.p.Level = 5 }, true)
	return bad
}

// Every pass's marks are the marks a pass keeping nothing makes, through a
// level up, a quest taken, finished, taken and dropped, an item gained (by the
// inventory's store and by bare SQL), worn and lost, a standing, a class, a
// race, a reload and a hold. The controls: with any one of the invalidations
// switched off, a kept pass differs at the step that needs it.
func TestQuestTakeCacheMatchesAFreshPass(t *testing.T) {
	if bad := takeCacheScenario(t, 0); len(bad) > 0 {
		t.Fatalf("kept passes differ from fresh ones after: %s", strings.Join(bad, ", "))
	}
	for _, c := range []struct {
		name   string
		ignore uint8
		step   string
	}{
		{"level", ignoreLevel, "level up"},
		{"quest rows", ignoreQuestRows, "quest finished"},
		{"catalog", ignoreCatalog, "catalog reloaded"},
		{"pack version", ignorePackVersion, "item gained"},
	} {
		t.Run("control without "+c.name, func(t *testing.T) {
			bad := takeCacheScenario(t, c.ignore)
			found := false
			for _, b := range bad {
				found = found || b == c.step
			}
			if !found {
				t.Fatalf("without the %s invalidation the kept passes differ after %q, want %q among them", c.name, bad, c.step)
			}
		})
	}
}

// A mark's place key rounds as "%.0f" did: the same places are one key, the
// places that text told apart are told apart (-0 from 0, half to even).
func TestMarkSpotKeyIsTheTextsKey(t *testing.T) {
	vals := []float32{0, float32(math.Copysign(0, -1)), 0.4, -0.4, 0.5, -0.5, 1.5, 2.5, -2.5, 25403.5, 25404.5, 15725.49,
		-15725.5, 1e7 + 0.5, 3.4e38, -3.4e38, float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()), 0.49999997}
	for i := -2000; i <= 2000; i++ {
		vals = append(vals, float32(i)/4)
	}
	text := func(x, z float32) string { return fmt.Sprintf("%d %.0f %.0f", 3, x, z) }
	for _, a := range vals {
		for _, b := range []float32{0, 0.5, -0.5, 7.5} {
			for _, c := range vals[:24] {
				ka, kc := markSpotOf(entity.NPCSpot{World: 3, X: a, Z: b}), markSpotOf(entity.NPCSpot{World: 3, X: c, Z: b})
				if (ka == kc) != (text(a, b) == text(c, b)) {
					t.Fatalf("%v and %v: keys equal %v, texts %q %q", a, c, ka == kc, text(a, b), text(c, b))
				}
			}
		}
	}
}

// newMarksBench is a database of n quests over n/5 givers for levels 1 to 60,
// as the live catalog has them (about 2,470): every fifth requires the one
// before, one in ten a hand-in done on the answer, one in twenty a standing,
// one in four a class's; and chars characters at level 30, each having
// finished a third of the quests below its level, with three under way and
// 60 rows in its pack. It returns the engine and each character's player,
// session and quests.
func newMarksBench(tb testing.TB, n, chars int) (*QuestEngine, []*entity.Player, []*session.Session, []*questChar) {
	tb.Helper()
	db, err := database.NewSQLite(tb.TempDir() + "/marks.db")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			tb.Fatal(err)
		}
	}
	gw := entity.NewWorld()
	mgr := session.NewManager()
	gw.SessionManager = mgr
	inv := database.NewInventoryStore(db)
	tx, _ := db.Begin()
	var items []int
	for i := 0; i < 200; i++ {
		res, err := tx.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (10, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`, fmt.Sprintf("bench item %d", i))
		if err != nil {
			tb.Fatal(err)
		}
		id, _ := res.LastInsertId()
		items = append(items, int(id))
	}
	givers := max(n/5, 1)
	for i := 0; i < n; i++ {
		id := 20000 + i
		lv := 1 + i*60/n
		req, classes, faction := "", "", ""
		if i%5 == 4 {
			req = fmt.Sprint(id - 1)
		}
		if i%4 == 3 {
			classes = fmt.Sprint(i % 15)
		}
		if i%20 == 7 {
			faction = "Bench Guild:100"
		}
		g := fmt.Sprintf("Giver %d", i%givers)
		if _, err := tx.Exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, requires, classes, factionreq, offer)
			VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`, id, fmt.Sprintf("Bench quest %d", i), g, lv, lv+10, req, classes, faction,
			g+": a task for you."); err != nil {
			tb.Fatal(err)
		}
		if i%10 == 5 {
			_, err = tx.Exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, events)
				VALUES (?, 1, 'give', ?, ?, 'Hand it in.', '[{"on":"start","do":"dostep"}]')`, id, g, fmt.Sprintf("%d*1", items[i%len(items)]))
		} else {
			_, err = tx.Exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', ?, 'Talk.'),
				(?, 2, 'kill', 'Bench Rat', 'Kill rats.')`, id, fmt.Sprintf("Giver %d", (i+1)%givers), id)
		}
		if err != nil {
			tb.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	for g := 0; g < givers; g++ {
		id := entity.EntityID(50000 + g)
		x, z := float32(20000+(g%40)*150), float32(15000+(g/40)*150)
		gw.NPCs[id] = &entity.NPC{ServerID: id, Name: fmt.Sprintf("Giver %d", g), SpawnX: x, SpawnZ: z, X: x, Z: z}
	}
	for r := 0; r < 50; r++ {
		id := entity.EntityID(60000 + r)
		x := float32(21000 + r*40)
		gw.NPCs[id] = &entity.NPC{ServerID: id, Name: "Bench Rat", SpawnX: x, SpawnZ: 16000, X: x, Z: 16000, Level: 3}
	}
	qe, err := NewQuestEngine(db, gw, mgr, inv, database.NewSpellbookStore(db))
	if err != nil {
		tb.Fatal(err)
	}
	var ps []*entity.Player
	var ss []*session.Session
	var cs []*questChar
	for k := 0; k < chars; k++ {
		rec := &database.CharacterRecord{CharName: fmt.Sprintf("Bencher%d", k), AccountID: 1}
		if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
			tb.Fatal(err)
		}
		tx, _ := db.Begin()
		for i := 0; i < 60; i++ {
			if _, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
				VALUES (?, 1, 0, 0, ?, ?, 0, ?)`, rec.ServerID, items[(i*7+k)%len(items)], map[bool]int{true: i % 10, false: -1}[i < 8], i); err != nil {
				tb.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			tb.Fatal(err)
		}
		p := &entity.Player{ServerID: entity.EntityID(0x7000000 + k), CharServerID: rec.ServerID, Name: rec.CharName, Level: 30,
			Class: k % 15, Race: k % 10, HP: 100, X: float32(20000 + k*300), Z: 15500, Factions: map[string]int{"Bench Guild": 50 * (k % 4)}}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 1, byte(k)), Port: 7000 + k}, uint32(100+k), 1, 1, 2, true)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		gw.Players[p.ServerID] = p
		for i := 0; i < n/2; i++ { // the quests below its level
			id := 20000 + i
			switch {
			case i%3 == 0:
				exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES (?, ?, 0, 1)`, rec.ServerID, id)
			case i%97 == 1 && i < 300:
				exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES (?, ?, 2, 0)`, rec.ServerID, id)
			}
		}
		qe.mu.Lock()
		c := qe.namedLocked(rec.ServerID, rec.CharName)
		qe.mu.Unlock()
		ps, ss, cs = append(ps, p), append(ss, s), append(cs, c)
	}
	return qe, ps, ss, cs
}

// BenchmarkQuestMarksSync is one sync of the marks (syncMarks' passes under
// one lock) over 34 characters and a catalog of the live one's size: "fresh"
// keeping nothing from pass to pass (every quest asked, the pack read: as
// before questtakecache.go), "kept" as the server runs.
func BenchmarkQuestMarksSync(b *testing.B) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	qe, ps, ss, cs := newMarksBench(b, 2470, 34)
	for _, mode := range []string{"fresh", "kept"} {
		b.Run(mode, func(b *testing.B) {
			qe.mu.Lock()
			qe.noTakeCache, qe.noPackCache = mode == "fresh", mode == "fresh"
			qe.mu.Unlock()
			sync := func() {
				qe.mu.Lock()
				defer qe.mu.Unlock()
				defer qe.beginSyncLocked()()
				for k := range ps {
					cs[k].marksSent = "" // each pass worked out to its send, as a changed set is
					qe.sendMarksLocked(ss[k], ps[k], cs[k])
				}
			}
			sync()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sync()
				for _, s := range ss {
					s.Queue().GetPending()
				}
			}
		})
	}
}

// The bench's characters' kept passes are the fresh ones (the bench measures
// the same work).
func TestQuestMarksBenchKeptIsFresh(t *testing.T) {
	qe, ps, ss, cs := newMarksBench(t, 600, 6)
	qe.mu.Lock()
	defer qe.mu.Unlock()
	marks := 0
	for k := range ps {
		var got [3]string
		for i, kept := range []bool{true, false, true} {
			qe.noTakeCache, qe.noPackCache = !kept, !kept
			end := qe.beginPassLocked(ps[k])
			m, ok := qe.marksLocked(qe.lookAt(ss[k]), ps[k], cs[k], time.Now())
			end()
			if !ok {
				t.Fatalf("character %d not in the world", k)
			}
			got[i] = string(encodeQuestMarks(m))
			marks += len(m)
		}
		qe.noTakeCache, qe.noPackCache = false, false
		if got[0] != got[1] || got[2] != got[1] {
			t.Fatalf("character %d: kept and fresh passes differ", k)
		}
	}
	if marks == 0 {
		t.Fatal("no marks at all: the bench measures nothing")
	}
}
