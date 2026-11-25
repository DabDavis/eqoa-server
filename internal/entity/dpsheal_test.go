package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// The damage meter's healing and power (dpsheal.go): what a player's heals
// restored, not what they were worth, and the power it gave another, counted
// on its group's fight under way; sent in layout 2 to a client that asked for
// it, and layout 1 as it was to one that did not.

// healOf is maker's healing, its pet's share of it and its power given in the
// fight p's meter is shown; ok false when it has no line.
func healOf(w *World, p, maker *Player) (heal, healPet, power int64, ok bool) {
	f := fightOf(w, p)
	if f == nil {
		return 0, 0, 0, false
	}
	l := f.lines[maker.ServerID]
	if l == nil {
		return 0, 0, 0, false
	}
	return l.heal, l.healPet, l.power, true
}

// Each heal path counts what it restored on both members' meters: a spell's
// heal on a friend short of its most (the figure told whole, the meter its
// part), on a full friend (told, counted nothing), on oneself, a heal over
// time's tick, a heal on its own pet, its pet's heal on itself (its pet's
// share), a proc's heal on a full player, a spell's HoT stat at the regen tick
// (as far as the tick restored); a meal's heal over time counts nothing. The
// control: with no fight under way a heal counts nothing and starts none.
func TestDPSMeterCountsWhatHealsRestore(t *testing.T) {
	heal := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: n, AmountMax: n} }
	r := newTagRig(t)
	meterClock(t, r.t0)
	group(r.w, 77, r.p, r.friend)
	r.p.dps, r.friend.dps = &dpsMeter{}, &dpsMeter{}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()

	// the control first: no fight, no count, no fight started
	r.friend.HP = r.friend.MaxHPNow() - 60
	r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.friend, r.t0)
	if fightOf(r.w, r.p) != nil || r.friend.HP != r.friend.MaxHPNow() {
		t.Fatalf("a heal out of a fight: fight %v, friend %d/%d", fightOf(r.w, r.p), r.friend.HP, r.friend.MaxHPNow())
	}
	r.sent()

	r.w.dpsCountLocked(r.p, 10, false, r.t0) // a hit: the fight under way
	want := int64(0)
	wantPet := int64(0)
	check := func(what string) {
		t.Helper()
		for _, v := range []*Player{r.p, r.friend} {
			h, hp, _, ok := healOf(r.w, v, r.p)
			if !ok || h != want || hp != wantPet {
				t.Fatalf("%s: %s's meter has %d healing (pet %d), want %d (pet %d)", what, v.Name, h, hp, want, wantPet)
			}
		}
	}

	r.friend.HP = r.friend.MaxHPNow() - 60
	r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.friend, r.t0)
	want += 60
	check("a heal of 100 on a friend 60 short")
	if fs := figTags(t, r.sess); len(fs) != 1 || fs[0].amount != 100 {
		t.Fatalf("told %v, want the heal's whole 100", fs)
	}

	r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.friend, r.t0)
	check("a heal on a full friend")
	if fs := figTags(t, r.sess); len(fs) != 1 || fs[0].amount != 100 {
		t.Fatalf("a full friend: told %v, want the 100 still", fs)
	}

	r.p.HP = r.p.MaxHPNow() - 30
	r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.p, r.t0)
	want += 30
	check("a heal on oneself 30 short")

	r.friend.HP = r.friend.MaxHPNow() - 10
	hot := &ActiveEffect{SpellID: 9300, Name: "Test Renewal", Caster: r.p.ServerID, Until: r.later, HPTick: 25, figures: figuresSpell}
	r.w.runTickLocked(r.friend, hot, r.t0)
	want += 10
	check("a heal over time's tick of 25, 10 short")

	r.pet.HP = r.pet.MaxHP - 300
	r.w.landOnPetLocked(r.cast(heal(100)), r.p, r.sess, r.pet, r.t0)
	want += 100
	check("a heal on its pet")

	r.w.tellPetHealLocked(r.ps, 40) // its pet's heal on itself, told as it restored it (pettemplate.go)
	want, wantPet = want+40, 40
	check("its pet's heal on itself")

	r.friend.HP = r.friend.MaxHPNow()
	r.w.procHealLocked(r.p, r.sess, r.friend, 70)
	check("a proc's heal on a full friend")
	r.sent()

	// a spell's HoT stat of the enchanter's on the friend, 20 short: the tick
	// restores 20 (its base and the stat), the enchanter's part counted as far
	r.friend.HP = r.friend.MaxHPNow() - 20
	r.setEffects(r.friend, &ActiveEffect{SpellID: 9301, Name: "Test Succor", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 50}})
	r.w.regenSystem(r.t0)
	want += 20
	check("a HoT stat's part at the regen tick, 20 short")
	if fs := figTags(t, r.sess); len(fs) == 0 || fs[0].amount != 50 {
		t.Fatalf("the HoT stat: told the enchanter %v, want its part, 50", fs)
	}

	// a meal's heal over time on oneself: told, no healer's
	r.setEffects(r.friend)
	r.p.HP = r.p.MaxHPNow() - 200
	r.setEffects(r.p, &ActiveEffect{SpellID: FoodEffectID, Name: "Ration", Until: r.later, Mods: StatMods{HoT: 30}})
	r.w.regenSystem(r.t0)
	check("a meal's heal over time")
	if fs := figTags(t, r.sess); len(fs) == 0 {
		t.Fatal("the meal's figure was not told")
	}
	if d, _, _ := line(r.w, r.p, r.p); d != 10 {
		t.Fatalf("the damage %d, want the 10 alone: healing is no damage", d)
	}
}

