package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// TestCMFullTreeIsTheCapturedRetailResponse pins the embedded blob to what
// retail actually sent, so a truncated or re-generated file is caught here
// rather than as a hung CM window on the PS2.
//
// CLIENT's Recv08 (decomp viclientpart4ec58.cpp) reads it as the type, the
// server's token (u32) and ReadList's u32 count, 0x60: 96 skills (not
// unspent points, as cm-wire-format.md read it).
func TestCMFullTreeIsTheCapturedRetailResponse(t *testing.T) {
	if len(cmFullTree) != 1302 {
		t.Fatalf("embedded tree is %d bytes, retail sent 1302", len(cmFullTree))
	}
	if cmFullTree[0] != cmRespFullTree {
		t.Errorf("type byte = %#x, want %#x (full tree)", cmFullTree[0], cmRespFullTree)
	}
	if got := binary.LittleEndian.Uint32(cmFullTree[1:5]); got != 0 {
		t.Errorf("token = %#x, want 0", got)
	}
	if got := binary.LittleEndian.Uint32(cmFullTree[5:9]); got != 0x60 {
		t.Errorf("count = %#x, want 0x60 (the captured list's)", got)
	}

	// The tree carries UTF-16LE skill names; "Vigor 1" is the first and is what
	// makes this recognisably a real tree rather than a zero-filled file that
	// would satisfy every check above.
	want := []byte{'V', 0, 'i', 0, 'g', 0, 'o', 0, 'r', 0, ' ', 0, '1', 0}
	if !contains(cmFullTree, want) {
		t.Error(`embedded tree does not contain UTF-16 "Vigor 1" — not the captured response`)
	}
}

// The share's answer is CLIENT's Recv01: the token, then the share it then
// shows. Retail answered `01 00 00 00 00 06 00 00 00` with the same bytes:
// a share of 6% (cm-wire-format.md had it as "select item 6").
func TestCMShareAnswerIsRecv01(t *testing.T) {
	want := []byte{0x01, 0, 0, 0, 0, 0x06, 0, 0, 0}
	if got := cmReply(cmShare, 0, 6); string(got) != string(want) {
		t.Errorf("share 6: % x, want % x", got, want)
	}
	if got := cmReply(cmShare, 0x2A, 100); binary.LittleEndian.Uint32(got[1:]) != 0x2A || binary.LittleEndian.Uint32(got[5:]) != 100 {
		t.Errorf("share 100 under token 0x2A: % x", got)
	}
}

// TestEveryCMRequestEndsTheWait is the regression this handler exists for.
//
// The CM window has NO client-side timeout. An unanswered 0x1401 leaves it
// searching forever and the player cannot even back out of the menu — the
// window is unclosable, so the only escape is to drop the session. CLIENT
// sends nothing more while it waits, and only Recv01, 02, 05, 06, 08 and 09
// end the wait. Every sub-op gets one of those, and none but the share gets
// Recv01, which puts its argument where CLIENT keeps the share. The
// control: the old answer to a buy (Recv01 with the skill's id) fails.
func TestEveryCMRequestEndsTheWait(t *testing.T) {
	ends := map[byte]bool{1: true, 2: true, 5: true, 6: true, 8: true, 9: true}
	check := func(sub byte, reply []byte) string {
		if len(reply) == 0 || !ends[reply[0]] {
			return "no answer that ends the wait"
		}
		if reply[0] == cmRespShare && sub != cmShare {
			return "Recv01 for a sub-op that is not the share"
		}
		return ""
	}
	for i := 0; i < 256; i++ {
		sub := byte(i)
		if why := check(sub, cmReply(sub, 7, 0)); why != "" {
			t.Errorf("sub-op %#x: %s — the CM window would hang or show a wrong share", sub, why)
		}
	}
	old := []byte{0x01, 0, 0, 0, 0, 0x6A, 0x01, 0, 0}
	if check(cmBuy, old) == "" {
		t.Fatal("the control: the old buy answer must fail")
	}
}

// The share a player sets is the player's, within 0-100 as CLIENT's Select
// clamps it, kept, and answered with what was kept. The control: a request
// for 150 is not kept as 150.
func TestTheShareIsKept(t *testing.T) {
	c := newChatWorld(t)
	sess, p := c.join(3, "Sharer", 25000, 15000, 0)
	h := NewClassMasteryHandler(c.sm)
	h.SetGameWorld(c.gw)
	kept := &keptMastery{}
	h.SetStore(kept)
	for _, c := range []struct{ asked, want int }{{42, 42}, {150, 100}, {-3, 0}, {100, 100}} {
		sess.Queue().GetPending()
		req := make([]byte, 9)
		req[0] = cmShare
		binary.LittleEndian.PutUint32(req[1:], 0)
		binary.LittleEndian.PutUint32(req[5:], uint32(int32(c.asked)))
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
		msg.SetData(req)
		if err := h.HandleClassMastery(sess, msg); err != nil {
			t.Fatal(err)
		}
		if p.Mastery.Percent != c.want || kept.rows[p.CharServerID].Percent != c.want {
			t.Errorf("asked %d: share %d, kept %d; want %d", c.asked, p.Mastery.Percent, kept.rows[p.CharServerID].Percent, c.want)
		}
		var reply []byte
		for _, m := range sess.Queue().PeekPending() {
			if d := m.Data(); protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassMasteryServer {
				reply = d[2:]
			}
		}
		if len(reply) != 9 || reply[0] != cmRespShare || binary.LittleEndian.Uint32(reply[5:]) != uint32(c.want) {
			t.Errorf("asked %d: answered % x", c.asked, reply)
		}
	}
	if p.Mastery.Percent == 150 {
		t.Fatal("the control: 150 must not be kept")
	}
}

// keptMastery is a store of mastery rows by character.
type keptMastery struct {
	rows map[uint32]database.MasteryRow
}

func (k *keptMastery) SaveMastery(id uint32, m database.MasteryRow) error {
	if k.rows == nil {
		k.rows = map[uint32]database.MasteryRow{}
	}
	k.rows[id] = m
	return nil
}

func contains(hay, needle []byte) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if string(hay[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
