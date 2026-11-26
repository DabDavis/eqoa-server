package entity

import (
	"testing"
	"time"
)

// Petition 5645 (Garrak, Live): a buff of two offensive damage procs. Shroud of Death (901474) as
// Live's effects have it on 2026-11-26: 110 + 10% INT, AR, at the enemy, and 110 + 10% STR, DR.
// Until the fix the second overwrote the first at the cast (its damage and scale), the resist
// staying the first's (rulesOf), so each proc was one hit of 110 + STR/10 resisted by AR: the
// log's "Shroud of Death hits ice drake for 147", over and over.
const shroudTwoProcs = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":900,"stack":2,"level":46,"min_level":1,
"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"id":"0774","type":"Offensive Proc","kind":"proc","procs":[
{"id":"0D79","type":"Dmg/Heal","kind":"hp","amount":-110,"scale":"int","scale_pct":10,"resist":"ar","on":"offense","scope":"target","target_type":"Enemy"},
{"id":"0D81","type":"Dmg/Heal","kind":"hp","amount":-110,"scale":"str","scale_pct":10,"resist":"dr","on":"offense","scope":"self","target_type":"Self"}]}]}`

// Heirloom's: a damage proc and a heal of its bearer (a life steal), which stays one proc.
const shroudLifeSteal = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":2,"level":46,"min_level":1,
"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"id":"0773","type":"Offensive Proc","kind":"proc","procs":[
{"id":"0D78","type":"Dmg/Heal","kind":"hp","amount":-112,"scale":"int","scale_pct":20,"resist":"ar","on":"offense","scope":"target","target_type":"Enemy"},
{"id":"0D80","type":"Dmg/Heal","kind":"hp","amount":56,"scale":"str","scale_pct":10,"resist":"dr","on":"offense","scope":"self","target_type":"Self"}]}]}`

// Stormlord's Rune (701232, both shards): the workbook's "336 ; Increase 10% based on DEX" as two
// rows, the figure and then the figure with its scale: one proc, as it always was.
const stormlordRows = `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":300,"stack":1,"level":60,"min_level":33,
"req_equipment":"Any","sheet_type":"Offensive Proc","arch_type":"Offensive Proc","effects":[{"id":"02FB","type":"Offensive Proc","kind":"proc","procs":[
{"id":"0D26","type":"Dmg/Heal","kind":"hp","amount":-336,"resist":"lr","on":"offense","scope":"target","target_type":"Enemy"},
{"id":"0D27","type":"Dmg/Heal","kind":"hp","amount":-336,"scale":"dex","scale_pct":10,"resist":"lr","on":"offense","scope":"target","target_type":"Enemy"}]}]}`

func landedProcBuff(t *testing.T, name, js string) *ActiveEffect {
	t.Helper()
	d, err := ParseSpellJSON(name, "", js)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	cs := &castState{spell: CastSpell{SpellID: 901474, Name: name, Def: d}}
	ae := lastingParts(cs, &Player{Level: 60}, time.Unix(1_800_000_000, 0))
	if ae == nil {
		t.Fatalf("%s: nothing lasts", name)
	}
	ae.Until = time.Unix(2_000_000_000, 0)
	return ae
}

// byStat is a bearer of those stats (the rest 0).
func byStat(m map[Stat]int) func(Stat) int {
	return func(s Stat) int { return m[s] }
}