// An aggressive merc's heal on a group mate other than its owner is told to
// no one of its side as the merc's, and counts for its owner, its share (the
// helper's);
// its heal on its owner the same, once.
func TestDPSMeterCountsAMercsHeals(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAggressive) // an aggressive merc heals its group mates (merccall.go)
	_, b := r.player("B", 9, 0)
	r.group(b)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps, b.dps = &dpsMeter{}, &dpsMeter{}
	r.w.dpsCountLocked(r.p, 5, false, now)
	b.HP = b.MaxHPNow() - 40
	r.w.mercHelpLocked(ms, ptr(mercHealSpell()), b.ServerID, now)
	if b.HP != b.MaxHPNow() {
		t.Fatalf("the merc's heal did not land on B: %d/%d", b.HP, b.MaxHPNow())
	}
	for _, v := range []*Player{r.p, b} {
		if h, hp, _, ok := healOf(r.w, v, r.p); !ok || h != 40 || hp != 40 {
			t.Fatalf("%s's meter: the owner's healing %d (its merc's %d), want 40 (40)", v.Name, h, hp)
		}
		if h, _, _, _ := healOf(r.w, v, b); h != 0 {
			t.Fatalf("%s's meter: B healed %d, want none of the merc's", v.Name, h)
		}
	}
	r.p.HP = r.p.MaxHPNow() - 25
	r.w.mercHelpLocked(ms, ptr(mercHealSpell()), r.p.ServerID, now)
	if h, hp, _, _ := healOf(r.w, r.p, r.p); h != 65 || hp != 65 {
		t.Fatalf("its heal on its owner: %d (%d), want 65 (65): counted once", h, hp)
	}
}

// Power given another counts at what it filled, on both meters: a spell's
// power on a friend short of its most, a power over time's tick, a spell's PoT
// stat at the regen tick, a drain's power for the group; power given oneself
// (the same spell on the caster, the drain's own share) counts nothing.
func TestDPSMeterCountsPowerGiven(t *testing.T) {
	power := func(n int) SpellEffect { return SpellEffect{Kind: EffectPower, Amount: n, AmountMax: n} }
	r := newTagRig(t)
	meterClock(t, r.t0)
	group(r.w, 77, r.p, r.friend)
	r.p.dps, r.friend.dps = &dpsMeter{}, &dpsMeter{}
	r.friend.MaxPower = 500
	r.p.MaxPower = 400
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.dpsCountLocked(r.p, 10, false, r.t0)
	want := int64(0)
	check := func(what string) {
		t.Helper()
		for _, v := range []*Player{r.p, r.friend} {
			if _, _, pw, ok := healOf(r.w, v, r.p); !ok || pw != want {
				t.Fatalf("%s: %s's meter has %d power given, want %d", what, v.Name, pw, want)
			}
		}
	}
	r.friend.Power = 350
	r.w.landOnPlayerLocked(r.cast(power(200)), r.p, r.sess, r.friend, r.t0)
	want += 150
	check("200 on a friend 150 short")

	r.p.Power = 100
	r.w.landOnPlayerLocked(r.cast(power(200)), r.p, r.sess, r.p, r.t0)
	if r.p.Power != 300 {
		t.Fatalf("on itself: power %d", r.p.Power)
	}
	check("on itself")

	r.friend.Power = 490
	r.w.runTickLocked(r.friend, &ActiveEffect{SpellID: 9310, Name: "Test Clarity", Caster: r.p.ServerID, Until: r.later, PowerTick: 30}, r.t0)
	want += 10
	check("a power over time's tick of 30, 10 short")

	r.friend.Power = 475
	r.setEffects(r.friend, &ActiveEffect{SpellID: 9311, Name: "Test Flow", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{PoT: 40}})
	r.w.regenSystem(r.t0)
	want += 25
	check("a PoT stat's part at the regen tick, 25 short")

	r.setEffects(r.friend)
	r.friend.Power, r.p.Power = 495, 0
	cs := r.cast(SpellEffect{Kind: EffectPower, Amount: -60, AmountMax: -60, Drain: DrainGroup})
	cs.spell.Range = 50
	r.w.drainLocked(cs, r.p, r.sess, SpellEffect{Kind: EffectPower, Drain: DrainGroup, Amount: -60}, 60, r.t0)
	if r.p.Power == 0 || r.friend.Power != 500 {
		t.Fatalf("the drain gave the caster %d, the friend %d/500", r.p.Power, r.friend.Power)
	}
	want += 5
	check("a drain's power for the group, the friend 5 short")
	if d, _, _ := line(r.w, r.p, r.p); d != 10 {
		t.Fatalf("damage %d: power is no damage", d)
	}
}

