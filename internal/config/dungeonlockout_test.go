package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// world.dungeons.lockout: unset is nil (the default, 10m, handlers/dungeons.go), 0 is off, a duration is itself.
func TestDungeonsLockoutYAML(t *testing.T) {
	for src, want := range map[string]*ZeroDuration{"enabled: true": nil, "lockout: 0": new(ZeroDuration), "lockout: 15m": ptrDur(15 * time.Minute), "lockout: \"30s\"": ptrDur(30 * time.Second)} {
		var c DungeonsConfig
		if err := yaml.Unmarshal([]byte(src), &c); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if (c.Lockout == nil) != (want == nil) || c.Lockout != nil && *c.Lockout != *want {
			t.Errorf("%q: lockout %v, want %v", src, c.Lockout, want)
		}
	}
	var c DungeonsConfig
	if err := yaml.Unmarshal([]byte("lockout: soon"), &c); err == nil {
		t.Error("lockout: soon read")
	}
}

func ptrDur(d time.Duration) *ZeroDuration { z := ZeroDuration(d); return &z }
