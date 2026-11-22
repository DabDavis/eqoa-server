package entity

import (
	"math/bits"
	"testing"
	"time"
)

// An illusion's body in either byte order (illusion.go discOrderLocked): a
// workbook sprite typed in the disc's order (petition 2301's 0xF23F517F) or an
// item's body typed as a sprite shows the disc's body, as the right order
// does; a body the disc has in neither order still shows nothing, and the
// right orders are untouched.
func TestIllusionBodyInEitherByteOrder(t *testing.T) {
	const wolf uint32 = 0xF23F517F // Form of the Predator's dire winter wolf (CHAR.ESF)
	SetModelRadii(map[uint32]float32{wolf: 1, gnomeMale: 0.5, trollMale: 0.8, trollFemale: 0.8, elfMale: 0.5, elfFemale: 0.5, clayGolem: 1})
	t.Cleanup(func() { SetModelRadii(nil) })

	form := func(sprite string) CastSpell {
		s := illusionSpell(700783, "Form of the Predator", "", "")
		s.Scope = 0
		s.Def = SpellDefFrom("Form of the Predator", "", "",
			`{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[{"kind":"illusion","sprite":"`+sprite+`"}]}`)
		return s
	}
	cases := []struct {
		name, sprite string
		want         uint32
	}{
		{"the workbook's order", "0x7F513FF2", wolf},
		{"the disc's order (a GM's edit)", "0xF23F517F", wolf},
		{"neither (the control)", "0x12345678", gnomeMale},
	}
	for _, tc := range cases {
		c := illusionRig(t)
		c.land(t, form(tc.sprite), 0xFFFFFFFF, 0)
		if m, _ := channelBody(c.p); m != tc.want {
			t.Errorf("%s (%s): model %#08x, want %#08x", tc.name, tc.sprite, m, tc.want)
		}
	}

	// an item's body (itemeffect.body, the disc's order) typed as a sprite: a
	// race's, so the bearer's sex's body of that race
	for _, body := range []uint32{trollMale, bits.ReverseBytes32(trollMale)} {
		c := illusionRig(t)
		c.p.Sex = 1
		s := illusionSpell(700001, "Troll Stone", "", "")
		s.Scope = 0
		s.Def = SpellDef{Target: TargetSelf, FormKind: FormSelf, FormModel: body, Duration: 10 * time.Minute,
			Effects: []SpellEffect{{Kind: EffectIllusion}}}
		c.land(t, s, 0xFFFFFFFF, 0)
		if m, _ := channelBody(c.p); m != trollFemale {
			t.Errorf("an item's body %#08x on a woman: model %#08x, want the troll woman's %#08x", body, m, trollFemale)
		}
	}

	// without the disc's table every body is taken as given
	SetModelRadii(nil)
	c := illusionRig(t)
	c.land(t, form("0xF23F517F"), 0xFFFFFFFF, 0)
	if m, _ := channelBody(c.p); m != bits.ReverseBytes32(wolf) {
		t.Errorf("no table: model %#08x, want the sprite turned round as read, %#08x", m, bits.ReverseBytes32(wolf))
	}
}
