package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The records' own words, each form the item database wrote a proc in, read
// as the proc they name; the controls are words that name none, a spell no
// record here is, and an epic's "This is a level 60 Paladin epic item." (the
// Sword of Virtue, whose EQOA-Data "proc 5" is not read).
func TestWeaponProcsReadFromTheirWords(t *testing.T) {
	for _, c := range []struct {
		desc string
		want WeaponProc
	}{
		{"Ice shard occasionally shoot off from this weapon. Procs a 100 point DD Spell.", WeaponProc{Kind: ProcHit, Damage: 100, Resist: StatCR}},
		{"A magical long sword enchanted with the power of living thorns. Procs 75-100", WeaponProc{Kind: ProcHit, Damage: 75, DamageTo: 100, Resist: StatAR}},
		{"A broad cleaver of a ship's make, heavy enough to split a keel. Procs 150 slashing damage.", WeaponProc{Kind: ProcHit, Damage: 150, Resist: StatSlash}},
		{"The blade of this longsword is covered in one large sheath of ice. Procs 150 Ice Damage.", WeaponProc{Kind: ProcHit, Damage: 150, Resist: StatCR}},
		{"An ancient and powerful sword. ... Proc a 25 point lifetap.", WeaponProc{Kind: ProcTap, Damage: 25, Resist: StatAR}},
		{"The creation of such a wand is a macabre ritual. Procs a 70 point Disease DoT.", WeaponProc{Kind: ProcDoT, Damage: 70, Resist: StatDR}},
		{"A legendary weapon. It damages goblins. 400 DD offensive proc, works only on goblins.", WeaponProc{Kind: ProcHit, Damage: 400, Resist: StatAR, Only: "goblin"}},
		{"...destroys the lifeblood found within. Procs for around 120. This is Shadowknight Epic Weapon.", WeaponProc{Kind: ProcHit, Damage: 120, Resist: StatAR}},
		{"...the cleaving force of this terrible axe. Procs 78-80. Reward for Lvl 49 Evil Warrior Quest.", WeaponProc{Kind: ProcHit, Damage: 78, DamageTo: 80, Resist: StatAR}},
		{"The blade of the sickle has been carved from the thigh bone of Gut Ripper the wolf. Procs: Life Tap", WeaponProc{Kind: ProcTap, Damage: 20, FX: fxProcLifeTap}},
	} {
		got, ok := WeaponProcFromText(c.desc)
		if !ok || got != c.want {
			t.Errorf("%q: %+v %v, want %+v", c.desc, got, ok, c.want)
		}
	}
	for _, desc := range []string{"", "A very simple sword used only for training.", "This is a level 60 Paladin epic item.",
		"This powerful magic staff instills dread into those it touches.\nProcs: Infectious Dread", "A flawless longbow. It reeks of poison.",
		"Procs (damage not known)"} {
		if got, ok := WeaponProcFromText(desc); ok {
			t.Errorf("the control %q reads %+v", desc, got)
		}
	}
}

// The epics and the described weapons with no figure go by their record's
// name, in any case, before their words; each gets its visual and AR when it
// says no element. The control: a name not listed with words of no proc.
func TestWeaponProcsByName(t *testing.T) {
	for name, want := range map[string]WeaponProc{
		"Selo's Jeweled Rapier": {Kind: ProcHit, Damage: 140, Resist: StatAR, FX: fxDamage},
		"SCIMITAR OF FURY":      {Kind: ProcHit, Damage: 100, Resist: StatAR, FX: fxDamage},
		"Blacktongue":           {Kind: ProcDoT, Damage: 50, Resist: StatPR, FX: fxDamage},
		"Sunsilver Blade":       {Kind: ProcCalm, Resist: StatAR, FX: fxBuff},
		"Bonesaw":               {Kind: ProcTap, Damage: 25, Resist: StatAR, FX: fxProcLifeTap},
	} {
		got, ok := WeaponProcOf(42, name, "A reward of the path quests.")
		want.Item, want.pattern = name, 42
		if !ok || got != want {
			t.Errorf("%s: %+v %v, want %+v", name, got, ok, want)
		}
	}
	if got, ok := WeaponProcOf(43, "Ballad of Mithril", "Upon this blade of mithril is written a healing ballad"); ok {
		t.Errorf("the control reads %+v", got)
	}
}

