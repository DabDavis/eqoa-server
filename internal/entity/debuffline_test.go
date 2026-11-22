package entity

import (
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// debuffSpell is a workbook debuff of no stack type (synthetic): its stat
// changes on its target for ten minutes, a damage tick beside them if dot.
func debuffSpell(id, level int, name string, stats map[string]int, dot int) CastSpell {
	effects := ""
	for _, st := range []string{"str", "dex", "agi", "ac"} {
		if n, ok := stats[st]; ok {
			if effects != "" {
				effects += ","
			}
			effects += `{"id":"0001","type":"Statistic Change","kind":"stat","stat":"` + st + `","amount":` + strconv.Itoa(n) + `}`
		}
	}
	if dot != 0 {
		effects += `,{"id":"0002","type":"Dmg/Heal","kind":"hp_tick","amount":` + strconv.Itoa(dot) + `}`
	}
	sp := wbSpell(id, level, 1, name, `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":600,"stack":0,"level":`+
		strconv.Itoa(level)+`,"effects":[`+effects+`]}`)
	sp.Recast = 0
	return sp
}

// landDebuff lands s from caster on npc as a cast does.
func landDebuff(w *World, caster *Player, npc *NPC, s CastSpell, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cs := &castState{spell: s, caster: caster.ServerID, targets: []EntityID{npc.ServerID}}
	w.landOnNPCLocked(cs, caster, nil, npc, now)
}

func debuffFoe(w *World) *NPC {
	if w.SessionManager == nil {
		w.SessionManager = session.NewManager()
	}
	npc := &NPC{ServerID: 7101, Name: "Test Crab", Level: 40, HP: 100000, MaxHP: 100000, X: 100, Y: 10, Z: 100}
	w.NPCs[npc.ServerID] = npc
	return npc
}

func debuffCaster(w *World, id EntityID, class int) *Player {
	p := &Player{ServerID: id, Name: "Caster" + strconv.Itoa(int(id)), Class: class, Level: 50, HP: 900, MaxHP: 900, Power: 5000, MaxPower: 5000,
		Str: 100, Sta: 100, Agi: 100, Dex: 100, Wis: 100, Int: 100, Cha: 100, X: 101, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	return p
}

// A workbook debuff of no stack type lands on a creature with its class's
// line: two ranks of one class's line from two casters count only the higher
// (its stat changes), and the lower's damage tick goes on.
func TestACreatureCountsOnlyTheBestRankOfAClasssDebuff(t *testing.T) {
	noResist(t)
	w := NewWorld()
	npc := debuffFoe(w)
	a, b := debuffCaster(w, 1001, 8), debuffCaster(w, 1002, 8)
	now := time.Unix(1_800_000_000, 0)
	lower := debuffSpell(9601, 20, "Test Ailment", map[string]int{"str": -40, "dex": -50}, -25)
	higher := debuffSpell(9602, 40, "Test Plague", map[string]int{"str": -90, "dex": -120}, 0)
	if got, want := DebuffLine(&lower, 8), "debuffpriest -DEX -STR"; got != want {
		t.Fatalf("the line %q, want %q", got, want)
	}
	landDebuff(w, a, npc, higher, now)
	landDebuff(w, b, npc, lower, now.Add(time.Second))
	if len(npc.Effects) != 2 {
		t.Fatalf("%d effects on the creature, want both", len(npc.Effects))
	}
	if m := npc.creatureMods(); m.Str != -90 || m.Dex != -120 {
		t.Errorf("STR %+d, DEX %+d; want the higher rank's -90, -120 alone", m.Str, m.Dex)
	}
	if npc.HP >= npc.MaxHP {
		t.Errorf("the lower rank's damage tick did not land (HP %d of %d)", npc.HP, npc.MaxHP)
	}
	var low *ActiveEffect
	for _, e := range npc.Effects {
		if e.SpellID == lower.SpellID {
			low = e
		}
	}
	if low == nil || low.HPTick >= 0 {
		t.Errorf("the lower rank's tick is gone: %+v", low)
	}

	// a BuffLine of its own is kept (a stack type's line), and a spell of no
	// stat change has no debuff line
	stacked := lower
	stacked.Def.Stack = 4
	if got := DebuffLine(&stacked, 8); got != "" {
		t.Errorf("a stack type's spell has the debuff line %q", got)
	}
	dot := debuffSpell(9603, 20, "Test Sting", nil, -30)
	if got := DebuffLine(&dot, 8); got != "" {
		t.Errorf("a damage tick alone has the debuff line %q", got)
	}
}

// Two classes' debuffs of the same stats are two lines and both count; one
// class's of other stats too.
func TestTwoClassesDebuffsOfTheSameStatsBothCount(t *testing.T) {
	noResist(t)
	w := NewWorld()
	npc := debuffFoe(w)
	sha, enc := debuffCaster(w, 1001, 8), debuffCaster(w, 1002, 12)
	now := time.Unix(1_800_000_000, 0)
	landDebuff(w, sha, npc, debuffSpell(9611, 40, "Test Wither", map[string]int{"str": -60}, 0), now)
	landDebuff(w, enc, npc, debuffSpell(9612, 30, "Test Weaken", map[string]int{"str": -50}, 0), now)
	landDebuff(w, sha, npc, debuffSpell(9613, 20, "Test Slump", map[string]int{"str": -30, "agi": -20}, 0), now)
	if m := npc.creatureMods(); m.Str != -140 || m.Agi != -20 {
		t.Errorf("STR %+d, AGI %+d; want -140 (three lines) and -20", m.Str, m.Agi)
	}
}

// Of two of one rank the stronger counts, whichever landed first.
func TestOfOneRankTheStrongerDebuffCounts(t *testing.T) {
	noResist(t)
	for _, strongFirst := range []bool{true, false} {
		w := NewWorld()
		npc := debuffFoe(w)
		a, b := debuffCaster(w, 1001, 9), debuffCaster(w, 1002, 9)
		now := time.Unix(1_800_000_000, 0)
		weak := debuffSpell(9621, 50, "Test Gust", map[string]int{"agi": -40, "ac": -50}, 0)
		strong := debuffSpell(9622, 50, "Test Gale", map[string]int{"agi": -60, "ac": -70}, 0)
		first, second := weak, strong
		if strongFirst {
			first, second = strong, weak
		}
		landDebuff(w, a, npc, first, now)
		landDebuff(w, b, npc, second, now.Add(time.Second))
		if m := npc.creatureMods(); m.Agi != -60 || m.AC != -70 {
			t.Errorf("strong first %v: AGI %+d, AC %+d; want the stronger's -60, -70", strongFirst, m.Agi, m.AC)
		}
	}
}

// A merc's debuff is of its own class's line: a player's and a merc's of one
// class are one line, the higher counting.
func TestAMercsAndAPlayersDebuffOfOneClassAreOneLine(t *testing.T) {
	noResist(t)
	r := newMercRig(t)
	r.p.Class = 8
	lower := debuffSpell(9631, 20, "Test Ailment", map[string]int{"str": -40}, 0)
	higher := debuffSpell(9632, 40, "Test Plague", map[string]int{"str": -90}, 0)
	ms := r.hire(r.kit(8, 0, 0, lower), MercPassive)
	crab := r.creature("Desert Crab", 28, 10)
	crab.HP, crab.MaxHP = 900000, 900000
	landDebuff(r.w, r.p, crab, higher, r.at(0))
	r.w.mu.Lock()
	r.w.mercHarmLocked(ms, &lower, crab, r.at(time.Second))
	r.w.mu.Unlock()
	if len(crab.Effects) != 2 {
		t.Fatalf("%d effects on the creature, want the player's and the merc's", len(crab.Effects))
	}
	for _, e := range crab.Effects {
		if e.Line != "debuffpriest -STR" {
			t.Errorf("%q's line %q, want debuffpriest -STR", e.Name, e.Line)
		}
	}
	if m := crab.creatureMods(); m.Str != -90 {
		t.Errorf("STR %+d, want the player's higher -90 alone", m.Str)
	}
}

// A creature's debuff kept and made again (a merc's effects over a restart:
// buffsave.go's rows) keeps its line and rank.
func TestACreatureDebuffKeptKeepsItsLine(t *testing.T) {
	noResist(t)
	w := NewWorld()
	npc := debuffFoe(w)
	a, b := debuffCaster(w, 1001, 8), debuffCaster(w, 1002, 8)
	now := time.Unix(1_800_000_000, 0)
	landDebuff(w, a, npc, debuffSpell(9641, 40, "Test Plague", map[string]int{"dex": -120}, 0), now)
	landDebuff(w, b, npc, debuffSpell(9642, 20, "Test Ailment", map[string]int{"dex": -50}, 0), now)
	rows := effectRows(npc.Effects, now, nil)
	if len(rows) != 2 {
		t.Fatalf("%d rows kept, want 2", len(rows))
	}
	back := &NPC{ServerID: 7102, Name: "Test Crab", Level: 40}
	for _, r := range rows {
		back.Effects = append(back.Effects, effectFromRow(r, now))
	}
	for i, e := range back.Effects {
		if e.Line != npc.Effects[i].Line || e.Rank != npc.Effects[i].Rank || e.Line == "" {
			t.Errorf("%q made again with line %q rank %d; kept %q rank %d", e.Name, e.Line, e.Rank, npc.Effects[i].Line, npc.Effects[i].Rank)
		}
	}
	if m := back.creatureMods(); m.Dex != -120 {
		t.Errorf("DEX %+d made again, want -120 alone", m.Dex)
	}
}

// A creature debuff's line is the caster's archetype's (the live game's notes
// of 2006-04): two classes of one archetype are one line, the stronger
// counting; two archetypes' debuffs of one stat both count.
func TestCreatureDebuffLinesGoByArchetype(t *testing.T) {
	noResist(t)
	w := NewWorld()
	npc := debuffFoe(w)
	nec, enc := debuffCaster(w, 1001, 11), debuffCaster(w, 1002, 12)
	now := time.Unix(1_800_000_000, 0)
	landDebuff(w, nec, npc, debuffSpell(9651, 40, "Test Rot", map[string]int{"str": -70}, 0), now)
	landDebuff(w, enc, npc, debuffSpell(9652, 30, "Test Sap", map[string]int{"str": -50}, 0), now.Add(time.Second))
	if m := npc.creatureMods(); m.Str != -70 {
		t.Errorf("a necromancer's and an enchanter's STR debuffs: STR %+d, want the stronger -70 alone", m.Str)
	}

	w2 := NewWorld()
	npc2 := debuffFoe(w2)
	sk, sha := debuffCaster(w2, 1001, 3), debuffCaster(w2, 1002, 8)
	landDebuff(w2, sk, npc2, debuffSpell(9653, 40, "Test Scream", map[string]int{"str": -70}, 0), now)
	landDebuff(w2, sha, npc2, debuffSpell(9654, 40, "Test Fever", map[string]int{"str": -50}, 0), now.Add(time.Second))
	if m := npc2.creatureMods(); m.Str != -120 {
		t.Errorf("a shadow knight's and a shaman's STR debuffs: STR %+d, want both, -120", m.Str)
	}

	for class, want := range map[int]string{0: "tank", 2: "tank", 3: "tank", 1: "melee", 4: "melee", 5: "melee", 6: "melee",
		10: "caster", 11: "caster", 12: "caster", 13: "caster", 14: "caster", 7: "priest", 8: "priest", 9: "priest"} {
		if got := debuffArchetype(class); got != want {
			t.Errorf("class %d's archetype %q, want %q", class, got, want)
		}
	}
}
