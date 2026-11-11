package entity

import (
	"fmt"
	"testing"
	"time"
)

// The owner, 2026-11-11: "largesse of plague when the pets proc is the wrong spell fx at the moment. it
// looks like a sickly cloud that dissipates quickly", "0x830d8a97 is the dictid for LoP proc". A proc's
// own look (proc_fx in the spell's effects JSON) at what each proc hits: a pet's, a player's, a creature
// template's pinned on the spell.

const fxLargesseProc = 0x830D8A97

// largesseJSON is Largesse of Plague's effects as the EQOAEmu import keeps them (spell 700794), with
// procFX (a JSON value, "" none) on its proc.
func largesseJSON(procFX string) string {
	fx := ""
	if procFX != "" {
		fx = `,"proc_fx":` + procFX
	}
	return `{"v":1,"scope":"pet","target_type":"Friendly","harmful":false,"duration_s":300,"stack":0,"level":60,` +
		`"sheet_type":"Offensive Proc","effects":[{"id":"04B5","type":"Offensive Proc","kind":"proc","procs":[{"id":"0D4E",` +
		`"type":"Dmg/Heal","kind":"hp","amount":-650,"resist":"dr","on":"offense","scope":"target","target_type":"Enemy"` + fx + `}]}]}`
}

// largesseOn is the Largesse buff as it lands (lasting: cast.go) from those effects.
func largesseOn(t *testing.T, procFX string, caster *Player, now time.Time) *ActiveEffect {
	t.Helper()
	d := SpellDefFrom("Largesse of Plague", "Pet, Decrease Hitpoints by 650, 5min", "", largesseJSON(procFX))
	cs := &castState{spell: CastSpell{SpellID: 700794, Name: "Largesse of Plague", Level: 60, Def: d}, caster: caster.ServerID}
	ae := lasting(cs, caster, now)
	if ae.Proc != 650 {
		t.Fatalf("Largesse landed with proc %d, want 650 (%+v)", ae.Proc, d.Effects)
	}
	return ae
}

// proc_fx reads as a number or a hex string, on the proc's entry or its Offensive Proc; none, 0.
func TestProcFXReads(t *testing.T) {
	for _, c := range []struct {
		name, json string
		want       uint32
	}{
		{"a number", largesseJSON("2198702743"), fxLargesseProc},
		{"a hex string", largesseJSON(`"0x830d8a97"`), fxLargesseProc},
		{"none (the control)", largesseJSON(""), 0},
		{"on its Offensive Proc", `{"v":1,"scope":"pet","duration_s":300,"effects":[{"id":"1","type":"Offensive Proc","kind":"proc",` +
			`"proc_fx":"0x830D8A97","procs":[{"id":"2","kind":"hp","amount":-650,"on":"offense"}]}]}`, fxLargesseProc},
	} {
		d, err := ParseSpellJSON("Largesse of Plague", "", c.json)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []uint32
		for _, e := range d.Effects {
			if e.Kind == EffectProc {
				got = append(got, e.ProcFX)
			}
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: proc looks %x, want %x", c.name, got, c.want)
		}
	}
	if _, err := ParseSpellJSON("x", "", largesseJSON(`"not hex"`)); err == nil {
		t.Error("a proc_fx of no DictID read")
	}
}

