package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The rest of the workbook's kinds (workbookmore.go), on made-up spells.

// A proc that casts a stat taken and a defensive one that taunts: the first
// fires on the bearer's melee hit at the proc's chance and leaves its debuff
// on the creature (the control, a roll against the chance, leaves none); the
// second at every hit on the bearer (the legend's 100%) and angers it.
func TestWorkbookSpellProcs(t *testing.T) {
	buff := wbSpell(1101, 10, 0, "Test Brand", `{"v":1,"scope":"self","duration_s":600,"effects":[
		{"kind":"proc","procs":[{"kind":"stat","stat":"ac","amount":-10,"on":"offense","duration_s":60,"resist":"ar"}]},
		{"kind":"def_proc","procs":[{"kind":"hate","amount":50,"on":"defense"}]}]}`)
	if len(buff.Def.ProcSpells) != 2 || buff.Def.ProcSpells[1].Chance != 100 || buff.Def.ProcSpells[0].Duration != time.Minute {
		t.Fatalf("its procs: %+v", buff.Def.ProcSpells)
	}
	for _, fires := range []bool{false, true} {
		c := newCaster(t)
		c.npc.HP, c.npc.MaxHP = 1000, 1000
		c.castOn(t, buff, 0)
		old := procIntn
		procIntn = func(int) int { return 99 } // no proc
		if fires {
			procIntn = func(int) int { return 0 } // every proc; the level 1 creature resists nothing
		}
		c.w.mu.Lock()
		now := c.at(10 * time.Second)
		c.w.playerProcsLocked(c.sess, c.p, c.npc, now)
		c.w.procSpellsLocked(c.p, c.sess, c.npc, true, now)
		c.w.mu.Unlock()
		procIntn = old
		debuffed := false
		for _, e := range c.npc.Effects {
			debuffed = debuffed || e.Mods.AC == -10
		}
		if debuffed != fires || c.npc.Threats[c.p.ServerID] < 50 {
			t.Errorf("fires %v: debuffed %v, threat %d", fires, debuffed, c.npc.Threats[c.p.ServerID])
		}
	}
}

// Melee resists: a creature's slash debuff of 35 makes a slashing weapon's
// hit 35% more and nothing else's; its spells of that basis are resisted the
// less; the workbook's "ranged" is pierce.
func TestWorkbookMeleeResists(t *testing.T) {
	now := time.Now()
	npc := &NPC{Level: 30, Effects: []*ActiveEffect{{Until: now.Add(time.Minute), Mods: StatMods{Slash: -35}}}}
	for _, tc := range []struct {
		hand byte
		want int
	}{{1, 135}, {2, 135}, {3, 100}, {5, 100}, {0, 100}} {
		if got := meleeDebuffed(&Player{PrimaryType: tc.hand}, npc, 100, now); got != tc.want {
			t.Errorf("hand %d: %d, want %d", tc.hand, got, tc.want)
		}
	}
	plain := &NPC{Level: 30}
	if a, b := npcSpellResistPct(20, plain, StatSlash, now), npcSpellResistPct(20, npc, StatSlash, now); b >= a {
		t.Errorf("a slash spell's resist under the debuff: %d, before %d", b, a)
	}
	d := SpellDefFrom("Test Expose", "", "", `{"v":1,"scope":"target","harmful":true,"duration_s":30,"effects":[{"kind":"stat","stat":"ranged","amount":-20}]}`)
	var m StatMods
	m.add(d.Effects[0].Stat, d.Effects[0].Amount)
	if len(d.Unsupported) != 0 || m.Pierce != -20 {
		t.Errorf("a ranged debuff: %+v, mods %+v", d, m)
	}
}

