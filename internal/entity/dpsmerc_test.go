package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// A merc's own line on the damage meter (dpsmerc.go, Heirloom petition 3134):
// each merc's damage, healing and power given count on a line of its own as
// well as on its owner's (layouts 1 and 2 as before); layout 3 sends the
// merc's line apart, flagged, its owner named, and the owner's without it.

// mercLineOf is merc's own line in the fight p's meter is shown, nil none.
func mercLineOf(w *World, p *Player, merc *mercState) *dpsMercLine {
	f := fightOf(w, p)
	if f == nil {
		return nil
	}
	return f.mercs[merc.npc.ServerID]
}

// Two mercs of one owner each count on a line of their own, told apart by
// which made each figure: the healer's hit, heal on its owner and heal over
// time's tick on it; the enchanter's hit and power given its owner. The owner's
// line still holds all of it as its helpers' share (layout 2 unchanged); its
// summoned pet's figure stays its pet share, on no merc's line, though a merc's
// action is told round it. Layout 3: the owner's line less its mercs', each
// merc's line flagged 4 (and 8 on its owner's own meter), its owner named, the
// wholes the same as layout 2's.
func TestDPSMeterMercLines(t *testing.T) {
	r := newMercRig(t)
	heal := bookSpell(700187, "Field Dress", 1, 0, 0, 0, "Target, Increase Hitpoints by 125")
	hot := bookSpell(464, "Healing", 1, 0, 0, 0, "Target, Increase HP per tic by 125, 24sec")
	k1 := r.kit(9, 0, 0, heal, hot)
	k2 := r.kit(12, 0, 0)
	k2.Name = "Kellasis"
	m1 := r.hireSlot(1, k1)
	m2 := r.hireSlot(2, k2)
	_, b := r.player("B", 9, 0)
	r.group(b)
	crab := r.creature("a crab", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, 0x01312E98
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps, b.dps = &dpsMeter{layout: 3}, &dpsMeter{layout: 3}
	r.w.dpsCountLocked(r.p, 100, false, now)
	r.w.dpsCountLocked(b, 40, false, now)
	r.w.mercDamageLocked(m1, crab, 50, numberMelee, now)
	r.w.mercDamageLocked(m2, crab, 30, numberSpell, now)
	r.p.HP = r.p.MaxHPNow() - 400
	before := r.p.HP
	r.w.mercHelpLocked(m1, &heal, r.p.ServerID, now)
	r.w.mercHelpLocked(m1, &hot, r.p.ServerID, now)
	var ae *ActiveEffect
	for _, e := range r.p.Effects {
		if e.SpellID == hot.SpellID {
			ae = e
		}
	}
	if ae == nil {
		t.Fatal("the healer's heal over time is not on its owner")
	}
	healed := r.p.HP
	r.w.runTickLocked(r.p, ae, now.Add(6*time.Second))
	tick := int64(r.p.HP - healed)
	restored := int64(r.p.HP - before) // the heal, its first tick at its landing and the tick
	r.p.Power = 0
	r.w.mercHelpLocked(m2, ptr(powerBoon()), r.p.ServerID, now)
	power := int64(r.p.Power)
	// the control: the owner's summoned pet's hit, told while a merc's action is
	r.w.dpsByLocked(m1.npc.ServerID, func() {
		r.w.sendDamageIndication(r.sess, r.sess.PlayerObjectID(), -9, 0x01312E98, numberMelee.byPet())
	})
	if tick <= 0 || power <= 0 {
		t.Fatalf("the tick restored %d, the boon gave %d", tick, power)
	}

	f := fightOf(r.w, r.p)
	o := f.lines[r.p.ServerID]
	if o.damage != 189 || o.pet != 89 || o.heal != restored || o.healPet != restored || o.power != power {
		t.Fatalf("the owner's line %+v: want damage 189 (pet 89), healing %d (all its mercs'), power %d", *o, restored, power)
	}
	l1, l2 := mercLineOf(r.w, r.p, m1), mercLineOf(r.w, r.p, m2)
	if l1 == nil || l2 == nil || len(f.mercs) != 2 {
		t.Fatalf("merc lines %+v", f.mercs)
	}
	if l1.name != "Ralthas" || l1.class != 9 || l1.damage != 50 || l1.heal != restored || l1.power != 0 || l1.owner != r.p.ServerID || l1.ownerName != "Owner" {
		t.Errorf("the healer's line %+v", *l1)
	}
	if l2.name != "Kellasis" || l2.class != 12 || l2.damage != 30 || l2.heal != 0 || l2.power != power {
		t.Errorf("the enchanter's line %+v", *l2)
	}

	// layout 2 as it was: the players' lines alone
	two := r.w.dpsRowsForLocked(r.p, f, 2)
	if len(two) != 2 {
		t.Fatalf("layout 2's rows: %+v", two)
	}
	for _, v := range []*Player{r.p, b} {
		rows := r.w.dpsRowsForLocked(v, f, 3)
		if len(rows) != 4 {
			t.Fatalf("%s's layout 3 rows: %+v", v.Name, rows)
		}
		by := map[string]dpsRow{}
		for _, row := range rows {
			by[row.line.name] = row
		}
		own := byte(0)
		if v == r.p {
			own = 8
		}
		if ow := by["Owner"].line; ow.damage != 109 || ow.pet != 9 || ow.heal != 0 || ow.healPet != 0 || ow.power != 0 {
			t.Errorf("%s's view, the owner's line less its mercs: %+v", v.Name, ow)
		}
		for _, name := range []string{"Ralthas", "Kellasis"} {
			if row := by[name]; row.flags != 4|own || row.owner != "Owner" {
				t.Errorf("%s's view, %s: flags %d owner %q, want %d, Owner", v.Name, name, row.flags, row.owner, 4|own)
			}
		}
		if by["B"].flags&^1 != 0 || by["B"].owner != "" {
			t.Errorf("%s's view, B: %+v", v.Name, by["B"])
		}
		for i := 1; i < len(rows); i++ {
			if rows[i].line.damage > rows[i-1].line.damage {
				t.Fatalf("not in the order of damage: %+v", rows)
			}
		}
		b2 := encodeDPSMeterLayout(2, f.number, false, time.Second, r.w.dpsRowsForLocked(v, f, 2))
		b3 := encodeDPSMeterLayout(3, f.number, false, time.Second, rows)
		if b3[0] != 3 || !bytes.Equal(b2[10:14], b3[10:14]) || !bytes.Equal(b2[18:26], b3[18:26]) {
			t.Errorf("the wholes differ: layout 2 % X, layout 3 % X", b2[:31], b3[:31])
		}
		if n := binary.LittleEndian.Uint16(b3[14:]); n != 4 {
			t.Errorf("layout 3's lines in all: %d", n)
		}
	}
}

// Layout 3 on the wire, byte for byte: layout 2's with each line's owner's name
// after its own (none a player's); a message the owners' names would make too
// long is sent without them, its lines the same.
func TestDPSMeterLayoutThreeOnTheWire(t *testing.T) {
	rows := []dpsRow{
		{id: 1, line: dpsLine{name: "Dab", class: 3, damage: 1000, heal: 50}, flags: 1},
		{id: 7, line: dpsLine{name: "Kel", class: 9, damage: 300, heal: 900, power: 40}, flags: 4 | 8, owner: "Dab"},
	}
	got := encodeDPSMeterLayout(3, 0x01020304, true, 1500*time.Millisecond, rows)
	want := []byte{3, 4, 3, 2, 1, 1, 0xDC, 5, 0, 0, 0x14, 5, 0, 0, 2, 0, 1, 0,
		0xB6, 3, 0, 0, 40, 0, 0, 0, 2, 0, 2, 0,
		2,
		1, 3, 0xE8, 3, 0, 0, 0, 0, 0, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 'D', 'a', 'b', 0,
		12, 9, 0x2C, 1, 0, 0, 0, 0, 0, 0, 0x84, 3, 0, 0, 0, 0, 0, 0, 40, 0, 0, 0, 3, 'K', 'e', 'l', 3, 'D', 'a', 'b'}
	if !bytes.Equal(got, want) {
		t.Fatalf("layout 3:\n got % X\nwant % X", got, want)
	}
	if two := encodeDPSMeterLayout(2, 0x01020304, true, 1500*time.Millisecond, rows); len(two) != len(want)-5 || two[0] != 2 {
		t.Fatalf("layout 2 of the same rows: % X", two)
	}

	// 24 mercs of long names, of owners of long names: no owners then
	long := "Abcdefghijklmnopqrstuvw" // 23 bytes
	var raid []dpsRow
	for i := range 30 {
		raid = append(raid, dpsRow{id: EntityID(100 + i), line: dpsLine{name: long, damage: int64(3000 - i)}, flags: 4, owner: long})
	}
	raid[0].flags, raid[0].owner = 1, ""
	b := encodeDPSMeterLayout(3, 9, false, time.Second, raid)
	if len(b) > dpsMeterBodyMost {
		t.Fatalf("%d bytes, most %d", len(b), dpsMeterBodyMost)
	}
	n, at := int(b[30]), 31
	if n != DPSMeterLinesMost {
		t.Fatalf("%d lines", n)
	}
	for range n {
		at += 2 + 20
		at += 1 + int(b[at])
		if b[at] != 0 {
			t.Fatalf("an owner's name in a message too long for them")
		}
		at++
	}
	if at != len(b) {
		t.Fatalf("%d bytes past the lines", len(b)-at)
	}
	// the control: a short raid's owners are sent
	if b := encodeDPSMeterLayout(3, 9, false, time.Second, raid[:5]); !bytes.Contains(b, append([]byte{23}, long...)) || len(b) != 31+5*(2+20+24)+4*24+1 {
		t.Fatalf("a short message: % X", b)
	}
}

// A necromancer merc's Power path (Power Squall: "Drain Group" power, a hit at
// its foe since petition 2302) gives its team power, as a player's does: its
// owner and its owner's other merc each up to its most, counted as the merc's
// power given (on its line and its owner's), the merc's own gain none.
func TestMercPowerPathGivesPower(t *testing.T) {
	squall := wbSpell(700341, 30, 1, "Power Squall", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"type":"Drain Group","kind":"drain_group","stat":"power","amount":-450}]}`)
	r := newMercRig(t)
	nec := r.kit(11, 0, 0, squall)
	nec.Name = "Morth"
	ms := r.hireSlot(1, nec)
	other := r.hireSlot(2, r.kit(12, 0, 0))
	crab := r.creature("a crab", 30, 3)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: 3}
	r.w.dpsCountLocked(r.p, 10, false, now)
	r.p.Power = 0
	other.npc.Power = other.npc.MaxPower - 100
	ms.npc.Power = 0
	r.w.mercHarmLocked(ms, &squall, crab, now)
	give := r.p.Power
	if give <= 0 || other.npc.Power != other.npc.MaxPower || ms.npc.Power <= 0 {
		t.Fatalf("Power Squall gave the owner %d, the other merc %d/%d, itself %d", give, other.npc.Power, other.npc.MaxPower, ms.npc.Power)
	}
	want := int64(give + 100)
	if l := mercLineOf(r.w, r.p, ms); l == nil || l.power != want {
		t.Fatalf("the necromancer's line %+v, want power %d", l, want)
	}
	if _, _, pw, _ := healOf(r.w, r.p, r.p); pw != want {
		t.Fatalf("the owner's line's power %d, want %d", pw, want)
	}
}

// A necromancer merc's Blood path (Blood Squall: "Drain Group" hit points,
// the owner's call on petition 3134's follow-up) heals as a player's does: the
// merc, its owner and an aggressive merc's group mate in reach and sight, each
// up to its most; a mate out of reach none. All of it
// is the merc's healing on its own line (its owner's merc share too); before,
// it healed the merc alone.
func TestMercBloodPathHealsTheGroup(t *testing.T) {
	blood := wbSpell(700323, 30, 1, "Blood Squall", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Drain Group","effects":[{"type":"Drain Group","kind":"drain_group","stat":"hp","amount":-450}]}`)
	r := newMercRig(t)
	nec := r.kit(11, 0, 0, blood)
	nec.Name = "Morth"
	ms := r.hire(nec, MercAggressive) // an aggressive merc's heals reach its group mates (merccall.go)
	_, b := r.player("B", 9, 0)
	_, far := r.player("Far", 9, 0)
	r.group(b, far)
	crab := r.creature("a crab", 30, 3)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	crab.MaxHP, crab.HP = 1_000_000, 1_000_000
	far.X += 500
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: 3}
	r.w.dpsCountLocked(r.p, 10, false, now)
	r.p.HP, b.HP, far.HP = r.p.MaxHPNow()-100, b.MaxHPNow()-80, far.MaxHPNow()-100
	ms.npc.HP = ms.npc.MaxHP - 30
	syncMercBody(ms)
	hp := map[string]int{"owner": r.p.HP, "b": b.HP, "far": far.HP, "merc": ms.npc.HP}
	r.w.mercHarmLocked(ms, &blood, crab, now)
	got := map[string]int{"owner": r.p.HP - hp["owner"], "b": b.HP - hp["b"], "far": far.HP - hp["far"], "merc": ms.npc.HP - hp["merc"]}
	if got["owner"] != 100 || got["merc"] != 30 || got["b"] != 80 || got["far"] != 0 {
		t.Fatalf("restored %v: want the owner 100, itself 30, B 80, Far (out of reach) none", got)
	}
	want := int64(got["owner"] + got["b"] + got["merc"])
	if l := mercLineOf(r.w, r.p, ms); l == nil || l.heal != want {
		t.Fatalf("the necromancer's line %+v, want healing %d", l, want)
	}
	if h, hp, _, _ := healOf(r.w, r.p, r.p); h != want || hp != want {
		t.Fatalf("the owner's line: healing %d (merc share %d), want %d", h, hp, want)
	}
}
