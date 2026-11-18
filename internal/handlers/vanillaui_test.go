package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// vanillaUISwitch turns world.vanilla_ui on for one test, off again after it.
func vanillaUISwitch(t *testing.T) {
	t.Helper()
	SetVanillaUI(true)
	t.Cleanup(func() { SetVanillaUI(false) })
}

// qolOffers is every quality-of-life offer world.vanilla_ui stops.
var qolOffers = []protocol.GameOpcode{
	protocol.OpcodePowerBars, protocol.OpcodePetEffects, protocol.OpcodeBuffWindow,
	protocol.OpcodeNumberTagsOffer, protocol.OpcodeDPSMeterOffer, protocol.OpcodeMobInfoOffer,
	protocol.OpcodeConLevelOffer, protocol.OpcodeQuestMarkers, protocol.OpcodeQuestProgress,
	protocol.OpcodeQuestHeadMarksOffer,
}

// worldEntryOps is what a world entry (two 0x0014s, as a client sends) queues,
// by opcode.
func worldEntryOps(t *testing.T) []protocol.GameOpcode {
	t.Helper()
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	return queuedOps(sess)
}

// Off (Heirloom's, the default), a world entry offers every quality-of-life
// addition as before and nothing of the game's own UI; on, none of them, and
// still nothing of OpcodeVanillaUI (it goes only to a client that answered the
// bug report offer). The world entry's other offers stay either way.
func TestVanillaUIStopsTheWorldEntrysOffers(t *testing.T) {
	SetConLevel(true)
	SetMobInfo(true)
	t.Cleanup(func() { SetConLevel(false); SetMobInfo(false) })

	before := worldEntryOps(t)
	for _, op := range qolOffers {
		if !slices.Contains(before, op) {
			t.Errorf("switch off: %v not offered", op)
		}
	}
	if slices.Contains(before, protocol.OpcodeVanillaUI) {
		t.Fatal("switch off: the game's own UI told")
	}

	vanillaUISwitch(t)
	after := worldEntryOps(t)
	for _, op := range qolOffers {
		if slices.Contains(after, op) {
			t.Errorf("switch on: %v still offered", op)
		}
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeEntityView, protocol.OpcodeBugReportOffer, protocol.OpcodeItemLinksOffer} {
		if !slices.Contains(after, op) {
			t.Errorf("switch on: %v, not a quality-of-life offer, gone", op)
		}
	}
	if slices.Contains(after, protocol.OpcodeVanillaUI) {
		t.Fatal("the game's own UI told before the client answered the bug report offer")
	}
	if len(before)-len(after) != len(qolOffers) {
		t.Fatalf("switch on took %d messages away, want %d", len(before)-len(after), len(qolOffers))
	}
}

// vanillaUISent is every OpcodeVanillaUI queued for s.
func vanillaUISent(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeVanillaUI {
			out = append(out, d)
		}
	}
	return out
}

// A client that answers the bug report offer 1 (vanilla) is told, on a shard
// with the switch, what is off: the opcode, version 1 and every bit. Without
// the switch, or answering 0 (a client without the key), or never answering
// (a PS2), nothing.
func TestVanillaUIToAClientThatAnswered(t *testing.T) {
	_, h, _, as, _ := bugWorld(t)
	yes := []byte{1, 0, 0, 0}
	as.Queue().GetPending()
	h.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, yes))
	if got := vanillaUISent(as); len(got) != 0 {
		t.Fatalf("switch off: sent % x", got)
	}

	vanillaUISwitch(t)
	as.Queue().GetPending()
	h.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, []byte{0, 0, 0, 0}))
	if got := vanillaUISent(as); len(got) != 0 {
		t.Fatalf("an answer of 0: sent % x", got)
	}
	as.Queue().GetPending()
	h.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, yes))
	want := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeVanillaUI))
	want = append(want, 1)
	want = binary.LittleEndian.AppendUint32(want, QoLAllOff)
	if got := vanillaUISent(as); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("sent % x, want one % x", got, want)
	}
	if QoLAllOff != 0x3FF {
		t.Fatalf("bits %#x: vanilla's settings::qol reads ten", QoLAllOff)
	}

	ps2 := entitySlotsSession()
	SendVanillaUI(ps2)
	if got := vanillaUISent(ps2); len(got) != 0 {
		t.Fatal("a client that never answered the bug report offer was sent it")
	}
}

// With the switch, a client's yes to an offer it was never sent (one asking
// from before, a keep restart's) is taken as no: no power, no meter, no tags,
// no monster's details; without it, as before.
func TestVanillaUITakesAnAnswerAsOff(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 8), Port: 6008}, 8, 8, 1, 2, true)
	p := &entity.Player{ServerID: 1001, Name: "Asker"}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	power, tags, mob, meter := NewPowerBarsHandler(w), NewNumberTagsHandler(), &MobInfoHandler{}, NewDPSMeterHandler(w)
	answerAll := func() {
		t.Helper()
		for _, err := range []error{
			power.HandlePowerBars(sess, incoming(protocol.OpcodePowerBars, []byte{1, 0, 0, 0})),
			tags.HandleNumberTags(sess, incoming(protocol.OpcodeNumberTagsAnswer, []byte{2})),
			mob.HandleMobInfoAnswer(sess, incoming(protocol.OpcodeMobInfoAnswer, []byte{1})),
			meter.HandleDPSMeter(sess, incoming(protocol.OpcodeDPSMeterAnswer, []byte{1})),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	answerAll()
	if !p.PowerBars || sess.NumberTagsLayout() != 2 || !sess.MobInfo() {
		t.Fatalf("switch off: power %v, tags %d, mob info %v", p.PowerBars, sess.NumberTagsLayout(), sess.MobInfo())
	}
	vanillaUISwitch(t)
	answerAll()
	if p.PowerBars || sess.NumberTagsLayout() != 0 || sess.MobInfo() {
		t.Fatalf("switch on: power %v, tags %d, mob info %v", p.PowerBars, sess.NumberTagsLayout(), sess.MobInfo())
	}

	// a keep restart's resume into the switch ends what the client had
	SetVanillaUI(false)
	answerAll()
	SetVanillaUI(true)
	stopKeptQoL(w, sess)
	if p.PowerBars || sess.NumberTagsLayout() != 0 || sess.MobInfo() {
		t.Fatalf("resumed: power %v, tags %d, mob info %v", p.PowerBars, sess.NumberTagsLayout(), sess.MobInfo())
	}
}
