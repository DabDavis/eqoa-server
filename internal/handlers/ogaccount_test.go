package handlers

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// ogSwitch turns world.og_accounts on for one test, off again after it.
func ogSwitch(t *testing.T) {
	t.Helper()
	SetOGAccounts(true)
	t.Cleanup(func() { SetOGAccounts(false) })
}

// worldEntryOpsOf is what a world entry queues for sess, by opcode.
func worldEntryOpsOf(t *testing.T, sess *session.Session) []protocol.GameOpcode {
	t.Helper()
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	return queuedOps(sess)
}

// An [OG] account's session (world.og_accounts, the shard's vanilla_ui unset)
// is offered none of the quality-of-life additions and no item links; another
// account's session on the same shard is offered every one, as before.
func TestOGSessionStopsItsOwnOffers(t *testing.T) {
	SetConLevel(true)
	SetMobInfo(true)
	t.Cleanup(func() { SetConLevel(false); SetMobInfo(false) })
	ogSwitch(t)

	plain := worldEntryOpsOf(t, entitySlotsSession())
	og := entitySlotsSession()
	og.SetOG(true)
	mine := worldEntryOpsOf(t, og)
	for _, op := range append(slices.Clone(qolOffers), protocol.OpcodeItemLinksOffer) {
		if !slices.Contains(plain, op) {
			t.Errorf("another account's session: %v not offered", op)
		}
		if slices.Contains(mine, op) {
			t.Errorf("the [OG] session: %v offered", op)
		}
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeEntityView, protocol.OpcodeBugReportOffer, protocol.OpcodeProfanityChoice} {
		if !slices.Contains(mine, op) {
			t.Errorf("the [OG] session: %v, not a quality-of-life offer, gone", op)
		}
	}
	if len(plain)-len(mine) != len(qolOffers)+1 {
		t.Fatalf("the [OG] session lost %d messages, want %d", len(plain)-len(mine), len(qolOffers)+1)
	}
}

// A vanilla client of an [OG] account (it answered the bug report offer) is
// told QoLOGOff: every addition, the map and links locked, no mouse, no touch;
// version 1 as vanilla's net::session reads it. Another account's on the same
// shard is told nothing (vanilla_ui unset); on a shard with vanilla_ui too, it
// is told QoLAllOff and the [OG] one still QoLOGOff.
func TestOGToAClientThatAnswered(t *testing.T) {
	_, h, _, as, bs := bugWorld(t)
	ogSwitch(t)
	as.SetOG(true)
	yes := []byte{1, 0, 0, 0}
	told := func(s *session.Session) [][]byte {
		s.Queue().GetPending()
		h.handleBugReportOffer(s, incoming(protocol.OpcodeBugReportOffer, yes))
		return vanillaUISent(s)
	}
	msg := func(bits uint32) []byte {
		b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeVanillaUI))
		return binary.LittleEndian.AppendUint32(append(b, 1), bits)
	}
	if got := told(as); len(got) != 1 || !bytes.Equal(got[0], msg(QoLOGOff)) {
		t.Fatalf("the [OG] session: % x, want % x", got, msg(QoLOGOff))
	}
	if QoLOGOff != 0x7FFF || QoLLocked != 1<<12 || QoLNoMouse != 1<<13 || QoLNoTouch != 1<<14 {
		t.Fatalf("bits %#x: vanilla's settings::qol reads fifteen", QoLOGOff)
	}
	if got := told(bs); len(got) != 0 {
		t.Fatalf("another account's session: % x", got)
	}
	vanillaUISwitch(t)
	if got := told(bs); len(got) != 1 || !bytes.Equal(got[0], msg(QoLAllOff)) {
		t.Fatalf("vanilla_ui, another account's: % x", got)
	}
	if got := told(as); len(got) != 1 || !bytes.Equal(got[0], msg(QoLOGOff)) {
		t.Fatalf("vanilla_ui, the [OG] session: % x", got)
	}
	// a PS2 of an [OG] account is told nothing (it never answers the offer,
	// and has none of it)
	ps2 := entitySlotsSession()
	ps2.SetOG(true)
	SendVanillaUI(ps2)
	if got := vanillaUISent(ps2); len(got) != 0 {
		t.Fatal("a client that never answered was sent it")
	}
}

