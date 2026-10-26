package handlers

import (
	"net"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// F3 20261026-135950: a player who bought a Master Class is shown as one in
// the info window. The 0x0010 reply's first wide line, which CLIENT's
// DrawTargetInfo shows in place of the class, is the master class (one of
// its ranks owned, the first's name bare or not), and the second, shown
// under the name, the guild. The controls: a player with a class mastery and
// no master class, and a creature, are sent neither.
func TestTargetInfoNamesTheMasterClass(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	enc := 1 << 12
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 6634, Category: 3, Classes: enc, Races: 2047, Mastery: "Power Burst", Rank: 1, Name: "Power Burst", Enabled: true},
		{ID: 6801, Category: entity.CMCategoryMaster, Classes: enc, Races: 2047, Mastery: "Spellbinder", Rank: 1, Name: "Spellbinder", Enabled: true},
		{ID: 6802, Category: entity.CMCategoryMaster, Classes: enc, Races: 2047, Mastery: "Spellbinder", Rank: 2, Name: "Spellbinder 2", Parent: 6801, Enabled: true},
		{ID: 6811, Category: entity.CMCategoryMaster, Classes: enc, Races: 2047, Mastery: "Animator", Rank: 1, Name: "Animator 1", Enabled: true},
	})
	player := func(port byte, p *entity.Player, self uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, port), Port: 7000 + int(port)}, uint32(port), 1, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(self)
		return s
	}
	asker := player(8, &entity.Player{ServerID: 1001, Name: "Asker", Level: 60}, 0x1424A1D0)
	player(9, &entity.Player{ServerID: 1002, Name: "Jibrish", Level: 60, Class: 12, Race: 3,
		CMs: map[int]int{6634: 50, 6801: 200, 6802: 75}, GuildName: "Anarchy"}, 0x14330D1E)
	player(10, &entity.Player{ServerID: 1003, Name: "Plain", Level: 60, Class: 12, Race: 3,
		CMs: map[int]int{6634: 50}}, 0x14330D1F)
	player(11, &entity.Player{ServerID: 1004, Name: "Animated", Level: 60, Class: 12, Race: 3,
		CMs: map[int]int{6811: 200}}, 0x14330D20)
	orc := &entity.NPC{ServerID: 2001, Name: "an orc", Level: 60, HP: 100, MaxHP: 100}
	w.NPCs[orc.ServerID] = orc
	vs := entity.GetOrCreateVisibility(asker)
	for ch, id := range []entity.EntityID{1002, 1003, 1004, orc.ServerID} {
		vs.ChannelEntity[ch+1], vs.ChannelObjectID[ch+1] = id, uint32(100001+ch)
	}

	h := NewTargetHandler(mgr)
	h.SetGameWorld(w)
	r := NewRegistry()
	h.RegisterHandlers(r)
	fn, ok := r.Get(protocol.OpcodeClassicTargetInfo)
	if !ok {
		t.Fatal("no handler for 0x0010")
	}
	// the reply's two wide lines for a target, as Msg0010 reads them
	lines := func(id uint32) (title, guild string) {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassicTargetInfo)
		msg.SetData(build(protocol.OpcodeClassicTargetInfo, 16, func(wr *buffer.Writer) { wr.WriteUint32(7); wr.WriteUint32(id) })[2:])
		if err := fn(asker, msg); err != nil {
			t.Fatal(err)
		}
		var reply []byte
		for _, m := range asker.Queue().GetPending() {
			if d := m.Data(); len(d) >= 2 && d[0] == 0x10 && d[1] == 0x00 {
				reply = append([]byte(nil), d...)
			}
		}
		if len(reply) != 265 {
			t.Fatalf("target %d: no 265-byte 0x0010 reply (%d bytes)", id, len(reply))
		}
		fixed := func(at int) string {
			u := make([]uint16, 64)
			for i := range u {
				u[i] = uint16(reply[at+2*i]) | uint16(reply[at+2*i+1])<<8
			}
			n := 0
			for n < len(u) && u[n] != 0 {
				n++
			}
			return string(utf16.Decode(u[:n]))
		}
		return fixed(2 + 3), fixed(2 + 3 + 128)
	}

	if ti, g := lines(100001); ti != "Spellbinder" || g != "Anarchy" {
		t.Fatalf("a Spellbinder in a guild: title %q guild %q, want Spellbinder and Anarchy", ti, g)
	}
	if ti, g := lines(100002); ti != "" || g != "" {
		t.Fatalf("a class mastery and no master class: title %q guild %q, want both empty (the class word shows)", ti, g)
	}
	if ti, _ := lines(100003); ti != "Animator" {
		t.Fatalf("a master class whose first rank keeps its number: title %q, want the mastery's name Animator", ti)
	}
	if ti, g := lines(100004); ti != "" || g != "" {
		t.Fatalf("a creature: title %q guild %q, want both empty", ti, g)
	}
}
