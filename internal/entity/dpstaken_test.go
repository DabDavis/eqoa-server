package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// Damage taken and every merc's row (dpstaken.go, Heirloom petition 3397).

// A player's damage taken is the damage told it on itself, into a fight under
// way only; a merc's is the hit points a creature's hit took off it, on its
// own line and its owner's merc share. Controls: nothing before a fight starts,
// and a figure the player made on a creature is no damage taken.
func TestDPSMeterCountsDamageTaken(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(0, 0, 0)
	k.Name = "Ordrin"
	ms := r.hireSlot(1, k)
	crab := r.creature("a crab", 30, 3)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: dpsMeterLayoutTaken}
	me := r.sess.PlayerObjectID()

	// no fight yet: a hit taken starts none
	r.w.sendDamageIndication(r.sess, 0x01312E98, -60, me, 0)
	ms.npc.HP = r.w.dpsTakeHPLocked(ms.npc, 25)
	if f := fightOf(r.w, r.p); f != nil {
		t.Fatalf("damage taken started a fight: %+v", f.lines[r.p.ServerID])
	}

	r.w.dpsCountLocked(r.p, 10, false, now)
	r.w.sendDamageIndication(r.sess, 0x01312E98, -75, me, 0)
	r.w.sendDamageIndication(r.sess, 0x01312E98, -5, me, numberDoT) // a tagged figure on itself: taken too
	r.w.sendDamageIndication(r.sess, me, -40, 0x01312E98, numberMelee) // its own hit on the crab: dealt, not taken
	before := ms.npc.HP
	ms.npc.HP = r.w.dpsTakeHPLocked(ms.npc, 30)
	if before-ms.npc.HP != 30 {
		t.Fatalf("the merc lost %d, want 30", before-ms.npc.HP)
	}
	crab.HP = r.w.dpsTakeHPLocked(crab, 30) // a creature's loss is no one's damage taken

	f := fightOf(r.w, r.p)
	l := f.lines[r.p.ServerID]
	if l.taken != 110 || l.takenMerc != 30 || l.damage != 50 {
		t.Fatalf("the owner's line: taken %d (mercs' %d), damage %d; want 110 (30), 50", l.taken, l.takenMerc, l.damage)
	}
	if ml := f.mercs[ms.npc.ServerID]; ml == nil || ml.taken != 30 {
		t.Fatalf("the merc's line %+v: want 30 taken", ml)
	}
}

