package entity

import (
	"strings"
	"testing"
	"time"
)

// The GM Jibrish's petition 1879: an instance's leader adds players beyond its party, up to its cap (4 for a heroic
// dungeon, 50 for a raid's), and takes them out: one standing in it is warned and sent to Freeport after 60 s. Over
// the dungeons rig: a group of two (Ann leads) in Test Guk; a member may add no one; Ann adds Cal (not in her group),
// who may then enter it (he is a member of Fay's group, opening nothing of his own); Dee makes four; a fifth is refused at the cap (the control: a raid's instance holds 50, so
// the same fifth is added once the group is a raid). Taken out, Cal standing inside stays the minute, then goes to
// the Freeport warden, and may not enter again until added again; the owner is never taken out.
func TestDungeonAddAndRemovePlayers(t *testing.T) {
	r := newDungeons(t, 6)
	ann, bob, cal, dee, eve, fay := r.ps[0], r.ps[1], r.ps[2], r.ps[3], r.ps[4], r.ps[5]
	r.group(ann, bob)
	r.group(fay, cal) // Cal a member elsewhere: he opens nothing of his own
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("Ann could not open Guk")
	}
	r.land()
	inst := ann.Instance
	if line := r.w.DungeonAddMember(r.ss[1], "Raider2", 0); line != instLineNotYours {
		t.Fatalf("Bob, a member, added: %q", line)
	}
	if _, ok := r.w.DungeonEnter(r.ss[2], "guk", r.now); ok {
		t.Fatal("Cal entered before he was added")
	}
	if line := r.w.DungeonAddMember(r.ss[0], cal.Name, 0); !strings.Contains(line, "(3 of 4 players)") {
		t.Fatalf("adding Cal: %q", line)
	}
	if _, ok := r.w.DungeonEnter(r.ss[2], "guk", r.now); !ok {
		t.Fatal("Cal, added, could not enter")
	}
	r.land()
	if cal.Instance != inst {
		t.Fatalf("Cal entered instance %d, want %d", cal.Instance, inst)
	}
	if line := r.w.DungeonAddMember(r.ss[0], dee.Name, 0); !strings.Contains(line, "(4 of 4 players)") {
		t.Fatalf("adding Dee: %q", line)
	}
	if line := r.w.DungeonAddMember(r.ss[0], eve.Name, 0); !strings.Contains(line, "holds 4 players, its most") {
		t.Fatalf("a fifth past the cap: %q", line)
	}
	// the control: a raid's instance holds 50
	r.w.mu.Lock()
	r.w.groups[ann.GroupID].flagged = true
	r.w.mu.Unlock()
	if line := r.w.DungeonAddMember(r.ss[0], eve.Name, 0); !strings.Contains(line, "(5 of 50 players)") {
		t.Fatalf("a raid's fifth: %q", line)
	}
	// Cal taken out, standing inside: warned, the minute, then Freeport
	if line := r.w.DungeonRemoveMember(r.ss[0], cal.Name, 0, r.now); !strings.Contains(line, "taken out") {
		t.Fatalf("taking Cal out: %q", line)
	}
	if !strings.Contains(strings.Join(linesOf(drain(r.ss[2])), "\n"), "in 60 seconds you are sent back to Freeport") {
		t.Error("Cal was not warned")
	}
	r.tick(30 * time.Second)
	if due := r.land(); len(due) != 0 {
		t.Fatalf("sent out within the minute: %+v", due)
	}
	r.tick(31 * time.Second)
	due := r.land()
	if len(due) != 1 || due[0].P != cal || cal.World != 0 || cal.X != 5000 {
		t.Fatalf("after the minute: %+v, Cal at world %d (%.0f, %.0f)", due, cal.World, cal.X, cal.Z)
	}
	if _, ok := r.w.DungeonEnter(r.ss[2], "guk", r.now); ok {
		t.Fatal("Cal, taken out, entered again")
	}
	// a member of the group taken out the same way, and the owner never
	if line := r.w.DungeonRemoveMember(r.ss[0], bob.Name, 0, r.now); !strings.Contains(line, "taken out") {
		t.Fatalf("taking Bob out: %q", line)
	}
	if _, ok := r.w.DungeonEnter(r.ss[1], "guk", r.now); ok {
		t.Fatal("Bob, taken out, entered")
	}
	if line := r.w.DungeonRemoveMember(r.ss[0], ann.Name, 0, r.now); line != instLineOwner {
		t.Fatalf("the owner taken out: %q", line)
	}
	// added again: in again
	if line := r.w.DungeonAddMember(r.ss[0], cal.Name, 0); !strings.Contains(line, "is added") {
		t.Fatalf("Cal added again: %q", line)
	}
	if _, ok := r.w.DungeonEnter(r.ss[2], "guk", r.now); !ok {
		t.Fatal("Cal, added again, could not enter")
	}
	// the loot: a kill after Bob was taken out seats him no more (Ann, at the kill, does)
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	var knight *NPC
	for _, c := range r.copies(inst) {
		knight = c
	}
	r.w.mu.Lock()
	r.w.groups[ann.GroupID].rules.loot = LootNeedGreed
	bob.World, bob.X, bob.Z, bob.Instance = 14, knight.X, knight.Z, inst
	ann.X, ann.Z = knight.X, knight.Z
	r.w.killNPC(knight)
	r.w.creditKillLocked(r.ss[0], ann, knight)
	r.w.mu.Unlock()
	if len(heard) == 0 || heard[len(heard)-1].Raid == nil {
		t.Fatal("no raid kill heard")
	}
	annSeat := false
	for _, st := range heard[len(heard)-1].Raid.Seats {
		if st.Name == bob.Name {
			t.Error("Bob, taken out, has a seat at a kill after it")
		}
		annSeat = annSeat || st.Name == ann.Name
	}
	if !annSeat {
		t.Error("Ann has no seat: the control")
	}
	// kept over a keep restart
	k := r.w.KeepDungeons(r.now)
	if k == nil || len(k.Insts) != 1 || len(k.Insts[0].Added) != 3 || len(k.Insts[0].Removed) != 1 {
		t.Fatalf("kept: %+v", k)
	}
}
