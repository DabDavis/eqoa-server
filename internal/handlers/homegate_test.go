package handlers

import (
	"encoding/binary"
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A homeland's gatekeeper (a world the server composed: the races' homelands)
// flies its people out by its quest alone. Talked to, it opens no travel menu:
// before the starter quests a new character would otherwise have flown on to
// Freeport unbound, or to another race's home. And Harbormaster Thand offers
// no flight into it, even to a game whose table holds the world (the July S2's
// whole table); the control is the same world not composed.
func TestAHomelandGatekeeperOpensNoMenu(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	for id, n := range map[entity.EntityID]*entity.NPC{
		800: {Name: "Harbormaster Thand", World: 0, SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5},
		801: {Name: "Gatekeeper Sulon", World: 2, SpawnX: 7338, SpawnY: 54.2, SpawnZ: 11124},
		805: {Name: "Gatekeeper Windwhistle", World: 10, SpawnX: 9170.9, SpawnY: 84.1, SpawnZ: 21636.7},
	} {
		n.ServerID = id
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		w.NPCs[id] = n
	}
	h := NewInteractHandler(mgr)
	h.SetGameWorld(w)
	talk := func(port byte, npc entity.EntityID) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, port), Port: 7000 + int(port)}, uint32(port), 1, 1, 2, true)
		n := w.NPCs[npc]
		p := &entity.Player{ServerID: entity.EntityID(1000 + int(port)), Name: "Traveller", World: n.World, X: n.X, Y: n.Y, Z: n.Z}
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetDiscVersion(protocol.GameVersionSandstormS2) // the whole table: world 10 too
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = npc, 100002
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteract)
		msg.SetData([]byte{0xA2, 0x86, 0x01, 0x00})
		if err := h.HandleInteract(s, msg); err != nil {
			t.Fatal(err)
		}
		return s
	}
	menuShown := func(s *session.Session) bool {
		for _, m := range s.Queue().GetPending() {
			if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestPopupMulti {
				return true
			}
		}
		return false
	}

	// the control: world 10 not composed, its gatekeeper a stop like any other
	if got := optionBox(t, talk(1, 800)); !slices.Contains(got, "Gatekeeper Windwhistle") {
		t.Fatalf("the control: Thand offers %q, want the gatekeeper of world 10 among them", got)
	}

	entity.SetComposedWorld(10, true)
	defer entity.SetComposedWorld(10, false)
	if got := optionBox(t, talk(2, 800)); slices.Contains(got, "Gatekeeper Windwhistle") || !slices.Contains(got, "Odus") {
		t.Errorf("Thand offers %q: want Odus, and no flight into a homeland", got)
	}
	if menuShown(talk(3, 805)) {
		t.Error("a homeland's gatekeeper opened a travel menu")
	}
}
