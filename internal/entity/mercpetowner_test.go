package entity

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 3133 (a shadow knight with a wizard merc): "I had a 5minute
// Familiar heal on me the character? If thats from the WIZ pet, that shouldn't
// be." A merc's pet's abilities on its owner (the Arch Familiar's Familiar Heal
// and Familiar Blessing, as the workbook's rows have them; a heal of its owner)
// land on its merc, never on the hirer: the merc's maximum hit points and power
// raised by them, the hirer's untouched.
func TestMercPetsOwnerAbilitiesAreItsMercs(t *testing.T) {
	spell := func(id int, name, js string) *CastSpell {
		t.Helper()
		d, err := ParseSpellJSON(name, "", js)
		if err != nil {
			t.Fatal(err)
		}
		return &CastSpell{SpellID: id, Name: name, Level: 55, Scope: 1, CastTime: 3 * time.Second, Def: d}
	}
	heal := spell(902274, "Familiar Heal (55)", `{"v":1,"scope":"target","target_type":"Owner","harmful":false,"duration_s":600,`+
		`"stack":1,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","arch_type":"Pet Heal","effects":[`+
		`{"id":"0BED","type":"Dmg/Heal","kind":"stat","stat":"maxhp","amount":550}]}`)
	bless := spell(902275, "Familiar Blessing (55)", `{"v":1,"scope":"target","target_type":"Owner","harmful":false,"duration_s":1800,`+
		`"stack":1,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Pet Buff","effects":[`+
		`{"id":"0BEE","type":"Statistic Change","kind":"stat","stat":"maxpower","amount":275},`+
		`{"id":"0BEE","type":"Statistic Change","kind":"stat","stat":"pot","amount":27}]}`)
	mend := spell(902900, "Familiar Mend", `{"v":1,"scope":"target","target_type":"Owner","harmful":false,"duration_s":0,`+
		`"stack":1,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[`+
		`{"id":"0BEF","type":"Dmg/Heal","kind":"hp","amount":200}]}`)

	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	ps := ms.pet
	ability := func(kind, target string, every time.Duration, s *CastSpell) templateAbility {
		return templateAbility{TemplateAbility: database.TemplateAbility{Kind: kind, Target: target, Below: 90},
			every: every, first: time.Second, pinned: s}
	}
	r.w.mu.Lock()
	ps.tmpl = &petTemplate{tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{TakenPct: 100, MeleePct: 100},
		abilities: []templateAbility{
			ability(database.AbilityBuff, database.AbilityOnOwner, time.Minute, heal),
			ability(database.AbilityBuff, database.AbilityOnOwner, 30*time.Minute, bless),
			ability(database.AbilityHeal, database.AbilityOnOwner, 6*time.Second, mend),
		}}}
	hpWas, powerWas := ms.npc.MaxHP, ms.npc.MaxPower
	ownerHP := r.p.MaxHPNow()
	r.p.HP = r.p.MaxHPNow() / 2 // its hirer hurt: a heal of the owner must not go to it
	ms.npc.HP = ms.npc.MaxHP / 2
	hurt := ms.npc.HP
	r.w.mu.Unlock()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	r.run(10100*time.Millisecond, 30*time.Second)
	log.SetOutput(os.Stderr)
	logged := buf.String()

	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, s := range []*CastSpell{heal, bless} {
		if hasEffect(r.p.Effects, s.SpellID) {
			t.Errorf("%q on the hirer", s.Name)
		}
		if !hasEffect(ms.npc.Effects, s.SpellID) {
			t.Errorf("%q not on the merc", s.Name)
		}
	}
	if r.p.MaxHPNow() != ownerHP {
		t.Errorf("the hirer's max HP %d, was %d", r.p.MaxHPNow(), ownerHP)
	}
	if r.p.HP != r.p.MaxHPNow()/2 && r.p.HP > ownerHP/2 {
		t.Errorf("the hirer healed to %d of %d by its merc's pet", r.p.HP, ownerHP)
	}
	if ms.npc.MaxHP != hpWas+550 || ms.body.MaxHPNow() != ms.npc.MaxHP {
		t.Errorf("the merc's max HP %d (body %d), want %d+550", ms.npc.MaxHP, ms.body.MaxHPNow(), hpWas)
	}
	if ms.npc.MaxPower != powerWas+275 {
		t.Errorf("the merc's max power %d, want %d+275", ms.npc.MaxPower, powerWas)
	}
	if ms.npc.HP <= hurt || !strings.Contains(logged, `heals its merc Ralthas with "Familiar Mend"`) {
		t.Errorf("the merc's HP %d, hurt to %d: its pet's heal of its owner never reached it", ms.npc.HP, hurt)
	}
	if strings.Contains(logged, "its owner with") {
		t.Errorf("a merc's pet's ability went to its owner:\n%s", logged)
	}
}
