package entity

import (
	"math"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

func TestHeroicMarksCreditAndMastery(t *testing.T) {
	r := newMythic(t, 2)
	w := r.w
	b := r.boss(t, r.enter(t, 0))
	var got string
	w.OnKill(func(c KillCredit) { got = c.HeroicBoss })
	credit := func(n *NPC) string {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.creditKillLocked(r.ss[0], r.ps[0], n)
		return got
	}
	if credit(b) != "" {
		t.Fatal("marks enabled by default")
	}
	w.heroicMarks = true
	for _, n := range []*NPC{
		{ServerID: mythicTestBoss, Name: "Lord Nagafen", World: 3},
		{ServerID: 1234, Name: MythicGuardName, World: b.World, Instance: b.Instance, mythic: b.mythic},
	} {
		if credit(n) != "" {
			t.Fatalf("%s gave heroic clear credit", n.Name)
		}
	}
	if credit(b) != EncounterMythicNagafen {
		t.Fatal("the instanced boss did not give heroic clear credit")
	}
	set := database.HeroicMarksFor(EncounterMythicNagafen)
	w.SetCMCatalog([]database.CMRankRow{
		{ID: set.Mastery.Base, Name: "God Touched", Mastery: "God Touched", Rank: 1, Cost: 75, Classes: 32767, Races: 2047, Enabled: true},
		{ID: set.Mastery.ID, Name: set.Mastery.Name, Mastery: "God Touched", Rank: 2, Parent: set.Mastery.Base, Cost: 75, Classes: 32767, Races: 2047, Effects: "FR:25", Enabled: true},
	})
	s := r.ss[0]
	p := r.ps[0]
	p.Mastery.Points = 150
	if _, shown := w.CMNodeOf(s, set.Mastery.ID, CMKnows{}); shown {
		t.Fatal("locked rank is shown")
	}
	if res := w.BuyCM(s, set.Mastery.ID, CMKnows{}); res.Refusal == "" {
		t.Fatal("locked rank bought")
	}
	unlocked := CMKnows{HeroicUnlocks: map[string]bool{set.Boss: true}}
	listed := w.CMList(s, unlocked)
	if !slices.ContainsFunc(listed, func(e CMEntry) bool { return e.ID == set.Mastery.ID && e.Cost == 75 && !e.Known }) {
		t.Fatalf("rank II should be visible and require rank I: %+v", listed)
	}
	if res := w.BuyCM(s, set.Mastery.ID, unlocked); res.Refusal == "" {
		t.Fatal("rank II bought before rank I")
	}
	if res := w.BuyCM(s, set.Mastery.Base, CMKnows{}); res.Refusal != "" {
		t.Fatal(res.Refusal)
	}
	if res := w.BuyCM(s, set.Mastery.ID, unlocked); res.Refusal != "" || res.Paid != 75 || res.Mastery.Points != 0 {
		t.Fatalf("rank II purchase: %+v", res)
	}
}

func TestHeroicMarksCombat(t *testing.T) {
	r := newProcRig(t)
	p, npc, w := r.friend, r.npc, r.w
	now := time.Now()
	set := database.HeroicMarksFor(EncounterMythicNagafen)
	defer func(before func(int) int) { procIntn = before }(procIntn)
	setRole := func(class int) {
		p.Class = class
		p.Gear.HeroicRelics = []int{set.RoleFor(class).RelicID}
	}
	setRole(0)
	w.heroicTaunts = map[int][]SpellEffect{1608: {{Kind: EffectHate, Amount: 594, Scale: StatStr, ScalePct: 11}}}
	npc.Threats = map[EntityID]int64{p.ServerID: 100, r.p.ServerID: 5_000}
	procIntn = func(int) int { return 25 }
	w.heroicAttackProcsLocked(r.fsess, p, npc, now)
	if npc.Threats[p.ServerID] != 100 {
		t.Fatal("tank proc rolled at 25")
	}
	procIntn = func(int) int { return 24 }
	want := 100 + pctOf(int64(playerTauntHate(594+p.StatNow(StatStr)*11/100)), 105)
	w.heroicAttackProcsLocked(r.fsess, p, npc, now)
	if got := npc.Threats[p.ServerID]; got != want {
		t.Fatalf("tank threat %d, want additive %d", got, want)
	}
	setRole(1)
	hp := npc.HP
	w.heroicAttackProcsLocked(r.fsess, p, npc, now)
	if loss := hp - npc.HP; loss != 25 {
		t.Fatalf("melee fire proc %d, want 25", loss)
	}
	wp, _ := WeaponProcOf(9001, "Selo's Jeweled Rapier", "")
	p.Gear.MainProc = &wp
	figures(r.fsess)
	procIntn = dice(0, 99, 0)
	r.hit()
	if got := figures(r.fsess); len(got) != 3 || got[1] != -140 || got[2] != -25 {
		t.Fatalf("auto attack and independent weapon and relic procs: %v", got)
	}
	p.Gear.MainProc = nil
	setRole(10)
	cast := func(roll int) (int, *castState) {
		procIntn = func(int) int { return roll }
		cs := &castState{targets: []EntityID{npc.ServerID}, spell: CastSpell{
			Name: "Test bolt", Def: SpellDef{Harmful: true, Effects: []SpellEffect{{Kind: EffectHP, Amount: -50, AmountMax: -50}}},
		}}
		hp := npc.HP
		w.landLocked(cs, p, now)
		return hp - npc.HP, cs
	}
	control, _ := cast(50)
	hit, cs := cast(49)
	copyForSplash := *cs
	if hit-control != 75 || copyForSplash.takeHeroicSpellRoll() {
		t.Fatalf("spell and relic damage %d, ordinary spell damage %d, want one 75-point proc", hit, control)
	}
	setRole(7)
	p.BaseCha = 600
	p.CMEffects[cmSlotCap+int(StatCha-StatStr)] = 250
	p.RecalcStats(false)
	if p.StatNow(StatCha) != 600 {
		t.Fatalf("healer has %d charisma, fixture requires 600", p.StatNow(StatCha))
	}
	for _, kind := range []EffectKind{EffectHP, EffectHPTick} {
		if got := spellAmount(SpellEffect{Kind: kind, Amount: 150, Cap: 150}, p); got != 180 {
			t.Fatalf("heal bonus on %v = %d, want 180 through the original cap", kind, got)
		}
	}
}

func TestHeroicMarksBridgeLanding(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR must contain Lavastorm collision and environment")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorld()
	w.SetGround(g)
	w.SetEnv(e)
	to := database.HeroicMarksFor(EncounterMythicNagafen).Teleport
	x, y, z := w.gateLandingLocked(database.SpellGate{World: uint8(to.World), X: to.X, Y: to.Y, Z: to.Z})
	if x != to.X || math.Abs(float64(y-to.Y)) > 0.01 || z != to.Z {
		t.Fatalf("bridge port moved to (%f,%f,%f), want (%f,%f,%f)", x, y, z, to.X, to.Y, to.Z)
	}
	for _, dx := range []float32{-6, 0, 6} {
		for _, dz := range []float32{-4, 0, 4} {
			floor, _, _, ok := g.Stand(uint8(to.World), x+dx, y, z+dz)
			flags, env := e.Flags(uint8(to.World), x+dx, y+0.5, z+dz)
			if !ok || math.Abs(float64(floor-y)) > 0.01 || !env || flags&(ground.EnvLava|ground.EnvWater) != 0 {
				t.Fatalf("unsafe bridge at (%g,%g): floor %g, valid %t, environment %t, flags %d", x+dx, z+dz, floor, ok, env, flags)
			}
		}
	}
}
