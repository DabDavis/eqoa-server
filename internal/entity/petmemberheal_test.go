package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The user, 2026-11-11: "lets allow mage water pets to heal other group/raid
// members also on heirloom server". With world.pet_heal_group on, the Ghilan
// Noble's Mending (an either heal, below 60%) goes to its owner when it is below
// 60% (petitions 1819/1820), else to the worst off of its side; off, to its
// owner or itself as before.

// memberRig is the mage pets' rig with the Ghilan Noble out, its owner at
// 2000/4000, and a raid group of seven: its owner, four at full, Raider (the
// sixth to join, past its owner's window of four) at 1000/4000, and Duelist at
// 400/4000 in a duel; Far, a member 40 off, at 200/4000; a stranger at
// 300/4000 beside it.
type memberRig struct {
	*petRig
	pet                             *NPC
	ps                              *petState
	raider, duelist, far, stranger  *Player
	mending                         *templateAbility
	fullA, fullB, fullC, fullD      *Player
	raiderStart, duelStart, farHP   int
	strangerHP, ownerHP, mendingIdx int
}

func newMemberRig(t *testing.T, on bool) *memberRig {
	t.Helper()
	r, _ := magePetRig(t, false)
	pet, _ := r.summon(t, summonGhilanNoble, 0)
	ps := r.w.pets[r.p.ServerID]
	ab := ps.petAbilities()
	if len(ab) < 2 || ab[1].pinned == nil || ab[1].pinned.SpellID != 905456 || ab[1].TargetOr() != database.AbilityOnEither {
		t.Fatalf("the Ghilan Noble's second ability: %+v; want Ghilan's Mending, either", ab[1].TemplateAbility)
	}
	r.w.SetPetHealGroup(on)
	player := func(id EntityID, name string, dx float32, hp int) *Player {
		p := &Player{ServerID: id, Name: name, Class: 0, Level: 60, HP: hp, MaxHP: 4000, X: r.p.X + dx, Y: r.p.Y, Z: r.p.Z,
			World: r.p.World}
		r.w.Players[id] = p
		r.w.SpatialIndex.Insert(id, p.X, p.Z)
		return p
	}
	m := &memberRig{petRig: r, pet: pet, ps: ps, mending: &ab[1], mendingIdx: 1}
	m.fullA, m.fullB, m.fullC, m.fullD = player(2001, "A", 3, 4000), player(2002, "B", 4, 4000), player(2003, "C", 5, 4000),
		player(2004, "D", 6, 4000)
	m.raider = player(2005, "Raider", 7, 1000)
	m.duelist = player(2006, "Duelist", 5, 400)
	m.far = player(2007, "Far", 40, 200)
	m.stranger = player(2008, "Stranger", 5, 300)
	rival := player(2009, "Rival", 6, 4000)
	g := &Group{ID: 9, Members: []EntityID{r.p.ServerID, m.fullA.ServerID, m.fullB.ServerID, m.fullC.ServerID, m.fullD.ServerID,
		m.raider.ServerID, m.duelist.ServerID, m.far.ServerID}}
	if r.w.groups == nil {
		r.w.groups = map[uint32]*Group{}
	}
	r.w.groups[g.ID] = g
	for _, id := range g.Members {
		r.w.Players[id].GroupID = g.ID
	}
	if !g.isRaid() {
		t.Fatal("not a raid group")
	}
	r.w.duels.add(&Duel{ID: 3, A: m.duelist.ServerID, B: rival.ServerID, who: [2]*Player{m.duelist, rival}, Phase: duelOn})
	r.p.MaxHP, r.p.HP = 4000, 2000
	pet.HP = pet.MaxHP
	return m
}

// mend casts the Mending now and lands it.
func (m *memberRig) mend(t *testing.T) {
	t.Helper()
	now := m.at(60 * time.Second)
	if !m.w.petWantsLocked(m.ps, m.mending, now) {
		t.Fatal("the Mending is not wanted")
	}
	if !m.w.petCastLocked(m.ps, m.mendingIdx, nil, now) {
		t.Fatal("the Mending was not cast")
	}
	m.w.petCastSystemLocked(now.Add(10 * time.Second))
}

// On, its owner above the threshold (75%): the Mending lands on Raider, a raid
// group's member past its owner's window at 25%; not on the duelist (10%, in a
// fight of its own), the member 40 off (5%) nor the stranger (7.5%).
func TestWaterPetHealsTheWorstOffOfItsRaid(t *testing.T) {
	m := newMemberRig(t, true)
	m.p.HP = 3000
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petHealTargetLocked(m.ps, m.mending); got != database.AbilityOnMember {
		t.Fatalf("its Mending's target with the switch on: %q, want member", got)
	}
	m.mend(t)
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"Raider", m.raider.HP, 1800}, {"its owner", m.p.HP, 3000}, {"the duelist", m.duelist.HP, 400},
		{"the far member", m.far.HP, 200}, {"the stranger", m.stranger.HP, 300}, {"itself", m.pet.HP, m.pet.MaxHP},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d hit points after the Mending, want %d", c.name, c.got, c.want)
		}
	}
}