// The [OG] session's answers are taken as off (item links too, locked), its
// /con is no command; another session's on the same shard as ever.
func TestOGSessionsAnswersAndCon(t *testing.T) {
	SetConLevel(true)
	t.Cleanup(func() { SetConLevel(false) })
	ogSwitch(t)
	c := newChatWorld(t)
	og, po := c.join(1, "Meshuggah", 25000, 16000, 0)
	plain, pp := c.join(2, "Bob", 25010, 16000, 0)
	og.SetOG(true)
	power, tags, meter := NewPowerBarsHandler(c.gw), NewNumberTagsHandler(), NewDPSMeterHandler(c.gw)
	for _, s := range []*session.Session{og, plain} {
		for _, err := range []error{
			power.HandlePowerBars(s, incoming(protocol.OpcodePowerBars, []byte{1, 0, 0, 0})),
			tags.HandleNumberTags(s, incoming(protocol.OpcodeNumberTagsAnswer, []byte{2})),
			meter.HandleDPSMeter(s, incoming(protocol.OpcodeDPSMeterAnswer, []byte{1})),
			c.h.HandleItemLinksAnswer(s, incoming(protocol.OpcodeItemLinksOffer, []byte{1})),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if po.PowerBars || og.NumberTagsLayout() != 0 || og.ItemLinks() {
		t.Fatalf("[OG]: power %v, tags %d, links %v", po.PowerBars, og.NumberTagsLayout(), og.ItemLinks())
	}
	if !pp.PowerBars || plain.NumberTagsLayout() != 2 || !plain.ItemLinks() {
		t.Fatalf("another: power %v, tags %d, links %v", pp.PowerBars, plain.NumberTagsLayout(), plain.ItemLinks())
	}
	if ConLevelOnFor(og) || !ConLevelOnFor(plain) {
		t.Fatalf("/con: [OG] %v, another %v", ConLevelOnFor(og), ConLevelOnFor(plain))
	}
	// a keep restart's resume ends what the [OG] session had
	og.SetItemLinks(true)
	og.SetNumberTagsLayout(2)
	stopKeptQoL(c.gw, og)
	if og.ItemLinks() || og.NumberTagsLayout() != 0 {
		t.Fatal("resumed: the [OG] session keeps its additions")
	}
}

// The choice is read from the account at a character's entry, only where
// world.og_accounts is set; the session and the player both get it, and the
// player is drawn "[OG] Name". The control: the switch off, nothing read,
// nothing drawn.
func TestApplyOGReadsTheAccount(t *testing.T) {
	db, store := newSafeDB(t)
	accs := database.NewSQLiteAccountStore(db)
	ogAcc, err := accs.Create("ogplayer", "x")
	if err != nil {
		t.Fatal(err)
	}
	plainAcc, err := accs.Create("plainplayer", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := accs.SetAccountOG(ogAcc.ID, true); err != nil {
		t.Fatal(err)
	}
	c := newChatWorld(t)
	h := &CharacterHandler{store: store, gameWorld: c.gw}
	og, po := c.join(1, "Meshuggah", 25000, 16000, 0)
	plain, pp := c.join(2, "Bob", 25010, 16000, 0)
	og.SetAccountID(ogAcc.ID)
	plain.SetAccountID(plainAcc.ID)

	h.applyOG(og, po)
	if og.OG() || po.OG || po.ShownName() != "Meshuggah" {
		t.Fatalf("switch off: session %v, player %v, drawn %q", og.OG(), po.OG, po.ShownName())
	}
	ogSwitch(t)
	h.applyOG(og, po)
	h.applyOG(plain, pp)
	if !og.OG() || !po.OG || po.ShownName() != "[OG] Meshuggah" {
		t.Fatalf("switch on: session %v, player %v, drawn %q", og.OG(), po.OG, po.ShownName())
	}
	if plain.OG() || pp.OG || pp.ShownName() != "Bob" {
		t.Fatalf("another account: session %v, drawn %q", plain.OG(), pp.ShownName())
	}
	// a GM's [OG] account: both tags joined, the GM's first (entity/hardcore.go)
	po.GM = true
	if got := po.ShownName(); got != "[GM][OG] Meshuggah" {
		t.Fatalf("a GM's: %q", got)
	}
	po.GM = false
}

// The name stays the character's own for everything keyed by it: a tell by
// the plain name, by the name as drawn ("/t [OG] Meshuggah ..."), /r's name
// (0x07A3) and the line the hearer reads are the plain name; a group invite by
// either name finds the character.
func TestOGTagKeepsNamesForTellsAndInvites(t *testing.T) {
	ogSwitch(t)
	c := newChatWorld(t)
	og, po := c.join(1, "Meshuggah", 25000, 16000, 0)
	bob, _ := c.join(2, "Bob", 25010, 16000, 0)
	og.SetOG(true)
	c.gw.SetPlayerOG(og, true)
	if po.ShownName() != "[OG] Meshuggah" {
		t.Fatalf("drawn %q", po.ShownName())
	}
	for _, typed := range []string{"/t Meshuggah hail", "/t [OG] Meshuggah hail", "/tell [og] meshuggah hail"} {
		flushQueue(og)
		flushQueue(bob)
		c.say(bob, typed)
		got := heard(t, og)
		if len(got) != 2 || got[0].op != protocol.OpcodeReplyName || got[0].text != "Bob" || got[1].text != "Bob tells you, 'hail'" {
			t.Fatalf("%q: Meshuggah heard %+v", typed, got)
		}
		if echo := texts(heard(t, bob)); len(echo) != 1 || echo[0] != "You told Meshuggah, 'hail'" {
			t.Fatalf("%q: Bob's echo %q", typed, echo)
		}
	}
	// the [OG] player's own tell: its reply name and line are its plain name
	flushQueue(bob)
	c.say(og, "/t Bob hi")
	if got := heard(t, bob); len(got) != 2 || got[0].text != "Meshuggah" || got[1].text != "Meshuggah tells you, 'hi'" {
		t.Fatalf("Bob heard %+v", got)
	}
	for _, name := range []string{"Meshuggah", "[OG] Meshuggah", "[GM] [OG] meshuggah"} {
		if p := c.gw.PlayerByName(name); p != po {
			t.Fatalf("PlayerByName(%q) = %v", name, p)
		}
		if p := findPlayerByName(c.gw, name); p != po {
			t.Fatalf("findPlayerByName(%q) = %v", name, p)
		}
	}
	// /invite by the drawn name: Bob's box to form the group names Meshuggah,
	// as by the plain one
	for _, name := range []string{"[OG] Meshuggah", "Meshuggah"} {
		flushQueue(bob)
		c.gw.InviteByName(bob, name)
		if got := texts(heard(t, bob)); !slices.Contains(got, "Accept the box to form a group and invite Meshuggah.") {
			t.Fatalf("/invite %s: Bob told %q", name, got)
		}
	}
}

// The lists vanilla keys by the name it draws carry every form: the plain
// name always, the tagged ones only where the tag is drawn.
func TestShownNamesForTheLists(t *testing.T) {
	if got := entity.ShownNames("Meshuggah", false, true, false); !slices.Equal(got, []string{"Meshuggah"}) {
		t.Fatalf("switch off: %q", got)
	}
	ogSwitch(t)
	if got := entity.ShownNames("Meshuggah", false, true, false); !slices.Equal(got, []string{"Meshuggah", "[OG] Meshuggah"}) {
		t.Fatalf("[OG]: %q", got)
	}
	// the tags joined, the space dropped for a full name (entity/hardcore.go's composeTags)
	if got := entity.ShownNames("Abcdefghijklmno", true, true, false); !slices.Equal(got,
		[]string{"Abcdefghijklmno", "[GM] Abcdefghijklmno", "[GM][OG]Abcdefghijklmno", "[OG] Abcdefghijklmno"}) {
		t.Fatalf("a GM's [OG] with a full name: %q", got)
	}
	if got := entity.ShownNames("Bob", true, false, false); !slices.Equal(got, []string{"Bob", "[GM] Bob"}) {
		t.Fatalf("a GM's: %q", got)
	}
}