// A pet with Largesse procs: the 0x00B1 from it at what it hits carries 0x830D8A97, once a proc. The
// control, the same buff with no proc_fx, sends no visual (its figure alone, as before).
func TestAPetsLargesseProcShowsItsLook(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	for _, c := range []struct {
		name, fx string
		want     []uint32
	}{
		{"proc_fx 0x830D8A97", `"0x830D8A97"`, []uint32{fxLargesseProc}},
		{"no proc_fx (the control)", "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTagRig(t)
			r.pet.Level = 60
			r.pet.Effects = []*ActiveEffect{largesseOn(t, c.fx, r.p, r.t0)}
			procIntn = dice(0, 99) // under the chance, over the resist
			hp := r.npc.HP
			r.w.mu.Lock()
			r.w.petProcsLocked(r.ps, r.npc, r.t0)
			r.w.mu.Unlock()
			if hp-r.npc.HP <= 0 {
				t.Fatalf("no proc landed (HP %d -> %d)", hp, r.npc.HP)
			}
			var fxs []uint32
			for _, b := range castsIn(r.sent()) {
				if b.caster != r.petO {
					t.Errorf("a visual from %08X, not the pet", b.caster)
				}
				fxs = append(fxs, b.fx)
			}
			if fmt.Sprintf("%x", fxs) != fmt.Sprintf("%x", c.want) {
				t.Errorf("the pet's proc's visuals %x, want %x", fxs, c.want)
			}
		})
	}
}

// A player's own Largesse proc (a proc buff on the player) shows its look; the control, no proc_fx, the
// damage shield's answer's as before.
func TestAPlayersProcShowsItsLook(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	for _, c := range []struct {
		name, fx string
		want     uint32
	}{
		{"proc_fx", "2198702743", fxLargesseProc},
		{"no proc_fx (the control)", "", fxShieldHit},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTagRig(t)
			r.p.Effects = []*ActiveEffect{largesseOn(t, c.fx, r.p, r.t0)}
			procIntn = dice(0, 99)
			r.w.mu.Lock()
			r.w.playerProcsLocked(r.sess, r.p, r.npc, r.t0)
			r.w.mu.Unlock()
			var fxs []uint32
			for _, b := range castsIn(r.sent()) {
				fxs = append(fxs, b.fx)
			}
			if len(fxs) != 1 || fxs[0] != c.want {
				t.Errorf("the proc's visuals %x, want %x", fxs, c.want)
			}
		})
	}
}

// A creature's template proc pinned on the spell (Innoruuk's Largesse) shows the spell's proc_fx, not the
// ability's own fx (#597's disease hit, 0x0CACDEE1); the control, the spell with no proc_fx, keeps the
// ability's.
func TestATemplateProcPinnedOnTheSpellShowsItsLook(t *testing.T) {
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	for _, c := range []struct {
		name, fx string
		want     uint32
	}{
		{"proc_fx", `"0x830D8A97"`, fxLargesseProc},
		{"no proc_fx (the control)", "", fxDiseaseHit},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTemplateRig(t)
			for _, q := range []string{
				`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire,
					spellname, spelldesc) VALUES (7301, 50, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Test Largesse', '')`,
				fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (7301, '', %d, 'test', '%s')`,
					fxLargesseBuff, largesseJSON(c.fx)),
			} {
				if _, err := r.db.Exec(q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			r.godTemplate("hated", ", class, melee_pct", ", 0, 10", fmt.Sprintf(`[{"kind":"proc","spell_id":7301,"fx":%d}]`, fxDiseaseHit), "gnoll brute")
			inno := r.npc("Gnoll Brute")
			pool(inno, 500_000)
			p, sp := r.player("Tank", inno.X+1, inno.Z)
			p.HP, p.MaxHP = 10_000_000, 10_000_000
			r.fight(inno, p.ServerID)
			drain(sp)
			templateIntn = func(int) int { return 0 }
			hp := p.HP
			r.w.mu.Lock()
			r.w.abilityProcsOnPlayerLocked(r.w.findSessionByEntityIDLocked(p.ServerID), p, inno, r.at(time.Minute))
			r.w.mu.Unlock()
			if hp-p.HP != 650 {
				t.Errorf("the proc took %d, want 650", hp-p.HP)
			}
			var fxs []uint32
			for _, b := range castsIn(drain(sp)) {
				fxs = append(fxs, b.fx)
			}
			if len(fxs) != 1 || fxs[0] != c.want {
				t.Errorf("the visuals at him %x, want %x alone", fxs, c.want)
			}
		})
	}
}
