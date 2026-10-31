package entity

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

// beatClient is CLIENT's own swing timer, as VIClient keeps it
// (ToggleAutoAttack, AutoAttackTick, viclient.cpp), on the client's clock:
// a press with the timer run out sends 0x000F and starts the timer from
// the press; each frame with the button held and a swing due, a target in
// its reach (its view of the creature, a latency old) swings and the timer
// goes a swing's delay on, one out of it sends 0x00D8 and goes 2 s on; the
// button rides in the 0x40, sent at its next slot.
type beatClient struct {
	due      time.Duration
	set      bool
	held     bool
	swings   []time.Duration
	outbox   []beatMsg
	lastSent bool
}

type beatMsg struct {
	at   time.Duration // when it reaches the server
	kind string        // "000F", "00D8", "button"
	held bool
}

// the test's world: one creature, the player standing still 2 off it; the
// creature steps 8 off between away and back (the server's time)
type beatWorld struct {
	lat       time.Duration
	away      [][2]time.Duration
	delay     time.Duration
	frame     time.Duration
	slot      time.Duration // the 0x40's send interval
	presses   []time.Duration
	releases  []time.Duration
	serverOff time.Duration // the server's ticks' phase
}

func (bw *beatWorld) far(at time.Duration) bool {
	for _, a := range bw.away {
		if at >= a[0] && at < a[1] {
			return true
		}
	}
	return false
}

// run plays the fight to `end` against the server's attackSystem and
// returns the client's swings and the server's hits' ticks.
func (bw *beatWorld) run(t *testing.T, end time.Duration) (swings, hits []time.Duration) {
	t.Helper()
	f := newFight(t)
	swing := Swing{Kind: 2, Delay: bw.delay, Reach: 4.5} // Attack3: hits 0.5 and 1 s into a round
	c := &beatClient{}
	var inbox []beatMsg
	nextTick := bw.serverOff
	seen := 0
	for now := time.Duration(0); now <= end; now += time.Millisecond {
		// the client: presses and releases, its frame, its 0x40 slot
		for _, p := range bw.presses {
			if p == now {
				if !c.set || c.due <= now {
					c.due, c.set = now, true
					inbox = append(inbox, beatMsg{at: now + bw.lat, kind: "000F"})
				}
				c.held = true
			}
		}
		for _, r := range bw.releases {
			if r == now {
				c.held = false
			}
		}
		if now%bw.frame == 0 && c.held && c.set && c.due <= now {
			if !bw.far(now - bw.lat) { // its view of the creature is a latency old
				c.swings = append(c.swings, now)
				c.due += bw.delay
			} else {
				inbox = append(inbox, beatMsg{at: now + bw.lat, kind: "00D8"})
				c.due += 2 * time.Second
			}
		}
		if now%bw.slot == 0 && c.held != c.lastSent {
			c.lastSent = c.held
			inbox = append(inbox, beatMsg{at: now + bw.lat, kind: "button", held: c.held})
		}
		// the server: what has reached it, then its tick
		at := f.t0.Add(now)
		keep := inbox[:0]
		for _, m := range inbox {
			if m.at > now {
				keep = append(keep, m)
				continue
			}
			switch m.kind {
			case "000F":
				f.w.EngageAttack(f.sess, f.npcID, swing, at)
			case "00D8":
				f.w.SwingOutOfReach(f.sess, at)
			case "button":
				f.p.AutoAttack = m.held
			}
		}
		inbox = keep
		if bw.far(now) {
			f.npc.X = 108
		} else {
			f.npc.X = 102
		}
		if now == nextTick {
			nextTick += 100 * time.Millisecond
			f.tick(now)
			if n := len(f.damage(t)); n > seen {
				for i := seen; i < n; i++ {
					hits = append(hits, now)
				}
				seen = n
			}
		}
	}
	return c.swings, hits
}

// want is each swing's hits, 0.5 and 1 s after it reaches the server, each
// at the server's first tick at or after it.
func (bw *beatWorld) want(swings []time.Duration, end time.Duration) []time.Duration {
	var out []time.Duration
	for _, s := range swings {
		for _, h := range []time.Duration{500 * time.Millisecond, time.Second} {
			due := s + bw.lat + h
			tick := bw.serverOff + (due-bw.serverOff+99*time.Millisecond)/(100*time.Millisecond)*(100*time.Millisecond)
			if tick <= end {
				out = append(out, tick)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func durs(ds []time.Duration) string {
	s := ""
	for _, d := range ds {
		s += fmt.Sprintf(" %.2f", d.Seconds())
	}
	return s
}

// The damage keeps the client's own swing, fight long (the user, 2026-10-30:
// "the animation and damage don't sync well for auto attacks especially on
// eqoaclassic"): every hit the server lands is one of a swing the client
// played, on that swing's times, a latency late, through a creature that
// steps out of reach and back (the client's 0x00D8s, its 2 s retries) and a
// release and two presses (one with its timer still running, which sends no
// 0x000F, and one after, which does). The control: a server that ignores
// 0x00D8 and starts its rounds again from its own reach test lands its hits
// off the client's swings for the rest of the fight.
func TestAutoAttackKeepsTheClientsBeat(t *testing.T) {
	bw := &beatWorld{
		lat:       60 * time.Millisecond,
		away:      [][2]time.Duration{{10 * time.Second, 14300 * time.Millisecond}},
		delay:     4 * time.Second,
		frame:     33 * time.Millisecond,
		slot:      100 * time.Millisecond,
		presses:   []time.Duration{0, 27500 * time.Millisecond, 37 * time.Second},
		releases:  []time.Duration{25 * time.Second, 33500 * time.Millisecond},
		serverOff: 37 * time.Millisecond,
	}
	const end = 50 * time.Second
	swings, hits := bw.run(t, end)
	want := bw.want(swings, end)
	if durs(hits) != durs(want) {
		t.Errorf("the client swung at%s\nthe server's hits at%s\nwant them at%s", durs(swings), durs(hits), durs(want))
	}
}
