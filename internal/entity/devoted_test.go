package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The owner, 2026-11-13: "Guided meditation doesn't seem to be working". The
// Jal'Raeth's group buff (tools/jalraeth_masteries_20261111.py: PoT and HoT 5
// and a fifth of the caster's level, radius 30) reaches the caster's
// companions: the merc of its group (area.go's groupNearLocked; until then a
// group spell with a radius took its others from the group's players and pets
// alone, so a merc never took one) and a workbook pet, whose tick now takes
// its effects' HoT and PoT (petstats.go's petSheetRegen).
const guidedMeditationJSON = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":760172,"level":6,"min_level":1,"radius":30,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"F077","type":"Statistic Change","kind":"stat","stat":"pot","amount":5,"level_pct":20},{"id":"F078","type":"Statistic Change","kind":"stat","stat":"hot","amount":5,"level_pct":20}]}`

func guidedMeditation(t *testing.T) CastSpell {
	t.Helper()
	sp := SpellOfRow(&database.SpellPatternRecord{SpellID: 760172, AbilityLvl: 6, Range: 16880, CastTime: 2, Power: 30,
		Scope: 2, Recast: 10, SpellName: "Guided Meditation"}, 12, "", guidedMeditationJSON, 0)
	if sp.Area.Radius != 30 {
		t.Fatalf("Guided Meditation's area %+v: want the workbook's group radius 30", sp.Area)
	}
	return sp
}

func TestGuidedMeditationReachesTheGroupsMerc(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	r.p.Power = r.p.MaxPower
	if !r.w.StartCast(r.sess, guidedMeditation(t), r.sess.PlayerObjectID(), r.at(time.Second)) {
		t.Fatal("Guided Meditation not begun")
	}
	r.run(time.Second, 6*time.Second)
	want := 5 + r.p.Level*20/100 // 11 at the rig's level 30
	if r.p.Mods.HoT != want || r.p.Mods.PoT != want {
		t.Errorf("the caster: HoT %d, PoT %d; want %d each", r.p.Mods.HoT, r.p.Mods.PoT, want)
	}
	if len(ms.npc.Effects) != 1 || ms.npc.Effects[0].SpellID != 760172 {
		t.Fatalf("the merc's effects %d: want Guided Meditation on it", len(ms.npc.Effects))
	}
	if ms.body.Mods.HoT != want || ms.body.Mods.PoT != want {
		t.Errorf("the merc's body: HoT %d, PoT %d; want %d each", ms.body.Mods.HoT, ms.body.Mods.PoT, want)
	}
	// and its tick gives it: hurt and drained, a tick's worth past its own
	b := ms.body
	ms.npc.HP, ms.npc.Power = 100, 100
	base := max(b.HoTNow()-want, 1)
	ms.npc.mercRegen()
	if got := ms.npc.HP - 100; got != base+want {
		t.Errorf("the merc's tick healed %d; want %d (its own %d and the buff's %d)", got, base+want, base, want)
	}
}

func TestAWorkbookPetsTickTakesItsEffectsHoTAndPoT(t *testing.T) {
	pet := &NPC{ServerID: 9, Level: 50, HP: 100, MaxHP: 5000, Power: 100, MaxPower: 2000, Owner: 1,
		sheet: &petSheet{PetStats: database.PetStats{HP: 5000, HoT: 40, PoT: 20}}}
	pet.petRegen()
	if pet.HP != 140 || pet.Power != 120 {
		t.Fatalf("no effect: HP %d, power %d; want 140 and 120 (its row's HoT 40, PoT 20)", pet.HP, pet.Power)
	}
	ae := &ActiveEffect{SpellID: 760172, Until: time.Now().Add(time.Hour)}
	ae.Mods.HoT, ae.Mods.PoT = 17, 17
	pet.Effects = []*ActiveEffect{ae}
	pet.petRegen()
	if pet.HP != 140+57 || pet.Power != 120+37 {
		t.Errorf("Guided Meditation on it: HP %d, power %d; want %d and %d (its row's and 17 each)", pet.HP, pet.Power, 140+57, 120+37)
	}
}

// The two ranks as tools/devoted_fix_20261113.py writes them (its test holds
// these strings to the tool's): Sacred Scripture one rank of WIS +10 and WIS
// cap +20, and Path of the Devout its buff's six parts as the rank's own
// stats, adding no ability.
const (
	sacredScriptureEffects = "WIS:10,WISMax:20"
	pathOfTheDevoutEffects = "HP:500,AC:250,Off Mod:2,Def Mod:2,STA:25,WIS:25"
)

func TestDevotedRanksAsTheToolWritesThem(t *testing.T) {
	s := ParseCMRank(database.CMRankRow{ID: 2164, Category: 1, Name: "Sacred Scripture", Cost: 25, Level: 6,
		Effects: sacredScriptureEffects, Enabled: true})
	var want [CMSlots]int
	want[4], want[cmSlotCap+4] = 10, 20
	if s.Effects != want {
		t.Errorf("Sacred Scripture's slots %v; want WIS 10 and WIS cap 20", s.Effects)
	}
	p := ParseCMRank(database.CMRankRow{ID: 2167, Category: 4, Name: "Path of the Devout", Parent: 2166, Cost: 75,
		Level: 60, Effects: pathOfTheDevoutEffects, Enabled: true})
	want = [CMSlots]int{}
	want[cmSlotHP], want[cmSlotAC], want[cmSlotOff], want[cmSlotDef], want[1], want[4] = 500, 250, 2, 2, 25, 25
	if p.Effects != want || len(p.Adds) != 0 {
		t.Errorf("Path of the Devout's slots %v, adds %v; want max HP 500, AC 250, OFF and DEF MOD 2, STA and WIS 25, no ability", p.Effects, p.Adds)
	}
}
