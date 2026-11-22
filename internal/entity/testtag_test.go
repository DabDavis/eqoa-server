package entity

import (
	"bytes"
	"slices"
	"testing"
)

// The owner, 2026-11-21: "lets give sub gms a tag [Test]". A sub GM's name is
// drawn "[Test] Name", joined with the other tags in their order and fitted to 23
// bytes: the space goes first, then [OG], then [Test], then [HC]; the name is
// never cut. A GM's account is drawn with [GM] alone; a hidden [GM] tag hides it
// all. Each drawn name gives the plain name back.
func TestTheTestTagFits(t *testing.T) {
	hardcoreOn(t)
	SetOGTags(true)
	t.Cleanup(func() { SetOGTags(false) })
	for _, c := range []struct {
		name                  string
		gm, hide, og, hc, sub bool
		want                  string
	}{
		{"Meshuggah", false, false, false, false, true, "[Test] Meshuggah"},
		{"Meshuggah", false, false, false, false, false, "Meshuggah"}, // the control
		{"Meshuggah", false, false, true, false, true, "[Test][OG] Meshuggah"},
		{"Meshuggah", false, false, false, true, true, "[Test][HC] Meshuggah"},
		{"Meshuggah", false, false, true, true, true, "[Test][OG][HC]Meshuggah"},        // 9 letters: no space
		{"Meshuggahh", false, false, true, true, true, "[Test][HC] Meshuggahh"},         // 10: [OG] goes
		{"Meshuggahhhhh", false, false, true, true, true, "[Test][HC]Meshuggahhhhh"},    // 13
		{"Meshuggahhhhhh", false, false, true, true, true, "[HC] Meshuggahhhhhh"},       // 14: [Test] goes
		{"Meshuggahhhhhhh", false, false, false, false, true, "[Test] Meshuggahhhhhhh"}, // 15 with [Test] alone
		{"Meshuggah", true, false, false, false, true, "[GM] Meshuggah"},                // a GM: [GM] alone
		{"Meshuggah", true, true, false, false, true, "Meshuggah"},                      // its tag hidden
	} {
		p := &Player{Name: c.name, GM: c.gm, HideGMTag: c.hide, OG: c.og, Hardcore: c.hc, SubGM: c.sub}
		got := p.ShownName()
		if got != c.want || len(got) > ShownNameMost {
			t.Errorf("%+v: %q, want %q", c, got, c.want)
		}
		if PlainName(got) != c.name {
			t.Errorf("PlainName(%q) = %q", got, PlainName(got))
		}
	}
	if !IsNameTag("[Test]") || !IsNameTag("[test][HC]") || IsNameTag("[Test]Bob") || PlainName("[TEST] Bob") != "Bob" {
		t.Error("IsNameTag/PlainName with [Test]")
	}
	if got := ShownNamesOf("Bob", false, true, false, false); !slices.Equal(got, []string{"Bob", "[AFK] Bob", "[Test] Bob", "[Test][AFK] Bob"}) {
		t.Errorf("a sub GM's names %q", got)
	}
	if got := ShownNamesOf("Bob", false, true, true, false); !slices.Equal(got, []string{"Bob", "[AFK] Bob", "[OG] Bob", "[OG][AFK] Bob", "[Test] Bob", "[Test][AFK] Bob", "[Test][OG] Bob", "[Test][OG][AFK] Bob"}) {
		t.Errorf("a sub GM's [OG] names %q", got)
	}
	if got := ShownNamesOf("Bob", true, true, false, false); !slices.Equal(got, []string{"Bob", "[AFK] Bob", "[GM] Bob", "[GM][AFK] Bob"}) {
		t.Errorf("a GM's names %q", got)
	}
	// on the wire: the channel's 24 bytes at 116
	buf := SerializePlayerUpdate(1, &Player{Name: "Neo", SubGM: true, Level: 60})
	if name := string(bytes.TrimRight(buf[116:140], "\x00")); name != "[Test] Neo" {
		t.Errorf("the channel's name %q", name)
	}
}
