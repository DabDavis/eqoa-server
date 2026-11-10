package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 1446: an Ascended record's proc and worn hit effect are its base's
// a tenth more, read by its base's record and name (its own name is
// "Ascended <name>"): an epic's proc by its name, a proc its description
// writes, a worn effect by its record or by its name. An import of an
// Ascended record is a tenth more, then damped. The controls: the bases do it
// as they did; a record named "Ascended ..." that is none (not in
// itemascended) is not read as its namesake; a proc's chance and kind do not
// move.
func TestAnAscendedRecordsProcAndWornEffectAreATenthMore(t *testing.T) {
	defer database.SetImportedShare(database.SetImportedShare(0.85))
	defer SetAscendedItems(nil)
	defer SetImportedItems(nil)
	SetAscendedItems(map[int]int{980001: 1100, 980002: 1101, 980003: 2251, 980004: 3203})
	SetImportedItems(map[int]int{990001: 980001})

	for _, c := range []struct {
		pattern    int
		name, desc string
		want       int
	}{
		{1100, "Selo's Jeweled Rapier", "", 140},
		{980001, "Ascended Selo's Jeweled Rapier", "", 154},
		{990001, "Ascended Selo's Jeweled Rapier", "", 131}, // 154 at 85%
		{1101, "a blade", "Procs a 100 point DD Spell.", 100},
		{980002, "Ascended a blade", "Procs a 100 point DD Spell.", 110},
		{424242, "Ascended Selo's Jeweled Rapier", "", 0}, // no Ascended record: no proc by that name
	} {
		wp, ok := WeaponProcOf(c.pattern, c.name, c.desc)
		if c.want == 0 {
			if ok {
				t.Errorf("record %d %q, no Ascended record: proc %+v", c.pattern, c.name, wp)
			}
			continue
		}
		if !ok || wp.Damage != c.want || wp.Item != c.name {
			t.Errorf("record %d %q: proc %+v (%v), want damage %d", c.pattern, c.name, wp, ok, c.want)
		}
	}
	whole, _ := WeaponProcOf(1101, "a blade", "Procs a 100 point DD Spell, a 10% chance each hit.")
	more, _ := WeaponProcOf(980002, "Ascended a blade", "Procs a 100 point DD Spell, a 10% chance each hit.")
	if whole.Chance != 10 || more.Chance != whole.Chance || more.Kind != whole.Kind || more.Damage != 110 {
		t.Errorf("the Ascended record's chance or kind moved: %+v against %+v", more, whole)
	}
	if hp, _ := WeaponProcOf(1102, "Hammer of the Paragon", ""); hp.Damage != 50 || hp.Power != 50 {
		t.Errorf("Hammer of the Paragon: %+v", hp)
	}

	if s, ok := WornShieldOf(2251, "Mithril Breastplate"); !ok || s.Power != 15 {
		t.Errorf("Mithril Breastplate: %+v (%v), want 15 power", s, ok)
	}
	if s, ok := WornShieldOf(980003, "Ascended Mithril Breastplate"); !ok || s.Power != 17 {
		t.Errorf("its Ascended record: %+v (%v), want 17 power (by its base's record)", s, ok)
	}
	if s, ok := WornShieldOf(980004, "Ascended Nightshade Gloves"); !ok || s.Damage != 33 || s.Heal != 17 || s.Item != "Ascended Nightshade Gloves" {
		t.Errorf("Nightshade Gloves' Ascended record: %+v (%v), want 33 damage and 17 heal (by its base's name)", s, ok)
	}
	if s, ok := WornShieldOf(3203, "Nightshade Gloves"); !ok || s.Damage != 30 || s.Heal != 15 {
		t.Errorf("Nightshade Gloves: %+v (%v), want 30 and 15", s, ok)
	}
	if s, ok := WornShieldOf(424243, "Ascended Nightshade Gloves"); ok {
		t.Errorf("a record named so that is no Ascended record: %+v", s)
	}
	if !IsAscended(980001) || IsAscended(1100) {
		t.Error("IsAscended")
	}
}
