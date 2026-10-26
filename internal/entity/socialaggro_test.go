package entity

import (
	"testing"
)

// The social pull (socialaggro.go): a hit on an NPC calls the social NPCs of
// its faction within their radius into the fight against the attacker; no one
// else, and not a creature's own notice of a player.

// socialFix is aggroFix with Freeport's factions: guards of The Freeport
// Militia (social, by the seed's rows) and townsfolk of Citizens of Freeport,
// as the town seed's factionhits make them.
func newSocialFix(t *testing.T) *aggroFix {
	t.Helper()
	f := newAggroFix(t, 40)
	f.w.SetFactionHits(nil)
	f.w.factions.hits = map[string][]FactionHit{
		"Guard Alinon":  {{"Citizens of Freeport", -25}, {"The Freeport Militia", -50}},
		"Guard Brenna":  {{"Citizens of Freeport", -25}, {"The Freeport Militia", -50}},
		"Guard Cadwell": {{"Citizens of Freeport", -25}, {"The Freeport Militia", -50}},
		"Baker Dunn":    {{"Citizens of Freeport", -25}},
		"Guard Ely":     {{"Citizens of Highpass", -25}, {"The Highpass Guard", -50}},
	}
	f.w.SetSocialRules(map[string]SocialRule{
		"Guard Alinon":  {Social: true, Faction: "The Freeport Militia"},
		"Guard Brenna":  {Social: true, Faction: "The Freeport Militia"},
		"Guard Cadwell": {Social: true, Faction: "The Freeport Militia"},
		"Guard Ely":     {Social: true, Faction: "The Highpass Guard"},
	})
	return f
}

// hit is a player's hit on n through the threat's entry every hit takes.
func (f *aggroFix) hit(n *NPC, damage int) {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.w.updateAggro(n, f.p.ServerID, damage)
}

// A player hits a guard: the guards of its militia within 30 of it come at the
// player; one 35 off, a guard of another town's guard, a townsperson and a
// guard with no row do not.
func TestAGuardHitCallsTheGuardsOfItsFactionNearIt(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	near := f.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	far := f.npc(102, "Guard Cadwell", 30, humanBody, 1035, 1000)
	other := f.npc(103, "Guard Ely", 30, humanBody, 1000, 1010)
	baker := f.npc(104, "Baker Dunn", 10, humanBody, 1000, 990)
	rowless := f.npc(105, "Guard Fenwick", 30, humanBody, 990, 1000)

	f.hit(victim, 12)

	if !f.attacks(victim) {
		t.Fatalf("the guard hit fights the player: enemy %d", victim.CurrentEnemy)
	}
	if !f.attacks(near) {
		t.Errorf("a militia guard 20 off came: enemy %d, want the player", near.CurrentEnemy)
	}
	for _, n := range []*NPC{far, other, baker, rowless} {
		if n.CurrentEnemy != 0 {
			t.Errorf("%s came (enemy %d); it should not", n.Name, n.CurrentEnemy)
		}
	}
}

// A townsperson hit calls no guard: the militia's guards help the militia,
// and the baker is of the citizens only.
func TestATownspersonHitCallsNoGuard(t *testing.T) {
	f := newSocialFix(t)
	baker := f.npc(104, "Baker Dunn", 10, humanBody, 1000, 1000)
	guard := f.npc(101, "Guard Brenna", 30, humanBody, 1010, 1000)
	f.hit(baker, 5)
	if guard.CurrentEnemy != 0 {
		t.Errorf("a militia guard came to a citizen's help (enemy %d)", guard.CurrentEnemy)
	}
}

// An attacked NPC with no row of its own is of every faction its kill lowers:
// a guard the seed left out still has the militia's help.
func TestAnNPCWithNoRowIsOfTheFactionsItsKillLowers(t *testing.T) {
	f := newSocialFix(t)
	f.w.factions.hits["Guard Gorm"] = []FactionHit{{"Citizens of Freeport", -25}, {"The Freeport Militia", -50}}
	victim := f.npc(106, "Guard Gorm", 30, humanBody, 1000, 1000)
	helper := f.npc(101, "Guard Brenna", 30, humanBody, 1015, 1000)
	f.hit(victim, 5)
	if !f.attacks(helper) {
		t.Errorf("the militia guard did not come to a rowless militia guard's help (enemy %d)", helper.CurrentEnemy)
	}
}

