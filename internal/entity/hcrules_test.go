package entity

import (
	"testing"
	"time"
)

// The Hardcore isolation rules (hcrules.go) on the rez rig: Gnolie (a cleric),
// Bran and Liss, Liss's pet, the crab.

// hcKill is the crab's hate list made of ids, then whom its kill is shared with
// (killSharersLocked) and the credit its hearers are told (creditKillLocked).
func hcKill(t *testing.T, r *rezRig, killer *Player, haters ...EntityID) ([]*Player, KillCredit) {
	t.Helper()
	crab := r.w.NPCs[3002]
	var heard KillCredit
	r.w.onKill = []KillFunc{func(c KillCredit) { heard = c }}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	crab.HP, crab.Dead, crab.Threats = crab.MaxHP, false, nil
	forgetHate(crab)
	for _, id := range haters {
		r.w.addThreatLocked(crab, id, 10)
	}
	crab.HP = 0
	crab.Dead, crab.DeathTime = true, r.at(time.Minute)
	sharers := r.w.killSharersLocked(killer, crab)
	r.w.creditKillLocked(nil, killer, crab)
	return sharers, heard
}

func hcNames(ps []*Player) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

// Rule 1: Bran, Hardcore, alone on the crab's list earns its kill; Liss (not
// Hardcore) on it at any time, her pet, or a merc, and he earns nothing: no
// share of its experience or faction, no quest credit, no loot right. Grouped
// with Gnolie, Hardcore too, both earn it together. The control: Liss, not
// Hardcore, helped by anyone earns as ever.
func TestAHardcoreKillHelpedEarnsNothing(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	r.b.Hardcore = true

	sharers, c := hcKill(t, r, r.b, r.b.ServerID)
	if len(sharers) != 1 || sharers[0] != r.b || !c.Hardcore || !c.HCEarned || len(c.Players) != 1 {
		t.Errorf("alone: sharers %v, credit %+v", hcNames(sharers), c)
	}
	sharers, c = hcKill(t, r, r.b, r.b.ServerID, r.c.ServerID)
	if len(sharers) != 0 || !c.Hardcore || c.HCEarned || len(c.Players) != 0 {
		t.Errorf("Liss helped: sharers %v, credit %+v", hcNames(sharers), c)
	}
	sharers, c = hcKill(t, r, r.b, r.b.ServerID, 3001) // Liss's pet
	if len(sharers) != 0 || c.HCEarned {
		t.Errorf("Liss's pet helped: sharers %v, credit %+v", hcNames(sharers), c)
	}
	merc := &NPC{ServerID: 3009, Name: "Hired Blade", merc: &mercState{}}
	r.w.NPCs[merc.ServerID] = merc
	sharers, c = hcKill(t, r, r.b, r.b.ServerID, merc.ServerID)
	if len(sharers) != 0 || c.HCEarned {
		t.Errorf("a merc helped: sharers %v, credit %+v", hcNames(sharers), c)
	}
	// raid rolls: none for a helped Hardcore group
	r.w.mu.Lock()
	if k := r.w.raidKillLocked(r.b, r.w.NPCs[3002], nil); k != nil {
		t.Errorf("a helped Hardcore kill has rolls: %+v", k)
	}
	r.w.mu.Unlock()

	// Gnolie, Hardcore, in Bran's group: both on the list, both earn
	r.a.Hardcore = true
	r.w.mu.Lock()
	g := r.w.formGroupLocked(r.b)
	g.Members = append(g.Members, r.a.ServerID)
	r.a.GroupID = g.ID
	r.w.mu.Unlock()
	sharers, c = hcKill(t, r, r.b, r.b.ServerID, r.a.ServerID)
	if len(sharers) != 2 || !c.HCEarned || len(c.Players) != 2 {
		t.Errorf("the Hardcore group: sharers %v, credit %+v", hcNames(sharers), c)
	}

	// the control: Liss earns with anyone's help
	sharers, c = hcKill(t, r, r.c, r.c.ServerID, r.b.ServerID)
	if len(sharers) != 1 || sharers[0] != r.c || c.Hardcore || len(c.Players) != 1 {
		t.Errorf("the control: sharers %v, credit %+v", hcNames(sharers), c)
	}
}

// The record is the creature's life's: a new life (its respawn) or its fight
// given up starts it anew, and so does a fight begun on it whole with no one
// on its list.
func TestTheHateRecordStartsAnew(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	r.b.Hardcore = true
	crab := r.w.NPCs[3002]
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.addThreatLocked(crab, r.c.ServerID, 10)
	crab.HP = crab.MaxHP / 2
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	if r.w.hcEarnedLocked(crab, r.b) {
		t.Fatal("Liss's hit did not taint the fight")
	}
	r.w.giveUpLocked(crab)
	crab.HP = crab.MaxHP
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	if !r.w.hcEarnedLocked(crab, r.b) {
		t.Error("a fight given up still taints the next")
	}
	crab.Threats = nil
	r.w.addThreatLocked(crab, r.c.ServerID, 10)
	crab.Threats = nil // Liss's hate dropped (she died), the crab back to full
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	if !r.w.hcEarnedLocked(crab, r.b) {
		t.Error("a fight begun whole with no one on the list kept the last one's taint")
	}
}

