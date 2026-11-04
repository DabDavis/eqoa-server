package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// The tick timer (the user, 2026-11-03: a per-system tick timer on the live
// server; until then nothing measured a tick, and the one profile of the
// 60,793-NPC world is claude-notes/server-tick-profile-20261103.md). Each of
// tickLocked's systems is a lap of a stopwatch started as the tick asks for
// the world's lock (the first lap is the wait for it, a handler holding it),
// the work after the lock is one more, and the listener's whole turn (the
// world's tick, then the sessions' sends: udp's tickAllSessions) is around
// them. Every tickReportEvery the figures go to the log in one line
// ("Tick: ..."); a turn past tickSlow goes there at once with its slowest
// laps, at most one such line in tickSlowEvery (the others counted). #ticks
// shows the last report in the game.

// A tick's laps, in tickLocked's order.
const (
	tsLockWait = iota // waiting for the world's lock
	tsAggro
	tsGuards
	tsAI
	tsTemplates
	tsHome
	tsAttack
	tsPets
	tsDuels
	tsCasts
	tsEffects
	tsConsumables
	tsPotions
	tsCorpses
	tsRoam
	tsPaths
	tsRespawns
	tsRegen
	tsVampires
	tsNPCRegen
	tsBreath
	tsSilentAir
	tsStateSync
	tsEntityChannels
	tsStatChannels
	tsBuffChannels
	tsGuildAnnounce
	tsVisibility
	tsGroupSync
	tsPowerSync
	tsVoiceSync
	tsAfterLock // after the lock: duels' purses, respawns, gates, travels, hands back, corpses gone
	tsCount
)

var tickLapNames = [tsCount]string{
	"lockwait", "aggro", "guards", "ai", "templates", "home", "attack", "pets", "duels", "casts",
	"effects", "consumables", "potions", "corpses", "roam", "paths", "respawns", "regen",
	"vampires", "npcregen", "breath", "silentair", "statesync", "entitychannels", "statchannels",
	"buffchannels", "guildannounce", "visibility", "groupsync", "powersync", "voicesync", "afterlock",
}

const (
	tickReportEvery = time.Minute           // a report line in the log
	tickSlow        = 50 * time.Millisecond // a turn this long is logged at once
	tickSlowEvery   = 10 * time.Second      // at most one slow turn's line in this
	tickLapShown    = 50 * time.Microsecond // a lap whose most is below this is left out
)

// durStat is a sum and a most of durations.
type durStat struct{ sum, max time.Duration }

func (d *durStat) add(x time.Duration) {
	d.sum += x
	d.max = max(d.max, x)
}

// tickTimer is the tick timer's state (the file's doc): the tick goroutine's
// alone but for the last report, which #ticks reads under mu.
type tickTimer struct {
	// this tick's laps, and what it saw at its end under the lock
	last          time.Time
	lap           [tsCount]time.Duration
	npcs, players int
	sets          [4]int // fights, corpses, regeneration, guards (npcsets.go)

	win tickWindow

	mu     sync.Mutex
	report []string
}

// tickWindow is the figures since the last report.
type tickWindow struct {
	from, lastStart time.Time
	interval        time.Duration
	turns           int
	laps            [tsCount]durStat
	locked          durStat // the systems under the lock
	world           durStat // the world's Tick, lock wait and after-lock work in
	sessions        durStat // the sessions' sends after it
	turn            durStat // the whole turn
	gap             time.Duration
	over, slow      int
	slowLogged      time.Time
	slowQuiet       int // slow turns not logged since the last slow turn's line
}

// start starts a tick's stopwatch at at.
func (t *tickTimer) start(at time.Time) {
	t.last = at
	t.lap = [tsCount]time.Duration{}
}

// mark ends the lap since the last mark as lap's.
func (t *tickTimer) mark(lap int) {
	now := time.Now()
	t.lap[lap] += now.Sub(t.last)
	t.last = now
}

// tickSeenLocked notes what the tick saw at its end: the NPCs, the players
// and the sets' sizes. Must be called with the lock held.
func (w *World) tickSeenLocked() {
	t := &w.ticks
	t.npcs, t.players = len(w.NPCs), len(w.Players)
	t.sets = [4]int{len(w.sets.ai.m), len(w.sets.corpse.m), len(w.sets.regen.m), len(w.sets.guard.m)}
}