// A creature noticing a player on its own (aggro.go) calls no one: jib's
// "the other won't assist unless they see you hit them". The friend stands 25
// off the creature and 50 off the player: out of its own notice, in the call's
// radius.
func TestACreaturesOwnNoticeCallsNoOne(t *testing.T) {
	f := newSocialFix(t)
	f.w.SetSocialRules(map[string]SocialRule{"Gnoll Pup": {Social: true, Faction: "Blackburrow Gnolls"}})
	a := f.npc(200, "Gnoll Pup", 20, bearBody, 1025, 1000)
	b := f.npc(201, "Gnoll Pup", 20, bearBody, 1050, 1000)
	c := f.npc(202, "Gnoll Pup", 20, bearBody, 1070, 1000)
	f.look()
	if !f.attacks(a) {
		t.Fatalf("the near gnoll noticed the player: enemy %d", a.CurrentEnemy)
	}
	if b.CurrentEnemy != 0 {
		t.Errorf("a gnoll 25 off the one that noticed came at the notice (enemy %d); only a hit calls", b.CurrentEnemy)
	}
	// the player hits it: its friend 25 off comes, one 45 off does not
	f.hit(a, 3)
	if !f.attacks(b) {
		t.Errorf("a gnoll 25 off the one hit came: enemy %d", b.CurrentEnemy)
	}
	if c.CurrentEnemy != 0 {
		t.Errorf("a gnoll 45 off the one hit came (enemy %d)", c.CurrentEnemy)
	}
}

// One busy, on its way home, with the NPC bit, dead, a pet or a dummy does not
// come; none comes for a pet hit.
func TestHelpersThatDoNotCome(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	busy := f.npc(101, "Guard Brenna", 30, humanBody, 1005, 1000)
	busy.CurrentEnemy = 4242
	busy.Threats = map[EntityID]int64{4242: 9}
	going := f.npc(102, "Guard Cadwell", 30, humanBody, 1010, 1000)
	going.chase.returning = true
	f.w.SetSocialRules(map[string]SocialRule{
		"Guard Alinon":  {Social: true, Faction: "The Freeport Militia"},
		"Guard Brenna":  {Social: true, Faction: "The Freeport Militia"},
		"Guard Cadwell": {Social: true, Faction: "The Freeport Militia"},
		"Guard Hale":    {Social: true, Faction: "The Freeport Militia"},
		"Guard Ivo":     {Social: true, Faction: "The Freeport Militia"},
		"Guard Jory":    {Social: true, Faction: "The Freeport Militia"},
	})
	friendly := f.npc(103, "Guard Hale", 30, humanBody, 1000, 1005)
	friendly.NPCType = 0x80
	dead := f.npc(104, "Guard Ivo", 30, humanBody, 995, 1000)
	dead.Dead = true
	pet := f.npc(105, "Guard Jory", 30, humanBody, 1000, 995)
	pet.Owner = 4343

	f.hit(victim, 7)

	if _, hates := busy.Threats[f.p.ServerID]; busy.CurrentEnemy != 4242 || hates {
		t.Errorf("a guard fighting someone else was called (enemy %d, threats %v)", busy.CurrentEnemy, busy.Threats)
	}
	for _, n := range []*NPC{going, friendly, dead, pet} {
		if n.CurrentEnemy != 0 {
			t.Errorf("%s came (enemy %d)", n.Name, n.CurrentEnemy)
		}
	}

	// a pet hit: no one comes to it
	f.w.SetSocialRules(map[string]SocialRule{"Guard Brenna": {Social: true, Faction: "The Freeport Militia"}})
	busy.CurrentEnemy, busy.Threats = 0, nil
	victim.Owner = f.p.ServerID
	f.hit(victim, 3)
	if busy.CurrentEnemy != 0 {
		t.Errorf("a guard came to a pet's help (enemy %d)", busy.CurrentEnemy)
	}
}

// A pet's hit sends the helpers at its owner (they do not run after pets).
func TestAPetsHitSendsTheHelpersAtItsOwner(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	helper := f.npc(101, "Guard Brenna", 30, humanBody, 1025, 1000)
	pet := f.npc(300, "Iron Golem", 30, bearBody, 1002, 1000)
	pet.Owner = f.p.ServerID
	f.w.mu.Lock()
	f.w.updateAggro(victim, pet.ServerID, 20)
	f.w.mu.Unlock()
	if victim.CurrentEnemy != pet.ServerID {
		t.Fatalf("the guard hit fights the pet: enemy %d", victim.CurrentEnemy)
	}
	if !f.attacks(helper) {
		t.Errorf("the helper went for %d, want the pet's owner %d", helper.CurrentEnemy, f.p.ServerID)
	}
}

