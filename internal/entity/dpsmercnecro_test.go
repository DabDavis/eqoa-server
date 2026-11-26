package entity

import (
	"testing"
	"time"
)

// A necromancer merc on the damage meter (Heirloom petition 3252, Cucumber:
// "My Power Scholar NEC Merc did not report anything in the DPS meters"):
// its damage over time's ticks and its life tap's hit count on its own line
// as its damage, the tap's take as its healing, its Erud's Teaching on
// another merc as its power given; with its owner's other two mercs out (so
// no "only merc" guess decides), each figure on the line of the merc that
// made it.
func TestDPSMeterNecroMercCounted(t *testing.T) {
	blight := wbSpell(700825, 60, 1, "Toxxulian Blight", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,"stack":0,"level":60,"min_level":1,
		"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"04C2","type":"Statistic Change","kind":"hp_tick","amount":-193,"scale":"dex","scale_pct":15,"resist":"dr"}]}`)
	bane := wbSpell(700722, 55, 1, "Life's Bane", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":55,"min_level":1,
		"req_equipment":"Any","sheet_type":"Drain","effects":[{"id":"04A2","type":"Drain","kind":"drain","amount":-550,"resist":"ar"}]}`)
	erud := wbSpell(901840, 60, 1, "Erud's Teaching", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"stack":0,"level":60,"min_level":1,
		"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"0967","type":"Statistic Change","kind":"power","amount":1000}]}`)
	r := newMercRig(t)
	cleric := r.hireSlot(1, r.kit(9, 0, 0))
	bard := r.kit(5, 0, 0)
	bard.Name = "Selethyn"
	r.hireSlot(2, bard)
	nk := r.kit(11, 8, 0, blight, bane, erud)
	nk.Name = "Peleth"
	nec := r.hireSlot(3, nk)
	crab := r.creature("a crab", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, 0x01312E98
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	crab.MaxHP, crab.HP = 1_000_000, 1_000_000
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: 3}
	r.w.dpsCountLocked(r.p, 10, false, now)

	// the damage over time: its landing tick, then one more
	hp := crab.HP
	r.w.mercHarmLocked(nec, &blight, crab, now)
	var dot *ActiveEffect
	for _, ae := range crab.Effects {
		if ae.SpellID == blight.SpellID {
			dot = ae
		}
	}
	if dot == nil || dot.merc != nec.npc.ServerID {
		t.Fatalf("Toxxulian Blight on the crab: %+v", dot)
	}
	r.w.mercTickLocked(crab, dot, now.Add(6*time.Second))
	ticks := int64(hp - crab.HP)
	// the life tap, the merc hurt: its hit, and its take back on itself
	nec.npc.HP = nec.npc.MaxHP - 200
	syncMercBody(nec)
	hp, was := crab.HP, nec.npc.HP
	r.w.mercHarmLocked(nec, &bane, crab, now)
	tap, took := int64(hp-crab.HP), int64(nec.npc.HP-was)
	// the power given another merc
	cleric.npc.Power = cleric.npc.MaxPower - 700
	syncMercBody(cleric)
	r.w.mercHelpLocked(nec, &erud, cleric.npc.ServerID, now)
	if ticks <= 0 || tap <= 0 || took <= 0 || cleric.npc.Power != cleric.npc.MaxPower {
		t.Fatalf("ticks %d, the tap %d (took %d), the cleric's power %d/%d", ticks, tap, took, cleric.npc.Power, cleric.npc.MaxPower)
	}

	l := mercLineOf(r.w, r.p, nec)
	if l == nil {
		t.Fatal("the necromancer has no line")
	}
	if l.name != "Peleth" || l.class != 11 || l.damage != ticks+tap || l.heal != took || l.power != 700 {
		t.Errorf("the necromancer's line %+v: want damage %d (ticks %d, tap %d), healing %d, power 700", *l, ticks+tap, ticks, tap, took)
	}
	f := fightOf(r.w, r.p)
	if len(f.mercs) != 1 {
		t.Errorf("merc lines %d, want the necromancer's alone (the others did nothing)", len(f.mercs))
	}
	if o := f.lines[r.p.ServerID]; o.merc != [3]int64{ticks + tap, took, 700} {
		t.Errorf("the owner's merc share %v, want %v", o.merc, [3]int64{ticks + tap, took, 700})
	}
	var row *dpsRow
	rows := r.w.dpsRowsForLocked(r.p, f, 3)
	for i := range rows {
		if rows[i].line.name == "Peleth" {
			row = &rows[i]
		}
	}
	if row == nil || row.flags != 4|8 || row.owner != "Owner" || row.line.damage != ticks+tap {
		t.Errorf("layout 3's rows: %+v", rows)
	}
}

