package entity

import "testing"

// The weaponsmiths' made weapons carry their proc in their words, as
// tools/seed_tradeskills.py --craft weapon writes them (a fiery, freezing,
// cursed or withering weapon's hit, a malign, venomous, volatile or ethereal
// one's damage over time, a vampiric one's life tap and a siphoning one's mana
// tap, each with its own chance): each reads as the proc it names. The
// control: a mundane weapon's words name none.
func TestCraftedWeaponProcsRead(t *testing.T) {
	for _, c := range []struct {
		desc string
		want WeaponProc
	}{
		{"A weapon forged of steel. Procs 20-40 fire damage, a 15% chance each hit.",
			WeaponProc{Kind: ProcHit, Damage: 20, DamageTo: 40, Resist: StatFR, Chance: 15}},
		{"A weapon forged of bronze. Procs 10-20 cold damage, a 10% chance each hit.",
			WeaponProc{Kind: ProcHit, Damage: 10, DamageTo: 20, Resist: StatCR, Chance: 10}},
		{"A weapon forged of mithril. Procs 30-60 arcane damage, a 20% chance each hit.",
			WeaponProc{Kind: ProcHit, Damage: 30, DamageTo: 60, Resist: StatAR, Chance: 20}},
		{"A weapon forged of brass. Procs a 12-24 point disease DoT, a 10% chance each hit.",
			WeaponProc{Kind: ProcDoT, Damage: 12, DamageTo: 24, Resist: StatDR, Chance: 10}},
		{"A weapon forged of seralite. Procs a 25-50 point poison DoT, a 25% chance each hit.",
			WeaponProc{Kind: ProcDoT, Damage: 25, DamageTo: 50, Resist: StatPR, Chance: 25}},
		{"A weapon forged of brellium. Procs a 10 point lifetap, a 20% chance each hit.",
			WeaponProc{Kind: ProcTap, Damage: 10, Resist: StatAR, Chance: 20}},
		{"A weapon forged of seralite. Procs a 10 point mana tap, a 25% chance each hit.",
			WeaponProc{Kind: ProcTap, Damage: 10, Resist: StatAR, Chance: 25, PowerBack: true}},
	} {
		got, ok := WeaponProcFromText(c.desc)
		if !ok || got != c.want {
			t.Errorf("%q: %+v %v, want %+v", c.desc, got, ok, c.want)
		}
	}
	for _, desc := range []string{"A weapon forged of crude iron.", "A spellsmith's parchment, its power drained."} {
		if got, ok := WeaponProcFromText(desc); ok {
			t.Errorf("the control %q reads %+v", desc, got)
		}
	}
}

// A mana tap's damage comes back to its wielder as power, its health as it
// was; the control, a life tap, heals.
func TestAWeaponManaTapGivesPower(t *testing.T) {
	r := weaponRig(t, "siphoning seralite katana", "A weapon forged of seralite. Procs a 10 point mana tap, a 25% chance each hit.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	if !r.friend.Gear.MainProc.PowerBack {
		t.Fatalf("the proc %+v is no mana tap", r.friend.Gear.MainProc)
	}
	r.friend.MaxPower, r.friend.Power, r.friend.HP = 1000, 100, 500
	procIntn = dice(0, 99)
	before := r.npc.HP
	r.hit()
	f := figures(r.fsess)
	if len(f) != 2 || f[1] != -10 || r.friend.Power != 110 || r.friend.HP != 500 || before-r.npc.HP != -f[0]+10 {
		t.Errorf("figures %v, power %d, health %d, the creature lost %d", f, r.friend.Power, r.friend.HP, before-r.npc.HP)
	}
	if s := r.friend.Gear.MainProc.String(); s != "10 mana tap at 25% a hit" {
		t.Errorf("#stats says %q", s)
	}
}