// With a weapon in each hand a round's odd hits are the main hand's and its
// even ones the off hand's; a two-hander, or one weapon, makes every hit. An
// off hand with no proc makes none on its hits.
func TestTheHandsTakeTurns(t *testing.T) {
	main, off := &WeaponProc{Item: "main"}, &WeaponProc{Item: "off"}
	name := func(wp *WeaponProc) string {
		if wp == nil {
			return "-"
		}
		return wp.Item
	}
	for _, c := range []struct {
		g    Gear
		want string
	}{
		{Gear{MainProc: main, OffProc: off, OffArmed: true}, "main off main off"},
		{Gear{MainProc: main, OffProc: off, OffArmed: true, WeaponTwoHanded: true}, "main main main main"},
		{Gear{MainProc: main}, "main main main main"},
		{Gear{MainProc: main, OffArmed: true}, "main - main -"},
		{Gear{OffProc: off, OffArmed: true}, "- off - off"},
	} {
		p := &Player{Gear: c.g}
		got := ""
		for hit := 1; hit <= 4; hit++ {
			if hit > 1 {
				got += " "
			}
			got += name(handProcLocked(p, hit))
		}
		if got != c.want {
			t.Errorf("%+v: %s, want %s", c.g, got, c.want)
		}
	}
}

// weaponRig is proc.go's rig, its warrior friend holding wp in the main hand.
func weaponRig(t *testing.T, name, desc string) *procRig {
	t.Helper()
	r := newProcRig(t)
	wp, ok := WeaponProcOf(9001, name, desc)
	if !ok {
		t.Fatalf("%s: no proc", name)
	}
	r.friend.Gear.MainProc = &wp
	r.fsess.Queue().GetPending()
	return r
}

func (r *procRig) hit() bool {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return r.w.playerHitLocked(r.fsess, r.friend, r.npc, r.npcF)
}

// A Selo's Jeweled Rapier's hit procs its 140 after the melee's figure; over
// the chance, or resisted, it does not, and the control, the same hit with
// no weapon proc, is the melee alone.
func TestAWeaponProcLands(t *testing.T) {
	r := weaponRig(t, "Selo's Jeweled Rapier", "")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = dice(0, 99) // under the chance, over the resist
	hp := r.npc.HP
	if r.hit() {
		t.Fatal("killed")
	}
	f := figures(r.fsess)
	if len(f) != 2 || f[1] != -140 || hp-r.npc.HP != -f[0]+140 {
		t.Errorf("figures %v, the creature lost %d", f, hp-r.npc.HP)
	}
	procIntn = dice(99, 99) // over the chance
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("over the chance: figures %v", f)
	}
	procIntn = dice(0, 0) // resisted: the least it does, never nothing
	r.hit()
	if f := figures(r.fsess); len(f) != 2 || f[1] != -140 {
		t.Errorf("resisted: figures %v, want the melee and 140", f)
	}
	r.friend.Gear.MainProc = nil
	procIntn = dice(0, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("the control, no weapon proc: figures %v", f)
	}
}

// A tap's damage comes back to its wielder as a heal.
func TestAWeaponTapHeals(t *testing.T) {
	r := weaponRig(t, "Flamberge of Horror", "Proc a 25 point lifetap.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	r.friend.HP = 500
	procIntn = dice(0, 99)
	before := r.npc.HP
	r.hit()
	f := figures(r.fsess)
	if len(f) != 3 || f[1] != -25 || f[2] != 25 || r.friend.HP != 525 || before-r.npc.HP != -f[0]+25 {
		t.Errorf("figures %v, the friend at %d, the creature lost %d", f, r.friend.HP, before-r.npc.HP)
	}
}

// A damage over time goes on the creature, ticks at once and every six
// seconds; a second proc begins it again (one effect, its end moved on).
func TestAWeaponDoTTicks(t *testing.T) {
	r := weaponRig(t, "Wand of Degradation", "Procs a 70 point Disease DoT.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = dice(0, 99)
	r.w.mu.Lock()
	now := time.Now()
	hp := r.npc.HP
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, now)
	if len(r.npc.Effects) != 1 || r.npc.Effects[0].HPTick != -70 || hp-r.npc.HP != 70 ||
		!r.npc.Effects[0].Until.Equal(now.Add(procDoTFor)) {
		t.Fatalf("the creature lost %d, effects %+v", hp-r.npc.HP, r.npc.Effects)
	}
	r.w.effectSystem(now.Add(effectTick))
	if hp-r.npc.HP != 140 {
		t.Errorf("after a tick it lost %d, want 140", hp-r.npc.HP)
	}
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, now.Add(10*time.Second))
	if len(r.npc.Effects) != 1 || !r.npc.Effects[0].Until.Equal(now.Add(10*time.Second+procDoTFor)) {
		t.Errorf("a second proc: %+v", r.npc.Effects)
	}
	r.w.mu.Unlock()
}

