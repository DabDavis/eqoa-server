package handlers

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Call of the Hero through the wire (callhero.go, entity/callhero.go): the
// gate trip's wizard calls its friend, far off, the hooks set as
// RegisterDefaultHandlers sets them; the friend's Yes (0x0035 echoing the
// box's id) lands it beside the wizard, its No does not. The spell, people
// and places are made up.

// callTrip is a gate trip whose friend stands far from the wizard, the call
// cast and landed, and the friend's box: its id and question.
func callTrip(t *testing.T) (*gateTrip, uint32, string, []string) {
	t.Helper()
	g := newGateTrip(t)
	g.w.OnHeroCall(askHeroCall)
	if _, _, ok := g.w.PlaceRespawn(g.friend.ServerID, 9000, 50, 4000, 0); !ok {
		t.Fatal("the friend did not move")
	}
	g.cast("Made-up Beckoning", "Summons a player to where you are")
	boxes := sentAll(g.fs, protocol.OpcodeQuestPopupMulti)
	if len(boxes) != 1 {
		t.Fatalf("the friend got %d boxes", len(boxes))
	}
	b := boxes[0]
	id := binary.LittleEndian.Uint32(b)
	q, rest := readWideLine(b[4:])
	var opts []string
	if len(rest) > 0 {
		n := int(rest[0])
		rest = rest[1:]
		for range n {
			var o string
			o, rest = readWideLine(rest)
			opts = append(opts, o)
		}
	}
	g.fs.Queue().GetPending()
	return g, id, q, opts
}

// pickBox is the player's 0x0035 to a box.
func pickBox(t *testing.T, ih *InteractHandler, s *session.Session, id uint32, n byte) {
	t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDialogueBoxOption)
	b := make([]byte, 5)
	binary.LittleEndian.PutUint32(b, id)
	b[4] = n
	msg.SetData(b)
	if err := ih.HandleDialogueOption(s, msg); err != nil {
		t.Fatal(err)
	}
}

// A call's box is told from a rehabilitator's: a Yes to a call is no
// rehabilitator's answer, and the other way round.
func TestACallsBoxIsNoRehabilitators(t *testing.T) {
	call := uint32(entity.HeroCallBoxMark | 7)
	if IsRehabBox(call) || !entity.IsHeroCallBox(call) {
		t.Errorf("a call's box %#x: rehab %v, call %v", call, IsRehabBox(call), entity.IsHeroCallBox(call))
	}
	if rehab := uint32(rehabBoxMark | 7); entity.IsHeroCallBox(rehab) || !IsRehabBox(rehab) {
		t.Errorf("a rehabilitator's box %#x: call %v, rehab %v", rehab, entity.IsHeroCallBox(rehab), IsRehabBox(rehab))
	}
}

// The friend far off is asked Yes or No under a call's id; the caster, by
// it, is not, and is told whom it called. Yes lands the friend beside the
// wizard at the next tick, by the teleport a gate sends.
func TestACallsYesLandsTheFriend(t *testing.T) {
	g, id, q, opts := callTrip(t)
	if !entity.IsHeroCallBox(id) || q != "Wizard calls you to their side. Accept?" || len(opts) != 2 || opts[0] != "Yes" || opts[1] != "No" {
		t.Fatalf("the box: %#x %q %q", id, q, opts)
	}
	if n := len(sentAll(g.cs, protocol.OpcodeQuestPopupMulti)); n != 0 {
		t.Errorf("the caster was asked %d times", n)
	}
	var told []string
	for _, l := range heard(t, g.cs) {
		told = append(told, l.text)
	}
	if !slices.Contains(told, "You call Friend to your side.") {
		t.Errorf("the caster was told %q", told)
	}
	ih := &InteractHandler{gameWorld: g.w}
	pickBox(t, ih, g.fs, id, 0)
	g.w.Tick(g.t0.Add(10 * time.Second))
	x, z, world, ok := teleported(g.fs)
	if !ok || x != 1000 || z != 1000 || world != 0 {
		t.Errorf("Yes: the friend teleported to (%.0f, %.0f) world %d (%v), want beside the wizard at (1000, 1000)", x, z, world, ok)
	}
	if snap, _ := g.w.PlayerSnapshot(g.friend.ServerID); snap.X != 1000 || snap.Z != 1000 {
		t.Errorf("the friend in the World at (%.0f, %.0f)", snap.X, snap.Z)
	}
}

// No lands no one, nor does a Yes to the same box after it.
func TestACallsNoLandsNoOne(t *testing.T) {
	g, id, _, _ := callTrip(t)
	ih := &InteractHandler{gameWorld: g.w}
	pickBox(t, ih, g.fs, id, 1)
	g.w.Tick(g.t0.Add(10 * time.Second))
	pickBox(t, ih, g.fs, id, 0)
	g.w.Tick(g.t0.Add(11 * time.Second))
	if _, _, _, ok := teleported(g.fs); ok {
		t.Error("No: the friend was teleported")
	}
	if snap, _ := g.w.PlayerSnapshot(g.friend.ServerID); snap.X != 9000 || snap.Z != 4000 {
		t.Errorf("the friend is at (%.0f, %.0f), want where it was", snap.X, snap.Z)
	}
}
