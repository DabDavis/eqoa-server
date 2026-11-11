package entity

import (
	"strings"
	"testing"
	"time"
)

// The GM Jibrish's petition 1522: Mr. Moogie "If he reaches 2% hp he completely heals to full and doubles his
// damage if a non player character is hitting him". Over the template rig's creatures: the guard stands at
// 2% or under once an NPC (the brute) is on its hate list, alive, and not before (players alone: the control);
// whole again, its line in /say, its melee and its kick twice what they were, for the rest of that fight; once
// a fight (at 2% again in it, nothing); a fight's end takes it, and the next fight may stand again. Above 2%,
// or with the NPC dead, nothing.
func TestALastStandAgainstANonPlayer(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("standing king", ", class, melee_pct, kick_damage, kick_every, kick_first", ", 0, 300, 1000, 2, 0",
		`[{"kind":"laststand","below":2,"damage_pct":200,"say":"You brought help?"}]`, "gnoll guard")
	guard, brute := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
	r.place(brute, guard.X+3, guard.Z)
	pool(guard, 100_000)
	pool(brute, 10_000_000)
	main, sess := r.player("Main", guard.X+1, guard.Z)
	main.HP, main.MaxHP = 10_000_000, 10_000_000
	fight := func(ids ...EntityID) {
		r.w.mu.Lock()
		guard.Threats = map[EntityID]int64{}
		for i, id := range ids {
			guard.Threats[id] = int64(1000 - i)
		}
		guard.CurrentEnemy = ids[0]
		r.w.wakeNPCLocked(guard)
		r.w.mu.Unlock()
	}
	if got := guard.meleeOut(100); got != 300 {
		t.Fatalf("its melee before: %d, want 300 (melee_pct 300)", got)
	}

	// players alone, at 1%: nothing
	fight(main.ServerID)
	guard.HP = guard.MaxHP / 100
	r.run(0, 2*time.Second)
	if guard.HP > guard.MaxHP*2/100 || (guard.tfight != nil && guard.tfight.lastStand != nil) {
		t.Fatalf("players alone at 1%%: HP %d of %d, want no last stand", guard.HP, guard.MaxHP)
	}
	// an NPC hitting it, at 3%: nothing
	fight(main.ServerID, brute.ServerID)
	guard.HP = guard.MaxHP * 3 / 100
	r.run(2100*time.Millisecond, 3*time.Second)
	if guard.HP > guard.MaxHP*4/100 || guard.tfight.lastStand != nil {
		t.Fatalf("an NPC hitting it at 3%%: HP %d of %d, want no last stand", guard.HP, guard.MaxHP)
	}
	// the NPC dead, at 1%: nothing
	brute.Dead = true
	guard.HP = guard.MaxHP / 100
	r.godTick(3100 * time.Millisecond)
	if guard.tfight.lastStand != nil || guard.HP > guard.MaxHP*2/100 {
		t.Fatalf("a dead NPC on its list: HP %d, want no last stand", guard.HP)
	}
	brute.Dead = false
	drain(sess)

	// the NPC alive and on its list, at 2% (no lower): it stands
	guard.HP = guard.MaxHP * 2 / 100
	r.godTick(3200 * time.Millisecond)
	if guard.tfight.lastStand == nil || guard.HP < guard.MaxHP*99/100 {
		t.Fatalf("at 2%% with the brute on its list: HP %d of %d, want healed whole", guard.HP, guard.MaxHP)
	}
	said := false
	for _, l := range linesOf(drain(sess)) {
		said = said || strings.Contains(l, "You brought help?")
	}
	if !said {
		t.Error("its line was not heard")
	}
	if got := guard.meleeOut(100); got != 600 {
		t.Errorf("its melee after: %d, want 600 (doubled)", got)
	}
	if got := guard.stoodOut(guard.shared(1000)); got != 2000 {
		t.Errorf("its kick after: %d, want 2000", got)
	}
	// once a fight: at 1% again, nothing
	guard.HP = guard.MaxHP / 100
	r.run(3300*time.Millisecond, 4*time.Second)
	if guard.HP > guard.MaxHP*3/100 {
		t.Errorf("a second time in one fight: HP %d of %d, want none", guard.HP, guard.MaxHP)
	}
	// its fight over: its damage its own; the next fight stands again
	r.endFight(guard, 10*time.Second)
	if guard.meleeOut(100) != 300 {
		t.Errorf("after its fight: melee %d, want its own 300", guard.meleeOut(100))
	}
	fight(brute.ServerID)
	guard.HP = guard.MaxHP / 100
	r.godTick(11 * time.Second)
	if guard.HP < guard.MaxHP*99/100 {
		t.Errorf("the next fight at 1%% with the brute: HP %d of %d, want healed whole", guard.HP, guard.MaxHP)
	}
}
