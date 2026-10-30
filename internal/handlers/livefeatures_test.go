package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// The feature word is every feature this server has, ORed in one place: the
// auction's menu, Ogre and Alchemist, the auction's stacks and duels, which is
// the 0x1B both retail save states hold (report.md section 2); bit 0x04 and
// those above 0x10 are unread by the live client and stay clear.
func TestLiveFeatureWord(t *testing.T) {
	if got := liveFeatures(); got != 0x1B {
		t.Fatalf("the feature word %#x, want retail's 0x1B", got)
	}
	for _, bit := range []uint32{LiveFeatureAuctionMenu, LiveFeatureExpansion, LiveFeatureAuctionStacks, LiveFeatureDuels} {
		if liveFeatures()&bit == 0 {
			t.Errorf("bit %#x missing", bit)
		}
	}
	if liveFeatures()&^uint32(0x1B) != 0 {
		t.Errorf("a bit the live client does not read is set: %#x", liveFeatures())
	}
}

// The first 0x0014 sends the word once and offers the live auction forms
// (0x7E0B, a u32 1) once, the word first (a client answers the offer by
// what the word's bit 0x08 told it); the client's later 0x0014s send
// neither again.
func TestLiveFeaturesAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 3 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queued(sess, protocol.OpcodeLiveFeatures), []byte{0xF5, 0x07, 0x1B, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the feature word %x, want one %x", got, want)
	}
	if got, want := queued(sess, protocol.OpcodeAuctionForm), []byte{0x0B, 0x7E, 1, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the auction form offer %x, want one %x", got, want)
	}
	word, offer := -1, -1
	for i, op := range queuedOps(sess) {
		switch op {
		case protocol.OpcodeLiveFeatures:
			word = i
		case protocol.OpcodeAuctionForm:
			offer = i
		}
	}
	if word < 0 || offer < 0 || word > offer {
		t.Errorf("the word is message %d and the offer %d: the word must go first", word, offer)
	}
}
