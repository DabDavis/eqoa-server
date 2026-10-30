package session

import "time"

// The time of day a client shows, kept for each session as the client keeps
// it. 0x0013 carries two readings of the server's clock; CLIENT sets one
// VISyncTime with each (VIClient::SetTime) and its time of day is the second's
// (VIClient::UpdateTimeOfDay, 0x00BC7B70):
//
//	hour = (float)(clockB.GetTime(now) % 3600000) / 3600000.0f * 24.0f
//
// clamped to 0..23.99999 by VIScene::SetTimeOfDay. The clock runs on VITimer,
// 983.04 ticks a real second (BUSCLK/16 at a compare of 18,750, plus 2 a
// tick: SUPPORT vitimer.cpp), so a game day is 3,600,000 ticks, 61.04 real
// minutes, and a client falls behind the server's own clock by 1.7% of the
// time since it was last set (a minute an hour). A later reading is not
// jumped to: VISyncTime::Set blends the difference in over 60,000 ticks.
// eqoa-rust-client-vanilla's net::SyncTime is the same, line for line, so a
// PS2, vanilla and the page show the same hour.
//
// The server writes the second reading as its clock plus ClockOffset
// (handlers' sendTimeOnly, as the C# reference and Sandstorm do: Sandstorm's
// 0x0013 of 2026-10-30 03:03:14 UTC carried 1793329394412 and
// 1793330744412), so at the moment of a 0x0013 a client's hour is
// ((ms + 1,350,000) % 3,600,000) / 150,000: midnight at 37.5 minutes past
// every real hour, the sun up from 52.5 past to 22.5 past the next.

// ClockOffset is what the server adds to its clock (Unix milliseconds) for
// the second reading of 0x0013, the one a client's time of day is read from.
const ClockOffset = 1350000

// GameDay is a game day in the clock's ticks.
const GameDay = 3600000

// clockBlend is VISyncTime's BLEND_INTERVAL: the ticks a new reading takes
// to be blended in.
const clockBlend = 60000

// clockBase is where the clients' VITimer is counted from here: only the
// ticks between two readings count, so any instant does.
var clockBase = time.Now()

// timerAt is a client's VITimer at t, from clockBase: 983.04 ticks a second.
func timerAt(t time.Time) int64 {
	return t.Sub(clockBase).Milliseconds() * 98304 / 100000
}

// gameClock is VISyncTime: ready once set; remote the synced time at the
// last setting, local the timer then, blend what is left to blend in.
type gameClock struct {
	ready         bool
	remote, local int64
	blend         int64
}

// set is VISyncTime::Set(t, base): the first reading taken as it is, a later
// one blended in from where the clock reads now.
func (c *gameClock) set(t, base int64) {
	if !c.ready {
		*c = gameClock{ready: true, remote: t, local: base}
		return
	}
	cur := c.get(base)
	c.blend = t - cur
	c.remote = cur
	c.local = base
}

// get is VISyncTime::GetTime(now): the reading carried forward on the timer,
// and the share of the blend its interval has let in (C's division, toward
// zero, as Go's).
func (c *gameClock) get(now int64) int64 {
	dt := now - c.local
	m := min(dt, clockBlend)
	return c.remote + dt + c.blend*m/clockBlend
}

// HourOf is UpdateTimeOfDay's hour of a clock reading, as SetTimeOfDay
// clamps it.
func HourOf(t int64) float32 {
	t %= GameDay
	if t < 0 {
		t += GameDay
	}
	h := float32(t) / float32(GameDay) * 24
	return min(23.99999, max(0, h))
}

// ServerHour is the hour a client set by a 0x0013 at t shows at once: the
// time of day of a session that has none of its own.
func ServerHour(t time.Time) float32 {
	return HourOf(t.UnixMilli() + ClockOffset)
}

// SetGameClock is a 0x0013 sent at t whose second reading is reading: the
// session's client sets its clock by it (blending it in, if it had one).
func (s *Session) SetGameClock(reading int64, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock.set(reading, timerAt(t))
}

// GameHour is the time of day the session's client shows at t, 0 to
// 23.99999; ok is false before a 0x0013 set its clock.
func (s *Session) GameHour(t time.Time) (hour float32, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.clock.ready {
		return 0, false
	}
	return HourOf(s.clock.get(timerAt(t))), true
}
