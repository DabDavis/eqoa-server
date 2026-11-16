package handlers

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Weapon Enchanter (2026-11-16): the held copy, which of its two auras, an
// aura of the catalog (paged: More), a colour or its own; the copy's
// instanceaura row written and tagged with the auras' names; a second aura
// beside the first, a first chosen again replacing it (two at most); the
// price, a token an aura, taken from the pack, refused with the count when
// short; taking auras off free; nothing held, said so; a held weapon
// swapped since the question, refused.
func TestTheWeaponEnchanter(t *testing.T) {
	t.Cleanup(func() { database.SetInstanceAuraCache(nil); SetEnchanterPrice(-1, "", 0) })
	tw := newTalkWorld(t, 100, 100)
	db, err := database.NewSQLite(t.TempDir() + "/enchant.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	ps := database.NewItemPatternStore(db)
	for _, p := range []database.ItemPatternRecord{
		{PatternID: 900200, ItemName: "keen sword", EquipSlot: 14, MaxHP: 100, MaxStack: 1, ClassUse: 32767, RaceUse: 32767, Rent: 1},
		{PatternID: 900201, ItemName: "Holly Token", MaxHP: 1, MaxStack: 20, ClassUse: 32767, RaceUse: 32767, Rent: 1},
	} {
		if err := ps.UpsertItemPattern(&p); err != nil {
			t.Fatal(err)
		}
	}
	inv := database.NewInventoryStore(db)
	sword, _ := inv.AddCharItem(rec.ServerID, 900200, 1, 100, 0, 0)
	inv.SetEquipLoc(rec.ServerID, sword, 14)
	if _, err := inv.AddCharItem(rec.ServerID, 900201, 2, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	rows, _ := inv.GetCharacterItems(rec.ServerID)
	tw.s.SetInventoryList(newInvList(rows))
	e := NewEnchanter(tw.w, db)
	enchanter := &entity.NPC{ServerID: 3201, Name: "Weapon Enchanter", NPCType: 0x80}
	questMsgs(tw.s)
	SetEnchanterPrice(1, "", 0)

	talk := func() []string {
		t.Helper()
		if !e.Talk(tw.s, tw.p, enchanter) {
			t.Fatal("the enchanter did not answer")
		}
		return questMsgs(tw.s)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		id := boxID(msgs)
		if id == 0 || !IsEnchanterBox(id) {
			t.Fatalf("no enchanter's question in %q", msgs)
		}
		if !e.Answer(tw.s, tw.p, id, n) {
			t.Fatalf("box %08X not the enchanter's", id)
		}
		return questMsgs(tw.s)
	}
	last := func(m []string) string {
		if len(m) == 0 {
			return ""
		}
		return m[len(m)-1]
	}
	row := func() database.InstanceAura {
		t.Helper()
		a, _, err := db.InstanceAuraOf(sword)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	tokens := func() int {
		n := 0
		rows, _ := inv.GetCharacterItems(rec.ServerID)
		for _, r := range rows {
			if r.Pattern.PatternID == 900201 {
				n += r.StackLeft
			}
		}
		return n
	}

	// nothing held
	if got := talk(); !strings.Contains(strings.Join(got, "\n"), "Hold the weapon") {
		t.Fatalf("nothing held: %q", got)
	}
	tw.p.SetWornPiece(entity.AuraMainHand, 900200)
	tw.p.WornItems[entity.AuraMainHand] = sword

	got := talk()
	if !strings.Contains(last(got), ":Which aura of the keen sword?|First aura: none|Second aura: none|Take both auras off") {
		t.Fatalf("which aura: %q", got)
	}
	got = pick(got, 0)
	if !strings.Contains(last(got), ":Which aura?|Virtuous|Freezing|Tainted|Twinkling|Druid Glow|More...") {
		t.Fatalf("the catalog's first page: %q", got)
	}
	got = pick(got, 0) // Virtuous
	if !strings.Contains(last(got), ":Which colour for Virtuous?|Its own colours|Whites, greys and blacks|") || !strings.HasSuffix(last(got), "|More...") {
		t.Fatalf("the colours: %q", got)
	}
	got = pick(got, 0) // its own colours
	if r := row(); r.Tag != "Virtuous" || r.First != (database.ItemAura{Kind: database.AuraEpic, Source: 0x88EC092D}) || r.Source != "enchanter" {
		t.Fatalf("Virtuous: %+v (%q)", r, got)
	}
	if tokens() != 1 {
		t.Fatalf("tokens left: %d", tokens())
	}
	if a := tw.w.PieceAurasOf(tw.p); a == nil || a[entity.AuraMainHand].Source != 0x88EC092D {
		t.Fatalf("not on the sword at once: %+v", a)
	}
	// the second: page two, Shadowed (no colour asked)
	got = pick(talk(), 1)
	got = pick(got, 5) // More...
	if !strings.Contains(last(got), ":Which aura?|Shadowed|Energized|No aura (take it off)|More...") {
		t.Fatalf("the catalog's second page: %q", got)
	}
	got = pick(got, 0)
	if r := row(); r.Tag != "Virtuous, Shadowed" || r.Second != (database.ItemAura{Kind: database.AuraParticles, Color: 0x000000FF}) {
		t.Fatalf("two auras: %+v (%q)", r, got)
	}
	// out of tokens: the count said, nothing written
	got = pick(talk(), 0)
	got = pick(got, 1) // Freezing
	got = pick(got, 1) // whites
	got = pick(got, 0) // white
	if !strings.Contains(strings.Join(got, "\n"), "An aura costs 1 Holly Token, and you carry 0.") || row().Tag != "Virtuous, Shadowed" {
		t.Fatalf("short of tokens: %q %+v", got, row())
	}
	// free: the first chosen again replaces it (two at most), in white
	SetEnchanterPrice(0, "", 0)
	got = pick(talk(), 0)
	got = pick(got, 1) // Freezing
	got = pick(got, 1) // whites
	got = pick(got, 0) // white
	if r := row(); r.Tag != "Freezing, Shadowed" || r.First != (database.ItemAura{Kind: database.AuraParticles, Source: 0x6C151A60, Color: 0xFFFFFFFF}) {
		t.Fatalf("the first replaced: %+v", r)
	}
	// swapped since the question
	got = pick(talk(), 0)
	tw.p.WornItems[entity.AuraMainHand] = sword + 1000
	got = pick(got, 5) // More...
	got = pick(got, 2) // No aura
	if !strings.Contains(strings.Join(got, "\n"), "no longer holding") {
		t.Fatalf("swapped: %q", got)
	}
	tw.p.WornItems[entity.AuraMainHand] = sword
	// both off: the row gone
	pick(talk(), 2)
	if _, has, _ := db.InstanceAuraOf(sword); has {
		t.Fatalf("both off: %+v", row())
	}
	var audits int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'instanceaura'`).Scan(&audits)
	if audits != 4 {
		t.Fatalf("audit lines: %d", audits)
	}
	if e.Talk(tw.s, tw.p, &entity.NPC{Name: "Equipment Dyer"}) {
		t.Fatal("a dyer is no enchanter")
	}
}

// An enchanted copy traded: its aura and tag go with it to the new row, and
// the taker's client is sent the record with "Aura: <tag>" heading its
// description; another copy of the same record stays untagged.
func TestAnEnchantedCopyIsTraded(t *testing.T) {
	t.Cleanup(func() { database.SetInstanceAuraCache(nil) })
	db, err := database.NewSQLite(t.TempDir() + "/trade.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv := database.NewInventoryStore(db)
	sword := 900100
	if err := database.NewItemPatternStore(db).UpsertItemPattern(&database.ItemPatternRecord{PatternID: sword, ItemName: "keen sword",
		EquipSlot: 14, MaxHP: 100, MaxStack: 1, ClassUse: 32767, RaceUse: 32767, Rent: 1, WeaponDamage: 50, AttackType: 1}); err != nil {
		t.Fatal(err)
	}
	id, _ := inv.AddCharItem(11, sword, 1, 100, 0, 0)
	other, _ := inv.AddCharItem(12, sword, 1, 100, 0, 0)
	taint := database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}
	if err := db.SetInstanceAuraAudited(database.InstanceAura{ItemID: id, Tag: "Tainted", First: taint}, database.GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	mk := func(port int, account int) *session.Session {
		s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 22), Port: port}, uint32(port), uint32(port), uint16(port), 2, true)
		s.SetAccountID(account)
		s.SetInGame(true)
		s.SetCharacterInWorld(true)
		s.UpdateActivity()
		return s
	}
	aSess, bSess := mk(7201, 1), mk(7202, 2)
	w.CreatePlayer(aSess, &database.CharacterRecord{ServerID: 11, CharName: "Giver", AccountID: 1, Level: 10, X: 100, Y: 54, Z: 100})
	taker := w.CreatePlayer(bSess, &database.CharacterRecord{ServerID: 12, CharName: "Taker", AccountID: 2, Level: 10, X: 102, Y: 54, Z: 100})
	for _, s := range []struct {
		sess *session.Session
		sid  uint32
	}{{aSess, 11}, {bSess, 12}} {
		rows, _ := inv.GetCharacterItems(s.sid)
		s.sess.SetInventoryList(newInvList(rows))
	}
	h := NewTradeHandler(sm, &NPCWindowHandler{inv: inv})
	h.SetGameWorld(w)
	ui := func(s *session.Session, sub byte, fill func(w *buffer.Writer)) {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeUI)
		b := make([]byte, 64)
		wr := buffer.NewWriter(b)
		wr.WriteByte(sub)
		wr.WriteUint32(0)
		if fill != nil {
			fill(wr)
		}
		msg.SetData(b[:wr.Position()])
		if err := h.HandleTradeUI(s, msg); err != nil {
			t.Fatal(err)
		}
	}
	ask := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeRequest)
	ask.SetData(u32b(0x7F000000 | uint32(taker.ServerID)))
	if err := h.HandleTradeRequest(aSess, ask); err != nil {
		t.Fatal(err)
	}
	ui(bSess, 2, nil)
	ui(aSess, 3, func(w *buffer.Writer) { w.WriteUint32(0); w.Write7BitEncodedInt64(1) })
	ui(aSess, 6, nil)
	ui(bSess, 6, nil)
	theirs, _ := inv.GetCharacterItems(12)
	var got *database.CharInventoryEntry
	for i := range theirs {
		if theirs[i].ItemID != other {
			got = &theirs[i]
		}
	}
	if len(theirs) != 2 || got == nil || got.ItemID == id {
		t.Fatalf("the taker holds %+v", theirs)
	}
	a, ok := database.InstanceAuraHeld(got.ItemID)
	if !ok || a.Tag != "Tainted" || a.First != taint || got.AuraTag != "Tainted" {
		t.Fatalf("the traded copy: %+v %v (%q)", a, ok, got.AuraTag)
	}
	if database.InstanceTag(other) != "" {
		t.Fatal("the taker's other copy tagged")
	}
	sent := false
	for _, m := range bSess.Queue().PeekPending() {
		if bytes.Contains(m.Data(), wideBytes("Aura: Tainted")) {
			sent = true
		}
	}
	if !sent {
		t.Fatal("the taker's client was not sent the tag")
	}
}

// /aura weapon's words: the hand, the tag (tag=, tag <word>, or the first word
// that is no aura word), the aura's words and pairs, second, off, notag; the
// older /aura words (no hand after "weapon") left to /aura.
func TestAuraWeaponWords(t *testing.T) {
	sp, ok, err := parseAuraWeapon(strings.Fields("mainhand Virtuous sword of virtue blue"))
	if !ok || err != nil || sp.Tag != "Virtuous" || sp.Aura == nil || sp.Aura.A.Kind != database.AuraEpic || sp.Aura.A.Source != 0x88EC092D || sp.Aura.A.Color != 0x3060FFFF {
		t.Fatalf("virtuous: %+v %v", sp, err)
	}
	sp, _, err = parseAuraWeapon(strings.Fields("offhand Shadowed kind 3 colour 000000FF"))
	if err != nil || sp.Tag != "Shadowed" || sp.Hands[0] != entity.AuraOffHand || sp.Aura.A != (database.ItemAura{Kind: database.AuraParticles, Color: 0x000000FF}) {
		t.Fatalf("shadowed: %+v %v", sp, err)
	}
	sp, _, err = parseAuraWeapon(strings.Fields("mainhand second particles source=1D875ECD along=100"))
	if err != nil || sp.HasTag || !sp.Aura.Second || sp.Aura.A.Source != 0x1D875ECD || sp.Aura.A.Nodes != 100 {
		t.Fatalf("second: %+v %v", sp, err)
	}
	sp, _, err = parseAuraWeapon(strings.Fields("weapons tag Freezing"))
	if err != nil || len(sp.Hands) != 2 || sp.Tag != "Freezing" || sp.Aura != nil {
		t.Fatalf("a tag alone: %+v %v", sp, err)
	}
	sp, _, err = parseAuraWeapon(strings.Fields("mainhand tag=Druid_Glow epic source=CBF4135A nodes 45"))
	if err != nil || sp.Tag != "Druid Glow" || sp.Aura.A.Nodes != 45 {
		t.Fatalf("druid glow: %+v %v", sp, err)
	}
	if sp, _, err = parseAuraWeapon([]string{"mainhand", "off"}); err != nil || !sp.Off {
		t.Fatalf("off: %+v %v", sp, err)
	}
	if sp, _, err = parseAuraWeapon([]string{"mainhand", "notag"}); err != nil || !sp.HasTag || sp.Tag != "" {
		t.Fatalf("notag: %+v %v", sp, err)
	}
	if sp, _, _ = parseAuraWeapon([]string{"mainhand"}); !sp.Show {
		t.Fatal("nothing but the hand: show")
	}
	for _, bad := range []string{"mainhand Bad|Tag particles", "mainhand Virtuous shell red", "mainhand Virtuous trim gold"} {
		if _, _, err := parseAuraWeapon(strings.Fields(bad)); err == nil {
			t.Errorf("%q read", bad)
		}
	}
	if _, ok, _ := parseAuraWeapon(strings.Fields("epic source=5FD4AE7C")); ok {
		t.Fatal("no hand: not /aura weapon's")
	}
}

// The tag heads a copy's description on the wire, a line of its own; an
// untagged record is as it was; the whole kept to CLIENT's 255.
func TestTheTagHeadsTheDescription(t *testing.T) {
	e := database.CharInventoryEntry{ItemID: 5, StackLeft: 1, AuraTag: "Virtuous",
		Pattern: database.ItemPatternRecord{PatternID: 1, ItemName: "keen sword", ItemDesc: strings.Repeat("x", 300)}}
	b := make([]byte, 2048)
	w := buffer.NewWriter(b)
	writeSingleItem(w, &e, 0, false)
	out := b[:w.Position()]
	if !bytes.Contains(out, wideBytes("Aura: Virtuous\nxxx")) || bytes.Contains(out, wideBytes(strings.Repeat("x", 300-14))) {
		t.Fatal("the tag is not at the head, or the description passed 255")
	}
	e.AuraTag = ""
	w = buffer.NewWriter(b)
	writeSingleItem(w, &e, 0, false)
	if bytes.Contains(b[:w.Position()], wideBytes("Aura:")) {
		t.Fatal("an untagged copy tagged")
	}
}