func TestTwoOffensiveProcsAreTheirOwn(t *testing.T) {
	ae := landedProcBuff(t, "Shroud of Death +", shroudTwoProcs)
	if ae.Proc != 110 || ae.ProcScale != StatInt || ae.ProcScalePct != 10 || ae.Rules.Resist != StatAR {
		t.Fatalf("the first proc: %d + %d%% of %v, %v; want 110 + 10%% INT, AR (it was overwritten)", ae.Proc, ae.ProcScalePct, ae.ProcScale, ae.Rules.Resist)
	}
	if len(ae.MoreProcs) != 1 {
		t.Fatalf("more procs: %+v, want the second", ae.MoreProcs)
	}
	if m := ae.MoreProcs[0]; m.Proc != 110 || m.Scale != StatStr || m.ScalePct != 10 || m.Resist != StatDR {
		t.Fatalf("the second proc: %+v; want 110 + 10%% STR, DR", m)
	}

	// both proc (the chance rolls 0) and neither is resisted (the resist rolls 99): one hit
	// each, its own scale and resist; the bearer's heal goes with the first alone
	now := time.Unix(1_900_000_000, 0)
	stats := byStat(map[Stat]int{StatInt: 200, StatStr: 370})
	hits, resisted := procRollsOf([]*ActiveEffect{ae}, stats, 60, 60, 0, notUndead, now, dice(0, 99))
	if resisted != 0 || len(hits) != 2 {
		t.Fatalf("hits %+v, resisted %d; want two", hits, resisted)
	}
	if h := hits[0]; h.n != 130 || h.resist != StatAR || !h.tap {
		t.Errorf("the first: %+v; want 130 (110 + INT 200/10), AR, the tap's", h)
	}
	if h := hits[1]; h.n != 147 || h.resist != StatDR || h.tap {
		t.Errorf("the second: %+v; want 147 (110 + STR 370/10), DR, no tap", h)
	}

	// each rolls its own chance: the first misses (99 over any chance), the second procs
	hits, _ = procRollsOf([]*ActiveEffect{ae}, stats, 60, 60, 0, notUndead, now, dice(99, 0, 99))
	if len(hits) != 1 || hits[0].n != 147 || hits[0].resist != StatDR {
		t.Errorf("the first missed: %+v; want the second alone", hits)
	}
	// and its own resist: the first resisted (its least, no scale), the second not
	hits, resisted = procRollsOf([]*ActiveEffect{ae}, stats, 60, 60, 0, notUndead, now, dice(0, 0, 0, 99))
	if resisted != 1 || len(hits) != 2 || hits[0].n != 110 || hits[1].n != 147 {
		t.Errorf("the first resisted: %+v (%d resisted); want 110 then 147", hits, resisted)
	}
}

// A buff of one proc rolls as it did: one roll of each die, its figure, its spell's resist.
func TestOneProcBuffAsBefore(t *testing.T) {
	ae := landedProcBuff(t, "Shroud of Death", shroudLifeSteal)
	if len(ae.MoreProcs) != 0 || ae.Proc != 112 || ae.ProcScale != StatInt || ae.Rules.ProcHeal != 56 || ae.Rules.ProcHealScale != StatStr {
		t.Fatalf("the life steal: proc %d (%v), heal %d (%v), more %+v", ae.Proc, ae.ProcScale, ae.Rules.ProcHeal, ae.Rules.ProcHealScale, ae.MoreProcs)
	}
	rolls := 0
	counted := func(n int) int { rolls++; return min(map[bool]int{true: 0, false: 99}[rolls == 1], n-1) }
	hits, _ := procRollsOf([]*ActiveEffect{ae}, flatStat(100), 60, 60, 0, notUndead, time.Unix(1_900_000_000, 0), counted)
	if len(hits) != 1 || hits[0].n != 132 || hits[0].resist != StatAR || !hits[0].tap || rolls != 2 {
		t.Errorf("one proc: %+v after %d rolls; want 132 (112 + 20%% of 100), AR, the tap's, 2 rolls", hits, rolls)
	}
}

func TestStormlordsTwoRowsAreOneProc(t *testing.T) {
	d, err := ParseSpellJSON("Stormlord's Rune", "", stormlordRows)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range d.Effects {
		if e.Kind == EffectProc {
			n++
			if e.Amount != 336 || e.Scale != StatDex || e.ScalePct != 10 || !e.ScaleBearer || e.Resist != StatLR {
				t.Errorf("its proc: %+v; want 336 + 10%% of its bearer's DEX, LR", e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d procs, want one: %+v", n, d.Effects)
	}
	ae := landedProcBuff(t, "Stormlord's Rune", stormlordRows)
	if ae.Proc != 336 || ae.ProcScale != StatDex || len(ae.MoreProcs) != 0 {
		t.Errorf("landed: %d + %v, more %+v", ae.Proc, ae.ProcScale, ae.MoreProcs)
	}
}

// The second proc is kept over a logout.
func TestMoreProcsKeptOverALogout(t *testing.T) {
	ae := landedProcBuff(t, "Shroud of Death +", shroudTwoProcs)
	now := time.Unix(1_900_000_000, 0)
	rows := effectRows([]*ActiveEffect{ae}, now, nil)
	if len(rows) != 1 {
		t.Fatalf("rows %+v", rows)
	}
	back := effectFromRow(rows[0], now)
	if back.Proc != 110 || len(back.MoreProcs) != 1 || back.MoreProcs[0] != ae.MoreProcs[0] {
		t.Errorf("back: proc %d, more %+v; want %+v", back.Proc, back.MoreProcs, ae.MoreProcs)
	}
}
