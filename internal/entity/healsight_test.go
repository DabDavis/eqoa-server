package entity

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The owner, 2026-11-18: "also healing shouldnt fire if there is no Line of
// sight" (sight.go). Each heal on another is held to its healer's sight; a
// heal on oneself is not.

// groundLocked sets w's ground while the test holds its lock (SetGround takes it).
func groundLocked(w *World, g Ground) {
	w.mu.Unlock()
	defer w.mu.Lock()
	w.SetGround(g)
}

// pillarAt is a solid column round (x, z), floor to sky: every line to one
// standing in it meets it.
func pillarAt(x, z float32) sightBox {
	return sightBox{x - 0.75, -90000, z - 0.75, x + 0.75, 90000, z + 0.75}
}

// A heal on oneself behind a wall lands whatever stands round its caster; the
// same heal on a friend behind the wall is refused before it begins, no power
// spent, "No line of sight." in red, and on the friend in sight lands.
func TestAHealOnOneselfNeedsNoSight(t *testing.T) {
	c := newCaster(t) // at (100, 10, 100)
	c.w.SetGround(sightGround{pillarAt(110, 100), wallAt(105)})
	behind := &Player{ServerID: 1002, Name: "Tank", HP: 100, MaxHP: 900, X: 110, Y: 10, Z: 100}
	c.w.Players[behind.ServerID] = behind
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = behind.ServerID, 0x1424A1D2
	heal := CastSpell{SpellID: 9101, Name: "Minor Healing", Scope: 1, Range: 30, CastTime: time.Second, PowerCost: 10,
		Def: ParseSpellNamed("Minor Healing", "Target, Increase Hitpoints by 30", "")}

	power := c.p.Power
	if c.w.StartCast(c.sess, heal, 0x1424A1D2, c.at(0)) {
		t.Fatal("a heal on a friend behind a wall began")
	}
	if lines := redLines(c.sess); c.p.Power != power || !slices.Contains(lines, castNotSeen) || behind.HP != 100 {
		t.Errorf("behind a wall: power %d (was %d), lines %q, HP %d", c.p.Power, power, lines, behind.HP)
	}
	c.sent()

	c.p.HP = 400
	c.land(t, heal, c.me, time.Minute)
	if c.p.HP != 430 {
		t.Errorf("a heal on itself, walled in: HP %d, want 430", c.p.HP)
	}

	c.w.SetGround(nil) // the control: nothing in the way
	c.land(t, heal, 0x1424A1D2, 2*time.Minute)
	if behind.HP != 130 {
		t.Errorf("the control, in sight: HP %d, want 130", behind.HP)
	}
}

// A pet heals its owner only in its sight (petCastLocked, petWantsLocked): its
// owner behind a pillar from it is not wanted (it heals itself, else waits); one
// that steps behind the pillar as it casts is spared as it lands. The control:
// in sight, the same heal lands on its owner.
func TestAPetHealsItsOwnerOnlyInSight(t *testing.T) {
	m := newMemberRig(t, false) // its Mending an either heal, its owner at 50%
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	groundLocked(m.w, sightGround{pillarAt(m.p.X, m.p.Z)})
	if m.pet.X == m.p.X && m.pet.Z == m.p.Z {
		t.Fatal("the pet stands in its owner's place")
	}
	now := m.at(60 * time.Second)
	if m.w.petWantsLocked(m.ps, m.mending, now) {
		t.Fatal("the Mending was wanted for an owner out of the pet's sight, the pet whole")
	}
	m.pet.HP = m.pet.MaxHP / 2
	if !m.w.petCastLocked(m.ps, m.mendingIdx, nil, now) {
		t.Fatal("the Mending was not cast on the pet itself")
	}
	m.w.petCastSystemLocked(now.Add(10 * time.Second))
	if m.p.HP != 2000 || m.pet.HP <= m.pet.MaxHP/2 {
		t.Fatalf("out of sight: its owner %d (want 2000), the pet %d of %d (want healed)", m.p.HP, m.pet.HP, m.pet.MaxHP)
	}
	m.pet.HP = m.pet.MaxHP

	// in sight as it begins, behind the pillar as it lands
	groundLocked(m.w, nil)
	m.mend(t)
	if m.p.HP != 2800 {
		t.Fatalf("the control, in sight: its owner %d, want 2800", m.p.HP)
	}
	m.p.HP = 2000
	later := m.at(120 * time.Second)
	m.ps.npc.fight().busy = time.Time{}
	if !m.w.petWantsLocked(m.ps, m.mending, later) || !m.w.petCastLocked(m.ps, m.mendingIdx, nil, later) {
		t.Fatal("the second Mending was not cast")
	}
	groundLocked(m.w, sightGround{pillarAt(m.p.X, m.p.Z)})
	m.w.petCastSystemLocked(later.Add(10 * time.Second))
	if m.p.HP != 2000 {
		t.Errorf("its owner behind the pillar as it landed: %d, want 2000", m.p.HP)
	}
}

// A water pet's member heal goes to the worst off it sees: Raider (25%) behind a
// pillar is passed over, for none while no one else in reach and sight is hurt,
// then for A at 50%. The control: in sight, Raider.
func TestAPetsMemberHealPassesOverOneOutOfSight(t *testing.T) {
	m := newMemberRig(t, true)
	m.p.HP = 3000 // its owner at 75%: above the threshold
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.raider.ServerID {
		t.Fatalf("the control: picked %d, want Raider %d", got, m.raider.ServerID)
	}
	groundLocked(m.w, sightGround{pillarAt(m.raider.X, m.raider.Z)})
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != 0 {
		t.Fatalf("Raider behind a pillar: picked %d, want none", got)
	}
	m.fullA.HP = 2000 // 50%, in sight
	if got := m.w.petMemberPickLocked(m.ps, m.mending); got != m.fullA.ServerID {
		t.Fatalf("Raider behind a pillar, A at 50%%: picked %d, want A %d", got, m.fullA.ServerID)
	}
}

