package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestTheLimiterCountsByAddressAndByName(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < FailsPerName; i++ {
		if w := l.Begin("198.51.100.1", "Victim"); w != 0 {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	// the name is spent, from any address, in any case
	if w := l.Begin("203.0.113.9", "victim"); w <= 0 || w > FailWindow {
		t.Fatalf("the %dth guess at a name from elsewhere: wait %v", FailsPerName+1, w)
	}
	// the address is spent, whatever the name
	l2 := NewLimiter()
	for i := 0; i < FailsPerAddr; i++ {
		l2.Begin("198.51.100.1", fmt.Sprintf("name%d", i))
	}
	if w := l2.Begin("198.51.100.1", "another"); w <= 0 {
		t.Fatal("an address past its limit was let try another name")
	}
	if w := l2.Begin("", "another"); w != 0 {
		t.Fatal("an attempt with no address was refused by another address's count")
	}
}

func TestARightPasswordTakesItsAttemptBackButARegistrationDoesNot(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < FailsPerName-1; i++ {
		l.Begin("198.51.100.1", "player")
	}
	l.Begin("198.51.100.1", "player")
	l.Succeed("198.51.100.1", "player")
	if w := l.Begin("198.51.100.1", "player"); w != 0 {
		t.Fatal("a right password did not clear the name")
	}
	addrs := func() int {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.byAddr[LimitKey("198.51.100.2")])
	}
	l.Begin("198.51.100.2", "new1")
	l.Succeed("", "new1") // registered: the address keeps it
	l.Begin("198.51.100.2", "known")
	l.Succeed("198.51.100.2", "known") // signed in: the address gets it back
	if n := addrs(); n != 1 {
		t.Fatalf("the address holds %d attempts, want 1 (the registration)", n)
	}
}

func TestTheLimiterIsBounded(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < limiterKeys+500; i++ {
		l.Begin(fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255), fmt.Sprintf("name%d", i))
	}
	if a, n := l.Sizes(); a > limiterKeys || n > limiterKeys {
		t.Fatalf("limiter maps: %d addresses, %d names", a, n)
	}
	// a name too long for any account is never kept under itself
	long := make([]byte, MaxNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	l2 := NewLimiter()
	l2.Begin("198.51.100.1", string(long))
	if _, n := l2.Sizes(); n != 0 {
		t.Fatal("an overlong name was kept")
	}
	// one machine's IPv6 addresses are one address
	if LimitKey("2001:db8:1:2::5") != LimitKey("2001:db8:1:2:aaaa::9") || LimitKey("2001:db8:1:3::5") == LimitKey("2001:db8:1:2::5") {
		t.Fatal("IPv6 addresses are not limited by their /64")
	}
}

func TestPasswordChecksRunAFewAtATime(t *testing.T) {
	l := NewLimiter()
	var held []func()
	for i := 0; i < passwordSlots; i++ {
		release, ok := l.Slot(time.Second)
		if !ok {
			t.Fatalf("slot %d not given", i+1)
		}
		held = append(held, release)
	}
	if _, ok := l.Slot(20 * time.Millisecond); ok {
		t.Fatal("a slot beyond the limit was given")
	}
	held[0]()
	if release, ok := l.Slot(time.Second); !ok {
		t.Fatal("a freed slot was not given again")
	} else {
		release()
	}
	if RetryMinutes(30*time.Second) != 1 || RetryMinutes(14*time.Minute+time.Second) != 15 {
		t.Fatal("RetryMinutes rounds wrong")
	}
}
