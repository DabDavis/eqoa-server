package gmweb

import "testing"

// A cityintro row holds what a PS2 can show; the controls pass.
func TestCheckCityIntroRow(t *testing.T) {
	long := make([]byte, 218)
	for i := range long {
		long[i] = 'a'
	}
	for _, bad := range []map[string]any{
		{"boxes": "a |x run"},
		{"yes_last": "100%"},
		{"no_last": string(long)},
		{"radius": 0.0},
		{"class": -2.0},
		{"race": 1.5},
	} {
		if err := checkRow("cityintro", bad); err == nil {
			t.Errorf("%v passed", bad)
		}
	}
	if err := checkRow("cityintro", map[string]any{"boxes": "Welcome.\nAs a new rogue...", "yes_last": "y",
		"radius": 60.0, "race": 4.0, "class": -1.0}); err != nil {
		t.Errorf("a good row: %v", err)
	}
}
