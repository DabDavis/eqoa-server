package handlers

import (
	"net"
	"sync"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Petition 2146: two 0x0033s for one bar slot handled at once (every packet
// has a goroutine of its own) must leave the book as the client is told it:
// the 0x00B4s applied in the order they were queued, one ability on the slot,
// and a 0x0032 of the slot casting that one. Before booklock.go each read the
// book before the other's write and both went onto the slot; the press cast
// the first of the book's (a player's Bloodwraith, not Shroud of Death +).
func TestConcurrentMemorizeKeepsOneAbilityOnTheSlot(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/book.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Trulytwo", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (700573, 59, 0, 0, 16880, 2, 21, 1, 2, 1, 3, -1, 'Bloodwraith', '')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (901474, 60, 0, 0, 0, 0, 0, 1, 2, 0, 3, -1, 'Shroud of Death +', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, 700573, 63, 0, -1, 0, -1), (?, 901474, 74, 0, -1, 0, -1)`,
		rec.ServerID, rec.ServerID); err != nil {
		t.Fatal(err)
	}
	store := database.NewSpellbookStore(db)
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, CharServerID: rec.ServerID, Name: "Trulytwo", Class: 3, Level: 60}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewSpellbookHandler(w, store)
	r := NewRegistry()
	h.RegisterHandlers(r)
	fn, ok := r.Get(protocol.OpcodeAddSkillToBar)
	if !ok {
		t.Fatal("no handler for 0x0033")
	}
	memorizeMsg := func(index, slot byte) *protocol.Message {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeAddSkillToBar)
		m.SetData(build(protocol.OpcodeAddSkillToBar, 16, func(wr *buffer.Writer) {
			wr.WriteUint32(0)
			wr.WriteByte(index)
			wr.WriteByte(slot)
		})[2:])
		return m
	}

	for round := 0; round < 200; round++ {
		if _, err := db.Exec(`UPDATE spells SET onHotBar = 0, whereonBar = -1 WHERE serverid = ?`, rec.ServerID); err != nil {
			t.Fatal(err)
		}
		sess.Queue().GetPending() // the last round's answers
		var start, done sync.WaitGroup
		start.Add(1)
		for _, idx := range []byte{63, 74} {
			done.Add(1)
			go func(idx byte) {
				defer done.Done()
				start.Wait()
				if err := fn(sess, memorizeMsg(idx, 5)); err != nil {
					t.Error(err)
				}
			}(idx)
		}
		start.Done()
		done.Wait()

		// the client's book: the 0x00B4s applied in their order
		client := []database.SpellRow{
			{SpellID: 700573, AddedOrder: 63, WhereOnHotBar: -1},
			{SpellID: 901474, AddedOrder: 74, WhereOnHotBar: -1},
		}
		for _, m := range sess.Queue().GetPending() {
			d := m.Data()
			if len(d) < 8 || d[0] != 0xB4 {
				continue
			}
			memorize(client, int(d[6]), int(d[7]))
		}
		server, err := store.GetSpellRows(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		on := 0
		for _, s := range server {
			if s.OnHotBar != 0 && s.WhereOnHotBar == 5 {
				on++
			}
			for _, c := range client {
				if c.SpellID == s.SpellID && (c.OnHotBar != s.OnHotBar || (c.OnHotBar != 0 && c.WhereOnHotBar != s.WhereOnHotBar)) {
					t.Fatalf("round %d: the book has %+v, the client %+v", round, s, c)
				}
			}
		}
		if on != 1 {
			t.Fatalf("round %d: %d abilities on slot 5: %+v", round, on, server)
		}
	}
}
