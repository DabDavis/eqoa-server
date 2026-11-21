package entity

import (
	"strings"
	"testing"
	"time"
)

// timeWorld is a world whose game hour is *hour, with Southwatch Keep's kinds:
// a mercenary by day (by name), a watchman by night (by name), a knight by night
// by his row alone, and a warlord of no time (the control).
func timeWorld(t *testing.T, hour *float32) (w *World, merc, watch, knight, grug *NPC) {
	t.Helper()
	w = NewWorld()
	mk := func(id EntityID, name string, x float32) *NPC {
		n := &NPC{ServerID: id, Name: name, HP: 100, MaxHP: 100, X: x, SpawnX: x, Z: 20900, SpawnZ: 20900}
		w.NPCs[id] = n
		w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	merc = mk(111130, "Deathfist mercenary", 18273)
	watch = mk(111133, "skeletal watchman", 18190)
	knight = mk(111141, "Ser Thesrick", 18226)
	grug = mk(111142, "Grug Ironhead", 18232)
	w.times.hourAt = func(time.Time) float32 { return *hour }
	w.SetNPCTimes(map[string]bool{
		"deathfist mercenary":   true,
		"skeletal watchman":     false,
		RowKey(knight.ServerID): false,
		"ser thesrick":          true, // the row's key wins over the name's
	}, time.Now())
	return w, merc, watch, knight, grug
}

// tickTimes runs the time of day's system as the tick does once a second.
func tickTimes(w *World, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TickCount = npcTimeTicks
	w.npcTimeSystem(now)
}

// The turn moves each kind in and out: by day the mercenaries stand and the
// watchmen and the knight are out of every view, by night the other way round;
// the warlord stands throughout.
func TestNPCTimeTurnsAtDawnAndDusk(t *testing.T) {
	hour := float32(12)
	w, merc, watch, knight, grug := timeWorld(t, &hour)
	now := time.Now()
	if merc.hiddenAt(now) || !watch.hiddenAt(now) || !knight.hiddenAt(now) || grug.hiddenAt(now) {
		t.Fatalf("noon: merc hidden %v, watchman %v, knight %v, warlord %v", merc.hiddenAt(now), watch.hiddenAt(now),
			knight.hiddenAt(now), grug.hiddenAt(now))
	}
	if w.npcStateLocked(watch).Movement == "" || !strings.Contains(w.npcStateLocked(watch).Movement, "time of day") {
		t.Errorf("a held watchman's movement says %q", w.npcStateLocked(watch).Movement)
	}
	hour = 18.5
	tickTimes(w, now)
	if !merc.hiddenAt(now) || watch.hiddenAt(now) || knight.hiddenAt(now) || grug.hiddenAt(now) {
		t.Fatalf("dusk: merc hidden %v, watchman %v, knight %v, warlord %v", merc.hiddenAt(now), watch.hiddenAt(now),
			knight.hiddenAt(now), grug.hiddenAt(now))
	}
	hour = 6.1
	tickTimes(w, now)
	if merc.hiddenAt(now) || !watch.hiddenAt(now) || !knight.hiddenAt(now) || grug.hiddenAt(now) {
		t.Fatal("dawn did not turn them back")
	}
	if !Daylight(6.1) || Daylight(5.9) || Daylight(18.1) || !Daylight(17.9) {
		t.Fatal("day is not 6:00-18:00")
	}
}

// One out of its time in a fight stays until the fight is over, then leaves,
// back on its spawn and whole.
func TestNPCTimeWaitsOnAFight(t *testing.T) {
	hour := float32(12)
	w, merc, _, _, _ := timeWorld(t, &hour)
	now := time.Now()
	w.mu.Lock()
	merc.CurrentEnemy = 5000
	merc.HP = 40
	merc.X = 18300
	w.wakeNPCLocked(merc)
	w.mu.Unlock()
	hour = 20
	tickTimes(w, now)
	if merc.hiddenAt(now) || merc.HP != 40 {
		t.Fatal("a mercenary in a fight left at dusk")
	}
	if line := w.npcTimeLineLocked(merc, now); !strings.Contains(line, "its time is over") {
		t.Errorf("#stats says %q", line)
	}
	w.mu.Lock()
	merc.CurrentEnemy = 0
	w.wakeNPCLocked(merc)
	w.mu.Unlock()
	tickTimes(w, now)
	if !merc.hiddenAt(now) || merc.HP != merc.MaxHP || merc.X != merc.SpawnX {
		t.Fatalf("its fight over, held %v, HP %d, x %.0f", merc.hiddenAt(now), merc.HP, merc.X)
	}
}

// A dead one's corpse lies out its decay; its respawn outside its time comes
// back held, out of every view, and it comes in at its time.
func TestNPCTimeDeadWaitsForItsTime(t *testing.T) {
	hour := float32(12)
	w, merc, _, _, _ := timeWorld(t, &hour)
	now := time.Now()
	w.mu.Lock()
	merc.Dead, merc.HP = true, 0
	merc.DeathTime = now.Add(-time.Hour)
	merc.CorpseGone = now.Add(-time.Hour)
	merc.RespawnTime = time.Minute
	w.wakeNPCLocked(merc)
	w.mu.Unlock()
	hour = 19
	tickTimes(w, now)
	if !merc.Dead {
		t.Fatal("the turn raised a corpse")
	}
	w.mu.Lock()
	w.corpseSystem(now)
	w.mu.Unlock()
	if merc.Dead || !merc.hiddenAt(now) {
		t.Fatalf("respawned at night: dead %v, hidden %v", merc.Dead, merc.hiddenAt(now))
	}
	hour = 7
	tickTimes(w, now)
	if merc.hiddenAt(now) {
		t.Fatal("not back at dawn")
	}
	// in its time a respawn is as ever
	w.mu.Lock()
	merc.Dead, merc.HP = true, 0
	merc.DeathTime, merc.CorpseGone = now.Add(-time.Hour), now.Add(-time.Hour)
	w.wakeNPCLocked(merc)
	w.corpseSystem(now)
	w.mu.Unlock()
	if merc.Dead || merc.hiddenAt(now) {
		t.Fatal("a respawn in its time was held")
	}
}

// A reload: a row taken away lets its NPCs back, a row added holds them at once;
// a copy of a held name is never held.
func TestNPCTimeReload(t *testing.T) {
	hour := float32(12)
	w, merc, watch, _, grug := timeWorld(t, &hour)
	now := time.Now()
	copyOf := &NPC{ServerID: 900001, Name: "skeletal watchman", HP: 10, MaxHP: 10, Temporary: true, CopyOf: watch.ServerID}
	w.NPCs[copyOf.ServerID] = copyOf
	w.SetNPCTimes(map[string]bool{"grug ironhead": false, "skeletal watchman": false}, now)
	if merc.hiddenAt(now) || grug.hiddenAt(now) == false || !watch.hiddenAt(now) {
		t.Fatalf("after the reload: merc %v, warlord %v, watchman %v", merc.hiddenAt(now), grug.hiddenAt(now), watch.hiddenAt(now))
	}
	if copyOf.hiddenAt(now) {
		t.Fatal("a copy of a watchman was held")
	}
	w.SetNPCTimes(nil, now)
	if grug.hiddenAt(now) || watch.hiddenAt(now) {
		t.Fatal("no rows, still held")
	}
	// a row's NPC remade (the panel) is found again within the rescan
	w.SetNPCTimes(map[string]bool{"skeletal watchman": false}, now)
	fresh := &NPC{ServerID: watch.ServerID, Name: watch.Name, HP: 100, MaxHP: 100, X: watch.SpawnX, SpawnX: watch.SpawnX}
	w.NPCs[watch.ServerID] = fresh
	tickTimes(w, now.Add(npcTimeRescan))
	if !fresh.hiddenAt(now) {
		t.Fatal("a remade watchman stood by day")
	}
}
