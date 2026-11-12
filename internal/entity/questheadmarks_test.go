package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// headSets takes the OpcodeQuestHeadMarks bodies queued for s since the last
// call.
func headSets(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestHeadMarks {
			out = append(out, d[2:])
		}
	}
	s.Queue().Clear()
	return out
}

// headWorld is a world with one player in it on a session, its client's
// slots 0-3 empty.
func headWorld(t *testing.T) (*World, *session.Session, *Player) {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7007}, 7, 1, 1, 2, true)
	p := &Player{ServerID: 1007, CharServerID: 77, Name: "Marked", Level: 1}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	GetOrCreateVisibility(s)
	return w, s, p
}

// onSlot puts an NPC in the world on slot i of s's view, known by id.
func onSlot(w *World, s *session.Session, i int, n *NPC, id uint32) {
	w.NPCs[n.ServerID] = n
	vs := GetOrCreateVisibility(s)
	vs.ChannelEntity[i], vs.ChannelObjectID[i] = n.ServerID, id
}

// The marks go by the view: each marked NPC on a slot by the id the client
// knows it by, "?" by spawn before its name's, none for an NPC unmarked or
// dead; the set is sent when it changes (an NPC comes into view, the engine's
// marks change, a corpse) and not again while nothing does. The controls: a
// client that never asked is sent nothing, and one that stopped nothing more.
func TestQuestHeadMarksGoByTheView(t *testing.T) {
	w, s, p := headWorld(t)
	onSlot(w, s, 0, &NPC{ServerID: 501, Name: "Commander Nothard"}, 100001)
	onSlot(w, s, 1, &NPC{ServerID: 502, Name: "Merchant Galosh"}, 100002)
	onSlot(w, s, 2, &NPC{ServerID: 503, Name: "a guard"}, 100003)
	w.SetQuestHeadMarks(p.CharServerID, &QuestHeadMarks{Names: map[string]byte{"commander nothard": QuestMarkTake}})

	w.SendQuestHeadMarks(s)
	if got := headSets(s); len(got) != 0 {
		t.Fatalf("the control: a client that never asked was sent %x", got)
	}
	if w.SetQuestHeadMarksWanted(s, true) != p {
		t.Fatal("the answer found no player")
	}
	w.SendQuestHeadMarks(s)
	want := EncodeQuestHeadMarks([]uint32{100001}, []byte{QuestMarkTake})
	if got := headSets(s); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("asked: %x, want %x", got, want)
	}
	if !bytes.Equal(want, []byte{1, 0, 0xA1, 0x86, 0x01, 0x00, 1}) {
		t.Fatalf("the layout: %x", want)
	}
	w.SendQuestHeadMarks(s)
	if got := headSets(s); len(got) != 0 {
		t.Fatalf("nothing changed, sent again: %x", got)
	}

	// the quest taken: "?" over Galosh, none over Nothard; a pinned spawn's
	// "?" over that spawn alone (another Galosh coming into view has none)
	w.SetQuestHeadMarks(p.CharServerID, &QuestHeadMarks{Names: map[string]byte{"merchant galosh": QuestMarkStep, "a guard": QuestMarkTake},
		Spawns: map[EntityID]byte{503: QuestMarkStep}})
	onSlot(w, s, 3, &NPC{ServerID: 504, Name: "Merchant Galosh "}, 100004)
	w.SendQuestHeadMarks(s)
	want = EncodeQuestHeadMarks([]uint32{100002, 100003, 100004}, []byte{QuestMarkStep, QuestMarkStep, QuestMarkStep})
	if got := headSets(s); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("taken: %x, want %x", got, want)
	}

	// a corpse has none; out of view, none
	w.NPCs[502].Dead = true
	vs := GetOrCreateVisibility(s)
	vs.ChannelEntity[3], vs.ChannelObjectID[3] = 0, 0
	w.SendQuestHeadMarks(s)
	want = EncodeQuestHeadMarks([]uint32{100003}, []byte{QuestMarkStep})
	if got := headSets(s); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("dead and gone: %x, want %x", got, want)
	}

	// the engine forgets them: the empty set, once
	w.SetQuestHeadMarks(p.CharServerID, nil)
	w.SendQuestHeadMarks(s)
	if got := headSets(s); len(got) != 1 || !bytes.Equal(got[0], []byte{0, 0}) {
		t.Fatalf("forgotten: %x", got)
	}

	// stopped: nothing more, whatever changes
	w.SetQuestHeadMarks(p.CharServerID, &QuestHeadMarks{Names: map[string]byte{"commander nothard": QuestMarkTake}})
	w.SetQuestHeadMarksWanted(s, false)
	w.SendQuestHeadMarks(s)
	if got := headSets(s); len(got) != 0 {
		t.Fatalf("the control: stopped, sent %x", got)
	}
	// asked again: the set at once, whatever was sent before
	w.WantQuestHeadMarks(p, true)
	w.SendQuestHeadMarks(s)
	if got := headSets(s); len(got) != 1 || !bytes.Equal(got[0], EncodeQuestHeadMarks([]uint32{100001}, []byte{QuestMarkTake})) {
		t.Fatalf("asked again: %x", got)
	}
}
