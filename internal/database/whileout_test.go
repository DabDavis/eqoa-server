package database

import "testing"

// while_out is a buff's on its owner alone (petition 1805: the Undead
// Servant's buff, off when it goes).
func TestWhileOutIsAnOwnerBuffs(t *testing.T) {
	if _, err := ParseTemplateAbilities(`[{"kind":"buff","every":30,"spell_id":907236,"target":"owner","while_out":true}]`); err != nil {
		t.Errorf("an owner buff while out: %v", err)
	}
	for _, bad := range []string{
		`[{"kind":"buff","every":30,"spell_id":907236,"target":"self","while_out":true}]`,
		`[{"kind":"buff","every":30,"spell_id":907236,"while_out":true}]`,
		`[{"kind":"heal","every":30,"spell_id":907236,"target":"owner","while_out":true}]`,
	} {
		if _, err := ParseTemplateAbilities(bad); err == nil {
			t.Errorf("%s: taken", bad)
		}
	}
}
