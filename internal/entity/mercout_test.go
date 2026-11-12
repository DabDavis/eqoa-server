package entity

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1504 (Lifeispain): "Logged back in here and immediately got a
// group invite from it. I declined but it is still following me around ... It
// still is fighting for me even though not in group ... when I just left the
// dummy the merc is still fighting it by itself." A merc fights only in its
// owner's group (mercgroup.go): while its box is up it stands and fights
// nothing, its no dismisses it (its hire ended), and one whose owner walked
// off from its target lets the target go (mercfight.go).

// Its box up, the merc stands where it is and fights nothing, though its
// owner fights; the no dismisses it, its owner told and the handlers told to
// end its hire. The control: the yes, and it fights its owner's creature.
func TestADeclinedMercIsDismissed(t *testing.T) {
	r := newMercRig(t)
	ms := r.hireAsked(r.kit(0, 0, 0), MercAssist)
	crab := r.creature("Desert Crab", 28, 4)
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 400)
	r.w.mu.Unlock()
	x, z := ms.npc.X, ms.npc.Z
	r.p.X += 8 // its owner walks off a little: it does not follow
	r.run(0, 3*time.Second)
	if ms.target != 0 || crab.HP != crab.MaxHP || ms.npc.X != x || ms.npc.Z != z {
		t.Fatalf("its box up, the merc fought %d (crab %d/%d) or moved (%v,%v -> %v,%v)",
			ms.target, crab.HP, crab.MaxHP, x, z, ms.npc.X, ms.npc.Z)
	}
	r.sent()
	r.w.GroupDecline(r.sess, r.sess.PlayerObjectID(), 1)
	if r.w.merc1(r.p.ServerID) != nil || r.w.NPCs[ms.npc.ServerID] != nil {
		t.Fatal("the declined merc stayed")
	}
	if len(r.w.mercNotes) != 1 || r.w.mercNotes[0].Kind != MercNoteDismissed {
		t.Errorf("notes after the no: %+v", r.w.mercNotes)
	}
	if !r.told(fmt.Sprintf(mercOutLine, "Ralthas")) {
		t.Error("the owner was not told its merc is dismissed")
	}
	r.run(3*time.Second, 4*time.Second)
	if r.p.GroupID != 0 {
		t.Errorf("a group after the no: %d", r.p.GroupID)
	}
	// the control: a yes, and it fights
	r.w.mercNotes = nil
	ms = r.hire(r.kit(0, 0, 0), MercAssist)
	r.run(4*time.Second, 7*time.Second)
	if r.w.merc1(r.p.ServerID) != ms || ms.target != crab.ServerID {
		t.Fatalf("after the yes the merc is %v and fights %d, not the crab", r.w.merc1(r.p.ServerID) == ms, ms.target)
	}
}

// A merc back at its owner's login (a hire, as handlers.Mercs.bring makes it)
// is asked into a group; its no, the PS2's answer 1, dismisses it. The PS2's
// answer 2 ("already grouped", its client holding a group the server has
// none of) is a yes, the control.
func TestALoginsMercDeclinedGoes(t *testing.T) {
	r := newMercRig(t)
	r.hireAsked(r.kit(9, 0, 0), MercAssist)
	r.w.GroupDecline(r.sess, r.sess.PlayerObjectID(), 2)
	if g := r.w.groups[r.p.GroupID]; g == nil || len(g.Members) != 2 {
		t.Fatalf("answer 2 made no group of the two: %+v", g)
	}
	r.w.GroupLeave(r.sess)
	r.run(0, time.Second)
	if r.w.merc1(r.p.ServerID) != nil {
		t.Fatal("out of the group of the two of them, the merc stayed")
	}
}

// Its box behind another player's (the group of the two gave way to an
// invitation): it waits, fighting nothing, and is asked again at that box's
// no; its own no then dismisses it.
func TestAMercWaitingBehindABoxHolds(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	isess, inviter := r.player("Inviter", 0, 0)
	r.w.GroupInvite(isess, r.w.IDSeenBy(isess, r.p))
	crab := r.creature("Desert Crab", 28, 4)
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.run(0, 2*time.Second)
	if r.w.merc1(r.p.ServerID) != ms || ms.target != 0 {
		t.Fatalf("behind another's box the merc went (%v) or fought %d", r.w.merc1(r.p.ServerID) != ms, ms.target)
	}
	r.w.GroupDecline(r.sess, r.w.IDSeenBy(r.sess, inviter), 1)
	r.sent()
	r.run(2*time.Second, 3*time.Second)
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 || r.w.merc1(r.p.ServerID) != ms {
		t.Fatalf("after the other box's no: asked %v, merc kept %v", got, r.w.merc1(r.p.ServerID) == ms)
	}
	r.w.GroupDecline(r.sess, r.sess.PlayerObjectID(), 1)
	if r.w.merc1(r.p.ServerID) != nil {
		t.Error("its own no left the merc")
	}
}

