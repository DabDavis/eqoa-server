package entity

import "testing"

// The weapon refusal's words (equipRefusalLine) for every required
// equipment the spell records carry (the heirloom shard's spelleffect,
// 2026-11-15), and a code it does not know.
func TestEquipRefusalLine(t *testing.T) {
	for _, c := range []struct{ req, want string }{
		{"1HP", "You need a one-handed piercing weapon equipped to use X."},
		{"1HB,1HS,1HP, 2HB,2HS,2HP", "You need a melee weapon equipped to use X."},
		{"1HS,2HS,1HB,2HB,1HP,2HP", "You need a melee weapon equipped to use X."},
		{"Range", "You need a ranged weapon equipped to use X."},
		{"1HS,2HS", "You need a slashing weapon equipped to use X."},
		{"1HS,2HS,1HP", "You need a slashing weapon or a one-handed piercing weapon equipped to use X."},
		{"1HS,1HB,1HP", "You need a one-handed weapon equipped to use X."},
		{"1HB,1HS,1HP", "You need a one-handed weapon equipped to use X."},
		{"1HS", "You need a one-handed slashing weapon equipped to use X."},
		{"1HB,2HB", "You need a blunt weapon equipped to use X."},
		{"Shield", "You need a shield equipped to use X."},
		{"1HB,2HB,Shield", "You need a blunt weapon or a shield equipped to use X."},
		{"2HB", "You need a two-handed blunt weapon equipped to use X."},
		{"2HS,2HB,2HP", "You need a two-handed weapon equipped to use X."},
		{"None", "You need empty hands to use X."},
		{"1HP,None", "You need a one-handed piercing weapon equipped or empty hands to use X."},
		{"Polearm", "You need the right weapon equipped to use X."},
	} {
		if got := equipRefusalLine(c.req, "X"); got != c.want {
			t.Errorf("%q: %q, want %q", c.req, got, c.want)
		}
	}
}
