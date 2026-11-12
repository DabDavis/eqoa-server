package entity

import "testing"

// The GM Jibrish's petition 1859 (the owner: "yes most damaage"): a raid brought Queen Qy'tha down and a player outside
// it landed the last blow with its pet. The kill is the raid's, given to its member who made the most damage; the
// control, the last blow's own group making the most, keeps the killer; a creature no player hurt keeps its killer too.
func TestTheKillIsTheMostDamagesGroups(t *testing.T) {
	w := NewWorld()
	mk := func(id EntityID, name string) *Player {
		p := &Player{ServerID: id, Name: name, Level: 60, HP: 100, MaxHP: 100, World: 5, CharServerID: uint32(id)}
		w.Players[id] = p
		return p
	}
	a, b, c, out := mk(1, "Durant"), mk(2, "Thorinoak"), mk(3, "Jibrish"), mk(4, "Violence")
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.formGroupLocked(a)
	for _, p := range []*Player{b, c} {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
	queen := &NPC{ServerID: 9001, Name: "Queen Qy'tha", World: 5, HP: 400056, MaxHP: 400056}
	w.tallyDamageLocked(queen, a, 120000)
	w.tallyDamageLocked(queen, b, 200000) // a pet's and a merc's counted to their owners as these are
	w.tallyDamageLocked(queen, c, 79747)
	w.tallyDamageLocked(queen, out, 309) // the last blow
	if _, got := w.killCreditLocked(queen, nil, out); got != b {
		t.Fatalf("the kill went to %v, want the raid's most damaging member Thorinoak", got)
	}
	// the control: the last blow's group made the most
	if _, got := w.killCreditLocked(queen, nil, c); got != c {
		t.Fatalf("the raid's own last blow: the kill went to %v, want Jibrish", got)
	}
	// alone against alone: the most damage's own
	solo := &NPC{ServerID: 9002, Name: "gnoll", World: 5, HP: 100, MaxHP: 100}
	w.tallyDamageLocked(solo, out, 10)
	if _, got := w.killCreditLocked(solo, nil, a); got != out {
		t.Fatalf("the solo kill went to %v, want Violence (10 of 10)", got)
	}
	// none counted: the killer keeps it
	if _, got := w.killCreditLocked(&NPC{ServerID: 9003, World: 5}, nil, out); got != out {
		t.Fatalf("an untallied kill went to %v", got)
	}
}