// On: its owner comes first (petitions 1819/1820, the GM Jibrish: "They should
// always prioritize the mage for heals but otherwise use them on other damaged
// group or raid members"): below 60% it is healed though Raider (25%) and the
// rest are worse off; above it the rest are healed.
func TestWaterPetHealsItsOwnerFirst(t *testing.T) {
	m := newMemberRig(t, true)
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.p.ServerID {
		t.Fatalf("its owner at 50%%, Raider at 25%%: picked %d, want its owner %d", got, m.p.ServerID)
	}
	m.mend(t)
	if m.p.HP != 2800 || m.raider.HP != 1000 {
		t.Fatalf("its owner %d, Raider %d after the Mending; want 2800 and 1000", m.p.HP, m.raider.HP)
	}
	m.p.HP = 3000 // 75%: the worst off of the rest
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.raider.ServerID {
		t.Fatalf("its owner at 75%%, Raider at 25%%: picked %d, want Raider %d", got, m.raider.ServerID)
	}
	m.p.HP = 2399 // just under 60%
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.p.ServerID {
		t.Fatalf("its owner just under 60%%: picked %d, want its owner", got)
	}
}

// On: a group mate is healed in a group of two too when its owner is above the
// threshold, and with no one but those it may not heal hurt below 60% it is not
// wanted.
func TestWaterPetHealsAGroupMateOverItsOwner(t *testing.T) {
	m := newMemberRig(t, true)
	g := m.w.groups[9]
	g.Members = []EntityID{m.p.ServerID, m.fullA.ServerID}
	m.fullA.HP = 1200
	m.p.HP = 2600
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	m.mend(t)
	if m.fullA.HP != 2000 || m.p.HP != 2600 {
		t.Fatalf("the mate %d, its owner %d after the Mending; want 2000 and 2600", m.fullA.HP, m.p.HP)
	}
	// its owner and the mate above 60%: the duelist, the far one and the stranger are not its to heal
	m.p.HP, m.fullA.HP = 3000, 3000
	g.Members = append(g.Members, m.duelist.ServerID, m.far.ServerID)
	if m.w.petWantsLocked(m.ps, m.mending, m.at(90*time.Second)) {
		t.Fatal("the Mending is wanted with no one of its side in reach below 60%")
	}
	// its owner is its own, wherever it stands
	m.p.HP = 1000
	m.p.X += 100
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.p.ServerID {
		t.Fatalf("its owner 100 off at 25%%: picked %d, want its owner", got)
	}
}

// Off: today's either heal: its owner (50%) over itself (full), the raid's
// worse off left alone.
func TestWaterPetHealWithTheSwitchOffIsAsBefore(t *testing.T) {
	m := newMemberRig(t, false)
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petHealTargetLocked(m.ps, m.mending); got != database.AbilityOnEither {
		t.Fatalf("its Mending's target with the switch off: %q, want either", got)
	}
	m.mend(t)
	if m.p.HP != 2800 || m.raider.HP != 1000 {
		t.Fatalf("its owner %d, Raider %d after the Mending; want 2800 and 1000", m.p.HP, m.raider.HP)
	}
}

// Only a water pet: the switch leaves the other pets' heals as they were.
func TestPetHealGroupIsTheWaterPetsAlone(t *testing.T) {
	m := newMemberRig(t, true)
	other := &petState{owner: m.p, npc: &NPC{ServerID: 3999, Name: "Earth"}, tmpl: &petTemplate{tmpl: &ClassTemplate{
		ClassTemplate: database.ClassTemplate{Name: "pet earth"}}}}
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petHealTargetLocked(other, m.mending); got != database.AbilityOnEither {
		t.Fatalf("an earth pet's either heal with the switch on: %q", got)
	}
	tmplOnly := &petState{owner: m.p, npc: &NPC{ServerID: 3998}, tmpl: &petTemplate{tmpl: &ClassTemplate{
		ClassTemplate: database.ClassTemplate{Name: "pet water"}}}}
	if !waterPet(tmplOnly) || !waterPet(m.ps) {
		t.Fatal("a pet of template pet water or of a water petstats row is not a water pet")
	}
}

// A merc's water pet puts its merc (the mage) first, in reach and below the
// threshold, over its hirer and the raid's worse off; above it the worst off.
func TestMercWaterPetHealsItsMercFirst(t *testing.T) {
	m := newMemberRig(t, true)
	merc := &NPC{ServerID: 3990, Name: "Mage", World: m.pet.World, X: m.pet.X + 2, Y: m.pet.Y, Z: m.pet.Z, HP: 2500, MaxHP: 5000}
	ms := &mercState{owner: m.p, slot: 1, npc: merc}
	if m.w.mercs == nil {
		m.w.mercs = map[EntityID][]*mercState{}
	}
	m.w.mercs[m.p.ServerID] = append(m.w.mercs[m.p.ServerID], ms)
	m.ps.merc = ms
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != merc.ServerID {
		t.Fatalf("its merc at 50%%, Raider at 25%%: picked %d, want its merc %d", got, merc.ServerID)
	}
	merc.HP = 4000
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.raider.ServerID {
		t.Fatalf("its merc at 80%%: picked %d, want Raider %d", got, m.raider.ServerID)
	}
	merc.HP, merc.X = 2500, m.pet.X+500 // out of reach: the worst off of the rest
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.raider.ServerID {
		t.Fatalf("its merc out of reach: picked %d, want Raider %d", got, m.raider.ServerID)
	}
}
