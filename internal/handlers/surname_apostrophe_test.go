package handlers

import "testing"

// The owner, 2026-11-28: "let drizzt surname be do'urden ... allow that in players surnames" (Heirloom petition 3456).
// A surname may hold an apostrophe inside it, cased with a capital after it; the controls: one first, last or doubled,
// a period, two words and a surname too long are refused, and one without an apostrophe is cased as before.
func TestASurnameMayHoldAnApostrophe(t *testing.T) {
	for _, c := range []struct {
		asked, want string
		ok          bool
	}{
		{"do'urden", "Do'Urden", true},
		{"O'BRIEN", "O'Brien", true},
		{"dourden", "Dourden", true},
		{"'urden", "", false},
		{"urden'", "", false},
		{"do''urden", "", false},
		{"do.urden", "", false},
		{"do'ur den", "", false},
		{"abcdefg'hijklmno", "", false}, // 16 characters
		{"d'o", "", false},              // two letters: the 3-letter least counts letters
	} {
		v, ok := surnameShape(c.asked)
		if ok != c.ok || (ok && v.name != c.want) {
			t.Errorf("surnameShape(%q) = %q %v (%s), want %q %v", c.asked, v.name, ok, v.reason, c.want, c.ok)
		}
	}
}