// TickTurn is the listener's word on a tick's turn: when it began, when the
// world's Tick returned, when the sessions' sends were done, and the ticker's
// interval (0 for none). The tick goroutine calls it after each Tick (the
// file's doc).
func (w *World) TickTurn(start, worldDone, end time.Time, interval time.Duration) {
	t := &w.ticks
	win := &t.win
	if win.from.IsZero() {
		win.from = start
	}
	if !win.lastStart.IsZero() {
		win.gap = max(win.gap, start.Sub(win.lastStart))
	}
	win.lastStart, win.interval = start, interval
	win.turns++
	var locked time.Duration
	for i, d := range t.lap {
		win.laps[i].add(d)
		if i != tsLockWait && i != tsAfterLock {
			locked += d
		}
	}
	win.locked.add(locked)
	world, sessions, turn := worldDone.Sub(start), end.Sub(worldDone), end.Sub(start)
	win.world.add(world)
	win.sessions.add(sessions)
	win.turn.add(turn)
	if interval > 0 && turn > interval {
		win.over++
	}
	if turn >= tickSlow {
		win.slow++
		if end.Sub(win.slowLogged) >= tickSlowEvery {
			quiet := ""
			if win.slowQuiet > 0 {
				quiet = fmt.Sprintf(" (and %d slow turns since the last line)", win.slowQuiet)
			}
			log.Printf("Tick: a slow turn, %s (the world %s, of it %s waiting for the lock; the sessions %s): %s%s",
				msText(turn), msText(world), msText(t.lap[tsLockWait]), msText(sessions), t.slowestLaps(6), quiet)
			win.slowLogged, win.slowQuiet = end, 0
		} else {
			win.slowQuiet++
		}
	}
	if end.Sub(win.from) >= tickReportEvery {
		lines := t.reportLines(end)
		log.Printf("Tick: %s", strings.Join(lines, "; "))
		t.mu.Lock()
		t.report = lines
		t.mu.Unlock()
		t.win = tickWindow{from: end, lastStart: win.lastStart, slowLogged: win.slowLogged, slowQuiet: win.slowQuiet}
	}
}

// TickReport is the tick timer's last report, a line a part, or nil before
// the first (a minute after the start).
func (w *World) TickReport() []string {
	w.ticks.mu.Lock()
	defer w.ticks.mu.Unlock()
	return append([]string(nil), w.ticks.report...)
}

// reportLines is the report of the figures since from, ending at end.
func (t *tickTimer) reportLines(end time.Time) []string {
	win := &t.win
	n := time.Duration(max(win.turns, 1))
	over := fmt.Sprintf("%d over the %s tick", win.over, win.interval)
	if win.interval <= 0 {
		over = fmt.Sprintf("%d over the tick", win.over)
	}
	lines := []string{
		fmt.Sprintf("%d turns in %.1f s: %s a turn on average, %s at most",
			win.turns, end.Sub(win.from).Seconds(), msText(win.turn.sum/n), msText(win.turn.max)),
		fmt.Sprintf("the world %s/%s (under the lock %s/%s, waiting for it %s/%s), the sessions %s/%s",
			msText(win.world.sum/n), msText(win.world.max), msText(win.locked.sum/n), msText(win.locked.max),
			msText(win.laps[tsLockWait].sum/n), msText(win.laps[tsLockWait].max),
			msText(win.sessions.sum/n), msText(win.sessions.max)),
		fmt.Sprintf("%s, %d past %s, the longest gap between turns %s", over, win.slow, tickSlow, msText(win.gap)),
		fmt.Sprintf("%d NPCs, %d players; in the sets: fights %d, corpses %d, regeneration %d, guards %d",
			t.npcs, t.players, t.sets[0], t.sets[1], t.sets[2], t.sets[3]),
	}
	type lapRow struct {
		name     string
		avg, max time.Duration
	}
	var rows []lapRow
	for i, d := range win.laps {
		if i == tsLockWait || d.max < tickLapShown {
			continue
		}
		rows = append(rows, lapRow{tickLapNames[i], d.sum / n, d.max})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].avg > rows[j].avg })
	for len(rows) > 0 {
		k := min(len(rows), 6)
		parts := make([]string, k)
		for i, r := range rows[:k] {
			parts[i] = fmt.Sprintf("%s %.2f/%.2f", r.name, float64(r.avg)/1e6, float64(r.max)/1e6)
		}
		lines = append(lines, "systems (ms, average/most): "+strings.Join(parts, ", "))
		rows = rows[k:]
	}
	return lines
}

// slowestLaps is this tick's k longest laps, longest first.
func (t *tickTimer) slowestLaps(k int) string {
	order := make([]int, 0, tsCount)
	for i := range t.lap {
		order = append(order, i)
	}
	sort.SliceStable(order, func(a, b int) bool { return t.lap[order[a]] > t.lap[order[b]] })
	parts := make([]string, 0, k)
	for _, i := range order[:min(k, len(order))] {
		parts = append(parts, fmt.Sprintf("%s %.2f", tickLapNames[i], float64(t.lap[i])/1e6))
	}
	return strings.Join(parts, ", ")
}

// msText is d in milliseconds, to a hundredth.
func msText(d time.Duration) string { return fmt.Sprintf("%.2f ms", float64(d)/1e6) }
