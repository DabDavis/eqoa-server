package entity

import (
	"testing"
	"time"
)

// Heirloom petition 3462: a female were in a lionwere form wears the
// four-legged lioness (Feral Lioness Form's body), at the form's own size; a
// male keeps the lionwere body.
func TestLionwereFormFemaleWearsLioness(t *testing.T) {
	SetModelRadii(map[uint32]float32{bodyLionwere: 1, bodyLionessForm: 1, bodyHumanMale: 0.5, bodyHumanFemale: 0.5})
	t.Cleanup(func() { SetModelRadii(nil) })

	wear := func(sex int) (uint32, float32, float32) {
		c := illusionRig(t)
		c.p.Sex = sex
		s := illusionSpell(1816, "Lionwere", "", "")
		s.Scope = 0
		s.Def = SpellDef{Target: TargetSelf, FormKind: FormSelf, FormModel: bodyLionwere, Duration: 10 * time.Minute,
			Effects: []SpellEffect{{Kind: EffectIllusion}}}
		c.land(t, s, 0xFFFFFFFF, 0)
		m, size := c.p.Body()
		return m, size, c.p.Size
	}
	if m, size, own := wear(1); m != bodyLionessForm || size != own {
		t.Errorf("female: model %#08x size %.2f, want %#08x at her size %.2f", m, size, uint32(bodyLionessForm), own)
	}
	if m, _, _ := wear(0); m != bodyLionwere {
		t.Errorf("male: model %#08x, want the lionwere's %#08x", m, uint32(bodyLionwere))
	}
}
