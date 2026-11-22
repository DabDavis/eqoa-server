package entity

import "testing"

// A Legacy record (database/legacy.go, work board 1714) is its record: a Legacy
// copy of an Ascended proc weapon procs at the tenth more, a Legacy worn
// piece answers as its record, and RecordOf gives the record. The control: a
// record named "Legacy ..." with no itemlegacy row is read as itself (none).
func TestALegacyRecordIsItsRecord(t *testing.T) {
	defer SetAscendedItems(nil)
	defer SetLegacyItems(nil)
	SetAscendedItems(map[int]int{980001: 1100, 980003: 2251})
	SetLegacyItems(map[int]int{967001: 980001, 967002: 980003})

	if wp, ok := WeaponProcOf(967001, "Legacy Ascended Selo's Jeweled Rapier", ""); !ok || wp.Damage != 154 {
		t.Errorf("the Legacy rapier: %+v (%v), want its Ascended record's 154", wp, ok)
	}
	if s, ok := WornShieldOf(967002, "Legacy Ascended Mithril Breastplate"); !ok || s.Power != 17 {
		t.Errorf("the Legacy breastplate: %+v (%v), want 17 power", s, ok)
	}
	if RecordOf(967001) != 980001 || RecordOf(1100) != 1100 {
		t.Errorf("RecordOf: %d, %d", RecordOf(967001), RecordOf(1100))
	}
	if wp, ok := WeaponProcOf(424242, "Legacy Ascended Selo's Jeweled Rapier", ""); ok {
		t.Errorf("the control, no itemlegacy row: proc %+v", wp)
	}
}
