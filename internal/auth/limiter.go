package auth

import (
	"net"
	"strings"
	"sync"
	"time"
)

// Limits on guessing a password, shared by every place this server checks
// one: the game's own login (port 9735, the browser client's through the web
// relay), the account pages' login and "log the other window out", and the
// GM panel. One Limiter serves them all, so guesses spread over all of them
// add up, by the client's address and by the account name.
const (
	FailWindow   = 15 * time.Minute
	FailsPerAddr = 20
	FailsPerName = 8

	// MaxNameLen is longer than any game name (16): a longer one is counted by
	// its address alone, never kept under its own name.
	MaxNameLen = 64

	// limiterKeys bounds each of the limiter's maps: a flood of names or
	// addresses can cost a little memory, never the game's.
	limiterKeys = 5000

	// passwordSlots is how many password checks run at once: each is a
	// deliberate few dozen milliseconds of a core.
	passwordSlots = 4
)

// Limiter counts sign-in attempts by address and by account name. An
// attempt is counted BEFORE its password is checked, so a burst of
// concurrent guesses gets no more tries than a slow one; a right password
// takes its own attempt back.
type Limiter struct {
	mu     sync.Mutex
	byAddr map[string][]time.Time
	byName map[string][]time.Time
	slots  chan struct{}
}

// NewLimiter makes an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{byAddr: map[string][]time.Time{}, byName: map[string][]time.Time{}, slots: make(chan struct{}, passwordSlots)}
}

// LimitKey is the address a limit is kept under: an IPv6 address by its
// /64, which is what one machine is given.
func LimitKey(ip string) string {
	p := net.ParseIP(ip)
	if p == nil || p.To4() != nil {
		return ip
	}
	return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func recent(ts []time.Time, now time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < FailWindow {
			out = append(out, t)
		}
	}
	return out
}

// room makes room in m for one more key: the expired go first, then any.
func room(m map[string][]time.Time, now time.Time) {
	if len(m) < limiterKeys {
		return
	}
	for k, v := range m {
		if len(recent(v, now)) == 0 {
			delete(m, k)
		}
	}
	for k := range m {
		if len(m) < limiterKeys {
			break
		}
		delete(m, k)
	}
}

// Begin counts an attempt by the address ("" for none: not counted by
// address) and the name, or says how long until one is allowed.
func (l *Limiter) Begin(addr, name string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) > MaxNameLen {
		name = ""
	}
	var a, n []time.Time
	if addr != "" {
		addr = LimitKey(addr)
		a = recent(l.byAddr[addr], now)
		if len(a) >= FailsPerAddr {
			l.byAddr[addr] = a
			return FailWindow - now.Sub(a[len(a)-FailsPerAddr])
		}
	}
	if name != "" {
		n = recent(l.byName[name], now)
		if len(n) >= FailsPerName {
			l.byName[name] = n
			return FailWindow - now.Sub(n[len(n)-FailsPerName])
		}
	}
	if addr != "" {
		if _, ok := l.byAddr[addr]; !ok {
			room(l.byAddr, now)
		}
		l.byAddr[addr] = append(a, now)
	}
	if name != "" {
		if _, ok := l.byName[name]; !ok {
			room(l.byName, now)
		}
		l.byName[name] = append(n, now)
	}
	return 0
}

// Succeed is a right password: the name's count is cleared and the address
// gets its attempt back. An account REGISTERED by its first login passes ""
// for the address, which keeps the attempt, so an address cannot register
// accounts any faster than it may guess.
func (l *Limiter) Succeed(addr, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byName, strings.ToLower(strings.TrimSpace(name)))
	if addr == "" {
		return
	}
	addr = LimitKey(addr)
	if f := l.byAddr[addr]; len(f) > 0 {
		l.byAddr[addr] = f[:len(f)-1]
	}
}

// Slot is one of the few password checks that run at once, waited for at
// most wait: release it when the check is done. ok is false when none came
// free.
func (l *Limiter) Slot(wait time.Duration) (release func(), ok bool) {
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, true
	case <-t.C:
		return nil, false
	}
}

// Sizes is how many addresses and names the limiter holds.
func (l *Limiter) Sizes() (addrs, names int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.byAddr), len(l.byName)
}

// RetryMinutes is a wait as the whole minutes a refusal names.
func RetryMinutes(wait time.Duration) int {
	m := int((wait + time.Minute - 1) / time.Minute)
	if m < 1 {
		m = 1
	}
	return m
}
