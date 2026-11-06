package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The 0x000F limiter lets the game's own presses through: CLIENT's timer
// puts only 2000 VITimer ticks (2.03 s) between two after a due swing at a
// target out of reach (AutoAttackTick), and eqoaclassic's log held presses
// 1,992 ms apart that the old 2.5 s limit dropped, a new target then never
// engaged. A flood is still refused.
func TestTheAttackLimiterLetsTheGamesRetryThrough(t *testing.T) {
	sm := NewSecurityMiddleware()
	p := sm.policies[protocol.OpcodeAttack]
	if p == nil || p.MinInterval <= 0 || p.MinInterval > 1500*time.Millisecond {
		t.Fatalf("0x000F's least interval %v; want a flood guard under the game's 2 s retry, with room for the network's jitter", p)
	}
}
