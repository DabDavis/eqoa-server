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

// Ticket 2011 through the database: table spellrecastgroup puts Wolf's Instinct
// (book place 7, bar slot 1) and Hunter's Instinct (place 9, slot 2) in one group
// with a third spell the book lacks. A 0x0032 of the wolf locks both of the book
// out, 0x00CD on 7 and 9 with the wolf's 1803.3 s, nothing for the third; a 0x0032
// of the hunter then is refused (the recovery line and its cloud, no 0x00CD, no
// power). The control: a spell in no group (place 12, slot 3) is cast, its own
// 0x00CD alone. RecastGroup reads the group back; "" takes a spell out.
func TestUseSkillLocksTheRecastGroup(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/rg.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Brother", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (701068, 51, 0, 0, 0, 3, 0, 1, 2, 0, 1800, -1, 'Wolf''s Instinct', 'Self, Increase AGI/STR/DEX by 175, 30min')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (760110, 51, 0, 0, 0, 3, 0, 1, 2, 0, 1800, -1, 'Hunter''s Instinct', 'Self, Increase AGI/STR/DEX by 175, 30min')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (650, 3, 0, 0, 0, 2, 0, 1, 2, 0, 3, -1, 'Skin', 'Self, Increase AC by 10, 10min')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range [][3]int{{701068, 7, 1}, {760110, 9, 2}, {650, 12, 3}} {
		if _, err := db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, 1, ?, 0, -1)`,
			rec.ServerID, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
	store := database.NewSpellbookStore(db)
	for _, id := range []int{701068, 760110, 999} {
		if err := store.SetRecastGroup(id, "ranger instinct"); err != nil {
			t.Fatal(err)
		}
	}
	if grp, ids, err := store.RecastGroup(760110); err != nil || grp != "ranger instinct" || len(ids) != 3 {
		t.Fatalf("RecastGroup %q %v %v", grp, ids, err)
	}
	if err := store.SetRecastGroup(999, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRecastGroup(998, "ranger instinct"); err != nil { // the one the book lacks
		t.Fatal(err)
	}
	if grp, ids, err := store.RecastGroup(650); err != nil || grp != "" || ids != nil {
		t.Fatalf("a spell in no group: %q %v %v", grp, ids, err)
	}

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, CharServerID: rec.ServerID, Name: "Brother", Class: 1, Level: 60, HP: 2000, MaxHP: 2000,
		Power: 500, MaxPower: 1025, X: 100, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14330D1E)

	h := NewSpellbookHandler(w, store)
	r := NewRegistry()
	h.RegisterHandlers(r)
	use := func(slot byte) [][]byte {
		payload := build(protocol.OpcodeUseSkill, 16, func(wr *buffer.Writer) {
			wr.WriteUint32(0)
			wr.WriteByte(slot)
			wr.WriteUint32(0xFFFFFFFF)
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
	cooldowns := func(msgs [][]byte) map[byte][]byte {
		out := map[byte][]byte{}
		for _, m := range msgs {
			if protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == protocol.OpcodeCooldownTimer {
				out[m[2]] = m[7:]
			}
		}
		return out
	}

	got := cooldowns(use(1))
	if len(got) != 2 || got[7] == nil || got[9] == nil || string(got[7]) != string(got[9]) {
		t.Fatalf("0x00CD after the wolf % X, want abilities 7 and 9, the same lockout", got)
	}
	if want := zz(1803300); string(got[9]) != string(want) {
		t.Errorf("ability 9's lockout % X, want % X (1803300 ms)", got[9], want)
	}
	power := p.Power
	p.Casting = nil // the wolf's cast time past (a cast within it is let go unanswered)
	msgs := use(2)
	if len(cooldowns(msgs)) != 0 || len(msgs) == 0 {
		t.Fatalf("the hunter inside the group's lockout: % X", msgs)
	}
	if len(msgs) != 2 || protocol.GameOpcode(uint16(msgs[0][0])|uint16(msgs[0][1])<<8) != protocol.OpcodeColoredChat ||
		protocol.GameOpcode(uint16(msgs[1][0])|uint16(msgs[1][1])<<8) != protocol.OpcodeCastingCombat {
		t.Errorf("the refusal % X, want the recovery line and its cloud", msgs)
	}
	if p.Power != power {
		t.Errorf("a refused cast took power %d -> %d", power, p.Power)
	}
	// the control, a spell of no group
	if got := cooldowns(use(3)); len(got) != 1 || got[12] == nil {
		t.Fatalf("a spell in no group: 0x00CD % X, want ability 12 alone", got)
	}
}

// zz is n as 0x00CD writes it (Write7BitEncodedInt64, zigzag).
func zz(n int64) []byte {
	var b [16]byte
	w := buffer.NewWriter(b[:])
	w.Write7BitEncodedInt64(n)
	return append([]byte(nil), b[:w.Position()]...)
}