// A merc's own pet is its merc's on the meter (petition 3252: a necromancer
// merc's Swarming Death made most of its damage, all of it counted as its
// hirer's pet's): its hit and the heal it told count on its merc's line, as
// that line's pet share, and on its owner's as its merc share; layout 3 sends
// the owner's line without it, layout 4 the owner's row with it in its merc
// share. Its power given is its merc's too. The control: the owner's own pet's
// hit stays its owner's pet's.
func TestDPSMeterMercPetCountsOnItsMerc(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	ps := ms.pet
	crab := r.creature("a crab", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, 0x01312E98
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = ps.npc.ServerID, 0x01312E99
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	crab.MaxHP, crab.HP = 1_000_000, 1_000_000
	now := r.at(20 * time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: 3}
	r.w.dpsCountLocked(r.p, 10, false, now)

	hp := crab.HP
	r.w.petLandHitLocked(ps, crab, 120, now, false, numberMelee)
	hit := int64(hp - crab.HP)
	ps.npc.HP = ps.npc.MaxHP - 50
	r.w.dpsWorthLocked(50, func() { ps.npc.HP += 50; r.w.tellPetHealLocked(ps, 80) })
	r.p.Power = 0
	r.w.dpsPowerLocked(ps.npc.ServerID, r.p.ServerID, 30)
	if hit <= 0 {
		t.Fatal("the pet's hit did nothing")
	}

	l := mercLineOf(r.w, r.p, ms)
	if l == nil || l.damage != hit || l.pet != hit || l.heal != 50 || l.healPet != 50 || l.power != 30 {
		t.Fatalf("the merc's line %+v: want its pet's damage %d (its pet share), healing 50, power 30", l, hit)
	}
	f := fightOf(r.w, r.p)
	o := f.lines[r.p.ServerID]
	if o.damage != 10+hit || o.pet != hit || o.merc != [3]int64{hit, 50, 30} {
		t.Fatalf("the owner's line %+v", *o)
	}
	for _, row := range r.w.dpsRowsForLocked(r.p, f, 3) {
		if row.line.name == "Owner" && (row.line.damage != 10 || row.line.pet != 0 || row.line.heal != 0 || row.line.power != 0) {
			t.Errorf("layout 3, the owner's line less its merc's: %+v", row.line)
		}
	}

	// the control: the owner's own pet
	own := &petState{owner: r.p, npc: &NPC{ServerID: 4950, Name: "a skeleton", Owner: r.p.ServerID, HP: 100, MaxHP: 100}, kind: petKindOurs}
	r.w.putNPCLocked(own.npc)
	r.w.dpsByPetLocked(own, func() {
		r.w.sendDamageIndication(r.sess, r.sess.PlayerObjectID(), -7, 0x01312E98, numberMelee.byPet())
	})
	if o.pet != hit+7 || o.merc[dpsDamage] != hit || l.damage != hit {
		t.Errorf("the owner's own pet's hit: owner %+v, merc line %+v", *o, *l)
	}
}

// A merc that dies in a fight and comes back (another NPC, another id) goes on
// on its one line (petition 3252's fight: Peleth died and came back, two lines
// of one name); another merc in another slot keeps its own.
func TestDPSMeterMercBackKeepsItsLine(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(11, 0, 0)
	k.Name = "Peleth"
	ms := r.hireSlot(1, k)
	other := r.kit(11, 0, 0)
	other.Name = "Peleth"
	ms2 := r.hireSlot(2, other) // the same name, another slot: another merc
	crab := r.creature("a crab", 30, 3)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, 0x01312E98
	r.w.mu.Lock()
	crab.MaxHP, crab.HP = 1_000_000, 1_000_000
	now := r.at(time.Second)
	meterClock(t, now)
	r.p.dps = &dpsMeter{layout: 3}
	r.w.mercDamageLocked(ms, crab, 40, numberSpell, now)
	r.w.mercDamageLocked(ms2, crab, 5, numberSpell, now)
	first := ms.npc.ServerID
	r.w.mu.Unlock()

	if why, ok := r.w.DismissMerc(r.sess, 1, now); !ok {
		t.Fatalf("the dismissal: %s", why)
	}
	back := r.hireSlot(1, k)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if back.npc.ServerID == first {
		t.Fatal("the merc came back under its old id: nothing to test")
	}
	r.w.mercDamageLocked(back, crab, 60, numberSpell, now)
	f := fightOf(r.w, r.p)
	if len(f.mercs) != 2 {
		t.Fatalf("merc lines %d, want 2: %+v", len(f.mercs), f.mercs)
	}
	if l := f.mercs[first]; l == nil || l.damage != 100 || l.slot != 1 {
		t.Errorf("slot 1's line %+v, want its damage before and after, 100", l)
	}
	if l := mercLineOf(r.w, r.p, ms2); l == nil || l.damage != 5 {
		t.Errorf("slot 2's line %+v, want 5", l)
	}
}
