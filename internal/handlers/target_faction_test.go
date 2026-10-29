package handlers

import (
	"net"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The 0x0010 reply's disposition for an NPC of a faction is the asker's
// standing with it (entity/aggro.go): a dark elf's start with Citizens of
// Neriak (factionstart), then its own standings, each band's face; with
// world.faction_aggro false the NPC's kind's face, as before.
func TestTargetFaceFollowsTheStanding(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7008}, 8, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, Name: "Asker", Level: 2, Race: 2, Class: 3}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1424A1D0)
	ghoul := &entity.NPC{ServerID: 2001, Name: "ghoul servant", Level: 55, HP: 100, MaxHP: 100,
		ModelID: 0x2C2C2C2C, Faction: entity.FactionNeutral}
	w.NPCs[ghoul.ServerID] = ghoul
	vs := entity.GetOrCreateVisibility(sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = ghoul.ServerID, 100001
	w.SetNPCFactions([]database.NPCFactionRow{{Name: "Ghoul Servant", World: -1, Faction: "Citizens of Neriak"}})
	w.SetFactionStarts([]database.FactionStartRow{{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Neriak", Standing: 1000}})

	h := NewTargetHandler(mgr)
	h.SetGameWorld(w)
	r := NewRegistry()
	h.RegisterHandlers(r)
	fn, _ := r.Get(protocol.OpcodeClassicTargetInfo)
	face := func() byte {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassicTargetInfo)
		msg.SetData(build(protocol.OpcodeClassicTargetInfo, 16, func(wr *buffer.Writer) { wr.WriteUint32(7); wr.WriteUint32(100001) })[2:])
		if err := fn(sess, msg); err != nil {
			t.Fatal(err)
		}
		var reply []byte
		for _, m := range sess.Queue().GetPending() {
			if d := m.Data(); len(d) >= 3 && d[0] == 0x10 && d[1] == 0x00 {
				reply = d
			}
		}
		if reply == nil {
			t.Fatal("no 0x0010 reply")
		}
		return reply[2]
	}
	if got := face(); got != entity.DispAllied {
		t.Errorf("a dark elf starting at 1000: face %d, want Allied %d", got, entity.DispAllied)
	}
	for _, c := range []struct {
		standing int
		want     byte
	}{{-2000, entity.DispAggressive}, {-500, entity.DispDubious}, {-200, entity.DispCautious}, {0, entity.DispNeutral},
		{200, entity.DispFriendly}, {400, entity.DispWarm}} {
		p.Factions = map[string]int{"Citizens of Neriak": c.standing}
		if got := face(); got != c.want {
			t.Errorf("standing %d: face %d, want %d", c.standing, got, c.want)
		}
	}
	w.SetFactionAggro(false)
	if got := face(); got != entity.FaceNeutral {
		t.Errorf("world.faction_aggro false: face %d, want the kind's %d", got, entity.FaceNeutral)
	}
}