// Layout 6 is layout 5 with the taken whole and the own place before the row
// count, each row's taken (its own, its mercs') last, and each merc line's taken
// last. Layout 5 is unchanged (the control).
func TestDPSMeterLayoutSixOnTheWire(t *testing.T) {
	rows := []dpsRow{
		{id: 1, line: dpsLine{name: "Dab", class: 0, damage: 1000, taken: 900, takenMerc: 300, merc: [3]int64{200}}, flags: 1,
			mercs: []dpsRow{{id: 50, line: dpsLine{name: "Kel", class: 9, damage: 200, taken: 300}, flags: 4 | 8, owner: "Dab"}}},
		{id: 2, line: dpsLine{name: "Jib", class: 9, damage: 300, taken: 2000}},
	}
	five := encodeDPSMeterLayout(5, 7, false, time.Second, rows)
	six := encodeDPSMeterLayout(6, 7, false, time.Second, rows)
	if six[0] != 6 {
		t.Fatalf("layout byte %d", six[0])
	}
	// the header: layout 5's 48 bytes, then the taken whole and the own place
	if !bytes.Equal(six[1:48], five[1:48]) {
		t.Fatalf("the header is not layout 5's:\n six % X\nfive % X", six[:48], five[:48])
	}
	if got := binary.LittleEndian.Uint32(six[48:]); got != 2900 {
		t.Errorf("taken whole %d, want 2900", got)
	}
	if got := binary.LittleEndian.Uint16(six[52:]); got != 2 {
		t.Errorf("own place by taken %d, want 2 (600 own against Jib's 2000)", got)
	}
	if six[54] != 2 || five[48] != 2 {
		t.Fatalf("rows: six %d five %d", six[54], five[48])
	}
	// the rows: each layout 5's, then u32 own taken, u32 mercs' taken
	at5, at6 := 49, 55
	for i, want := range [][2]uint32{{600, 300}, {2000, 0}} {
		n := 23 + int(five[at5+22])
		if five[at5]&4 != 0 {
			n += 12
		}
		if !bytes.Equal(six[at6:at6+n], five[at5:at5+n]) {
			t.Fatalf("row %d is not layout 5's", i)
		}
		at5, at6 = at5+n, at6+n
		if a, b := binary.LittleEndian.Uint32(six[at6:]), binary.LittleEndian.Uint32(six[at6+4:]); a != want[0] || b != want[1] {
			t.Errorf("row %d taken %d, mercs' %d; want %d, %d", i, a, b, want[0], want[1])
		}
		at6 += 8
	}
	// the merc lines: layout 5's, each with its taken last
	if six[at6] != 1 || five[at5] != 1 {
		t.Fatalf("merc lines: six %d five %d", six[at6], five[at5])
	}
	m5 := five[at5+1:]
	m6 := six[at6+1:]
	if !bytes.Equal(m6[:len(m5)], m5) {
		t.Fatalf("the merc line is not layout 5's:\n six % X\nfive % X", m6, m5)
	}
	if got := binary.LittleEndian.Uint32(m6[len(m5):]); got != 300 || len(m6) != len(m5)+4 {
		t.Errorf("the merc line's taken %d (%d bytes past), want 300", got, len(m6)-len(m5))
	}
	// layout 5 never carries taken: the same rows without it encode the same
	bare := []dpsRow{rows[0], rows[1]}
	bare[0].line.taken, bare[0].line.takenMerc, bare[1].line.taken = 0, 0, 0
	bare[0].mercs = []dpsRow{rows[0].mercs[0]}
	bare[0].mercs[0].line.taken = 0
	if !bytes.Equal(encodeDPSMeterLayout(5, 7, false, time.Second, bare), five) {
		t.Errorf("layout 5 changed with damage taken")
	}
}

// A raid of long names in layout 6 fits a message, the receiver's own row in it.
func TestDPSMeterLayoutSixFits(t *testing.T) {
	long := "Abcdefghijklmnopqrstuvw"
	raid := make([]dpsRow, 30)
	for i := range raid {
		raid[i] = dpsRow{id: EntityID(100 + i), line: dpsLine{name: long, damage: int64(3000 - i), taken: int64(i), merc: [3]int64{30}}}
		for k := range 3 {
			raid[i].mercs = append(raid[i].mercs, dpsRow{id: EntityID(1000 + 3*i + k), line: dpsLine{name: long, damage: int64(10 + k)}, flags: 4})
		}
	}
	raid[29].flags = 1
	b := encodeDPSMeterLayout(6, 9, false, time.Second, raid)
	if len(b) > dpsMeterBodyMost {
		t.Fatalf("%d bytes, most %d", len(b), dpsMeterBodyMost)
	}
	sent, at := int(b[54]), 55
	own := false
	for range sent {
		row := b[at:]
		own = own || row[0]&1 != 0
		at += 23 + int(row[22]) + 8
		if row[0]&4 != 0 {
			at += 12
		}
	}
	n, at := int(b[at]), at+1
	if !own || sent == 0 || n != 3*sent {
		t.Fatalf("%d rows (own %t), %d merc lines: want every row's three", sent, own, n)
	}
	for range n {
		at += 24 + int(b[at+23]) + 4
	}
	if at != len(b) {
		t.Fatalf("%d bytes past the merc lines", len(b)-at)
	}
}

