package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 1314: an import's copy keeps 70% of what its record's proc and
// worn hit effect do, read as its base's are: an epic's proc by its name, a
// proc its description writes, a worn effect by its record or its name. The
// controls: the same records not imported do it whole, and a proc's chance is
// not damped.
func TestAnImportedCopysProcAndWornEffectAreDamped(t *testing.T) {
	defer database.SetImportedShare(database.SetImportedShare(0.7)) // the figures below are at 70%
	defer SetImportedItems(nil)
	SetImportedItems(map[int]int{990001: 1100, 990002: 1101, 990003: 2251, 990004: 3203})

	for _, c := range []struct {
		pattern    int
		name, desc string
		want       int
	}{
		{1100, "Selo's Jeweled Rapier", "", 140},
		{990001, "Selo's Jeweled Rapier", "", 98},
		{1101, "a blade", "Procs a 100 point DD Spell.", 100},
		{990002, "a blade", "Procs a 100 point DD Spell.", 70},
	} {
		wp, ok := WeaponProcOf(c.pattern, c.name, c.desc)
		if !ok || wp.Damage != c.want {
			t.Errorf("record %d %q: proc %+v (%v), want damage %d", c.pattern, c.name, wp, ok, c.want)
		}
	}
	whole, _ := WeaponProcOf(1101, "a blade", "Procs a 100 point DD Spell.")
	damped, _ := WeaponProcOf(990002, "a blade", "Procs a 100 point DD Spell.")
	if whole.Chance != damped.Chance || whole.Kind != damped.Kind {
		t.Errorf("the copy's chance or kind moved: %+v against %+v", damped, whole)
	}

	if s, ok := WornShieldOf(2251, "Mithril Breastplate"); !ok || s.Power != 15 {
		t.Errorf("Mithril Breastplate: %+v (%v), want 15 power", s, ok)
	}
	if s, ok := WornShieldOf(990003, "Mithril Breastplate"); !ok || s.Power != 11 {
		t.Errorf("its copy: %+v (%v), want 11 power (by its base's record)", s, ok)
	}
	if s, ok := WornShieldOf(990004, "Nightshade Gloves"); !ok || s.Damage != 21 || s.Heal != 11 {
		t.Errorf("Nightshade Gloves' copy: %+v (%v), want 21 damage and 11 heal (by its name)", s, ok)
	}
	if s, ok := WornShieldOf(3203, "Nightshade Gloves"); !ok || s.Damage != 30 || s.Heal != 15 {
		t.Errorf("Nightshade Gloves: %+v (%v), want 30 and 15", s, ok)
	}
}
