package handlers

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// talkWorld is a world with an interact handler and one player standing at
// (x, z) of world 0, its session knowing each NPC added by talkSee.
type talkWorld struct {
	t   *testing.T
	w   *entity.World
	mgr *session.Manager
	h   *InteractHandler
	s   *session.Session
	p   *entity.Player
}

func newTalkWorld(t *testing.T, x, z float32) *talkWorld {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	h := NewInteractHandler(mgr)
	h.SetGameWorld(w)
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7009}, 9, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1009, Name: "Talker", X: x, Y: 54, Z: z, Level: 30}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
	return &talkWorld{t: t, w: w, mgr: mgr, h: h, s: s, p: p}
}

// add puts an NPC in the world, standing at its spawn, on the session's
// channel slot; it returns the object id the session knows it by.
func (tw *talkWorld) add(slot int, n *entity.NPC) uint32 {
	n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
	tw.w.NPCs[n.ServerID] = n
	vs := entity.GetOrCreateVisibility(tw.s)
	id := uint32(100000 + slot)
	vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = n.ServerID, id
	return id
}

// talk is the player's 0x0004 to the object id.
func (tw *talkWorld) talk(id uint32) {
	tw.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteract)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, id)
	msg.SetData(b)
	if err := tw.h.HandleInteract(tw.s, msg); err != nil {
		tw.t.Fatal(err)
	}
}

// pick is the player's 0x0035, option n of the box from the object id.
func (tw *talkWorld) pick(id uint32, n byte) {
	tw.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDialogueBoxOption)
	b := make([]byte, 5)
	binary.LittleEndian.PutUint32(b, id)
	b[4] = n
	msg.SetData(b)
	if err := tw.h.HandleDialogueOption(tw.s, msg); err != nil {
		tw.t.Fatal(err)
	}
}

// opcodes are the messages queued for the session, taken.
func opcodes(s *session.Session) []protocol.GameOpcode {
	var out []protocol.GameOpcode
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, protocol.GameOpcode(binary.LittleEndian.Uint16(d)))
		}
	}
	return out
}

// Talking to an NPC with nothing to say (no words, no route, no shop) does
// nothing (F3 20261025-171528: it opened a box of its server ids, model and
// gear). The control: a greeter with words still answers.
func TestASilentNPCAnswersNothing(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	silent := tw.add(1, &entity.NPC{ServerID: 691, Name: "Vahji Shandun", SpawnX: 25403.2, SpawnY: 54.1, SpawnZ: 15725.3})
	tw.talk(silent)
	if got := opcodes(tw.s); len(got) != 0 {
		t.Errorf("Vahji Shandun, who has nothing to say, answered %v", got)
	}
	SetNPCText(map[string]string{"Sign Post": "Freeport: east"})
	defer SetNPCText(nil)
	sign := tw.add(2, &entity.NPC{ServerID: 692, Name: "Sign Post", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	tw.talk(sign)
	if got := opcodes(tw.s); len(got) != 1 || got[0] != protocol.OpcodeDialogueBox {
		t.Errorf("the control: the sign post answered %v, want its words (a dialogue box)", got)
	}
}

// #npcinfo shows a GM the target's ids, model, size and spawn, in a box; a
// player's is an unknown command, and nothing is shown.
func TestNPCInfoIsAGMs(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	n := &entity.NPC{ServerID: 691, Name: "Vahji Shandun", Level: 21, ModelID: 1893243078, Size: 1,
		SpawnX: 25003, SpawnY: 54.1, SpawnZ: 15005}
	n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
	c.gw.NPCs[n.ServerID] = n
	for _, s := range []*session.Session{gs, ps} {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ServerID, 100107
	}
	gp.TargetObjectID, pp.TargetObjectID = 100107, 100107

	c.say(gs, "#npcinfo")
	var box string
	for _, m := range gs.Queue().GetPending() {
		if d := m.Data(); len(d) > 10 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeDialogueBox {
			box = wideText(d[6:])
		}
	}
	for _, want := range []string{"[Vahji Shandun] ServerID=691 ObjID=100107", "Level=21", "Model=1893243078 Size=1.00", "Spawn=(25003.0, 54.1, 15005.0)"} {
		if !strings.Contains(box, want) {
			t.Errorf("the GM's #npcinfo box lacks %q:\n%s", want, box)
		}
	}
	c.say(ps, "/npcinfo")
	for _, op := range opcodes(ps) {
		if op == protocol.OpcodeDialogueBox {
			t.Error("a player's /npcinfo showed the box")
		}
	}
}

// wideText reads a u32 count and that many UTF-16 units.
func wideText(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	n := int(binary.LittleEndian.Uint32(b))
	var r []rune
	for i := 0; i < n && 4+2*i+1 < len(b); i++ {
		r = append(r, rune(binary.LittleEndian.Uint16(b[4+2*i:])))
	}
	return string(r)
}
