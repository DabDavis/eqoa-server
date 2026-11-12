package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// mercui.go and groupreg.go (the GM Jibrish's petitions 1475, 1476 and 1488,
// 2026-11-11): the offers' bytes, the answers kept on the session, Dismiss
// Mercenary as /merc dismiss, and "Allow anyone in my group to invite" kept
// for the character and read back at the next start.

// The offers are their opcodes and a u8 layout, 1; the mercs' goes only
// where world.mercs is on.
func TestTheMercAndGroupRegOffers(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Alda", 100, 100, 0)
	SetMercs(false)
	SendMercsOffer(s)
	SendGroupRegOffer(s)
	if got := queued(s, protocol.OpcodeMercsOffer); len(got) != 0 {
		t.Errorf("the mercs' offer with world.mercs off: % x", got)
	}
	if got := queued(s, protocol.OpcodeGroupRegOffer); len(got) != 1 || !bytes.Equal(got[0], []byte{0x5D, 0x7E, 1}) {
		t.Errorf("the group search's offer % x", got)
	}
	SetMercs(true)
	t.Cleanup(func() { SetMercs(false) })
	SendMercsOffer(s)
	if got := queued(s, protocol.OpcodeMercsOffer); len(got) != 1 || !bytes.Equal(got[0], []byte{0x5A, 0x7E, 1}) {
		t.Errorf("the mercs' offer % x", got)
	}
}

// The answers are kept (1 on, 0 off); a short one changes nothing.
func TestTheAnswersAreKept(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Alda", 100, 100, 0)
	mh := NewMercUIHandler(c.gw, nil)
	gh := NewGroupRegHandler(c.gw, nil)
	mh.HandleAnswer(s, incoming(protocol.OpcodeMercsOffer, []byte{1}))
	gh.HandleAnswer(s, incoming(protocol.OpcodeGroupRegOffer, []byte{1}))
	if !s.MercsTold() || !s.GroupReg() {
		t.Fatalf("after 1s: mercs %v, group search %v", s.MercsTold(), s.GroupReg())
	}
	mh.HandleAnswer(s, incoming(protocol.OpcodeMercsOffer, nil))
	gh.HandleAnswer(s, incoming(protocol.OpcodeGroupRegOffer, nil))
	if !s.MercsTold() || !s.GroupReg() {
		t.Error("a short answer changed them")
	}
	mh.HandleAnswer(s, incoming(protocol.OpcodeMercsOffer, []byte{0}))
	gh.HandleAnswer(s, incoming(protocol.OpcodeGroupRegOffer, []byte{0}))
	if s.MercsTold() || s.GroupReg() {
		t.Error("0s left them on")
	}
	// the mercs' 2 (each with its hirer's name), 3 (its class and mode too), and more than 3 read as 3
	for _, a := range []struct{ answer, layout uint8 }{{1, 1}, {2, 2}, {3, 3}, {9, 3}} {
		mh.HandleAnswer(s, incoming(protocol.OpcodeMercsOffer, []byte{a.answer}))
		if !s.MercsTold() || s.MercsLayout() != a.layout {
			t.Errorf("answer %d: told %v layout %d, want %d", a.answer, s.MercsTold(), s.MercsLayout(), a.layout)
		}
	}
}

// Dismiss Mercenary is /merc dismiss: the merc gone, its hire ended, the
// line said; with none, the refusal; with mercenaries off, the off line.
func TestDismissMercenaryFromTheMenu(t *testing.T) {
	r, m, db := newMercFixture(t)
	h := NewMercUIHandler(r.gw, m)
	m.hire(r.anna, r.pa, 0, 0, 0)
	if _, ok := r.gw.MercInfoOf(r.anna); !ok {
		t.Fatal("no merc hired")
	}
	r.drain()
	h.HandleDismiss(r.anna, incoming(protocol.OpcodeMercDismiss, []byte{1}))
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Error("Dismiss Mercenary left the merc")
	}
	if _, kept, _ := db.Merc(r.pa.CharServerID, 1); kept {
		t.Error("the dismissal kept the hire")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:You dismiss *") {
		t.Errorf("after Dismiss Mercenary: %q", got)
	}
	h.HandleDismiss(r.anna, incoming(protocol.OpcodeMercDismiss, []byte{1}))
	if got := r.msgs(r.anna); !hasMsg(got, "red:You have no mercenary out.") {
		t.Errorf("a second Dismiss Mercenary: %q", got)
	}
	SetMercs(false)
	h.HandleDismiss(r.anna, incoming(protocol.OpcodeMercDismiss, []byte{1}))
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+mercOffLine) {
		t.Errorf("with mercenaries off: %q", got)
	}
}

// The setting: off, kept in chargroupinvites and in force (a member's invite
// refused); a new handler over the same database (the next start) reads it
// back; on again, kept as 1 and the member's invite goes.
func TestGroupInvitesSettingKept(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/gi.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alda", 100, 100, 0)
	sb, pb := c.join(2, "Bren", 102, 100, 0)
	sc, pc := c.join(3, "Cael", 104, 100, 0)
	c.group(t, sa, pa, sb, pb)
	h := NewGroupRegHandler(c.gw, db)
	h.HandleSetting(sa, incoming(protocol.OpcodeGroupInvitesSetting, []byte{0}))
	shut, err := db.GroupInvitesShut()
	if err != nil || !shut[pa.CharServerID] || len(shut) != 1 {
		t.Fatalf("kept: %v %v", shut, err)
	}
	if got := heard(t, sa); len(got) == 0 || got[len(got)-1].text != groupInvitesShutLine+groupInvitesRaidLine {
		t.Errorf("Alda was told %+v", got)
	}
	c.gw.GroupInvite(sb, c.gw.IDSeenBy(sb, pc))
	if pc.GroupID != 0 || len(queued(sc, protocol.OpcodeGroupInviteReceived)) != 0 {
		t.Error("Bren's invite went with Alda's setting off")
	}
	// the next start: a new world over the same rows
	c2 := newChatWorld(t)
	sa2, pa2 := c2.join(1, "Alda", 100, 100, 0)
	sb2, pb2 := c2.join(2, "Bren", 102, 100, 0)
	sc2, pc2 := c2.join(3, "Cael", 104, 100, 0)
	c2.group(t, sa2, pa2, sb2, pb2)
	NewGroupRegHandler(c2.gw, db)
	c2.gw.GroupInvite(sb2, c2.gw.IDSeenBy(sb2, pc2))
	if len(queued(sc2, protocol.OpcodeGroupInviteReceived)) != 0 {
		t.Error("after the restart Bren's invite went: the setting was not read back")
	}
	// on again
	NewGroupRegHandler(c2.gw, db).HandleSetting(sa2, incoming(protocol.OpcodeGroupInvitesSetting, []byte{1}))
	if shut, _ := db.GroupInvitesShut(); len(shut) != 0 {
		t.Errorf("on again, still shut: %v", shut)
	}
	c2.gw.GroupInvite(sb2, c2.gw.IDSeenBy(sb2, pc2))
	if len(queued(sc2, protocol.OpcodeGroupInviteReceived)) != 1 {
		t.Error("with the setting on, Bren's invite did not go")
	}
	_ = entity.GroupWindowSize
}