// Every merc out of a member has a row in layout 6, of nothing too, flagged
// 4 (8 the receiver's own); a merc already counted is not doubled. Layout 5
// leaves the idle merc out (the control).
func TestDPSMeterLayoutSixRowsEveryMerc(t *testing.T) {
	r := newMercRig(t)
	k1 := r.kit(9, 0, 0)
	k1.Name = "Kel"
	k2 := r.kit(0, 0, 0)
	k2.Name = "Ord"
	idle := r.hireSlot(1, k1)
	busy := r.hireSlot(2, k2)
	_, b := r.player("B", 9, 0)
	r.group(b)
	crab := r.creature("a crab", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, 0x01312E98
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	crab.MaxHP, crab.HP = 1_000_000, 1_000_000
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: dpsMeterLayoutTaken}
	r.w.dpsCountLocked(r.p, 10, false, now)
	r.w.mercDamageLocked(busy, crab, 40, numberSpell, now)
	f := fightOf(r.w, r.p)

	rows := r.w.dpsRowsForLocked(r.p, f, dpsMeterLayoutTaken)
	var own *dpsRow
	for i := range rows {
		if rows[i].id == r.p.ServerID {
			own = &rows[i]
		}
	}
	if own == nil || len(own.mercs) != 2 {
		t.Fatalf("the owner's row %+v: want both mercs", own)
	}
	names := map[string]dpsRow{}
	for _, m := range own.mercs {
		names[m.line.name] = m
	}
	if m, ok := names["Kel"]; !ok || m.id != idle.npc.ServerID || m.line.damage != 0 || m.flags != 4|8 || m.line.class != 9 {
		t.Errorf("the idle merc's row %+v", m)
	}
	if m := names["Ord"]; m.line.damage != 40 {
		t.Errorf("the busy merc's row %+v", m)
	}
	// the group's other member is on the meter too, with no merc of its own
	if len(rows) != 2 {
		t.Errorf("%d rows, want the owner's and B's", len(rows))
	}
	// the control: layout 5 has only the merc that counted
	for _, row := range r.w.dpsRowsForLocked(r.p, f, 5) {
		if row.id == r.p.ServerID && len(row.mercs) != 1 {
			t.Errorf("layout 5's row has %d merc lines, want 1", len(row.mercs))
		}
	}
}

// Layout 6 byte for byte: the same fixture as the Rust client's layout 6
// decoder test (layout 4's rows and layout 5's mercs, with damage taken).
func TestDPSMeterLayoutSixFixture(t *testing.T) {
	rows := []dpsRow{
		{id: 1, line: dpsLine{name: "Dab", class: 3, damage: 1000, pet: 420, heal: 950, healPet: 910, power: 40, merc: [3]int64{300, 900, 40}, taken: 900, takenMerc: 300}, flags: 1,
			mercs: []dpsRow{
				{id: 50, line: dpsLine{name: "Kel", class: 9, damage: 200, heal: 900, power: 40, taken: 300}, flags: 4 | 8, owner: "Dab"},
				{id: 51, line: dpsLine{name: "Ord", class: 0, damage: 100, pet: 10}, flags: 4 | 8, owner: "Dab"},
			}},
		{id: 2, line: dpsLine{name: "Jib", class: 9, damage: 300, heal: 120, power: 10, taken: 2000}, flags: 2},
	}
	want := []byte{
		6, 4, 3, 2, 1, 1, 0xDC, 5, 0, 0, 0x14, 5, 0, 0, 2, 0, 1, 0,
		0x2E, 4, 0, 0, 50, 0, 0, 0, 1, 0, 1, 0,
		0xE8, 3, 0, 0, 170, 0, 0, 0, 10, 0, 0, 0, 1, 0, 2, 0, 2, 0,
		0x54, 0x0B, 0, 0, 2, 0, // taken 2900, own place 2
		2,
		5, 3, 0xBC, 2, 0, 0, 120, 0, 0, 0, 50, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 3, 'D', 'a', 'b',
		0x2C, 1, 0, 0, 0x84, 3, 0, 0, 40, 0, 0, 0,
		0x58, 2, 0, 0, 0x2C, 1, 0, 0, // taken 600, its mercs' 300
		2, 9, 0x2C, 1, 0, 0, 0, 0, 0, 0, 120, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 3, 'J', 'i', 'b',
		0xD0, 7, 0, 0, 0, 0, 0, 0, // taken 2000, none of mercs
		2,
		4 | 8, 9, 200, 0, 0, 0, 0, 0, 0, 0, 0x84, 3, 0, 0, 0, 0, 0, 0, 40, 0, 0, 0, 0, 3, 'K', 'e', 'l', 0x2C, 1, 0, 0,
		4 | 8, 0, 100, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 'O', 'r', 'd', 0, 0, 0, 0,
	}
	if got := encodeDPSMeterLayout(6, 0x01020304, true, 1500*time.Millisecond, rows); !bytes.Equal(got, want) {
		t.Fatalf("layout 6:\n got % X\nwant % X", got, want)
	}
}

