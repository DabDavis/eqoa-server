package handlers

import "testing"

// A Spiritbinder binds as a Spiritmaster does (petition 2389); other names do not.
func TestSpiritbinderBinds(t *testing.T) {
	for name, want := range map[string]bool{
		"Spiritmaster Zole":  true,
		"Spiritbinder Pluto": true,
		"Spiritbinder":       false,
		"spirit binder Bob":  false,
		"Guard Spiritbinder": false,
	} {
		if got := IsSpiritmaster(name); got != want {
			t.Errorf("IsSpiritmaster(%q) = %v, want %v", name, got, want)
		}
	}
}
