package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// merc1 is the owner's first merc out (dead or not), nil for none: the tests
// of one merc a player.
func (w *World) merc1(owner EntityID) *mercState {
	if list := w.mercs[owner]; len(list) > 0 {
		return list[0]
	}
	return nil
}

// benchRig is a mercRig whose MercNoteBacks are answered as the handlers
// answer them (handlers/merc.go's comeBackSlot): the kit of the slot hired
// again.
type benchRig struct {
	*mercRig
	kits map[EntityID]map[int]MercKit
}

func newBenchRig(t *testing.T) *benchRig {
	return &benchRig{mercRig: newMercRig(t), kits: map[EntityID]map[int]MercKit{}}
}

// hireAt hires a merc named name in the slot for p (sess its), as the
// liaison does, and keeps its kit for its comings back.
func (r *benchRig) hireAt(sess *session.Session, p *Player, slot int, name string) (standby bool) {
	r.t.Helper()
	k := r.kit(0, 0, 0)
	k.Name = name
	if r.kits[p.ServerID] == nil {
		r.kits[p.ServerID] = map[int]MercKit{}
	}
	r.kits[p.ServerID][slot] = k
	_, why, standby := r.w.HireMercAt(sess, slot, k, MercAssist, r.at(0))
	if why != "" {
		r.t.Fatalf("%s's hire of %s was refused: %s", p.Name, name, why)
	}
	return standby
}

// answerNotes answers the merc notes queued, as the handlers do: a
// MercNoteBack's slot hired again from its kit. It reports the slots asked
// back, by owner.
func (r *benchRig) answerNotes(d time.Duration) map[EntityID][]int {
	r.w.mu.Lock()
	notes := r.w.mercNotes
	r.w.mercNotes = nil
	r.w.mu.Unlock()
	out := map[EntityID][]int{}
	for _, n := range notes {
		if n.Kind != MercNoteBack {
			continue
		}
		out[n.Owner.ServerID] = append(out[n.Owner.ServerID], n.Slot)
		r.w.HireMercAt(n.Sess, n.Slot, r.kits[n.Owner.ServerID][n.Slot], MercAssist, r.at(d))
	}
	return out
}

// names is g's members by name.
func (r *benchRig) names(g *Group) string {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []string
	for _, id := range g.Members {
		if v, ok := r.w.memberViewLocked(id); ok {
			out = append(out, v.name)
		} else {
			out = append(out, "?")
		}
	}
	return strings.Join(out, ",")
}

// standby is p's standby, by name, in the order they stepped out.
func (r *benchRig) standby(p *Player) string {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []string
	for _, b := range r.w.benchedOfLocked(p) {
		out = append(out, b.name)
	}
	return strings.Join(out, ",")
}

// chats is the chat lines p's session was sent since the last look.
func chats(sess *session.Session) []string {
	var out []string
	for _, m := range sess.Queue().GetPending() {
		d := m.Data()
		if opOfMsg(d) == protocol.OpcodeClientMessage {
			out = append(out, strings.ReplaceAll(string(d), "\x00", ""))
		}
	}
	return out
}

