package handlers

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A cape dyed alone, not with the rest as "All my armor" dyes it (the owner,
// 2026-11-09: "i dont see the cape changing colors on its own, only when you
// select all armor"): from the dyer's answer, or #dye, to the capes its client
// is sent next (entity/capes.go). Item names are made up.

// capeSetsSent takes the capes-in-view sets queued for s: OpcodeCapesInView's
// bodies, a long one put back together from its fragments.
func capeSetsSent(s *session.Session) [][]byte {
	var out [][]byte
	var frags []byte
	for _, m := range s.Queue().GetPending() {
		if m.Type == protocol.MessageTypeSegmentReliable {
			frags = append(frags, m.Data()...)
			continue
		}
		d := append(frags, m.Data()...)
		frags = nil
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCapesInView {
			out = append(out, d[2:])
		}
	}
	return out
}

// oneCape is a set of one cape: the wearer's id, its style and its colour.
func oneCape(id uint32, style uint8, color uint32) []byte {
	b := append(binary.LittleEndian.AppendUint32([]byte{1}, id), style)
	return binary.LittleEndian.AppendUint32(b, color)
}

// A cape dyed alone goes out at once in its dye: the Armor Dyer's "Cape" (the
// second answer of "Robe, cape, shield or weapons") and #dye cape each put the
// dye on the character where it stands, and the next tick sends its client its
// capes in view with the cape in the new colour. The controls: the robe dyed
// alone sends no capes, nor does a tick with nothing changed.
func TestACapeDyedAloneGoesOutAtOnce(t *testing.T) {
	capesOn(t)
	tw := newTalkWorld(t, 100, 100)
	capeSession(tw.s)
	tw.s.SetPlayerObjectID(4242)
	db, err := database.NewSQLite(t.TempDir() + "/dyer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	tw.p.CapeStyle = 2
	d := NewDyer(tw.w, db)
	dyer := &entity.NPC{ServerID: 3101, Name: "Armor Dyer", NPCType: 0x80}
	tick := func() [][]byte {
		t.Helper()
		tw.w.Tick(time.Now())
		return capeSetsSent(tw.s)
	}
	// which: the robe's question's answer (0 the robe, 1 the cape), dyed
	// Reds and pinks' Crimson
	dyeAlone := func(which byte) {
		t.Helper()
		if !d.Talk(tw.s, tw.p, dyer) {
			t.Fatal("the dyer did not answer")
		}
		msgs := questMsgs(tw.s)
		for _, n := range []byte{4, which, 1, 1} {
			if !d.Answer(tw.s, tw.p, boxID(msgs), n) {
				t.Fatalf("not the dyer's box: %q", msgs)
			}
			msgs = questMsgs(tw.s)
		}
		if !hasMsg(msgs, "box:Armor Dyer: There: crimson.") {
			t.Fatalf("the dyer's last word: %q", msgs)
		}
	}

	if got, want := tick(), oneCape(4242, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the first set: %x, want %x", got, want)
	}
	dyeAlone(0)
	if got := tick(); len(got) != 0 {
		t.Fatalf("the control: the robe dyed alone sent capes %x", got)
	}
	dyeAlone(1)
	if got, want := tick(), oneCape(4242, 2, 0xA0001EFF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the cape dyed alone at the dyer: %x, want %x (crimson)", got, want)
	}
	if got := tick(); len(got) != 0 {
		t.Fatalf("the control: nothing changed, sent again: %x", got)
	}

	// #dye cape, on the GM's own cape (no target)
	c, gs, gp, _, _ := gmWorld(t)
	store, _ := gmRows(t, c)
	c.h.SetDyeStore(store.DB())
	capeSession(gs)
	gs.SetPlayerObjectID(5555)
	gp.CapeStyle, gp.CapeColor = 5, 0xC01010FF
	c.gw.Tick(time.Now())
	if got, want := capeSetsSent(gs), oneCape(5555, 5, 0xC01010FF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the GM's first set: %x, want %x (the record's colour)", got, want)
	}
	c.say(gs, "#dye cape 2050e0")
	if got := gmReplies(t, gs); !hasLine(got, "cape: 2050E0FF") {
		t.Fatalf("#dye cape said %q", got)
	}
	c.gw.Tick(time.Now())
	if got, want := capeSetsSent(gs), oneCape(5555, 5, 0x2050E0FF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("#dye cape: %x, want %x (the dye)", got, want)
	}
}

// "My cape", a dyer's third answer to "What shall I dye?" on a server with
// capes (the owner, 2026-11-09: "up front"), goes straight to the colours and
// dyes the cape alone: chardyes' cape slot and no other, and the next tick's
// capes in view in the dye. CLIENT's answer box shows three answers a page, so
// it ends the first page, and the robe's question and "Take every dye off" are
// on the second. The control: without capes the question is its four answers
// as it was, and its third answer the robe's question, with no cape in it.
func TestMyCapeGoesStraightToTheColours(t *testing.T) {
	capesOn(t)
	tw := newTalkWorld(t, 100, 100)
	capeSession(tw.s)
	tw.s.SetPlayerObjectID(4242)
	db, err := database.NewSQLite(t.TempDir() + "/dyer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	tw.p.CapeStyle = 2
	d := NewDyer(tw.w, db)
	dyer := &entity.NPC{ServerID: 3101, Name: "Armor Dyer", NPCType: 0x80}
	talk := func() []string {
		t.Helper()
		if !d.Talk(tw.s, tw.p, dyer) {
			t.Fatal("the dyer did not answer")
		}
		return questMsgs(tw.s)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		if !d.Answer(tw.s, tw.p, boxID(msgs), n) {
			t.Fatalf("not the dyer's box: %q", msgs)
		}
		return questMsgs(tw.s)
	}
	last := func(msgs []string) string {
		if len(msgs) == 0 {
			return ""
		}
		return msgs[len(msgs)-1]
	}
	var families []string
	for _, f := range database.DyeFamilies {
		families = append(families, f.Label)
	}
	tw.w.Tick(time.Now())
	if got, want := capeSetsSent(tw.s), oneCape(4242, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the first set: %x, want %x", got, want)
	}

	got := talk()
	if !strings.HasSuffix(last(got), ":What shall I dye?|All my armor|One piece of armor|My cape|Take every dye off|Robe, cape, shield or weapons") {
		t.Fatalf("the first question with capes: %q", got)
	}
	got = pick(got, 2)
	if !strings.HasSuffix(last(got), ":Which colors for my cape?|"+strings.Join(families, "|")) {
		t.Fatalf("My cape: %q, want the colours' families at once", got)
	}
	got = pick(got, 1) // Reds and pinks
	if !strings.Contains(last(got), ":Which color for my cape?|") {
		t.Fatalf("the family's colours: %q", got)
	}
	if got = pick(got, 1); !hasMsg(got, "box:Armor Dyer: There: crimson.") { // Crimson
		t.Fatalf("the dyer's last word: %q", got)
	}
	dyes, err := db.CharDyes(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	for slot, c := range dyes {
		if want := map[bool]uint32{true: 0xA0001EFF}[slot == database.DyeCape]; c != want {
			t.Fatalf("My cape dyed %s %08X, want %08X", database.DyeSlotNames[slot], c, want)
		}
	}
	tw.w.Tick(time.Now())
	if got, want := capeSetsSent(tw.s), oneCape(4242, 2, 0xA0001EFF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("My cape's dye: %x, want %x (crimson)", got, want)
	}

	entity.SetCapes(false)
	got = talk()
	if !strings.HasSuffix(last(got), ":What shall I dye?|All my armor|One piece of armor|Robe, shield or weapons|Take every dye off") {
		t.Fatalf("the control, no capes: %q, want the four answers as they were", got)
	}
	if got = pick(got, 2); !strings.HasSuffix(last(got), ":Which one?|Robe|Shield|Primary weapon|Secondary weapon") {
		t.Fatalf("the control: the third answer without capes asked %q, want the robe's question", got)
	}
}
