package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// OpcodePolicy defines rate limiting and validation for a single opcode.
type OpcodePolicy struct {
	MinInterval  time.Duration // minimum time between calls (0 = no limit)
	MaxPerMin    int           // max calls per minute (0 = no limit)
	RequireWorld bool          // must be in-world to use
}

// SecurityMiddleware wraps handler dispatch with rate limiting, validation, and logging.
type SecurityMiddleware struct {
	policies map[protocol.GameOpcode]*OpcodePolicy

	// Per-session rate tracking
	mu       sync.RWMutex
	sessions map[uint32]*sessionRateState // instanceID → state
}

type sessionRateState struct {
	lastCall   map[protocol.GameOpcode]time.Time
	callCount  map[protocol.GameOpcode]int
	countReset time.Time // when to reset per-minute counters
	violations int       // total security violations
}

// NewSecurityMiddleware creates the middleware with default policies.
func NewSecurityMiddleware() *SecurityMiddleware {
	sm := &SecurityMiddleware{
		policies: make(map[protocol.GameOpcode]*OpcodePolicy),
		sessions: make(map[uint32]*sessionRateState),
	}
	sm.registerDefaults()
	return sm
}

func (sm *SecurityMiddleware) registerDefaults() {
	// Combat — a flood guard only. CLIENT sends 0x000F at most once per run
	// of its own swing timer (ToggleAutoAttack waits for m_p071E8), which a
	// swing goes its delay on (3000 VITimer ticks at the least, 3.05 s) but a
	// due swing at a target out of its reach only 2000 (AutoAttackTick: 2.03
	// s): presses 2 s apart are the game's own. Until 2026-11-06 this was 2.5
	// s, and the press it dropped was lost: a new target's never engaged (the
	// client swinging at it for nothing, the user's "players cant attack"; 59
	// drops logged in a day, the log stopping at 5 a session). The server
	// keeps the client's timer itself now (entity/attack.go pressed: a press
	// starts no round before the timer, less pressSlack), so the swings are
	// held to the weapon's delay, not to this.
	sm.policies[protocol.OpcodeAttack] = &OpcodePolicy{
		MinInterval:  time.Second,
		RequireWorld: true,
	}

	// The quick-chat menu's changes go to the database (hotkeys.go): a change
	// is a few messages (a branch dropped, one for each set under it), so a
	// cap a minute and no least interval.
	for _, op := range []protocol.GameOpcode{protocol.OpcodeHotkeyChoice, protocol.OpcodeHotkeySet, protocol.OpcodeHotkeyDrop} {
		sm.policies[op] = &OpcodePolicy{MaxPerMin: 300, RequireWorld: true}
	}

	// Emotes are shown to every player who can see the sender (0x0775).
	for _, op := range []protocol.GameOpcode{protocol.OpcodeEmoteWave, protocol.OpcodeEmoteBow, protocol.OpcodeEmotePoint, protocol.OpcodeEmoteCheer} {
		sm.policies[op] = &OpcodePolicy{MinInterval: time.Second, RequireWorld: true}
	}

	// Spells — prevent spam casting
	sm.policies[protocol.OpcodeUseSkill] = &OpcodePolicy{
		MinInterval:  500 * time.Millisecond,
		MaxPerMin:    60,
		RequireWorld: true,
	}

	// Merchant — prevent rapid buy/sell
	sm.policies[protocol.OpcodeMerchantBuy] = &OpcodePolicy{
		MinInterval:  200 * time.Millisecond,
		MaxPerMin:    120,
		RequireWorld: true,
	}
	sm.policies[protocol.OpcodeMerchantSell] = &OpcodePolicy{
		MinInterval:  200 * time.Millisecond,
		MaxPerMin:    120,
		RequireWorld: true,
	}

	// Equipment — prevent rapid equip/unequip
	sm.policies[protocol.OpcodeInteractItem] = &OpcodePolicy{
		MinInterval:  200 * time.Millisecond,
		MaxPerMin:    60,
		RequireWorld: true,
	}

	// Chat — prevent flooding
	sm.policies[protocol.OpcodeClientSayChat] = &OpcodePolicy{
		MinInterval:  500 * time.Millisecond,
		MaxPerMin:    30,
		RequireWorld: true,
	}

	// Interact — prevent rapid NPC interaction
	sm.policies[protocol.OpcodeInteract] = &OpcodePolicy{
		MinInterval:  300 * time.Millisecond,
		MaxPerMin:    60,
		RequireWorld: true,
	}

	// Character management — prevent spam creation/deletion
	sm.policies[protocol.OpcodeCreateCharacter] = &OpcodePolicy{
		MinInterval: 5 * time.Second,
		MaxPerMin:   5,
	}
	sm.policies[protocol.OpcodeDelCharacter] = &OpcodePolicy{
		MinInterval: 5 * time.Second,
		MaxPerMin:   5,
	}
}