func hasChat(lines []string, part string) bool {
	for _, l := range lines {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// The owner's brief (2026-11-12): three mercs a player; alone with them, a
// group of four; a player's yes puts the merc that joined last on standby,
// out of the world, its hire kept; the player's leaving brings it back by
// itself, beside its owner and into the group. Its three hires are asked into
// a group in ONE box.
func TestThreeMercsMakeWayForAPlayer(t *testing.T) {
	r := newBenchRig(t)
	r.sent()
	for i, name := range []string{"One", "Two", "Three"} {
		if r.hireAt(r.sess, r.p, i+1, name) {
			t.Fatalf("%s went on standby with its owner in no group", name)
		}
	}
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("three hires asked %d boxes, want one", got[protocol.OpcodeGroupInviteReceived])
	}
	r.run(0, time.Second) // no box of its own while the first is up
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 0 {
		t.Fatalf("the second and third hires asked boxes of their own: %v", got)
	}
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	g := r.w.groups[r.p.GroupID]
	if g == nil || r.names(g) != "Owner,One,Two,Three" {
		t.Fatalf("the yes made %v, want the owner and its three", g)
	}
	r.run(time.Second, 2*time.Second)
	if len(r.w.mercsOfLocked(r.p)) != 3 || r.standby(r.p) != "" {
		t.Fatalf("after the upkeep: %d out, standby %q", len(r.w.mercsOfLocked(r.p)), r.standby(r.p))
	}
	// each in its own follow place
	r.w.mu.Lock()
	seen := map[followSlot]bool{}
	for _, ms := range r.w.mercsOfLocked(r.p) {
		seen[mercFollowSlot(ms.slot)] = true
	}
	r.w.mu.Unlock()
	if len(seen) != 3 {
		t.Errorf("the three share follow slots: %v", seen)
	}

	// a player invited into the group of four: not refused, and its yes puts
	// Three (the merc that joined last) on standby
	bsess, b := r.player("Bertha", 1, 0)
	three := r.w.mercAtLocked(r.p, 3).npc
	chats(r.sess)
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, b))
	r.w.GroupAccept(bsess, r.w.IDSeenBy(bsess, r.p))
	if b.GroupID != g.ID || r.names(g) != "Owner,One,Two,Bertha" {
		t.Fatalf("after Bertha's yes the group is %q (hers %d)", r.names(g), b.GroupID)
	}
	if r.standby(r.p) != "Three" || r.w.NPCs[three.ServerID] != nil || r.w.mercAtLocked(r.p, 3) != nil {
		t.Fatalf("Three is not on standby out of the world: standby %q", r.standby(r.p))
	}
	if !hasChat(chats(r.sess), "Three steps out of the group to make room for Bertha") {
		t.Error("the owner was not told why Three stepped out")
	}
	// the standby stays while the group is full
	r.run(2*time.Second, 4*time.Second)
	if back := r.answerNotes(4 * time.Second); len(back) != 0 || r.standby(r.p) != "Three" {
		t.Fatalf("a full group asked Three back: %v", back)
	}
	// Bertha leaves: Three comes back by itself, beside its owner, in the group
	r.w.GroupLeave(bsess)
	r.run(4*time.Second, 5*time.Second)
	if back := r.answerNotes(5 * time.Second); len(back[r.p.ServerID]) != 1 || back[r.p.ServerID][0] != 3 {
		t.Fatalf("Bertha's leaving asked back %v, want slot 3", back)
	}
	r.run(5*time.Second, 6*time.Second)
	if r.names(g) != "Owner,One,Two,Three" || r.standby(r.p) != "" {
		t.Fatalf("after Bertha left: %q, standby %q", r.names(g), r.standby(r.p))
	}
	if back := r.answerNotes(6 * time.Second); len(back) != 0 {
		t.Errorf("asked back again once it came: %v", back)
	}
	ms := r.w.mercAtLocked(r.p, 3)
	if ms == nil || distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z) > 2*followRadius {
		t.Fatal("Three is not back beside its owner")
	}

	// the control: a raid flag's group takes a player and keeps every merc
	g.flagged = true
	r.w.mu.Lock()
	g.Members = append(g.Members, b.ServerID)
	b.GroupID = g.ID
	r.w.mercsMakeRoomLocked(g, b, r.at(6*time.Second))
	r.w.mu.Unlock()
	r.run(6*time.Second, 7*time.Second)
	if len(g.Members) != 5 || r.standby(r.p) != "" {
		t.Errorf("a raid flag's group put a merc on standby: %q, standby %q", r.names(g), r.standby(r.p))
	}
}

// Two owners in one group: each merc is its owner's; a player's yes puts the
// merc that joined last on standby, and a place freeing brings back the
// standby that stepped out FIRST (its owner lost one first). A joiner's own
// merc comes in only to a place still free, else on standby.
func TestTwoOwnersShareTheMercPlaces(t *testing.T) {
	r := newBenchRig(t)
	r.hireAt(r.sess, r.p, 1, "AnnOne")
	r.hireAt(r.sess, r.p, 2, "AnnTwo")
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	g := r.w.groups[r.p.GroupID]
	if g == nil || r.names(g) != "Owner,AnnOne,AnnTwo" {
		t.Fatalf("the owner's group: %v", g)
	}
	// Cato with a merc of his own (in a group of the two of them): it gives
	// way to the invitation, and his merc finds no place: standby
	csess, c := r.player("Cato", 9, 0)
	r.hireAt(csess, c, 1, "CatoOne")
	r.w.GroupAccept(csess, csess.PlayerObjectID())
	if c.GroupID == 0 {
		t.Fatal("Cato and his merc are no group")
	}
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, c))
	r.w.GroupAccept(csess, r.w.IDSeenBy(csess, r.p))
	if r.names(g) != "Owner,AnnOne,AnnTwo,Cato" || r.standby(c) != "CatoOne" || r.standby(r.p) != "" {
		t.Fatalf("after Cato's yes: %q, his standby %q, the owner's %q", r.names(g), r.standby(c), r.standby(r.p))
	}
	// Dara joins: AnnTwo (the merc that joined last) steps out
	dsess, d := r.player("Dara", 5, 0)
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, d))
	r.w.GroupAccept(dsess, r.w.IDSeenBy(dsess, r.p))
	if r.names(g) != "Owner,AnnOne,Cato,Dara" || r.standby(r.p) != "AnnTwo" {
		t.Fatalf("after Dara's yes: %q, the owner's standby %q", r.names(g), r.standby(r.p))
	}
	// Edda joins: four players, every merc on standby, AnnOne last
	esess, e := r.player("Edda", 5, 0)
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, e))
	r.w.GroupAccept(esess, r.w.IDSeenBy(esess, r.p))
	if r.names(g) != "Owner,Cato,Dara,Edda" || r.standby(r.p) != "AnnTwo,AnnOne" {
		t.Fatalf("four players: %q, the owner's standby %q", r.names(g), r.standby(r.p))
	}
	// a fifth player: no place (four players)
	fsess, f := r.player("Fern", 5, 0)
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, f))
	r.w.GroupAccept(fsess, r.w.IDSeenBy(fsess, r.p))
	if f.GroupID != 0 || len(g.Members) != 4 {
		t.Fatalf("a fifth player joined a plain group: %q", r.names(g))
	}
	// Edda leaves: one place, CatoOne's (it stepped out first), not the owner's
	r.w.GroupLeave(esess)
	r.run(0, time.Second)
	back := r.answerNotes(time.Second)
	if len(back[c.ServerID]) != 1 || len(back[r.p.ServerID]) != 0 {
		t.Fatalf("one free place asked back %v, want Cato's merc alone", back)
	}
	r.run(time.Second, 2*time.Second)
	if r.names(g) != "Owner,Cato,Dara,CatoOne" || r.standby(c) != "" {
		t.Fatalf("CatoOne is not back in the group: %q", r.names(g))
	}
	// Dara leaves: AnnTwo, the owner's first out
	r.w.GroupLeave(dsess)
	r.run(2*time.Second, 3*time.Second)
	r.answerNotes(3 * time.Second)
	if r.names(g) != "Owner,Cato,CatoOne,AnnTwo" || r.standby(r.p) != "AnnOne" {
		t.Fatalf("after Dara left: %q, standby %q", r.names(g), r.standby(r.p))
	}
	// Cato leaves (his merc with him): the owner's last back
	r.w.GroupLeave(csess)
	r.run(2*time.Second, 3*time.Second)
	r.run(3*time.Second, 4*time.Second)
	r.answerNotes(4 * time.Second)
	if r.names(g) != "Owner,AnnTwo,AnnOne" {
		t.Fatalf("after Cato left: %q, standby %q", r.names(g), r.standby(r.p))
	}
	if r.standby(r.p) != "" {
		t.Errorf("the owner's standby after two places freed: %q", r.standby(r.p))
	}
}

