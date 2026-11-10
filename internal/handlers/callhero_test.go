package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Call of the Hero through the handlers (callhero.go, entity/callhero.go; jib's
// PR #1): the portals' rig with Anna's group, the hooks set as
// RegisterDefaultHandlers sets them. The spell is made up in the workbook's
// shape of the live one; the people and places are the rig's.

// callRig is the portals' rig with the calls' questions and Anna, Bert and
// Cora grouped: Cora by Anna, Bert far off, all of level 51.
func callRig(t *testing.T) (*portalRig, *HeroCalls) {
	t.Helper()
	r := newPortalRig(t)
	hc := NewHeroCalls(r.gw)
	hc.now = func() time.Time { return r.now }
	r.ih.heroCalls = hc
	r.gw.OnHeroCall(hc.Ask)
	for _, p := range []*entity.Player{r.pa, r.pb, r.pc, r.pd} {
		p.Level, p.Power, p.MaxPower = 51, 1000, 1000
	}
	// grouped side by side, then Bert goes far off
	r.pb.X, r.pb.Z = 5002, 5002
	r.group(t, r.anna, r.pa, r.bert, r.pb)
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pc))
	r.gw.GroupAccept(r.cora, r.gw.IDSeenBy(r.cora, r.pa))
	if len(r.gw.GroupMembers(r.pa)) != 3 {
		t.Fatalf("the group: %v", r.gw.GroupMembers(r.pa))
	}
	r.pb.X, r.pb.Z = 20000, 20000
	r.drain()
	return r, hc
}

// callOfTheHero is the call as the workbook gives it: a group spell of its
// caster's, of the members of level 30 and up.
func callOfTheHero() entity.CastSpell {
	sp := &database.SpellPatternRecord{SpellID: 9701, AbilityLvl: 51, Range: 0x41F0, CastTime: 1, Power: 10, Scope: 2,
		SpellName: "Made-up Call", Icon: 9701}
	return entity.SpellOfRow(sp, 5, "Group, Summon Player", `{"v":1,"scope":"group","target_type":"Friendly","min_level":30,"radius":30,
		"effects":[{"kind":"summon_player"}]}`, 0)
}

// castCall is Anna's call, cast and landed; the box Bert was asked.
func (r *portalRig) castCall(t *testing.T) uint32 {
	t.Helper()
	r.drain()
	if !r.gw.StartCast(r.anna, callOfTheHero(), 0xFFFFFFFF, r.now) {
		t.Fatalf("the call did not begin: %q", r.msgs(r.anna))
	}
	r.now = r.now.Add(3 * time.Second)
	r.tick()
	got := r.msgs(r.bert)
	if len(got) != 1 || !strings.HasSuffix(got[0], ":Anna calls you to their side. Go?|Yes|No") {
		t.Fatalf("Bert was sent %q", got)
	}
	box := boxID(got)
	if !IsHeroCallBox(box) || box&0xFF000000 != 0x63000000 {
		t.Fatalf("the call's box id %08X", box)
	}
	return box
}

// The far member is asked under the call's mark and comes on its Yes, to where
// its caster stands then; the caster is told whom it called and who answered.
// The controls: Cora, by Anna, is neither asked nor moved; Bert stays until he
// answers.
func TestACallOfTheHeroEndToEnd(t *testing.T) {
	r, _ := callRig(t)
	box := r.castCall(t)
	if got := r.msgs(r.cora); len(got) != 0 {
		t.Errorf("Cora, at Anna's side, was sent %q", got)
	}
	if !hasMsg(r.msgs(r.anna), "chat:You call Bert to your side.") {
		t.Error("Anna was not told whom she called")
	}
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
		t.Fatalf("Bert moved before answering: x %.0f", snap.X)
	}
	if snap, _ := r.gw.PlayerSnapshot(r.pc.ServerID); snap.X != 5001 {
		t.Fatalf("Cora was moved: x %.0f", snap.X)
	}

	r.pa.X, r.pa.Z = 5010, 5020 // Anna walked on while Bert thought
	r.now = r.now.Add(4 * time.Second)
	r.pick(r.bert, box, 0)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 5010 || snap.Z != 5020 {
		t.Errorf("Bert stands at (%.0f, %.0f), want where Anna stands (5010, 5020)", snap.X, snap.Z)
	}
	if x, z, _, ok := teleported(r.bert); !ok || x != 5010 || z != 5020 {
		t.Errorf("Bert's client was told (%.0f, %.0f) (%v)", x, z, ok)
	}
	r.want("Anna", r.msgs(r.anna), "chat:Bert answers your call.")

	// the box is spent: a second Yes says the call has passed
	r.drain()
	r.pick(r.bert, box, 0)
	r.want("Bert", r.msgs(r.bert), "chat:"+heroPassed)
}

// No lands no one and tells the caster; a box CLIENT declines at once (a menu
// up) is no answer, said once and asked again, and a Yes after it comes.
func TestACallOfTheHeroDeclined(t *testing.T) {
	r, hc := callRig(t)
	box := r.castCall(t)
	r.drain()
	r.now = r.now.Add(2 * time.Second)
	r.pick(r.bert, box, 1)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
		t.Errorf("No: Bert moved to x %.0f", snap.X)
	}
	r.want("Anna", r.msgs(r.anna), "chat:Bert declines your call.")

	r.now = r.now.Add(time.Minute) // the spell's lockout is the record's: none here
	box = r.castCall(t)
	r.drain()
	r.now = r.now.Add(100 * time.Millisecond)
	r.pick(r.bert, box, 255)
	r.want("Bert", r.msgs(r.bert), "chat:Anna calls you to their side. Close your menu to answer.")
	r.now = r.now.Add(portalAskAgain)
	hc.sweep()
	if got := r.msgs(r.bert); len(got) != 1 || boxID(got) != box {
		t.Fatalf("asked again: %q", got)
	}
	r.now = r.now.Add(2 * time.Second)
	r.pick(r.bert, box, 0)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 5000 {
		t.Errorf("a Yes after the client's decline: x %.0f", snap.X)
	}
}

// A call no one answers lapses for both, each told; the control: until then it
// waits (a sweep before its time says nothing).
func TestACallOfTheHeroLapses(t *testing.T) {
	r, hc := callRig(t)
	r.castCall(t)
	r.drain()
	r.now = r.now.Add(entity.HeroCallWait - time.Second)
	hc.sweep()
	r.want("Bert", r.msgs(r.bert))
	r.now = r.now.Add(2 * time.Second)
	hc.sweep()
	r.want("Bert", r.msgs(r.bert), "chat:Anna's call has passed.")
	r.want("Anna", r.msgs(r.anna), "chat:Bert does not answer your call.")
}

// A member already answering a call is not asked a second, its caster told.
func TestACallOfTheHeroBusyMember(t *testing.T) {
	r, hc := callRig(t)
	r.castCall(t)
	r.drain()
	hc.Ask(entity.HeroCall{Caster: r.pc.ServerID, Member: r.pb.ServerID, CasterName: "Cora", MemberName: "Bert",
		Sess: r.bert, CasterSess: r.cora, World: 0, Spell: "Made-up Call", Ring: 1})
	r.want("Bert", r.msgs(r.bert))
	r.want("Cora", r.msgs(r.cora), "red:Bert is already answering a call. Try again in a moment.")
}
