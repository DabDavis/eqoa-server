package entity

import (
	"strings"
	"testing"
	"time"
)

// Petition 1805 (Skywalker, an Erudite shadow knight): "Summon Undead Servant
// (Familiar) ... The pet Buffs 300 Hp and 300 Power along with a 80 lifetap
// proc that after the mob is debuffed, will often proc 160 instead". The rows
// as tools/undead_servant_20261112.py writes them.

const (
	servitudeBuffEffects = `{"v":1,"scope":"target","target_type":"Owner","harmful":false,"duration_s":1800,"stack":0,` +
		`"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Pet Buff","effects":[` +
		`{"id":"F805","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300},` +
		`{"id":"F806","type":"Statistic Change","kind":"stat","stat":"maxpower","amount":300},` +
		`{"id":"F807","type":"Offensive Proc","kind":"proc","procs":[` +
		`{"id":"F808","type":"Dmg/Heal","kind":"hp","amount":-80,"debuffed_pct":100,"on":"offense","scope":"target","target_type":"Enemy"},` +
		`{"id":"F809","type":"Dmg/Heal","kind":"hp","amount":80,"debuffed_pct":100,"on":"offense","scope":"self","target_type":"Self"}]}]}`
	servitudeAbilities = `[{"every":30.0,"first":1.0,"kind":"buff","spell_id":907236,"target":"owner","while_out":true}]`
)

var summonServant = summonSpell(701236, 60, "Summon Servant", "Summons Pet")

func servitudeBuff(t *testing.T) CastSpell {
	t.Helper()
	d, err := ParseSpellJSON("Undead Servitude", "", servitudeBuffEffects)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Unsupported) != 0 {
		t.Fatalf("unsupported: %v", d.Unsupported)
	}
	return CastSpell{SpellID: 907236, Name: "Undead Servitude", Level: 60, Scope: 1, CastTime: 2 * time.Second, Def: d}
}

// The buff: +300 maximum HP and power, a proc of 80 that heals 80, each 100%
// more on what its bearer has debuffed.
func TestUndeadServitudeIsHPPowerAndALifetap(t *testing.T) {
	s := servitudeBuff(t)
	o := &Player{ServerID: 7, Name: "Skywalker", Level: 60, Int: 300}
	ae := lasting(&castState{spell: s, caster: o.ServerID, targets: []EntityID{o.ServerID}}, o, time.Now())
	if ae == nil {
		t.Fatal("nothing lasts")
	}
	if ae.Proc != 80 || ae.ProcMax != 80 || ae.Rules.ProcHeal != 80 || ae.Rules.ProcDebuffedPct != 100 {
		t.Errorf("proc %d-%d, heal %d, debuffed %d%%: want 80, 80, 100%%", ae.Proc, ae.ProcMax, ae.Rules.ProcHeal,
			ae.Rules.ProcDebuffedPct)
	}
	if ae.Mods.MaxHP != 300 || ae.Mods.MaxPower != 300 {
		t.Errorf("mods %+v: want max HP and power +300", ae.Mods)
	}
}

