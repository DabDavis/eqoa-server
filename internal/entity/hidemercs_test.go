package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// /hide mercs (hidemercs.go), held to petition 1858 (the GM Jibrish: "this
// hide the character model and nameplate for mercenaries, or restore them"):
// a player that hid them is sent no other player's merc, its own still; a
// merc in view leaves the client as an entity leaving view does and comes back
// at /show mercs; and the game goes on as before, the hidden merc healing,
// buffed and sharing the viewer's place.

// slotOf is the slot index (ChannelEntity's) id is on in sess's view, -1 none.
func slotOf(sess *session.Session, id EntityID) int {
	for i, e := range GetOrCreateVisibility(sess).ChannelEntity {
		if e == id {
			return i
		}
	}
	return -1
}

// inView reports id on one of sess's slots after a visibility pass.
func inView(w *World, sess *session.Session, id EntityID) bool {
	for _, e := range visible(w, sess) {
		if e == id {
			return true
		}
	}
	return false
}

// A viewer that hid the mercs is sent no other player's merc but its own, and
// still the other player itself; the other player, hiding nothing, sees both
// mercs. The control: before the hide the viewer sees the other's merc.
func TestHideMercsKeepsItsOwn(t *testing.T) {
	r := newMercRig(t)
	theirs := r.hire(r.kit(0, 0, 0), MercAssist)
	vs, viewer := r.player("Viewer", 0, 0)
	if _, why := r.w.HireMerc(vs, r.kit(9, 0, 0), MercAssist, r.at(0)); why != "" {
		t.Fatalf("the viewer's hire was refused: %s", why)
	}
	own := r.w.merc1(viewer.ServerID)
	if own == nil {
		t.Fatal("no merc of the viewer's")
	}
	if !inView(r.w, vs, theirs.npc.ServerID) {
		t.Fatal("the control: the other player's merc out of view with nothing hidden")
	}
	if !r.w.SetMercsHidden(vs, true) {
		t.Fatal("SetMercsHidden: the viewer out of the world")
	}
	if on, ok := r.w.MercsHidden(vs); !ok || !on {
		t.Fatalf("MercsHidden %v %v after the hide", on, ok)
	}
	if inView(r.w, vs, theirs.npc.ServerID) {
		t.Error("the other player's merc sent to a viewer that hid the mercs")
	}
	if !inView(r.w, vs, own.npc.ServerID) {
		t.Error("the viewer's own merc hidden: it commands it from the merc menu")
	}
	if !inView(r.w, vs, r.p.ServerID) {
		t.Error("the other player itself hidden")
	}
	if !inView(r.w, r.sess, own.npc.ServerID) || !inView(r.w, r.sess, theirs.npc.ServerID) {
		t.Error("the other player, hiding nothing, lost a merc from its view")
	}
}

// A merc in view when the viewer hides them leaves its slot as an entity
// leaving view does (its channel marked removed, the slot freed), and /show
// mercs puts it back on a slot at the next pass.
func TestHideMercsTakesTheMercInViewAwayAndBack(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	vs, _ := r.player("Viewer", 0, 0)
	visible(r.w, vs)
	slot := slotOf(vs, ms.npc.ServerID)
	if slot < 0 {
		t.Fatal("the control: the merc not in view")
	}
	conn := vs.Connection()
	if conn.GetServerObject(slot + 1).DescribesNoEntity() {
		t.Fatal("the control: the merc's slot describes no entity")
	}
	r.w.SetMercsHidden(vs, true)
	visible(r.w, vs)
	if slotOf(vs, ms.npc.ServerID) >= 0 {
		t.Fatal("the merc kept its slot after the hide")
	}
	if !conn.GetServerObject(slot + 1).DescribesNoEntity() {
		t.Error("the merc's slot not marked removed: the client keeps its body and name plate")
	}
	r.w.SetMercsHidden(vs, false)
	visible(r.w, vs)
	back := slotOf(vs, ms.npc.ServerID)
	if back < 0 {
		t.Fatal("the merc not back in view after /show mercs")
	}
	if conn.GetServerObject(back + 1).DescribesNoEntity() {
		t.Error("the merc's slot back but describing no entity")
	}
}

// A merc hidden from a group mate is out of that mate's view and still plays:
// it shares the mate's place (npcSharesLocked), heals the mate, and the mate's
// group buff reaches it.
func TestAHiddenMercStillPlays(t *testing.T) {
	r := newMercRig(t)
	mateSess, mate := r.player("Mate", 0, 0)
	r.group(mate)
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	ms := r.hire(r.kit(9, 0, 0, heal), MercAssist)
	r.w.SetMercsHidden(mateSess, true)
	if inView(r.w, mateSess, ms.npc.ServerID) {
		t.Fatal("the merc in the view of a mate that hid the mercs")
	}
	r.w.mu.Lock()
	shares := r.w.npcSharesLocked(ms.npc, mate)
	r.w.mu.Unlock()
	if !shares {
		t.Error("the hidden merc no longer shares the mate's place")
	}
	top := mate.MaxHPNow()
	mate.HP = top / 2
	r.run(0, 9*time.Second)
	if mate.HP <= top/2 {
		t.Errorf("the hidden merc did not heal the mate: %d/%d", mate.HP, top)
	}
	buff := bookSpell(700189, "Group Daring", 2, 0, 0, 0, "Group, Increase AC by 200, Increase Maximum HP by 200, Radius 30, 30min")
	r.w.mu.Lock()
	ids, why := r.w.castTargetsLocked(mateSess, mate, &buff, 0)
	r.w.mu.Unlock()
	found := false
	for _, id := range ids {
		found = found || id == ms.npc.ServerID
	}
	if why != "" || !found {
		t.Errorf("the mate's group buff's targets %v (%q): not the hidden merc", ids, why)
	}
	if inView(r.w, mateSess, ms.npc.ServerID) {
		t.Error("the merc back in the mate's view after playing")
	}
}