// Its owner walks off from a training dummy they both fought: the merc lets it
// go and follows its owner, though the dummy fights it. The controls: its
// owner beside the dummy, it fights on; /merc attack's order holds to the
// leash.
func TestAMercLetsGoWhatItsOwnerLeft(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	dummy := r.creature("Training Dummy Lv30", 30, 4)
	dummy.Dummy = TrainingDummy
	r.p.AttackTarget, r.p.AutoAttack = dummy.ServerID, true
	r.run(0, 4*time.Second)
	if ms.target != dummy.ServerID {
		t.Fatalf("the merc fights %d, not its owner's dummy", ms.target)
	}
	// its owner stops and stands beside it: the merc fights on (the dummy
	// fights it)
	r.p.disengage()
	r.p.AutoAttack = false
	r.run(4*time.Second+100*time.Millisecond, 6*time.Second)
	if ms.target != dummy.ServerID {
		t.Fatalf("its owner beside the dummy, the merc let it go (target %d)", ms.target)
	}
	// its owner walks off past petAssist
	r.w.mu.Lock()
	oldX := r.p.X
	r.p.X += petAssist + 10
	r.w.SpatialIndex.Update(r.p.ServerID, oldX, r.p.Z, r.p.X, r.p.Z)
	r.w.mu.Unlock()
	r.run(6*time.Second+100*time.Millisecond, 12*time.Second)
	if ms.target != 0 {
		t.Fatalf("its owner %v from the dummy, the merc still fights it", distXZ(r.p.X, r.p.Z, dummy.X, dummy.Z))
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); d > petFollowStart+1 {
		t.Errorf("the merc did not follow its owner: %v away", d)
	}
	// the control: /merc attack's order (as MercAttack sets it) holds it on
	// the dummy to the leash
	ms.ordered, ms.target = dummy.ServerID, dummy.ServerID
	r.run(12*time.Second+100*time.Millisecond, 13*time.Second)
	if ms.target != dummy.ServerID {
		t.Errorf("/merc attack's order was let go: target %d", ms.target)
	}
}

// told reports the line among what the owner's session was sent since the
// last look (UTF-16, as sendChatToSession writes it), the rest dropped.
func (r *mercRig) told(line string) bool {
	want := make([]byte, 0, 2*len(line))
	for _, c := range line {
		want = append(want, byte(c), byte(c>>8))
	}
	found := false
	for _, m := range r.sess.Queue().GetPending() {
		found = found || bytes.Contains(m.Data(), want)
	}
	return found
}

// A merc dying in a raid group says nothing to the raid (the owner,
// 2026-11-11: "dont show it in the raid chat as them leaving, it looks like a
// player left"): it leaves the group, its owner is told it died, and no member
// hears "<merc> left the raid group". The control: the raid's own line for a
// player leaving is raidLeftLocked's, unchanged (raid_test.go).
func TestAMercDyingInARaidSaysNothingToTheRaid(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	r.w.mu.Lock()
	g := r.w.groups[r.p.GroupID]
	if g == nil || !groupHas(g, ms.npc.ServerID) {
		r.w.mu.Unlock()
		t.Fatal("the merc is not in its owner's group")
	}
	g.raid = true
	drain(r.sess)
	r.w.mercDiedLocked(ms, r.sess, r.at(time.Second))
	stillIn := groupHas(g, ms.npc.ServerID)
	r.w.mu.Unlock()
	if stillIn {
		t.Fatal("the dead merc is still in the group")
	}
	got := linesOf(drain(r.sess))
	for _, l := range got {
		if strings.Contains(l, "left the raid group") {
			t.Errorf("the raid was told %q", l)
		}
	}
	if !hasLine(got, fmt.Sprintf(mercDiedLine, ms.npc.Name, int(MercRest/time.Second))) {
		t.Errorf("the owner was not told it died: %q", got)
	}
}
