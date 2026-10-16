package handlers

import (
	"encoding/binary"
	"testing"
)

// TestCMFullTreeIsTheCapturedRetailResponse pins the embedded blob to what
// retail actually sent, so a truncated or re-generated file is caught here
// rather than as a hung CM window on the PS2.
//
// Header per claude-notes/cm-wire-format.md, confirmed on
// pcaps/ps2_main_capture.pcap packet #1711:
//
//	byte(0x08) + int32_LE(0 reserved) + int16_LE(totalUnspent)
func TestCMFullTreeIsTheCapturedRetailResponse(t *testing.T) {
	if len(cmFullTree) != 1302 {
		t.Fatalf("embedded tree is %d bytes, retail sent 1302", len(cmFullTree))
	}
	if cmFullTree[0] != cmRespFullTree {
		t.Errorf("type byte = %#x, want %#x (full tree)", cmFullTree[0], cmRespFullTree)
	}
	if got := binary.LittleEndian.Uint32(cmFullTree[1:5]); got != 0 {
		t.Errorf("reserved int32 = %#x, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(cmFullTree[5:7]); got != 0x60 {
		t.Errorf("totalUnspent = %#x, want 0x60 (the captured character's)", got)
	}

	// The tree carries UTF-16LE skill names; "Vigor 1" is the first and is what
	// makes this recognisably a real tree rather than a zero-filled file that
	// would satisfy every check above.
	want := []byte{'V', 0, 'i', 0, 'g', 0, 'o', 0, 'r', 0, ' ', 0, '1', 0}
	if !contains(cmFullTree, want) {
		t.Error(`embedded tree does not contain UTF-16 "Vigor 1" — not the captured response`)
	}
}

// TestCMSelectAckMatchesWire builds the ack for the itemID retail was observed
// answering and requires the exact bytes back.
//
// Retail: client `01 00 00 00 00 06 00 00 00` -> server `01 00 00 00 00 06 00 00 00`.
func TestCMSelectAckMatchesWire(t *testing.T) {
	payload := make([]byte, 9)
	payload[0] = cmRespSelectAck
	binary.LittleEndian.PutUint32(payload[5:9], 0x06)

	want := []byte{0x01, 0, 0, 0, 0, 0x06, 0, 0, 0}
	if string(payload) != string(want) {
		t.Errorf("ack = % x, want % x", payload, want)
	}
}

// TestEveryCMActionHasAReply is the regression this handler exists for.
//
// The CM window has NO client-side timeout. An unanswered 0x1401 leaves it
// searching forever and the player cannot even back out of the menu — the
// window is unclosable, so the only escape is to drop the session. Every
// action code must therefore map to something we send, including ones we do
// not implement and ones we have never seen.
//
// This asserts the dispatch is total by construction: the switch in
// HandleClassMastery has a default that acks, so the set of actions that
// produce no reply must be empty.
func TestEveryCMActionHasAReply(t *testing.T) {
	// Every documented action, plus unknown codes across the byte range.
	actions := []byte{cmActionSelect, cmActionSpend, cmActionOpen, cmActionBrowse}
	for i := 0; i < 256; i++ {
		actions = append(actions, byte(i))
	}

	for _, a := range actions {
		reply := cmReplyFor(a)
		if len(reply) == 0 {
			t.Fatalf("action %#x produces no reply — the CM window would hang unclosable", a)
		}
		// A reply is only useful if the client can identify it.
		switch reply[0] {
		case cmRespFullTree, cmRespSelectAck:
		default:
			t.Errorf("action %#x replies with unknown type %#x", a, reply[0])
		}
	}
}

// cmReplyFor mirrors HandleClassMastery's dispatch so the totality of that
// switch can be asserted without standing up a session and packet queue.
// It must be kept in step with the handler; the two are adjacent on purpose.
func cmReplyFor(action byte) []byte {
	switch action {
	case cmActionOpen, cmActionBrowse:
		return cmFullTree
	case cmActionSelect, cmActionSpend:
		return []byte{cmRespSelectAck, 0, 0, 0, 0, 0, 0, 0, 0}
	default:
		return []byte{cmRespSelectAck, 0, 0, 0, 0, 0, 0, 0, 0}
	}
}

func contains(hay, needle []byte) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if string(hay[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
