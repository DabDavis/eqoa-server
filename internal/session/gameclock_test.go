package session

import (
	"testing"
	"time"
)

// The clock a client keeps (VISyncTime, as eqoa-rust-client-vanilla's
// net::SyncTime has it, with its test's numbers): the first reading taken as
// it is and carried forward on the timer; a later one blended in over 60,000
// ticks (half the difference halfway, all of it after), never jumped to; C's
// division toward zero. The control: a clock that jumps to the reading.
func TestTheClientsClockBlendsANewReadingIn(t *testing.T) {
	var c gameClock
	c.set(1_000_000, 500)
	if a, b := c.get(500), c.get(10_500); a != 1_000_000 || b != 1_010_000 {
		t.Fatalf("the first reading carried forward: %d %d", a, b)
	}
	// 2,000 ahead at timer 20,500, where the clock read 1,020,000
	c.set(1_022_000, 20_500)
	if got := c.get(20_500); got != 1_020_000 {
		t.Errorf("a new reading jumped to: %d", got)
	}
	if got := c.get(50_500); got != 1_020_000+30_000+1_000 {
		t.Errorf("halfway: %d, want half the difference in", got)
	}
	if got := c.get(80_500); got != 1_022_000+60_000 {
		t.Errorf("after the interval: %d", got)
	}
	if got := c.get(180_500); got != 1_022_000+160_000 {
		t.Errorf("long after: %d", got)
	}
	at := c.get(200_000)
	c.set(at-7, 200_000)
	if got := c.get(200_000 + clockBlend/2); got != at+clockBlend/2-3 {
		t.Errorf("toward zero: %d, want %d", got, at+clockBlend/2-3)
	}
	jumped := func(t, base, now int64) int64 { return t + (now - base) }
	if jumped(1_022_000, 20_500, 50_500) == 1_020_000+30_000+1_000 {
		t.Error("the control: a jumping clock reads as the blended one")
	}
}

// UpdateTimeOfDay: the second reading modulo 3,600,000 as 24 hours, clamped
// as SetTimeOfDay clamps. Sandstorm's 0x0013 of 2026-10-30 03:03:14.412 UTC
// (spellfx24's afkqq-1 session) set its client to 10:17:45, when the sun
// burned Afkqq.
func TestTheTimeOfDayIsTheSecondReadingRoundAnHour(t *testing.T) {
	for _, c := range []struct {
		t    int64
		want float32
	}{
		{3_600_000*500_000 + 1_800_000, 12},
		{3_600_000*500_000 + 2_700_000, 18},
		{3_600_000 * 500_000, 0},
		{3_600_000*500_000 + 3_599_999, 23.99999},
	} {
		if got := HourOf(c.t); got != c.want {
			t.Errorf("HourOf(%d) = %v, want %v", c.t, got, c.want)
		}
	}
	at := time.UnixMilli(1793329394412)
	if h := ServerHour(at); h < 10.29 || h > 10.30 {
		t.Errorf("Sandstorm's 0x0013 shows %v, want 10.296", h)
	}
	if h := HourOf(1793330744412); h != ServerHour(at) {
		t.Errorf("its second reading, %v, is not the hour ServerHour says, %v", h, ServerHour(at))
	}
}

// A session's hour is its client's: set by the 0x0013 it was sent and run on
// the client's timer, 983.04 ticks a second, so an hour after a reading that
// showed 6:12 (the sun just up) the client shows 5:47, the sun not yet up,
// where the server's own clock has gone a whole 60-minute day round to 6:12
// again (the control). None before a 0x0013; a later one blends in.
func TestASessionsHourIsItsClients(t *testing.T) {
	s := &Session{}
	t0 := time.Unix(1_700_000_000, 0)
	if _, ok := s.GameHour(t0); ok {
		t.Fatal("an hour before any 0x0013")
	}
	reading := int64(3_600_000*472_000) + 930_000 // 6:12
	s.SetGameClock(reading, t0)
	if h, ok := s.GameHour(t0); !ok || h != 6.2 {
		t.Fatalf("at the reading: %v %v, want 6.2", h, ok)
	}
	later := t0.Add(time.Hour)
	h, _ := s.GameHour(later)
	if h < 5.79 || h > 5.80 {
		t.Errorf("an hour on: %v, want 5.793 (3,538,944 ticks on)", h)
	}
	if server := HourOf(reading + later.Sub(t0).Milliseconds()); server != 6.2 {
		t.Fatalf("the control: the server's clock shows %v an hour on, want 6.2", server)
	}
	// a 0x0013 now, of the server's clock, is blended in: no jump, and all of
	// it in by 61 s on (60,000 ticks), the clock the new reading's from then:
	// 62 s (60,948 ticks) after it, 6.606
	s.SetGameClock(reading+3_600_000, later)
	if h2, _ := s.GameHour(later); h2 != h {
		t.Errorf("a new reading jumped the hour from %v to %v", h, h2)
	}
	if h3, _ := s.GameHour(later.Add(62 * time.Second)); h3 < 6.60 || h3 > 6.61 {
		t.Errorf("62 s after the new reading: %v, want 6.606", h3)
	}
}
