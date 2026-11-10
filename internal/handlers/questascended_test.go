package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// ascendedQuestFixture is the Plane of Sky's armourers' kind of quest (petition
// 1446's example: the Ring of Veeshan handed in for a breastplate): Stark asks
// a talisman and a ring of a tuned boss's for a breastplate (the magician's,
// by class) and a draught; Verna two rings for one; Toi a ring shown; Kell a
// ring looted. The ring has its Ascended record, the breastplate none yet.
type ascendedQuestFixture struct {
	*questFixture
	stark, verna, toi, kell                  uint32
	talisman, ring, plate, draught, ascended int
}

const (
	ascendedGive = 30001 + iota
	ascendedTwo
	ascendedShow
	ascendedLoot
)

// gearPattern adds an item worn in wear slot slot, of that AC, and returns
// its patternid.
func (f *questFixture) gearPattern(name string, slot, ac int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color, AC)
		VALUES (500, 0, 209763529, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, 1, 300, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0, ?)`,
		slot, name, ac)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func newAscendedQuestFixture(t *testing.T) *ascendedQuestFixture {
	t.Helper()
	f := &ascendedQuestFixture{questFixture: newQuestFixture(t)}
	at := func(slot int, id uint32, name string) uint32 {
		return f.tw.add(slot, &entity.NPC{ServerID: entity.EntityID(id), Name: name, SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	}
	f.stark, f.verna, f.toi, f.kell = at(1, 701, "Stark"), at(2, 702, "Verna"), at(3, 703, "Toi"), at(4, 704, "Kell")
	f.talisman = f.pattern("talisman of the test", 20)
	f.draught = f.pattern("draught of the test", 20)
	f.ring = f.gearPattern("Ring of the Test", 8, 14)
	f.plate = f.gearPattern("Breastplate of the Test", 5, 100)
	id, made, err := f.qe.items.AscendedOf(f.ring, "test")
	if err != nil || !made {
		t.Fatalf("the ring's Ascended record: %d %v %v", id, made, err)
	}
	f.ascended = id
	quest := func(id int, name, giver, kind, items string) {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, repeatable, offer, accept, decline, declined, started)
			VALUES (?, ?, 1, ?, 1, 10, '10', 1, ?, 'I will.', 'Not now.', 'Then not.', 'Go on.')`, id, name, giver, "Will you make "+name+"?")
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, journal, says, reminder, rewarditems, byclass)
			VALUES (?, 1, ?, ?, ?, 1, 'Do it.', 'There.', 'Not all of it yet.', ?, ?)`,
			id, kind, giver, items, fmt.Sprint(f.draught), fmt.Sprintf(`{"10": {"items": "%d"}}`, f.plate))
	}
	quest(ascendedGive, "the breastplate", "Stark", "give", fmt.Sprintf("%d, %d", f.talisman, f.ring))
	quest(ascendedTwo, "the second breastplate", "Verna", "give", fmt.Sprintf("%d*2", f.ring))
	quest(ascendedShow, "the shown breastplate", "Toi", "show", fmt.Sprint(f.ring))
	quest(ascendedLoot, "the looted breastplate", "Kell", "loot", fmt.Sprint(f.ring))
	f.exec(`UPDATE queststeps SET target = 'drake' WHERE questid = ?`, ascendedLoot)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	entity.SetAscendedItems(map[int]int{f.ascended: f.ring})
	t.Cleanup(func() { entity.SetAscendedItems(nil) })
	return f
}

// take is the quest at the NPC taken, then the NPC talked to again: the step
// done (or not), and what the client was told.
func (f *ascendedQuestFixture) take(npc uint32, question string) []string {
	f.t.Helper()
	f.msgs()
	f.tw.talk(npc)
	m := f.msgs()
	if !asks(m, question) {
		f.t.Fatalf("not offered %q: %q", question, m)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	f.tw.talk(npc)
	return f.msgs()
}

// ascendedOf is the Ascended record of base, 0 for none.
func (f *ascendedQuestFixture) ascendedOf(base int) int {
	var id int
	f.db.QueryRow(`SELECT patternid FROM itemascended WHERE base = ?`, base).Scan(&id)
	return id
}

// Petition 1446: the NPC accepts the ring or its Ascended record, and the
// Ascended one gives the reward gear's Ascended record, made at once (the
// breastplate had none); what is no gear (the draught) is given as it is.
func TestAQuestTakesTheAscendedItemAndRewardsAscended(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.own(f.talisman, 1)
	f.own(f.ascended, 1)
	m := f.take(f.stark, "Will you make the breastplate?")
	f.want(m, "box:Stark: There.", "box:"+questFinished, "box:You have given away Ascended Ring of the Test.",
		"box:You have received Ascended Breastplate of the Test.", "box:You have received draught of the test.")
	plate := f.ascendedOf(f.plate)
	if plate == 0 {
		t.Fatal("no Ascended breastplate made")
	}
	if f.held(plate) != 1 || f.held(f.plate) != 0 || f.held(f.draught) != 1 || f.held(f.ascended) != 0 || f.held(f.talisman) != 0 {
		t.Errorf("held: Ascended plate %d, plate %d, draught %d, Ascended ring %d, talisman %d",
			f.held(plate), f.held(f.plate), f.held(f.draught), f.held(f.ascended), f.held(f.talisman))
	}
	if b, ok := entity.AscendedBase(plate); !ok || b != f.plate {
		t.Errorf("the World does not know the new record as Ascended: %d %v", b, ok)
	}
	if f.ascendedOf(f.draught) != 0 {
		t.Error("the draught, no gear, was made Ascended")
	}
}

// The control: the regular ring gives the regular breastplate, and no
// Ascended record is made.
func TestTheRegularItemGivesTheRegularReward(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.own(f.talisman, 1)
	f.own(f.ring, 1)
	m := f.take(f.stark, "Will you make the breastplate?")
	f.want(m, "box:You have given away Ring of the Test.", "box:You have received Breastplate of the Test.")
	if f.held(f.plate) != 1 || f.held(f.ring) != 0 || f.ascendedOf(f.plate) != 0 {
		t.Errorf("held: plate %d, ring %d; Ascended plate %d", f.held(f.plate), f.held(f.ring), f.ascendedOf(f.plate))
	}
}

// Holding both, the Ascended one is handed in (the better reward) and the
// regular ring kept; only enough Ascended records for every unit make the
// step Ascended: a ring and an Ascended ring for two rings is the regular
// reward, both taken.
func TestTheAscendedItemGoesFirstAndOnlyWhenItMeetsTheStep(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.own(f.talisman, 1)
	f.own(f.ring, 1)
	f.own(f.ascended, 1)
	f.take(f.stark, "Will you make the breastplate?")
	if f.held(f.ring) != 1 || f.held(f.ascended) != 0 || f.held(f.ascendedOf(f.plate)) != 1 || f.held(f.plate) != 0 {
		t.Fatalf("held: ring %d, Ascended ring %d, Ascended plate %d, plate %d",
			f.held(f.ring), f.held(f.ascended), f.held(f.ascendedOf(f.plate)), f.held(f.plate))
	}
	f.own(f.ascended, 1) // a ring and an Ascended ring
	m := f.take(f.verna, "Will you make the second breastplate?")
	f.want(m, "box:You have given away Ring of the Test.", "box:You have given away Ascended Ring of the Test.",
		"box:You have received Breastplate of the Test.")
	if f.held(f.ring) != 0 || f.held(f.ascended) != 0 || f.held(f.plate) != 1 {
		t.Errorf("held: ring %d, Ascended ring %d, plate %d", f.held(f.ring), f.held(f.ascended), f.held(f.plate))
	}
}

// A step that lacks counts the Ascended record for its item: one Ascended
// ring is not two rings, and two are.
func TestAStepLacksUntilTheAscendedRecordsMeetIt(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.own(f.ascended, 1)
	m := f.take(f.verna, "Will you make the second breastplate?")
	f.want(m, "box:Verna: Not all of it yet.")
	if f.held(f.ascended) != 1 {
		t.Fatal("a ring was taken for a step that lacks")
	}
	f.own(f.ascended, 1)
	f.msgs()
	f.tw.talk(f.verna)
	f.want(f.msgs(), "box:Verna: There.", "box:You have given away 2 Ascended Ring of the Test.",
		"box:You have received Ascended Breastplate of the Test.")
}

// A show step keeps what is shown: an Ascended ring shown is kept and the
// rewards are Ascended.
func TestAShownAscendedItemIsKeptAndRewardsAscended(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.own(f.ascended, 1)
	m := f.take(f.toi, "Will you make the shown breastplate?")
	f.want(m, "box:Toi: There.", "box:You have received Ascended Breastplate of the Test.")
	if f.held(f.ascended) != 1 || f.held(f.ascendedOf(f.plate)) != 1 {
		t.Errorf("held: Ascended ring %d, Ascended plate %d", f.held(f.ascended), f.held(f.ascendedOf(f.plate)))
	}
}

// A loot step counts an Ascended record taken off a corpse for its item; the
// control: another item counts nothing. The tracker counts it too.
func TestALootStepAndTheTrackerCountTheAscendedRecord(t *testing.T) {
	f := newAscendedQuestFixture(t)
	f.take(f.kell, "Will you make the looted breastplate?")
	if step, _ := f.progress(ascendedLoot); step != 1 {
		t.Fatalf("the loot quest is at step %d", step)
	}
	f.qe.Looted(f.tw.s, f.tw.p, f.talisman, 1)
	if step, _ := f.progress(ascendedLoot); step != 1 {
		t.Fatalf("a talisman looted moved the step: %d", step)
	}
	f.qe.Looted(f.tw.s, f.tw.p, f.ascended, 1)
	if step, done := f.progress(ascendedLoot); step != 0 || done != 1 {
		t.Errorf("the Ascended ring looted: step %d, done %d; want it finished", step, done)
	}
	f.own(f.ascended, 1)
	if l := sessionInvList(f.tw.s); l.heldStandingFor(f.ring) != 1 || l.held(f.ring) != 0 || l.heldStandingFor(f.talisman) != 0 {
		t.Errorf("the tracker's count: %d (held %d)", l.heldStandingFor(f.ring), l.held(f.ring))
	}
}