// Check validates an incoming opcode against rate limits and policies.
// Returns true if the opcode should be processed, false if rejected.
func (sm *SecurityMiddleware) Check(sess *session.Session, opcode protocol.GameOpcode) bool {
	policy, exists := sm.policies[opcode]
	if !exists {
		return true // no policy = allow
	}

	// Check world requirement
	if policy.RequireWorld && !sess.CharacterInWorld() {
		log.Printf("SECURITY: session %s sent 0x%04X (%s) without being in-world — rejected",
			sess.RemoteAddr(), uint16(opcode), opcode.String())
		return false
	}

	instanceID := sess.InstanceID()

	sm.mu.Lock()
	state, ok := sm.sessions[instanceID]
	if !ok {
		state = &sessionRateState{
			lastCall:   make(map[protocol.GameOpcode]time.Time),
			callCount:  make(map[protocol.GameOpcode]int),
			countReset: time.Now().Add(time.Minute),
		}
		sm.sessions[instanceID] = state
	}
	sm.mu.Unlock()

	now := time.Now()

	// Reset per-minute counters
	if now.After(state.countReset) {
		state.callCount = make(map[protocol.GameOpcode]int)
		state.countReset = now.Add(time.Minute)
	}

	// Check minimum interval
	if policy.MinInterval > 0 {
		if last, ok := state.lastCall[opcode]; ok {
			elapsed := now.Sub(last)
			if elapsed < policy.MinInterval {
				state.violations++
				if state.violations <= 5 || state.violations%100 == 0 {
					log.Printf("SECURITY: rate limit 0x%04X (%s) from %s — %dms since last (min %dms) [violation #%d]",
						uint16(opcode), opcode.String(), sess.RemoteAddr(),
						elapsed.Milliseconds(), policy.MinInterval.Milliseconds(), state.violations)
				}
				return false
			}
		}
	}

	// Check per-minute cap
	if policy.MaxPerMin > 0 {
		state.callCount[opcode]++
		if state.callCount[opcode] > policy.MaxPerMin {
			state.violations++
			if state.violations <= 5 || state.violations%100 == 0 {
				log.Printf("SECURITY: per-minute cap 0x%04X (%s) from %s — %d calls (max %d) [violation #%d]",
					uint16(opcode), opcode.String(), sess.RemoteAddr(),
					state.callCount[opcode], policy.MaxPerMin, state.violations)
			}
			return false
		}
	}

	state.lastCall[opcode] = now
	return true
}

// CleanupSession removes rate state for a disconnected session.
func (sm *SecurityMiddleware) CleanupSession(instanceID uint32) {
	sm.mu.Lock()
	delete(sm.sessions, instanceID)
	sm.mu.Unlock()
}

// GetViolations returns the violation count for a session.
func (sm *SecurityMiddleware) GetViolations(instanceID uint32) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if state, ok := sm.sessions[instanceID]; ok {
		return state.violations
	}
	return 0
}