// Sunsilver halves the creature's hate for its wielder; the control, a roll
// over the chance, leaves it.
func TestSunsilverCalms(t *testing.T) {
	r := weaponRig(t, "Sunsilver Blade", "A reward of the path quests.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.npc.Threats = map[EntityID]int64{r.friend.ServerID: 1000, r.p.ServerID: 700}
	procIntn = dice(99)
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
	if r.npc.Threats[r.friend.ServerID] != 1000 {
		t.Errorf("the control, over the chance: its hate %d", r.npc.Threats[r.friend.ServerID])
	}
	procIntn = dice(0, 99)
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
	if r.npc.Threats[r.friend.ServerID] != 500 || r.npc.CurrentEnemy != r.p.ServerID {
		t.Errorf("its hate %d, its enemy %d", r.npc.Threats[r.friend.ServerID], r.npc.CurrentEnemy)
	}
}

// Gorereaver's 400 fires on a goblin alone; the control is the same roll on
// a creature that is not one.
func TestGorereaverHitsGoblins(t *testing.T) {
	r := weaponRig(t, "Gorereaver", "It damages goblins. 400 DD offensive proc, works only on goblins.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = dice(0, 99)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	hp := r.npc.HP
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
	if r.npc.HP != hp {
		t.Errorf("the control, %s: it lost %d", r.npc.Name, hp-r.npc.HP)
	}
	r.npc.Name = "Goblin Raider"
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
	if hp-r.npc.HP != 400 {
		t.Errorf("a goblin lost %d, want 400", hp-r.npc.HP)
	}
}

// katanaWords are the Katana of Severing's words with its proc (petition 1360,
// Zaeire: "proc should be set to 2k with a 10% chance to proc").
const katanaWords = "This katana severs heads with great efficiency. Procs 2000 slashing damage, a 10% chance each hit."

// A proc's own chance in its words: "N% chance", "N% a hit", "per swing", from
// 1 to 100. The controls: no chance in the words (DEX's rule, Chance 0), a
// scale's percent, 0% and past 100%.
func TestAWeaponProcsOwnChance(t *testing.T) {
	want := WeaponProc{Kind: ProcHit, Damage: 2000, Resist: StatSlash, Chance: 10}
	if got, ok := WeaponProcFromText(katanaWords); !ok || got != want {
		t.Errorf("the Katana: %#v %v, want %#v", got, ok, want)
	}
	for _, c := range []struct {
		desc   string
		chance int
	}{
		{"Procs 150 Ice Damage, 5% a hit.", 5},
		{"Procs a 100 point DD Spell at 12% per swing.", 12},
		{"Procs 150 Ice Damage. 100% chance.", 100},
		{"Procs 150 Ice Damage.", 0},
		{"Procs 154 slashing damage ; Increase 30% based on DEX.", 0},
		{"Procs 150 Ice Damage, a 0% chance.", 0},
		{"Procs 150 Ice Damage, a 150% chance.", 0},
	} {
		if got, ok := WeaponProcFromText(c.desc); !ok || got.Chance != c.chance {
			t.Errorf("%q: %#v %v, want chance %d", c.desc, got, ok, c.chance)
		}
	}
	if got, _ := WeaponProcOf(713079, "Katana of Severing", katanaWords); got.String() != "2000 at 10% a hit" {
		t.Errorf("its words: %q", got.String())
	}
}

// The Katana's 10% holds whatever its wielder's DEX: a roll of 9 procs its
// 2,000 and a roll of 10 does not, on a wielder whose DEX gives the rule more
// than 10; #stats says its chance and no DEX's. The control: the same words
// with no chance procs on that roll of 10, by the rule.
func TestAWeaponProcsOwnChanceHoldsAtAnyDEX(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	r := weaponRig(t, "Katana of Severing", katanaWords)
	r.friend.Dex = 400
	if c := weaponProcChancePct(r.friend.StatNow(StatDex)); c <= 10 {
		t.Fatalf("the wielder's DEX gives the rule %d%%, not past 10", c)
	}
	r.npc.HP, r.npc.MaxHP = 50000, 50000
	procIntn = dice(9, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 2 || f[1] != -2000 {
		t.Errorf("a roll of 9: figures %v, want the hit and -2000", f)
	}
	procIntn = dice(10, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("a roll of 10: figures %v, want the hit alone", f)
	}
	if w := weaponProcWords(r.friend); w != "Weapon procs: Katana of Severing 2000 at 10% a hit" {
		t.Errorf("#stats: %q", w)
	}

	c := weaponRig(t, "Katana of Severing", "This katana severs heads with great efficiency. Procs 2000 slashing damage.")
	c.friend.Dex = 400
	c.npc.HP, c.npc.MaxHP = 50000, 50000
	procIntn = dice(10, 99)
	c.hit()
	if f := figures(c.fsess); len(f) != 2 || f[1] != -2000 {
		t.Errorf("the control, no chance in its words, a roll of 10: figures %v", f)
	}
	if w := weaponProcWords(c.friend); !strings.Contains(w, "a hit of its hand (DEX ") {
		t.Errorf("the control's #stats: %q", w)
	}
}

// Petition 1360 (Zaeire: "weapon proc chance scales off dex while ability proc
// chance scales off int"): a weapon of no chance of its own procs by its
// wielder's DEX, whatever its INT: a roll one under the DEX rule's chance
// procs and a roll at it does not. The control: a wielder of low DEX and high
// INT does not proc on a roll the INT rule would have taken.
func TestAWeaponProcGoesByDEXNotINT(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	if got := weaponProcChancePct(400); got != 23 {
		t.Fatalf("DEX 400: %d%%, want 23", got)
	}
	if got := weaponProcChancePct(655); got != procMostPct {
		t.Fatalf("DEX 655: %d%%, want the most, %d", got, procMostPct)
	}
	words := "This katana severs heads with great efficiency. Procs 2000 slashing damage."
	r := weaponRig(t, "Katana of Severing", words)
	r.friend.Dex, r.friend.Int = 400, 0
	pct := weaponProcChancePct(r.friend.StatNow(StatDex))
	if pct <= procChancePct(r.friend.StatNow(StatInt)) {
		t.Fatalf("the rig: DEX's rule %d%% is not past INT's %d%%", pct, procChancePct(r.friend.StatNow(StatInt)))
	}
	r.npc.HP, r.npc.MaxHP = 50000, 50000
	procIntn = dice(pct-1, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 2 || f[1] != -2000 {
		t.Errorf("a roll of %d under DEX's %d%%: figures %v, want the hit and -2000", pct-1, pct, f)
	}
	procIntn = dice(pct, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("a roll of %d at DEX's %d%%: figures %v, want the hit alone", pct, pct, f)
	}
	if w := weaponProcWords(r.friend); !strings.Contains(w, fmt.Sprintf("%d%% a hit of its hand (DEX %d)", pct, r.friend.StatNow(StatDex))) {
		t.Errorf("#stats: %q", w)
	}

	c := weaponRig(t, "Katana of Severing", words)
	c.friend.Dex, c.friend.Int = 0, 400
	dexPct, intPct := weaponProcChancePct(c.friend.StatNow(StatDex)), procChancePct(c.friend.StatNow(StatInt))
	if intPct <= dexPct {
		t.Fatalf("the control: INT's rule %d%% is not past DEX's %d%%", intPct, dexPct)
	}
	c.npc.HP, c.npc.MaxHP = 50000, 50000
	procIntn = dice(dexPct, 99) // under INT's chance, at DEX's
	c.hit()
	if f := figures(c.fsess); len(f) != 1 {
		t.Errorf("the control, a roll of %d (INT's %d%%, DEX's %d%%): figures %v, want the hit alone", dexPct, intPct, dexPct, f)
	}
}