// A logout lets the standby go (the login brings every hire back); a keep
// restart carries it, out of the world on standby again; a dismissal of one
// on standby ends it.
func TestMercStandbyKeptAndDismissed(t *testing.T) {
	r := newBenchRig(t)
	r.hireAt(r.sess, r.p, 1, "One")
	r.hireAt(r.sess, r.p, 2, "Two")
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	var others []*Player
	for _, n := range []string{"Bo", "Cy"} {
		_, o := r.player(n, 0, 0)
		others = append(others, o)
	}
	g := r.group(others...)
	r.w.mu.Lock()
	r.w.mercsMakeRoomLocked(g, others[1], r.at(0))
	r.w.mu.Unlock()
	if r.standby(r.p) != "Two" {
		t.Fatalf("standby %q, want Two", r.standby(r.p))
	}
	kp, ok := r.w.KeepPlayer(r.sess, r.at(0))
	if !ok || len(kp.Mercs) != 2 || kp.Mercs[0].Name != "One" || kp.Mercs[0].Standby || kp.Mercs[1].Slot != 2 || !kp.Mercs[1].Standby {
		t.Fatalf("kept: %+v", kp.Mercs)
	}
	// the new process
	r2 := newBenchRig(t)
	for _, k := range kp.Mercs {
		kit := r2.kit(0, 0, 0)
		kit.Name = k.Name
		came := r2.w.RestoreMerc(r2.sess, kit, k, r2.at(0))
		if came == k.Standby {
			t.Fatalf("%s came %v, its standby %v", k.Name, came, k.Standby)
		}
	}
	if r2.standby(r2.p) != "Two" || r2.w.mercAtLocked(r2.p, 2) != nil || r2.w.mercAtLocked(r2.p, 1) == nil {
		t.Fatalf("restored: standby %q", r2.standby(r2.p))
	}
	// a process before three: its one merc is slot 1
	old := KeptPlayer{Merc: &KeptMerc{Name: "Solo"}}
	if got := KeptMercsOf(&old); len(got) != 1 || got[0].Slot != 1 {
		t.Errorf("an old keep's merc: %+v", got)
	}
	// /merc dismiss of the one on standby ends it
	if name, ok := r.w.DismissMerc(r.sess, 2, r.at(time.Second)); !ok || name != "Two" || r.standby(r.p) != "" {
		t.Fatalf("dismissing the standby: %q %v, standby %q", name, ok, r.standby(r.p))
	}
	// a logout lets the standby go
	r.w.mu.Lock()
	r.w.benchSlotLocked(r.p, nil, 3, "Three", "", r.at(0))
	r.w.mercLeavingLocked(r.p)
	r.w.mu.Unlock()
	if r.standby(r.p) != "" || len(r.w.mercs[r.p.ServerID]) != 0 {
		t.Errorf("after the logout: standby %q, %d out", r.standby(r.p), len(r.w.mercs[r.p.ServerID]))
	}
}