// The owner's melee procs 80 (and heals 80) on a creature it has put nothing
// on, or only another's harm; 160 (and 160) on one carrying a harmful spell of
// its own still lasting.
func TestTheServantsLifetapIs160OnWhatItsOwnerDebuffed(t *testing.T) {
	s := servitudeBuff(t)
	r := newProcRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	now := time.Now()
	ae := lasting(&castState{spell: s, caster: r.friend.ServerID, targets: []EntityID{r.friend.ServerID}}, r.friend, now)
	r.friend.Effects = []*ActiveEffect{ae}
	for _, c := range []struct {
		name string
		on   []*ActiveEffect
		want int
	}{
		{"nothing on it", nil, 80},
		{"another's debuff", []*ActiveEffect{{SpellID: 1423, Caster: 9999, Until: now.Add(time.Minute), Rules: EffectRules{Harm: true}}}, 80},
		{"its own debuff, over", []*ActiveEffect{{SpellID: 1423, Caster: r.friend.ServerID, Until: now.Add(-time.Second),
			Rules: EffectRules{Harm: true}}}, 80},
		{"its own buff-like effect", []*ActiveEffect{{SpellID: 1, Caster: r.friend.ServerID, Until: now.Add(time.Minute)}}, 80},
		{"its own debuff", []*ActiveEffect{{SpellID: 1423, Caster: r.friend.ServerID, Until: now.Add(time.Minute),
			Rules: EffectRules{Harm: true}}}, 160},
	} {
		t.Run(c.name, func(t *testing.T) {
			r.npc.Effects = c.on
			r.npc.HP = r.npc.MaxHP
			r.friend.HP = 100
			procIntn = dice(0, 99)
			r.fsess.Queue().GetPending()
			r.w.mu.Lock()
			r.w.playerHitLocked(r.fsess, r.friend, r.npc, r.npcF)
			r.w.mu.Unlock()
			f := figures(r.fsess)
			if len(f) < 2 || f[1] != -c.want {
				t.Errorf("figures %v: want the proc's -%d second", f, c.want)
			}
			if r.friend.HP != 100+c.want {
				t.Errorf("the tap healed %d, want %d", r.friend.HP-100, c.want)
			}
		})
	}
}

// The servant summoned buffs its owner (its row's ability), the buff is not
// kept over a logout, and it comes off when the servant goes.
func TestTheUndeadServantBuffsItsOwnerWhileOut(t *testing.T) {
	r, db := magePetRig(t, false)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (907236, 60, 0, 0, 16880, 2, 0, 0, 0, 1, 30, -1, 'Undead Servitude', '')`)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (907236, '', 4089483784, 'test', ?)`,
		servitudeBuffEffects)
	exec(`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, stances, ac, hp, wpn1_dmg, wpn1_type,
		abilities, source) VALUES (701236, '0064', 'Undead Servant', 1794738819, 45, 'melee', 'passive', 1350, 3203, 540, 'slash',
		?, 'test')`, servitudeAbilities)
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.w.ClassTemplateProblems() {
		if strings.Contains(p, "701236") {
			t.Fatalf("the servant's row: %s", p)
		}
	}
	r.p.Class = 3
	hp0, pow0 := r.p.MaxHPNow(), r.p.MaxPowerNow()
	pet, _ := r.summon(t, summonServant, 0)
	has := func() *ActiveEffect {
		for _, e := range r.p.Effects {
			if e.SpellID == 907236 {
				return e
			}
		}
		return nil
	}
	for ms := 9000; ms < 20000; ms += 100 {
		r.w.mu.Lock()
		r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
		r.w.mu.Unlock()
	}
	ae := has()
	if ae == nil {
		t.Fatalf("the owner has no Undead Servitude: %d effects", len(r.p.Effects))
	}
	if ae.Proc != 80 || ae.Rules.ProcHeal != 80 || !ae.petBound {
		t.Errorf("proc %d, heal %d, bound to the pet %v", ae.Proc, ae.Rules.ProcHeal, ae.petBound)
	}
	if got := r.p.MaxHPNow() - hp0; got != 300 {
		t.Errorf("max HP +%d, want +300", got)
	}
	if got := r.p.MaxPowerNow() - pow0; got != 300 {
		t.Errorf("max power +%d, want +300", got)
	}
	for _, row := range savedEffects(r.p, r.at(20*time.Second)) {
		if row.SpellID == 907236 {
			t.Error("the servant's buff is kept over a logout")
		}
	}
	r.w.mu.Lock()
	r.w.dismissPetLocked(r.w.petStateLocked(r.p), nil, "", r.at(21*time.Second))
	r.w.mu.Unlock()
	if has() != nil || r.p.MaxHPNow() != hp0 {
		t.Errorf("the servant gone (dead %v), its buff stays: max HP %d (was %d)", pet.Dead, r.p.MaxHPNow(), hp0)
	}
}