// Layout 2 on the wire, byte for byte: the header's healing and power and the
// own places by each, each line's healing, its pet's share and its power after
// its pet's damage. Layout 1 of the same fight is as it was. A raid's lines past
// DPSMeterLinesMost: layout 2 sends its top healer though its damage is the
// least, layout 1 the most damage alone; both pin the receiver's own.
func TestDPSMeterLayoutTwoOnTheWire(t *testing.T) {
	rows := []dpsRow{
		{id: 1, line: dpsLine{name: "Dab", class: 3, damage: 1000, heal: 50, power: 0}, flags: 1},
		{id: 2, line: dpsLine{name: "Jib", class: 9, damage: 300, pet: 120, heal: 900, healPet: 100, power: 40}, flags: 2},
	}
	got := encodeDPSMeterLayout(2, 0x01020304, true, 1500*time.Millisecond, rows)
	want := []byte{2, 4, 3, 2, 1, 1, 0xDC, 5, 0, 0, 0x14, 5, 0, 0, 2, 0, 1, 0,
		0xB6, 3, 0, 0, 40, 0, 0, 0, 2, 0, 2, 0, // healing 950, power 40; Dab 2nd by each
		2,
		1, 3, 0xE8, 3, 0, 0, 0, 0, 0, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 'D', 'a', 'b',
		2, 9, 0x2C, 1, 0, 0, 0x78, 0, 0, 0, 0x84, 3, 0, 0, 100, 0, 0, 0, 40, 0, 0, 0, 3, 'J', 'i', 'b'}
	if !bytes.Equal(got, want) {
		t.Fatalf("layout 2:\n got % X\nwant % X", got, want)
	}
	one := encodeDPSMeterLayout(1, 0x01020304, true, 1500*time.Millisecond, rows)
	if !bytes.Equal(one, encodeDPSMeter(0x01020304, true, 1500*time.Millisecond, rows)) || one[0] != 1 || len(one) != 19+2*(2+8+1+3) {
		t.Fatalf("layout 1 is not as it was: % X", one)
	}

	// a raid of 30: the healer the least damage, the receiver 28th by damage
	var raid []dpsRow
	for i := range 30 {
		raid = append(raid, dpsRow{id: EntityID(100 + i), line: dpsLine{name: string(rune('A'+i%26)) + "x", damage: int64(3000 - 10*i)}})
	}
	raid[27].flags = 1
	raid[27].line.heal, raid[27].line.power = 100, 50 // the receiver: 2nd by each
	raid[29].line.heal = 5000                         // the healer
	raid[28].line.power = 700                         // the power giver
	m2 := readLayout2(t, encodeDPSMeterLayout(2, 9, false, time.Second, raid))
	if len(m2.ids) != DPSMeterLinesMost {
		t.Fatalf("layout 2 sent %d lines", len(m2.ids))
	}
	has := func(ids []int64, damage int64) bool {
		for _, d := range ids {
			if d == damage {
				return true
			}
		}
		return false
	}
	for _, i := range []int{29, 28, 27, 0} {
		if !has(m2.ids, raid[i].line.damage) {
			t.Errorf("layout 2 left out line %d (damage %d): %v", i, raid[i].line.damage, m2.ids)
		}
	}
	if m2.healPlace != 2 || m2.powerPlace != 2 || m2.place != 28 {
		t.Errorf("the own places: damage %d heal %d power %d, want 28, 2, 2", m2.place, m2.healPlace, m2.powerPlace)
	}
	for i := 1; i < len(m2.ids); i++ {
		if m2.ids[i] > m2.ids[i-1] {
			t.Fatalf("layout 2's lines not in the order of damage: %v", m2.ids)
		}
	}
	one = encodeDPSMeterLayout(1, 9, false, time.Second, raid)
	var m1 meterMsg
	m1 = readMeterMsg(t, one)
	for _, l := range m1.lines {
		if l.damage == uint32(raid[29].line.damage) {
			t.Error("layout 1 sent the healer: the most damage alone")
		}
	}
}