// Rule 4: help lands within a Hardcore character's own group alone; both told.
func TestHardcoreAidIsRefused(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	r.b.Hardcore = true
	heal := CastSpell{SpellID: 4242, Name: "Minor Healing", Def: SpellDef{Effects: []SpellEffect{{Kind: EffectHP, Amount: 50, AmountMax: 50}}}}
	r.b.HP, r.c.HP = 100, 100
	drain(r.sa)
	drain(r.sb)
	r.w.mu.Lock()
	r.w.landOnPlayerLocked(&castState{spell: heal, caster: r.a.ServerID, targets: []EntityID{r.b.ServerID}}, r.a, r.sa, r.b, r.at(0))
	r.w.mu.Unlock()
	if r.b.HP != 100 {
		t.Errorf("Gnolie's heal landed on Hardcore Bran: HP %d", r.b.HP)
	}
	if !hasLine(linesOf(drain(r.sa)), hcAidCasterWords) || !hasLine(linesOf(drain(r.sb)), "Gnolie tried to aid you: a Hardcore adventurer cannot accept aid.") {
		t.Error("the refusal not told")
	}
	r.w.mu.Lock()
	r.w.landOnPlayerLocked(&castState{spell: heal, caster: r.b.ServerID, targets: []EntityID{r.c.ServerID}}, r.b, r.sb, r.c, r.at(0))
	r.w.mu.Unlock()
	if r.c.HP != 100 {
		t.Errorf("Bran's heal landed on Liss: HP %d", r.c.HP)
	}
	// Hardcore Gnolie in Bran's group heals him; the control: Gnolie heals Liss
	r.a.Hardcore = true
	r.w.mu.Lock()
	g := r.w.formGroupLocked(r.b)
	g.Members = append(g.Members, r.a.ServerID)
	r.a.GroupID = g.ID
	r.w.landOnPlayerLocked(&castState{spell: heal, caster: r.a.ServerID, targets: []EntityID{r.b.ServerID}}, r.a, r.sa, r.b, r.at(0))
	r.w.mu.Unlock()
	if r.b.HP != 150 {
		t.Errorf("a Hardcore group member's heal: HP %d, want 150", r.b.HP)
	}
	r.a.Hardcore = false
	r.a.GroupID, r.b.GroupID = 0, 0
	r.w.mu.Lock()
	r.w.landOnPlayerLocked(&castState{spell: heal, caster: r.a.ServerID, targets: []EntityID{r.c.ServerID}}, r.a, r.sa, r.c, r.at(0))
	r.w.mu.Unlock()
	if r.c.HP != 150 {
		t.Errorf("the control: Liss's HP %d, want 150", r.c.HP)
	}
}

// Rule 3: a group is all Hardcore or all not, both ways; and rule 7: no merc.
func TestHardcoreGroupsAndMercs(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	r.b.Hardcore = true
	drain(r.sa)
	r.w.mu.Lock()
	r.w.groupInviteLocked(r.sa, r.a, r.b)
	_, asked := r.w.invites[r.b.ServerID][r.a.ServerID]
	r.w.mu.Unlock()
	if asked || !hasLine(linesOf(drain(r.sa)), hcGroupWords) {
		t.Error("Gnolie's invitation of Hardcore Bran was not refused")
	}
	r.w.mu.Lock()
	g := r.w.formGroupLocked(r.a)
	refused := r.w.hcGroupRefusedLocked(r.sb, r.b, g)
	r.w.mu.Unlock()
	if !refused {
		t.Error("Hardcore Bran joined Gnolie's group")
	}
	// the control: Gnolie invites Liss
	r.w.mu.Lock()
	r.w.groupInviteLocked(r.sa, r.a, r.c)
	_, asked = r.w.invites[r.c.ServerID][r.a.ServerID]
	r.w.mu.Unlock()
	if !asked {
		t.Error("the control: Liss was not invited")
	}

	if _, why, _ := r.w.HireMercAt(r.sb, 1, MercKit{Name: "Hired Blade"}, 0, r.at(0)); why != hcMercWords {
		t.Errorf("Hardcore Bran hires: %q", why)
	}
	if r.w.RestoreMerc(r.sb, MercKit{Name: "Hired Blade"}, KeptMerc{Slot: 1}, r.at(0)) {
		t.Error("a restart restored Hardcore Bran's merc")
	}
}
