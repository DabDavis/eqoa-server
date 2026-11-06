package database

import "testing"

// A GM's #dye takes every dyer colour's name, a name of two words also run
// together (#dye reads one word), "gray" as "grey"; the control: a name no
// family has is refused.
func TestDyeNamesAreTheDyersColours(t *testing.T) {
	for _, f := range DyeFamilies {
		for _, c := range f.Colours {
			got, err := ParseDyeColor(c.Label)
			if err != nil || got != c.RGBA {
				t.Errorf("%q: %08X %v, want %08X", c.Label, got, err, c.RGBA)
			}
		}
	}
	for in, want := range map[string]uint32{"skyblue": RGBA(DiscTints[2]), "Royalblue": 0x2050E0FF, "gray": 0x808080FF, "none": 0} {
		if got, err := ParseDyeColor(in); err != nil || got != want {
			t.Errorf("%q: %08X %v, want %08X", in, got, err, want)
		}
	}
	if _, err := ParseDyeColor("ultraviolet"); err == nil {
		t.Error("a name no family has was taken")
	}
}
