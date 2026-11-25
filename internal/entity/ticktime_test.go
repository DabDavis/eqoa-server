package entity

import (
	"strings"
	"testing"
	"time"
)

func TestSlowPostCallbackSurvivesUntilMinuteReport(t *testing.T) {
	w := NewWorld()
	start := time.Now()
	w.ticks.start(start.Add(-150 * time.Millisecond))
	w.ticks.markPost("progress saves")
	w.ticks.markPost("ambient dialogue")
	if w.ticks.lap[tsAfterLock] < 150*time.Millisecond {
		t.Fatal("callback time missing from afterlock")
	}
	w.TickTurn(start, start, start, 100*time.Millisecond)
	next := start.Add(time.Minute)
	w.ticks.start(next)
	w.TickTurn(next, next, next, 100*time.Millisecond)
	if text := strings.Join(w.TickReport(), "\n"); !strings.Contains(text, "longest afterlock callback: progress saves") {
		t.Fatalf("missing slow callback in %s", text)
	}
	if w.ticks.win.longestPost.took != 0 || w.ticks.longestPost.took != 0 {
		t.Fatal("stale callback carried into next sample")
	}
}

// A minute of turns makes one report: the turns, the slow one counted, the
// longest gap (a turn missed), and the systems by their average, longest
// first.
func TestTickTimerReportsAMinute(t *testing.T) {
	w := NewWorld()
	base := time.Unix(1_800_000_000, 0)
	for i := 0; i <= 600; i++ {
		if i == 401 {
			continue // a missed turn: the gap
		}
		start := base.Add(time.Duration(i) * 100 * time.Millisecond)
		w.ticks.lap = [tsCount]time.Duration{}
		w.ticks.lap[tsAI] = 2 * time.Millisecond
		w.ticks.lap[tsCorpses] = time.Millisecond
		if i == 300 {
			w.ticks.lap[tsAI] = 70 * time.Millisecond // one slow turn
		}
		worldDone := start.Add(w.ticks.lap[tsAI] + w.ticks.lap[tsCorpses])
		if i < 600 && len(w.TickReport()) != 0 {
			t.Fatalf("a report after %d turns, before a minute", i)
		}
		w.TickTurn(start, worldDone, worldDone.Add(500*time.Microsecond), 100*time.Millisecond)
	}
	lines := w.TickReport()
	if len(lines) < 5 {
		t.Fatalf("report %q", lines)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"600 turns", "1 past 50ms", "0 over the 100ms tick", "the longest gap between turns 200.00 ms"} {
		if !strings.Contains(text, want) {
			t.Errorf("report has no %q:\n%s", want, text)
		}
	}
	sys := lines[4]
	if ai, corpses := strings.Index(sys, "ai "), strings.Index(sys, "corpses "); ai < 0 || corpses < 0 || ai > corpses {
		t.Errorf("systems %q: want ai before corpses", sys)
	}
	if strings.Contains(text, "lockwait") || strings.Contains(text, "roam ") {
		t.Errorf("a lap that took nothing is in the report:\n%s", text)
	}
	// the next minute starts afresh
	w.TickTurn(base.Add(61*time.Second), base.Add(61*time.Second+time.Millisecond), base.Add(61*time.Second+2*time.Millisecond), 100*time.Millisecond)
	if w.ticks.win.turns != 1 || w.ticks.win.slow != 0 {
		t.Errorf("after the report: %d turns and %d slow in the new window, want 1 and 0", w.ticks.win.turns, w.ticks.win.slow)
	}
}

// A tick's laps are its systems' times: Tick fills them (each lap at least
// as long as nothing), and they add up to the tick's time.
func TestTickLapsAddUpToTheTick(t *testing.T) {
	w, _ := setsWorld(t, 200)
	start := time.Now()
	// A quiet test tick can finish inside one Windows clock step. Supply
	// a real lock wait so there is measurable time for the laps to cover.
	w.mu.Lock()
	time.AfterFunc(20*time.Millisecond, w.mu.Unlock)
	w.Tick(start)
	took := time.Since(start)
	var sum time.Duration
	for i, d := range w.ticks.lap {
		if d < 0 {
			t.Fatalf("lap %s is %v", tickLapNames[i], d)
		}
		sum += d
	}
	if sum <= 0 || sum > took {
		t.Fatalf("the laps add up to %v of a tick of %v", sum, took)
	}
	if w.ticks.npcs != 200 || w.ticks.players != 1 {
		t.Fatalf("the tick saw %d NPCs and %d players, want 200 and 1", w.ticks.npcs, w.ticks.players)
	}
}
