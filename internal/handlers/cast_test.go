package handlers

import (
	"bytes"
	"net"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A 0x0032 resolved through the database to the World's cast: the spell on
// the bar slot (not the display index), its record's cast time, recast and
// range, its line and its recorded visual. Answered with Sandstorm's own
// bytes for Fiery Burst (display index 12, cast 2 s, recast 3 s; its ids).
func TestUseSkillCastsTheSlotsSpell(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/cast.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Caster", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (650, 3, 0, 0, 16880, 2, 21, 1, 2, 1, 3, -1, 'Burst', '')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (651, 3, 0, 0, 16880, 2, 21, 1, 2, 1, 3, -1, 'Other', '')`,
		`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (650, 'Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX', 3883934937, 'test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range [][4]int{{651, 1, 1, 6}, {650, 12, 1, 6}} {
		// 651 is memorized nowhere that counts: off the bar
		on := r[2]
		if r[0] == 651 {
			on = 0
		}
		if _, err := db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, ?, ?, 0, -1)`,
			rec.ServerID, r[0], r[1], on, r[3]); err != nil {
			t.Fatal(err)
		}
	}

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, CharServerID: rec.ServerID, Name: "Caster", Class: 7, Level: 23, HP: 536, MaxHP: 536,
		Power: 500, MaxPower: 1025, Dex: 105, X: 100, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14330D1E)
	npc := &entity.NPC{ServerID: 2002, Name: "Rotting Flesh", Level: 1, HP: 300, MaxHP: 300, X: 102, Y: 10, Z: 100}
	w.NPCs[npc.ServerID] = npc
	vs := entity.GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = 0x01312E98

	h := NewSpellbookHandler(w, database.NewSpellbookStore(db))
	r := NewRegistry()
	h.RegisterHandlers(r)
	use := func(slot byte) [][]byte {
		payload := build(protocol.OpcodeUseSkill, 16, func(wr *buffer.Writer) {
			wr.WriteUint32(0)
			wr.WriteByte(slot)
			wr.WriteUint32(0x01312E98)
		})[2:]
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUseSkill)
		msg.SetData(payload)
		fn, ok := r.Get(protocol.OpcodeUseSkill)
		if !ok {
			t.Fatal("no handler for 0x0032")
		}
		if err := fn(sess, msg); err != nil {
			t.Fatal(err)
		}
		var out [][]byte
		for _, m := range sess.Queue().GetPending() {
			out = append(out, append([]byte(nil), m.Data()...))
		}
		return out
	}
	if msgs := use(5); len(msgs) != 0 {
		t.Errorf("an empty slot was answered: %x", msgs)
	}
	msgs := use(6)
	if len(msgs) != 2 {
		t.Fatalf("sent %x", msgs)
	}
	if want := mustHex(t, "cd000c00000000e852"); !bytes.Equal(msgs[0], want) {
		t.Errorf("0x00CD % X, want Sandstorm's % X", msgs[0], want)
	}
	if want := mustHex(t, "b1001e0d3314982e310100d0070000d92480e7"); !bytes.Equal(msgs[1], want) {
		t.Errorf("0x00B1 % X, want Sandstorm's % X", msgs[1], want)
	}
	if p.Power != 479 {
		t.Errorf("power %d, want 479", p.Power)
	}
}
