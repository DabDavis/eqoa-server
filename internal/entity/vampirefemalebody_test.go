package entity

import (
	"testing"
	"time"
)

const bodyDawnMaiden = 0xD7540FDA

// world.vampire_female_body: unset, a female vampire wears the human woman's
// body as before; set to the Dawn Maiden's, hers; the male form never changes.
func TestVampireFemaleBodyConfig(t *testing.T) {
	SetModelRadii(map[uint32]float32{bodyFledglingVampire: 1, bodyHumanMale: 0.5, bodyHumanFemale: 0.5, bodyDawnMaiden: 0.5})
	t.Cleanup(func() { SetModelRadii(nil) })

	wear := func(set uint32, sex int) uint32 {
		c := illusionRig(t)
		c.w.SetVampireFemaleBody(set)
		c.p.Sex = sex
		s := illusionSpell(700900, "Vampire Form", "", "")
		s.Scope = 0
		s.Def = SpellDef{Target: TargetSelf, FormKind: FormSelf, FormModel: bodyFledglingVampire, Duration: 10 * time.Minute,
			Effects: []SpellEffect{{Kind: EffectIllusion}}}
		c.land(t, s, 0xFFFFFFFF, 0)
		m, _ := channelBody(c.p)
		return m
	}
	cases := []struct {
		name string
		set  uint32
		sex  int
		want uint32
	}{
		{"default, female: the human woman's", 0, 1, bodyHumanFemale},
		{"default, male: the fledgling's", 0, 0, bodyFledglingVampire},
		{"Dawn Maiden, female", bodyDawnMaiden, 1, bodyDawnMaiden},
		{"Dawn Maiden set, male unchanged", bodyDawnMaiden, 0, bodyFledglingVampire},
	}
	for _, tc := range cases {
		if got := wear(tc.set, tc.sex); got != tc.want {
			t.Errorf("%s: model %#08x, want %#08x", tc.name, got, tc.want)
		}
	}
}
