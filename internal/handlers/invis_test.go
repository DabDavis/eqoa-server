package handlers

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The user, 2026-11-02: "invisibility was dropping when emoting, pressing
// square, but only should drop if casting a spell, or attacking a mob".
// Through the registry as the server wires it (RegisterDefaultHandlers),
// the four emotes, square at a creature (0x0004, talking to it) and the
// other messages that ended it from F3 20261026-014527 to that day (an item
// used, a loot, a pet command, a merchant, a trade) leave an invisible
// player invisible; the attack button at the creature ends it (the
// control; a cast's end is entity's TestInvisibilityEndsWithACastOrAnAttack).
func TestOnlyACastOrAnAttackEndsInvisibility(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/invis.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	r := NewRegistry()
	RegisterDefaultHandlers(r, database.NewSQLiteCharacterStore(db), database.NewSQLiteAccountStore(db),
		mgr, "127.0.0.1", w, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	// the registry starts the quest, trigger and auction engines, which are
	// the process's: stopped here, or every later test (and -count's next
	// run) finds them running over this closed database
	t.Cleanup(func() {
		if q := Quests(); q != nil {
			q.Stop()
		}
		if e := Triggers(); e != nil {
			e.Stop()
		}
		if a := Auctions(); a != nil {
			a.Stop()
		}
	})
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 12), Port: 7003}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, Name: "Hider", Class: 6, Level: 20, HP: 400, MaxHP: 400, X: 100, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x00AA0002)
	npc := &entity.NPC{ServerID: 2003, Name: "Mire Toad", Level: 18, HP: 5000, MaxHP: 5000, X: 103, Y: 10, Z: 100}
	w.NPCs[npc.ServerID] = npc
	const npcO = 0x0131_4022
	vs := entity.GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = npcO

	invisible := func() bool {
		for _, e := range p.Effects {
			if e.Invis {
				return true
			}
		}
		return false
	}
	hide := func() {
		p.Effects = append(p.Effects, &entity.ActiveEffect{SpellID: 9000, Name: "Invisibility",
			Invis: true, Until: time.Now().Add(time.Minute)})
	}
	at := func(op protocol.GameOpcode) []byte {
		return build(op, 8, func(wr *buffer.Writer) { wr.WriteUint32(npcO) })[2:]
	}
	send := func(op protocol.GameOpcode, data []byte) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
		msg.SetData(data)
		_ = r.Handle(sess, msg) // a handler's own refusal is not this test's
	}

	hide()
	for _, c := range []struct {
		what string
		op   protocol.GameOpcode
		data []byte
	}{
		{"a wave", protocol.OpcodeEmoteWave, nil},
		{"a bow", protocol.OpcodeEmoteBow, nil},
		{"a point", protocol.OpcodeEmotePoint, nil},
		{"a cheer", protocol.OpcodeEmoteCheer, nil},
		{"square at the creature", protocol.OpcodeInteract, at(protocol.OpcodeInteract)},
		{"an item used", protocol.OpcodeInteractItem, nil},
		{"a loot", protocol.OpcodeLootCorpse, at(protocol.OpcodeLootCorpse)},
		{"a pet command", protocol.OpcodePetCommands, nil},
		{"a merchant", protocol.OpcodeMerchantDiag, at(protocol.OpcodeMerchantDiag)},
		{"a trade", protocol.OpcodeTradeRequest, at(protocol.OpcodeTradeRequest)},
	} {
		send(c.op, c.data)
		if !invisible() {
			t.Errorf("%s (0x%04X) ended the invisibility", c.what, uint16(c.op))
			hide()
		}
	}
	send(protocol.OpcodeAttack, at(protocol.OpcodeAttack))
	if invisible() {
		t.Error("the attack button at the creature left the player invisible")
	}
}