// A pet's group heal leaves out a member behind a pillar from it; the one in
// sight is healed.
func TestAPetsGroupHealLeavesOutOneOutOfSight(t *testing.T) {
	m := newMemberRig(t, true)
	m.w.mu.Lock()
	defer m.w.mu.Unlock()
	m.fullA.HP, m.raider.HP = 1000, 1000
	groundLocked(m.w, sightGround{pillarAt(m.raider.X, m.raider.Z)})
	m.w.petGroupHealLocked(m.ps, &CastSpell{Name: "Tide", Range: 30}, 500)
	if m.fullA.HP != 1500 || m.raider.HP != 1000 {
		t.Errorf("A %d (want 1500), Raider behind a pillar %d (want 1000)", m.fullA.HP, m.raider.HP)
	}
	// a group heal is not wanted for a hurt member it cannot see alone
	m.fullA.HP, m.p.HP = 4000, 4000
	m.pet.HP = m.pet.MaxHP
	a := templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityHeal, Target: database.AbilityOnGroup, Below: 60}}
	if m.w.petGroupHurtLocked(m.ps, &a) {
		t.Error("a group heal wanted for Raider alone, behind a pillar")
	}
	groundLocked(m.w, nil)
	if !m.w.petGroupHurtLocked(m.ps, &a) {
		t.Error("the control: no group heal wanted for Raider in sight")
	}
}

// Healbot heals his owner only in his sight: walled in, his owner at 50% waits;
// out again, he heals.
func TestHealbotHealsOnlyInSight(t *testing.T) {
	r := newMercRig(t)
	fs := r.callFamiliar(0)
	top := r.p.MaxHPNow()
	r.p.HP = top / 2
	r.w.SetGround(sightGround{pillarAt(r.p.X, r.p.Z)})
	if fs.npc.X == r.p.X && fs.npc.Z == r.p.Z {
		t.Fatal("Healbot stands in his owner's place")
	}
	r.run(5*time.Second, 12*time.Second)
	if r.p.HP != top/2 {
		t.Fatalf("Healbot healed his owner behind a pillar: %d -> %d", top/2, r.p.HP)
	}
	r.w.SetGround(nil)
	r.tick(13 * time.Second)
	if want := min(top/2+600, top); r.p.HP != want {
		t.Errorf("the control, in sight: %d, want %d", r.p.HP, want)
	}
}

// A boss's raid heal (bossmoments.go) lands on those of its hate list it sees:
// not on one behind a pillar from it.
func TestABossRaidHealNeedsSight(t *testing.T) {
	f := newAggroFix(t, 60)
	f.p.HP = 500
	other := &Player{ServerID: 1002, CharServerID: 78, Name: "Other", Level: 60, HP: 100, MaxHP: 20000, X: 1005, Y: 10, Z: 1005}
	f.w.Players[other.ServerID] = other
	w := f.npc(3100, "The Watchman", 62, creationBody, 1010, 1000)
	w.HP, w.MaxHP = 300000, 300000
	w.tmpl = &ClassTemplate{moments: []database.TemplateAbility{{Kind: database.AbilityRaidHeal, Every: 30, First: 10, Damage: 10000, Count: 3}}}
	w.CurrentEnemy = f.p.ServerID
	w.Threats = map[EntityID]int64{f.p.ServerID: 10, other.ServerID: 5}
	f.w.SetGround(sightGround{pillarAt(other.X, other.Z)})
	fight := w.fight()
	f.w.bossMomentsLocked(w, fight, f.now)
	f.w.bossMomentsLocked(w, fight, f.now.Add(10*time.Second))
	if other.HP != 100 || f.p.HP != 1000 {
		t.Errorf("other behind a pillar %d (want 100), the player in sight %d (want 1000)", other.HP, f.p.HP)
	}
	f.w.SetGround(nil) // the control: nothing in the way
	f.w.bossMomentsLocked(w, fight, f.now.Add(40*time.Second))
	if other.HP != 10100 {
		t.Errorf("the control, in sight: other %d, want 10100", other.HP)
	}
}

// A weapon's restoring proc heals its wielder and the members it sees: not one
// behind a wall, though in its reach.
func TestARestoringProcHealsOnlyWhomItsWielderSees(t *testing.T) {
	c := newCaster(t)
	c.w.SetGround(sightGround{wallAt(105)})
	behind := &Player{ServerID: 1002, Name: "Tank", HP: 100, MaxHP: 900, X: 110, Y: 10, Z: 100}
	near := &Player{ServerID: 1004, Name: "Near", HP: 100, MaxHP: 900, X: 103, Y: 10, Z: 101}
	c.w.Players[behind.ServerID], c.w.Players[near.ServerID] = behind, near
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, behind.ServerID, near.ServerID}}}
	c.p.GroupID, behind.GroupID, near.GroupID = 7, 7, 7
	c.p.HP = 100
	c.w.mu.Lock()
	c.w.restoreProcLocked(c.sess, c.p, &WeaponProc{Item: "Test Mace", Kind: ProcRestore, Damage: 50}, 50)
	c.w.mu.Unlock()
	if c.p.HP != 150 || near.HP != 150 || behind.HP != 100 {
		t.Errorf("wielder %d (want 150), near %d (want 150), behind a wall %d (want 100)", c.p.HP, near.HP, behind.HP)
	}
}