// The helpers' joining calls no one else: a guard 25 off the helper but 45 off
// the guard hit stays, until the player hits the helper.
func TestJoiningCallsNoOneElse(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	helper := f.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	third := f.npc(102, "Guard Cadwell", 30, humanBody, 1045, 1000)
	f.hit(victim, 4)
	if !f.attacks(helper) {
		t.Fatalf("the near guard came: enemy %d", helper.CurrentEnemy)
	}
	if third.CurrentEnemy != 0 {
		t.Errorf("a guard 45 off the guard hit came through the helper (enemy %d)", third.CurrentEnemy)
	}
	f.hit(helper, 4)
	if !f.attacks(third) {
		t.Errorf("a hit on the helper called the guard 25 off it: enemy %d", third.CurrentEnemy)
	}
}

// Every hit calls again: a guard that walks up after the first hit comes at
// the next one it is near.
func TestEveryHitCallsAgain(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	late := f.npc(101, "Guard Brenna", 30, humanBody, 1060, 1000)
	f.hit(victim, 4)
	if late.CurrentEnemy != 0 {
		t.Fatalf("a guard 60 off came (enemy %d)", late.CurrentEnemy)
	}
	f.w.mu.Lock()
	f.w.SpatialIndex.Update(late.ServerID, late.X, late.Z, 1028, 1000)
	late.X = 1028
	f.w.mu.Unlock()
	f.hit(victim, 4)
	if !f.attacks(late) {
		t.Errorf("the guard that walked up came at the next hit: enemy %d", late.CurrentEnemy)
	}
}

// A row's radius widens the call; with no rules at all nothing comes.
func TestARowsRadiusAndNoRules(t *testing.T) {
	f := newSocialFix(t)
	f.w.SetSocialRules(map[string]SocialRule{
		"Guard Brenna": {Social: true, Faction: "The Freeport Militia", Radius: 60},
	})
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	wide := f.npc(101, "Guard Brenna", 30, humanBody, 1055, 1000)
	f.hit(victim, 4)
	if !f.attacks(wide) {
		t.Errorf("a guard 55 off with a radius of 60 came: enemy %d", wide.CurrentEnemy)
	}

	// each its own radius: the widest row (60) sets how far the call looks,
	// and a guard of radius 10 twenty off stays
	h := newSocialFix(t)
	h.w.SetSocialRules(map[string]SocialRule{
		"Guard Brenna":  {Social: true, Faction: "The Freeport Militia", Radius: 10},
		"Guard Cadwell": {Social: true, Faction: "The Freeport Militia", Radius: 60},
	})
	hv := h.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	short := h.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	long := h.npc(102, "Guard Cadwell", 30, humanBody, 1000, 1050)
	h.hit(hv, 4)
	if short.CurrentEnemy != 0 {
		t.Errorf("a guard of radius 10 came from 20 off (enemy %d)", short.CurrentEnemy)
	}
	if long.CurrentEnemy != h.p.ServerID {
		t.Errorf("a guard of radius 60 did not come from 50 off (enemy %d)", long.CurrentEnemy)
	}

	g := newSocialFix(t)
	g.w.SetSocialRules(nil)
	v := g.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	n := g.npc(101, "Guard Brenna", 30, humanBody, 1005, 1000)
	g.hit(v, 4)
	if n.CurrentEnemy != 0 {
		t.Errorf("with no rules a guard came (enemy %d)", n.CurrentEnemy)
	}
}

// A melee hit, through the attack's own path, calls them.
func TestAMeleeHitCallsTheHelpers(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1002, 1000)
	victim.HP, victim.MaxHP = 100000, 100000
	helper := f.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	f.p.Swing = SwingFor(0, 40, [21]bool{}, false, 0, 0)
	f.w.mu.Lock()
	f.w.playerHitLocked(f.sess, f.p, victim, 0x1234)
	f.w.mu.Unlock()
	if !f.attacks(helper) {
		t.Errorf("a melee hit on a guard called its friend: enemy %d", helper.CurrentEnemy)
	}
}