// Heirloom petition 3397's cleric: three mercs out, the cleric nuking (a
// merc's spell hit through its cast's landing, mercHarmLocked) has a line of
// its own, its damage its own and not the bard's or the necromancer's; the
// other two keep theirs; and with the target out of the owner's view (no
// figure told) its nuke counts all the same.
func TestDPSMeterThreeMercsTheClericsNuke(t *testing.T) {
	justice := bookSpell(700793, "Justice", 1, 0, 0, 0, "Target, Decrease Hitpoints by 300")
	r := newMercRig(t)
	kc := r.kit(9, 0, 0, justice)
	kc.Name = "Orardar"
	kb := r.kit(5, 0, 0)
	kb.Name = "Selethyn"
	kn := r.kit(11, 0, 0)
	kn.Name = "Peleth"
	cleric, bard, nec := r.hireSlot(1, kc), r.hireSlot(2, kb), r.hireSlot(3, kn)
	scout := r.creature("erych scout", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = scout.ServerID, 0x01312E98
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	scout.MaxHP, scout.HP = 1_000_000, 1_000_000
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: dpsMeterLayoutTaken}
	r.w.dpsCountLocked(r.p, 10, false, now)
	r.w.mercDamageLocked(bard, scout, 40, numberMelee, now)
	r.w.mercDamageLocked(nec, scout, 60, numberSpell, now)
	hp := scout.HP
	r.w.mercHarmLocked(cleric, &justice, scout, now)
	nuke := int64(hp - scout.HP)
	if nuke <= 0 {
		t.Fatalf("Justice did nothing")
	}
	for _, c := range []struct {
		ms   *mercState
		want int64
	}{{cleric, nuke}, {bard, 40}, {nec, 60}} {
		if l := mercLineOf(r.w, r.p, c.ms); l == nil || l.damage != c.want {
			t.Errorf("%s's line %+v, want damage %d", c.ms.npc.Name, l, c.want)
		}
	}
	f := fightOf(r.w, r.p)
	rows := r.w.dpsRowsForLocked(r.p, f, dpsMeterLayoutTaken)
	if len(rows) != 1 || len(rows[0].mercs) != 3 || rows[0].mercs[0].line.name != "Orardar" && nuke > 60 {
		t.Errorf("the rows %+v: want the owner's with three mercs", rows)
	}
	// the scout out of the owner's view: its nuke untold, counted all the same
	// on the cleric's line (and the owner's merc share)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = 0, 0
	hp = scout.HP
	r.w.mercHarmLocked(cleric, &justice, scout, now)
	second := int64(hp - scout.HP)
	if l := mercLineOf(r.w, r.p, cleric); l.damage != nuke+second {
		t.Errorf("an untold nuke: the cleric's line %d, want %d", l.damage, nuke+second)
	}
	if l := f.lines[r.p.ServerID]; l.merc[dpsDamage] != 40+60+nuke+second {
		t.Errorf("the owner's merc share %d, want %d", l.merc[dpsDamage], 40+60+nuke+second)
	}
}