// OFF mods count the foe lower and DEF mods the creature's melee lower;
// none (the control) as before. HP factors add a level's worth each.
func TestWorkbookOffDefModsAndHPFactor(t *testing.T) {
	npc := &NPC{Level: 40}
	if a, b := targetDefenceAt(npc, 40), targetDefenceAt(npc, 30); b >= a || a != targetDefence(npc) {
		t.Errorf("defence at 40 %v, counted 30 %v, its own %v", a, b, targetDefence(npc))
	}
	p := &Player{Class: 0, Level: 40, Gear: Gear{WeaponDamage: 30, WeaponCond: 1}}
	_, _, high := playerMeleeRoll(p, npc)
	p.Mods.OffMod = 10
	if _, _, h := playerMeleeRoll(p, npc); h <= high {
		t.Errorf("an OFF mod of 10: high %d, before %d", h, high)
	}
	if a, b := npcMeleeRollFor(npc, &Player{}).Top, npcMeleeRollFor(npc, &Player{Mods: StatMods{DefMod: 5}}).Top; b >= a {
		t.Errorf("a DEF mod of 5: the creature's top %v, before %v", b, a)
	}
	c := newCaster(t)
	before := c.p.MaxHPNow()
	c.p.Mods.HPFactor = 2
	if got := c.p.MaxHPNow(); got != before+2*c.p.Level {
		t.Errorf("an HP factor of 2 at %d: %d, before %d", c.p.Level, got, before)
	}
	d := SpellDefFrom("Test Passive", "", "", `{"v":1,"scope":"self","duration_s":60,"effects":[{"kind":"hp_factor","amount":10},
		{"kind":"off_mod","amount":1},{"kind":"def_mod","amount":2}]}`)
	if len(d.Effects) != 3 || d.Effects[0].Stat != StatHPFactor || d.Effects[1].Stat != StatOffMod || d.Effects[2].Stat != StatDefMod {
		t.Errorf("the mods read: %+v", d.Effects)
	}
}

// A pickpocket takes tunar and angers nothing; again at once there is
// nothing left; caught, the creature fights.
func TestWorkbookPickpocket(t *testing.T) {
	pick := wbSpell(1111, 8, 1, "Test Pick", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"effects":[{"kind":"pickpocket"}]}`)
	c := newCaster(t)
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 20, 1000, 1000
	old := pickIntn
	defer func() { pickIntn = old }()
	pickIntn = func(int) int { return 0 }
	cast := func(at time.Duration) {
		t.Helper()
		if !c.w.StartCast(c.sess, pick, c.npcO, c.at(at)) {
			t.Fatal("not begun")
		}
		c.tick(at + pick.CastTime + castLand)
	}
	cast(0)
	if c.p.Tunar != 20 || c.npc.CurrentEnemy != 0 {
		t.Fatalf("after a pick: %d tunar, enemy %d", c.p.Tunar, c.npc.CurrentEnemy)
	}
	cast(10 * time.Second)
	if c.p.Tunar != 20 {
		t.Errorf("picked again at once: %d tunar", c.p.Tunar)
	}
	c.npc.pickedAt = time.Time{}
	pickIntn = func(int) int { return 99 }
	cast(20 * time.Second)
	if c.p.Tunar != 20 || c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("caught: %d tunar, enemy %d", c.p.Tunar, c.npc.CurrentEnemy)
	}
}

// A disguise keeps a faction's NPC from attacking on sight; the control, the
// same player without it, is attacked.
func TestWorkbookDisguise(t *testing.T) {
	w := NewWorld()
	w.SetNPCFactions([]database.NPCFactionRow{{Name: "Test Warden", World: -1, Faction: "The Freeport Militia"}})
	npc := &NPC{ServerID: 3, Name: "Test Warden", Level: 30, HP: 100, MaxHP: 100}
	p := &Player{ServerID: 4, Level: 30, Factions: map[string]int{"The Freeport Militia": -1000}}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.threatensLocked(npc, p) {
		t.Fatal("the control: not attacked")
	}
	p.Effects = []*ActiveEffect{{Until: time.Now().Add(time.Minute), Rules: EffectRules{Disguise: true}}}
	if w.threatensLocked(npc, p) {
		t.Error("disguised, attacked")
	}
	d := SpellDefFrom("Illusion: Test", "", "", `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"illusion","sprite":"0x12345678"},{"kind":"faction"}]}`)
	if len(d.Unsupported) != 0 || len(d.Effects) != 2 || d.Effects[1].Kind != EffectDisguise {
		t.Errorf("the illusion's faction change: %+v", d)
	}
}

// A summoned player goes beside its caster, a gate's travel; the caster
// itself does not.
func TestWorkbookSummonPlayer(t *testing.T) {
	c := newCaster(t)
	other := &Player{ServerID: 1003, Name: "Far", Level: 30, HP: 100, MaxHP: 100, World: 2, X: 5000}
	c.w.Players[other.ServerID] = other
	sess := c.w.SessionManager.CreateSession(nil, 2, 2, 2, 3, true)
	sess.SetECSEntity(other)
	sess.SetCharacterInWorld(true)
	call := wbSpell(1121, 51, 2, "Test Call", `{"v":1,"scope":"group","target_type":"Friendly","effects":[{"kind":"summon_player"}]}`)
	c.w.mu.Lock()
	c.w.summonPlayerLocked(&castState{spell: call}, c.p, other)
	c.w.summonPlayerLocked(&castState{spell: call}, c.p, c.p)
	due := c.w.gates.due
	c.w.mu.Unlock()
	if len(due) != 1 || due[0].P != other || due[0].To.World != c.p.World || due[0].To.X != c.p.X || due[0].To.Z != c.p.Z {
		t.Errorf("the travels: %+v", due)
	}
}

// Required equipment: a cast is refused unless a hand holds a weapon it
// names; any, a shield, bare hands, a bow.
func TestWorkbookRequiredEquipment(t *testing.T) {
	for _, tc := range []struct {
		req             string
		primary, second byte
		shield          uint32
		fits            bool
	}{
		{"1HS,2HS", 1, 0, 0, true}, {"1HS,2HS", 2, 0, 0, true}, {"1HS,2HS", 3, 0, 0, false},
		{"1HB,1HS,1HP, 2HB,2HS,2HP", 6, 0, 0, true}, {"1HP", 0, 0, 0, false},
		{"Shield", 1, 0, 9, true}, {"Shield", 1, 0, 0, false},
		{"None", 0, 0, 0, true}, {"None", 11, 0, 0, true}, {"None", 1, 0, 0, false},
		{"Range", 1, 7, 0, true}, {"Range", 1, 0, 0, false}, {"Any", 0, 0, 0, true},
	} {
		p := &Player{PrimaryType: tc.primary, SecondaryType: tc.second, Shield: tc.shield}
		if got := equipFits(p, tc.req); got != tc.fits {
			t.Errorf("%q with %d/%d shield %d: fits %v, want %v", tc.req, tc.primary, tc.second, tc.shield, got, tc.fits)
		}
	}
	art := wbSpell(1131, 5, 1, "Test Slash", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"req_equipment":"1HS,2HS",
		"effects":[{"kind":"hp","amount":-10}]}`)
	c := newCaster(t)
	c.p.PrimaryType = 3
	if c.w.StartCast(c.sess, art, c.npcO, c.at(0)) {
		t.Fatal("begun with a mace")
	}
	var told bool
	for _, m := range c.sent() {
		told = told || strings.Contains(string(m), "w\x00e\x00a\x00p\x00o\x00n")
	}
	if !told {
		t.Error("the refusal not told")
	}
	c.p.PrimaryType = 1
	if !c.w.StartCast(c.sess, art, c.npcO, c.at(time.Second)) {
		t.Error("refused with a sword")
	}
}

