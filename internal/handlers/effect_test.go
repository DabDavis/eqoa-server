package handlers

import "testing"

// #effect takes the icon in hex, with or without 0x, and the rest of the words
// as the name; an icon that is not hex, or none, is refused.
func TestParseEffect(t *testing.T) {
	e, err := parseEffect([]string{"1A2B3C4D", "Two", "Words"})
	if err != nil || e.Icon != 0x1A2B3C4D || e.Name != "Two Words" {
		t.Errorf("got %08X %q %v", e.Icon, e.Name, err)
	}
	if e, err := parseEffect([]string{"0xC0FFEE"}); err != nil || e.Icon != 0xC0FFEE || e.Name != "" {
		t.Errorf("got %08X %q %v", e.Icon, e.Name, err)
	}
	for _, bad := range [][]string{nil, {"zz", "Name"}, {"123456789", "x"}} {
		if _, err := parseEffect(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}