// layout2Msg is what TestDPSMeterLayoutTwoOnTheWire reads of a layout 2 body:
// its own places and its lines' damage, in order.
type layout2Msg struct {
	place, healPlace, powerPlace int
	ids                          []int64
}

func readLayout2(t *testing.T, b []byte) layout2Msg {
	t.Helper()
	if b[0] != 2 {
		t.Fatalf("layout %d", b[0])
	}
	m := layout2Msg{place: int(binary.LittleEndian.Uint16(b[16:]))}
	m.healPlace, m.powerPlace = int(binary.LittleEndian.Uint16(b[26:])), int(binary.LittleEndian.Uint16(b[28:]))
	n, at := int(b[30]), 31
	for range n {
		m.ids = append(m.ids, int64(binary.LittleEndian.Uint32(b[at+2:])))
		at += 2 + 20
		at += 1 + int(b[at])
	}
	if at != len(b) {
		t.Fatalf("% X: %d bytes past its lines", b, len(b)-at)
	}
	return m
}

// The system sends each client the layout it asked: the friend's layout 2
// carries the enchanter's healing, the enchanter's layout 1 is the damage
// alone, both of the same fight.
func TestDPSMeterSendsEachItsLayout(t *testing.T) {
	r := newTagRig(t)
	meterClock(t, r.t0)
	group(r.w, 77, r.p, r.friend)
	r.p.dps, r.friend.dps = &dpsMeter{layout: 1}, &dpsMeter{layout: 2}
	r.w.mu.Lock()
	r.w.dpsCountLocked(r.p, 100, false, r.t0)
	r.friend.HP = r.friend.MaxHPNow() - 70
	r.w.landOnPlayerLocked(r.cast(SpellEffect{Kind: EffectHP, Amount: 90, AmountMax: 90}), r.p, r.sess, r.friend, r.t0)
	r.sent()
	r.fsess.Queue().GetPending()
	r.w.dpsMeterSystem(r.t0)
	r.w.mu.Unlock()
	mine := meterMsgs(t, r.sess) // layout 1: readMeterMsg fails on another
	if len(mine) != 1 || mine[0].lines[0].damage != 100 {
		t.Fatalf("the enchanter's layout 1: %+v", mine)
	}
	var friends [][]byte
	for _, m := range r.fsess.Queue().GetPending() {
		if b := m.Data(); len(b) > 2 && b[0] == 0x3C && b[1] == 0x7E {
			friends = append(friends, b[2:])
		}
	}
	if len(friends) != 1 || friends[0][0] != 2 || binary.LittleEndian.Uint32(friends[0][18:]) != 70 {
		t.Fatalf("the friend's layout 2: % X", friends)
	}
}

// The layout asked is kept over a keep restart; a keep file of before layout 2
// (no layout) is layout 1.
func TestDPSMeterLayoutKept(t *testing.T) {
	d := newDuelists(t)
	now := time.Now()
	d.w.SetDPSMeterLayout(d.sa, 2)
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.DPSMeter || k.DPSMeterLayout != 2 {
		t.Fatalf("kept %+v", k)
	}
	for _, c := range []struct{ kept, want uint8 }{{2, 2}, {1, 1}, {0, 1}} {
		d.a.dps = nil
		k.DPSMeterLayout = c.kept
		d.w.RestoreKeptPlayer(d.sa, k, now)
		if got := d.w.DPSMeterLayout(d.sa); got != c.want {
			t.Errorf("kept %d: restored layout %d, want %d", c.kept, got, c.want)
		}
	}
}

// setEffects puts effects on p as the world does (its Mods summed again).
func (r *tagRig) setEffects(p *Player, effects ...*ActiveEffect) {
	p.Effects = effects
	r.w.effectsChangedLocked(p, p.SpeedNow())
}