// A spell for animals alone is refused at anything else, one for summoned
// creatures at anything not summoned.
func TestWorkbookTargetKinds(t *testing.T) {
	w := NewWorld()
	rat := &NPC{ServerID: 1, Name: "giant rat", Level: 3}
	orc := &NPC{ServerID: 2, Name: "orc pawn", Level: 3}
	elem := &NPC{ServerID: 3, Name: "lesser fire elemental", Level: 3}
	for _, n := range []*NPC{rat, orc, elem} {
		w.NPCs[n.ServerID] = n
	}
	animal := SpellDefFrom("Test Charm Beast", "", "", `{"v":1,"scope":"target","target_type":"Enemy - Animal","harmful":true,"effects":[{"kind":"charm"}]}`)
	summoned := SpellDefFrom("Test Dismiss", "", "", `{"v":1,"scope":"target","target_type":"Enemy - Summoned","harmful":true,"effects":[{"kind":"hp","amount":-50}]}`)
	if animal.TargetKind != "animal" || summoned.TargetKind != "summoned" {
		t.Fatalf("the kinds: %q %q", animal.TargetKind, summoned.TargetKind)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, tc := range []struct {
		d       *SpellDef
		npc     *NPC
		refused bool
	}{{&animal, rat, false}, {&animal, orc, true}, {&summoned, elem, false}, {&summoned, rat, true}} {
		why, _ := w.targetKindStartLocked(tc.d, []EntityID{tc.npc.ServerID})
		if (why != "") != tc.refused {
			t.Errorf("%s at %s: %q", tc.d.TargetKind, tc.npc.Name, why)
		}
	}
}
