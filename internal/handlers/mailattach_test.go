package handlers

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The records the mail tests use.
const (
	mtArrows  = 910001 // stacks to 20
	mtSword   = 910002 // one, tradeable
	mtNoTrade = 910003
	mtNoRent  = 910004
	mtGem     = 910005 // Lore
)

// mailRig is a database, three characters in the world (Ann 21 with 10
// arrows, a sword, a No Trade and a No Rent item, a Lore gem and 1000 tunar;
// Bob 22 with 100 tunar; Hal 23, a Hardcore character) and a mail handler
// that keeps grams and moves attachments, its sessions answered the offer.
type mailRig struct {
	t      *testing.T
	db     *database.SQLiteDB
	inv    *database.InventoryStore
	gw     *entity.World
	sm     *session.Manager
	m      *MailHandler
	items  *NPCWindowHandler
	sess   map[string]*session.Session
	player map[string]*entity.Player
	path   string
}

func newMailRig(t *testing.T) *mailRig {
	t.Helper()
	path := t.TempDir() + "/mail.db"
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z, tunar, hardcore) VALUES
			('Ann', 1, 21, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1000, 0),
			('Bob', 1, 22, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0),
			('Hal', 2, 23, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1)`); err != nil {
		t.Fatalf("characters: %v", err)
	}
	ps := database.NewItemPatternStore(db)
	for _, p := range []database.ItemPatternRecord{
		{PatternID: mtArrows, ItemName: "Bone Arrow", MaxStack: 20, Rent: 1, ClassUse: 32767, RaceUse: 32767},
		{PatternID: mtSword, ItemName: "Keen Sword", EquipSlot: 14, MaxHP: 100, MaxStack: 1, Rent: 1, ClassUse: 32767, RaceUse: 32767},
		{PatternID: mtNoTrade, ItemName: "Bound Ring", MaxStack: 1, Rent: 1, Trade: 1, ClassUse: 32767, RaceUse: 32767},
		{PatternID: mtNoRent, ItemName: "Conjured Bread", MaxStack: 1, Rent: 0, ClassUse: 32767, RaceUse: 32767},
		{PatternID: mtGem, ItemName: "Lore Gem", MaxStack: 1, Rent: 1, Lore: 1, ClassUse: 32767, RaceUse: 32767},
	} {
		if err := ps.UpsertItemPattern(&p); err != nil {
			t.Fatal(err)
		}
	}
	inv := database.NewInventoryStore(db)
	for _, it := range []struct{ pid, n int }{{mtArrows, 10}, {mtSword, 1}, {mtNoTrade, 1}, {mtNoRent, 1}, {mtGem, 1}} {
		if _, err := inv.AddCharItem(21, it.pid, it.n, 100, 0, database.LocPack); err != nil {
			t.Fatal(err)
		}
	}
	r := &mailRig{t: t, db: db, inv: inv, path: path, sess: map[string]*session.Session{}, player: map[string]*entity.Player{}}
	r.gw = entity.NewWorld()
	r.sm = session.NewManager()
	r.gw.SessionManager = r.sm
	r.items = &NPCWindowHandler{inv: inv}
	for i, c := range []struct {
		name  string
		id    uint32
		tunar int
		hc    bool
	}{{"Ann", 21, 1000, false}, {"Bob", 22, 100, false}, {"Hal", 23, 0, true}} {
		s := r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 40), Port: 7200 + i}, uint32(7200+i), uint32(7200+i), uint16(7200+i), 2, true)
		s.SetInGame(true)
		s.SetCharacterInWorld(true)
		s.UpdateActivity()
		p := r.gw.CreatePlayer(s, &database.CharacterRecord{ServerID: c.id, CharName: c.name, AccountID: 1, Level: 10, Tunar: c.tunar, X: 100, Y: 54, Z: 100})
		p.Tunar = c.tunar
		p.Hardcore = c.hc
		rows, err := inv.GetCharacterItems(c.id)
		if err != nil {
			t.Fatal(err)
		}
		s.SetInventoryList(newInvList(rows))
		s.SetMailAttach(true)
		s.SetMailLiveForms(true)
		r.sess[c.name], r.player[c.name] = s, p
	}
	entity.SetHardcoreOn(true)
	t.Cleanup(func() { entity.SetHardcoreOn(false) })
	r.m = r.handler()
	return r
}

// handler is a mail handler on the rig's database, as one made at a start
// (a restart reads every gram back).
func (r *mailRig) handler() *MailHandler {
	m := NewMailHandler(r.gw, database.NewSQLiteCharacterStore(r.db))
	m.SetItems(r.items)
	m.RegisterHandlers(NewRegistry())
	return m
}

// key is the pack key of the character's record of a pattern.
func (r *mailRig) key(name string, pid int) uint32 {
	r.t.Helper()
	l := sessionInvList(r.sess[name])
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, rec := range l.recs {
		if rec.patternID == pid {
			return rec.key
		}
	}
	r.t.Fatalf("%s has no record of %d", name, pid)
	return 0
}

// held is how many of a pattern the character's rows hold.
func (r *mailRig) held(id uint32, pid int) int {
	r.t.Helper()
	rows, err := r.inv.GetCharacterItems(id)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, e := range rows {
		if e.Pattern.PatternID == pid {
			n += e.StackLeft
		}
	}
	return n
}

// purse is the character's purse as kept.
func (r *mailRig) purse(id uint32) int {
	var n int
	if err := r.db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, id).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

// send is a live send with attachments from name.
func (r *mailRig) send(from, to string, tunar int, items ...mailItemReq) {
	r.t.Helper()
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	w.WriteByte(mailAttachSend)
	w.Write7BitEncodedInt64(1)
	writeString8(w, to, 1024)
	writeWideString(w, "A parcel", 1024)
	writeWideString(w, "For you.", 1024)
	w.WriteUint32(uint32(tunar))
	w.Write7BitEncodedInt64(int64(len(items)))
	for _, it := range items {
		w.WriteUint32(it.key)
		w.Write7BitEncodedInt64(int64(it.n))
	}
	if err := r.m.HandleMailAttach(r.sess[from], incoming(protocol.OpcodeMailAttach, buf[:w.Position()])); err != nil {
		r.t.Fatal(err)
	}
}

// take asks to take gram id's attachments as name; the answer's status.
func (r *mailRig) take(name string, id uint32) bool {
	r.t.Helper()
	s := r.sess[name]
	s.Queue().GetPending()
	b := []byte{mailAttachTake, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[1:], id)
	if err := r.m.HandleMailAttach(s, incoming(protocol.OpcodeMailAttach, b)); err != nil {
		r.t.Fatal(err)
	}
	for _, d := range queued(s, protocol.OpcodeMailAttach) {
		if len(d) >= 8 && d[2] == mailAttachTake {
			return d[7] == 1
		}
	}
	r.t.Fatal("no answer to the take")
	return false
}

// box is the grams in name's box.
func (r *mailRig) box(m *MailHandler, name string) []*gram {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*gram(nil), m.boxes[strings.ToLower(name)]...)
}

// lines is the text of every chat line a session was sent (0x00CE, 0x0A7B).
func mailLines(s *session.Session) string {
	var out []string
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeErrorMessage, protocol.OpcodeColoredChat, protocol.OpcodeClientMessage:
			text, _ := readWideLine(d[2:])
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n")
}

// peekGramReplies is gramReplies leaving the queue as it is.
func peekGramReplies(s *session.Session) [][]byte {
	var out [][]byte
	for _, d := range queued(s, opcodeGramReply) {
		out = append(out, d[2:])
	}
	return out
}

// sendStatus is the status byte of the last 0x0D41 sub-op 1 a session was sent.
func sendStatus(s *session.Session) byte {
	st := byte(0xFF)
	for _, d := range peekGramReplies(s) {
		if len(d) >= 2 && d[0] == 1 {
			st = d[1]
		}
	}
	return st
}

// A gram carries tunar and part of a stack: both leave the sender at the send
// (its pack row smaller, its purse less, kept and told), held on the gram (two
// attachments, logged), and come to the recipient at its take (the arrows a new
// record, the tunar in its purse), logged. A second take gives nothing.
func TestMailAttachSendAndTake(t *testing.T) {
	r := newMailRig(t)
	ann, bob := r.sess["Ann"], r.sess["Bob"]
	ann.Queue().GetPending()
	r.send("Ann", "Bob", 500, mailItemReq{key: r.key("Ann", mtArrows), n: 3})
	if st := sendStatus(ann); st != gramSent {
		t.Fatalf("the send's status %d, lines %q", st, mailLines(ann))
	}
	if got := r.held(21, mtArrows); got != 7 {
		t.Fatalf("Ann holds %d arrows after sending 3 of 10", got)
	}
	if r.player["Ann"].Tunar != 500 || r.purse(21) != 500 {
		t.Fatalf("Ann's purse %d, kept %d; want 500", r.player["Ann"].Tunar, r.purse(21))
	}
	if len(queued(ann, protocol.OpcodeRemoveInvItem)) != 1 || len(queued(ann, protocol.OpcodePlayerTunar)) != 1 {
		t.Fatal("Ann was not told her pack and purse changed")
	}
	box := r.box(r.m, "Bob")
	if len(box) != 1 || len(box[0].atts) != 2 {
		t.Fatalf("Bob's box %+v", box)
	}
	id := box[0].id
	if log, _ := r.db.GramAttachmentLog(10); len(log) != 2 || log[0]["event"] != "sent" {
		t.Fatalf("the log %v", log)
	}
	if !r.take("Bob", id) {
		t.Fatalf("the take was refused: %q", mailLines(bob))
	}
	if got := r.held(22, mtArrows); got != 3 {
		t.Fatalf("Bob holds %d arrows", got)
	}
	if r.player["Bob"].Tunar != 600 || r.purse(22) != 600 {
		t.Fatalf("Bob's purse %d, kept %d; want 600", r.player["Bob"].Tunar, r.purse(22))
	}
	if len(queued(bob, protocol.OpcodeAddInvItem)) != 1 {
		t.Fatal("Bob was not sent the arrows' record")
	}
	if log, _ := r.db.GramAttachmentLog(10); len(log) != 4 || log[0]["event"] != "taken" {
		t.Fatalf("the log %v", log)
	}
	// the double take
	if r.take("Bob", id) {
		t.Fatal("a second take was answered taken")
	}
	if r.held(22, mtArrows) != 3 || r.purse(22) != 600 {
		t.Fatalf("a second take gave more: %d arrows, %d tunar", r.held(22, mtArrows), r.purse(22))
	}
}

// The take's transaction gives once: the same attachments taken twice (a
// second handler, a stale copy) is ErrGramTaken the second time, nothing
// given; and only the gram's own recipient takes.
func TestMailAttachDoubleTakeInTheDatabase(t *testing.T) {
	r := newMailRig(t)
	r.send("Ann", "Bob", 200)
	g := r.box(r.m, "Bob")[0]
	ids := []int64{g.atts[0].ID}
	if _, err := r.inv.TakeGramAttachments(database.GramTake{GramID: g.id, IDs: ids, ToID: 21, ToName: "Ann", At: time.Now()}); !errors.Is(err, database.ErrGramGone) {
		t.Fatalf("another character's take: %v", err)
	}
	got, err := r.inv.TakeGramAttachments(database.GramTake{GramID: g.id, IDs: ids, ToID: 22, ToName: "Bob", Purse: 100, At: time.Now()})
	if err != nil || got.Tunar != 200 || got.Purse != 300 {
		t.Fatalf("the take: %+v, %v", got, err)
	}
	if _, err := r.inv.TakeGramAttachments(database.GramTake{GramID: g.id, IDs: ids, ToID: 22, ToName: "Bob", Purse: 300, At: time.Now()}); !errors.Is(err, database.ErrGramTaken) {
		t.Fatalf("the second take: %v", err)
	}
	if r.purse(22) != 300 {
		t.Fatalf("Bob's purse %d after a second take", r.purse(22))
	}
}

// A gram and what it carries outlive a restart: a handler made again on the
// database (as a restart or a keep restart makes one) has the gram in Bob's
// box with its attachment, and Bob takes it from there.
func TestMailAttachSurvivesARestart(t *testing.T) {
	r := newMailRig(t)
	r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtSword), n: 1})
	r.m = r.handler()
	box := r.box(r.m, "Bob")
	if len(box) != 1 || len(box[0].atts) != 1 || box[0].atts[0].PatternID != mtSword {
		t.Fatalf("after the restart Bob's box %+v", box)
	}
	if !r.take("Bob", box[0].id) || r.held(22, mtSword) != 1 {
		t.Fatalf("the take after the restart: Bob holds %d swords", r.held(22, mtSword))
	}
}

// The sender's item is gone from its pack at the send (the row deleted), so
// deleting it then fails and the recipient still takes it. The control: a
// send naming a record whose row the sender lost before it (deleted under
// the list) is refused whole, nothing taken, no gram filed, the tunar kept.
func TestMailAttachClaimAfterTheSenderDeletedTheItem(t *testing.T) {
	r := newMailRig(t)
	swordKey := r.key("Ann", mtSword)
	l := sessionInvList(r.sess["Ann"])
	rec, _ := l.at(l.index(swordKey))
	r.send("Ann", "Bob", 0, mailItemReq{key: swordKey, n: 1})
	if err := r.inv.DeleteCharItem(21, rec.itemID); err == nil {
		t.Fatal("the sender deleted a row the gram holds")
	}
	g := r.box(r.m, "Bob")[0]
	if !r.take("Bob", g.id) || r.held(22, mtSword) != 1 {
		t.Fatal("Bob did not get the sword after Ann's delete")
	}

	// the control
	r2 := newMailRig(t)
	arrowKey := r2.key("Ann", mtArrows)
	l2 := sessionInvList(r2.sess["Ann"])
	rec2, _ := l2.at(l2.index(arrowKey))
	if err := r2.inv.DeleteCharItem(21, rec2.itemID); err != nil {
		t.Fatal(err)
	}
	r2.sess["Ann"].Queue().GetPending()
	r2.send("Ann", "Bob", 300, mailItemReq{key: arrowKey, n: 2})
	if st := sendStatus(r2.sess["Ann"]); st != 12 {
		t.Fatalf("a send of a row gone: status %d", st)
	}
	if len(r2.box(r2.m, "Bob")) != 0 || r2.purse(21) != 1000 || r2.player["Ann"].Tunar != 1000 {
		t.Fatalf("a refused send moved something: box %d, purse %d", len(r2.box(r2.m, "Bob")), r2.purse(21))
	}
}

// What cannot be mailed, each refused with its line, nothing moved: a No Trade
// item, a No Rent one, a worn one, a Lore item to one who has one coming,
// several names, the sender itself, more tunar than carried. The control: the
// sword goes.
func TestMailAttachRules(t *testing.T) {
	r := newMailRig(t)
	ann := r.sess["Ann"]
	refused := func(what, want string, send func()) {
		t.Helper()
		ann.Queue().GetPending()
		send()
		var why string
		for _, d := range queued(ann, protocol.OpcodeMailAttach) {
			if len(d) > 3 && d[2] == mailAttachRefused {
				why, _ = readWideLine(d[3:])
			}
		}
		if sendStatus(ann) != 12 || !strings.Contains(why, want) {
			t.Fatalf("%s: status %d, %q (want %q)", what, sendStatus(ann), why, want)
		}
	}
	refused("no trade", "No Trade", func() { r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtNoTrade), n: 1}) })
	refused("no rent", "No Rent", func() { r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtNoRent), n: 1}) })
	refused("self", "yourself", func() { r.send("Ann", "Ann", 10) })
	refused("tunar", "do not carry", func() { r.send("Ann", "Bob", 5000) })
	refused("too many", "do not have that many", func() { r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtArrows), n: 11}) })
	ann.Queue().GetPending()
	buf := make([]byte, 512)
	w := buffer.NewWriter(buf)
	w.WriteByte(mailAttachSend)
	w.Write7BitEncodedInt64(2)
	writeString8(w, "Bob", 64)
	writeString8(w, "Hal", 64)
	writeWideString(w, "s", 64)
	writeWideString(w, "t", 64)
	w.WriteUint32(5)
	w.Write7BitEncodedInt64(0)
	r.m.HandleMailAttach(ann, incoming(protocol.OpcodeMailAttach, buf[:w.Position()]))
	if sendStatus(ann) != 12 {
		t.Fatal("tunar to two names was not refused")
	}
	// a worn sword
	l := sessionInvList(ann)
	rec, _ := l.at(l.index(r.key("Ann", mtSword)))
	if err := r.inv.SetEquipLoc(21, rec.itemID, 14); err != nil {
		t.Fatal(err)
	}
	refused("worn", "wearing", func() { r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtSword), n: 1}) })
	if err := r.inv.SetEquipLoc(21, rec.itemID, -1); err != nil {
		t.Fatal(err)
	}
	// a Lore gem, then a second to the same character
	r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtGem), n: 1})
	if sendStatus(ann) != gramSent {
		t.Fatalf("the first gem: %q", mailLines(ann))
	}
	if _, err := r.inv.AddCharItem(21, mtGem, 1, 100, 0, database.LocPack); err != nil {
		t.Fatal(err)
	}
	rows, _ := r.inv.GetCharacterItems(21)
	ann.SetInventoryList(newInvList(rows))
	refused("lore coming", "Lore", func() { r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtGem), n: 1}) })
	if r.held(21, mtNoTrade) != 1 || r.held(21, mtNoRent) != 1 || r.purse(21) != 1000 {
		t.Fatal("a refused send moved something")
	}
	// the control
	ann.Queue().GetPending()
	r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtSword), n: 1})
	if sendStatus(ann) != gramSent || r.held(21, mtSword) != 0 {
		t.Fatalf("the sword: status %d, lines %q", sendStatus(ann), mailLines(ann))
	}
}

// A take the pack has no room for is refused and the attachment stays on the
// gram; a Lore item held already likewise. Made room, it is taken.
func TestMailAttachPackFull(t *testing.T) {
	r := newMailRig(t)
	r.send("Ann", "Bob", 0, mailItemReq{key: r.key("Ann", mtSword), n: 1})
	g := r.box(r.m, "Bob")[0]
	bob := r.sess["Bob"]
	bob.SetInventoryList(newInvListCap(nil, 1))
	sessionInvList(bob).add(999999, mtArrows, 1, 20) // the one record it holds
	if r.take("Bob", g.id) {
		t.Fatal("a full pack took the sword")
	}
	if !strings.Contains(mailLines(bob), "no room") || len(r.box(r.m, "Bob")[0].atts) != 1 || r.held(22, mtSword) != 0 {
		t.Fatalf("after a full pack's take: %q", mailLines(bob))
	}
	bob.SetInventoryList(newInvList(nil))
	if !r.take("Bob", g.id) || r.held(22, mtSword) != 1 {
		t.Fatal("the take with room")
	}
}

// Deleting a gram that still carries something returns it to its sender: a
// returned gram in the sender's box carrying it (logged), which cannot be
// deleted until it is taken; taken, it can. A gram of text alone just goes.
func TestMailAttachDeleteReturnsToTheSender(t *testing.T) {
	r := newMailRig(t)
	r.send("Ann", "Bob", 250, mailItemReq{key: r.key("Ann", mtArrows), n: 4})
	g := r.box(r.m, "Bob")[0]
	del := func(name string, id uint32) byte {
		s := r.sess[name]
		s.Queue().GetPending()
		r.m.handleRequest(s, incoming(opcodeGramRequest, gramRequestBytes(4, false, func(w *buffer.Writer) { w.Write7BitEncodedInt64(int64(id)) })))
		for _, d := range peekGramReplies(s) {
			if d[0] == 4 {
				return d[1]
			}
		}
		return 0xFF
	}
	if st := del("Bob", g.id); st != 13 {
		t.Fatalf("Bob's delete: %d", st)
	}
	if len(r.box(r.m, "Bob")) != 0 {
		t.Fatal("the gram is still in Bob's box")
	}
	back := r.box(r.m, "Ann")
	if len(back) != 1 || !back[0].returned || len(back[0].atts) != 2 || !strings.HasPrefix(back[0].subject, "Returned: ") {
		t.Fatalf("Ann's box %+v", back)
	}
	if log, _ := r.db.GramAttachmentLog(1); log[0]["event"] != "returned" {
		t.Fatalf("the log %v", log)
	}
	if st := del("Ann", back[0].id); st == 13 {
		t.Fatal("a returned gram carrying something was deleted")
	}
	if !r.take("Ann", back[0].id) || r.held(21, mtArrows) != 10 || r.purse(21) != 1000 {
		t.Fatalf("Ann took her parcel back: %d arrows, %d tunar", r.held(21, mtArrows), r.purse(21))
	}
	if st := del("Ann", back[0].id); st != 13 {
		t.Fatalf("the emptied returned gram's delete: %d", st)
	}
	// after a restart too
	r.m = r.handler()
	if len(r.box(r.m, "Ann")) != 0 || len(r.box(r.m, "Bob")) != 0 {
		t.Fatal("deleted grams came back at the restart")
	}
}

// A Hardcore character uses no mail: its send (text) is refused and it is
// told; a gram to it is refused at the send, the sender told (the disc's form
// 12, the live one 4 naming it), with attachments too; it takes nothing. The
// control: a normal character mails another, text and a parcel.
func TestMailHardcore(t *testing.T) {
	r := newMailRig(t)
	ann, hal := r.sess["Ann"], r.sess["Hal"]
	// Hal sends
	hal.Queue().GetPending()
	r.m.handleRequest(hal, incoming(opcodeGramRequest, liveSend([]string{"Bob"}, "Hi", "x")))
	if sendStatus(hal) != 12 || !strings.Contains(mailLines(hal), hcNoMailWords) || len(r.box(r.m, "Bob")) != 0 {
		t.Fatalf("Hal's send: status %d, %q", sendStatus(hal), mailLines(hal))
	}
	// to Hal, live
	ann.Queue().GetPending()
	r.m.handleRequest(ann, incoming(opcodeGramRequest, liveSend([]string{"Hal"}, "Hi", "x")))
	if sendStatus(ann) != gramPartly || !strings.Contains(mailLines(ann), "Hal is a Hardcore character and cannot receive mail.") || len(r.box(r.m, "Hal")) != 0 {
		t.Fatalf("to Hal (live): status %d, %q", sendStatus(ann), mailLines(ann))
	}
	// to Hal, the disc's form
	ann.Queue().GetPending()
	r.m.handleRequest(ann, incoming(opcodeGramRequest, gramRequestBytes(1, true, func(w *buffer.Writer) {
		writeString8(w, "Hal", 24)
		writeWideString(w, "Disc", 1024)
		writeWideString(w, "x", 1024)
	})))
	if sendStatus(ann) != 12 || len(r.box(r.m, "Hal")) != 0 {
		t.Fatalf("to Hal (disc): status %d", sendStatus(ann))
	}
	// with a parcel
	ann.Queue().GetPending()
	r.send("Ann", "Hal", 10, mailItemReq{key: r.key("Ann", mtSword), n: 1})
	if sendStatus(ann) != 12 || r.held(21, mtSword) != 1 || r.purse(21) != 1000 {
		t.Fatal("a parcel to Hal was not refused whole")
	}
	// Hal opening the inbox is told
	hal.Queue().GetPending()
	r.m.handleRequest(hal, incoming(opcodeGramRequest, gramRequestBytes(2, false, nil)))
	if !strings.Contains(mailLines(hal), hcNoMailWords) {
		t.Fatalf("Hal's inbox: %q", mailLines(hal))
	}
	// a gram Hal held from before: readable, nothing taken
	r.m.mu.Lock()
	old := &gram{from: "Ann", fromID: 21, ownerID: 23, to: []string{"Hal"}, subject: "Old", text: "From before", date: mailNow()}
	r.m.fileLocked("Hal", old)
	r.m.mu.Unlock()
	hal.Queue().GetPending()
	r.m.handleRequest(hal, incoming(opcodeGramRequest, gramRequestBytes(3, false, func(w *buffer.Writer) { w.Write7BitEncodedInt64(int64(old.id)) })))
	if rep := peekGramReplies(hal); len(rep) != 1 || rep[0][1] != 3 {
		t.Fatalf("Hal's old gram: % X", rep)
	}
	old.atts = []database.GramAttachment{{ID: 1, GramID: old.id, Tunar: 5}}
	if r.take("Hal", old.id) {
		t.Fatal("Hal took from a gram")
	}

	// the control
	ann.Queue().GetPending()
	r.m.handleRequest(ann, incoming(opcodeGramRequest, liveSend([]string{"Bob"}, "Hi", "x")))
	if sendStatus(ann) != gramSent || len(r.box(r.m, "Bob")) != 1 {
		t.Fatalf("Ann to Bob: status %d", sendStatus(ann))
	}
	r.send("Ann", "Bob", 10)
	if sendStatus(ann) != gramSent || len(r.box(r.m, "Bob")) != 2 {
		t.Fatalf("Ann's parcel to Bob: status %d, %q", sendStatus(ann), mailLines(ann))
	}
}

// A client that takes attachments is told what an opened gram carries (sub-op
// 2: the tunar, the record by its name) and which grams of its inbox carry
// something (sub-op 3), its text as written; one that does not (a PS2, an
// older vanilla) is told in the gram's text, with no '|' in it.
func TestMailAttachShownOrNoted(t *testing.T) {
	r := newMailRig(t)
	r.send("Ann", "Bob", 75, mailItemReq{key: r.key("Ann", mtArrows), n: 2})
	g := r.box(r.m, "Bob")[0]
	bob := r.sess["Bob"]
	open := func() []byte {
		bob.Queue().GetPending()
		r.m.handleRequest(bob, incoming(opcodeGramRequest, gramRequestBytes(3, false, func(w *buffer.Writer) { w.Write7BitEncodedInt64(int64(g.id)) })))
		rep := peekGramReplies(bob)
		if len(rep) != 1 {
			t.Fatalf("the open: % X", rep)
		}
		return rep[0]
	}
	rep := open()
	if bytes.Contains(rep, []byte("A\x00t\x00t\x00a\x00c\x00h\x00")) {
		t.Fatal("a client that takes attachments had the note in its text")
	}
	var show []byte
	for _, d := range queued(bob, protocol.OpcodeMailAttach) {
		if d[2] == mailAttachShow {
			show = d[3:]
		}
	}
	if len(show) < 9 || binary.LittleEndian.Uint32(show) != g.id || binary.LittleEndian.Uint32(show[4:]) != 75 || show[8] != 2 ||
		!bytes.Contains(show, []byte("B\x00o\x00n\x00e\x00")) {
		t.Fatalf("what the gram carries: % X", show)
	}
	bob.Queue().GetPending()
	r.m.handleRequest(bob, incoming(opcodeGramRequest, gramRequestBytes(2, false, nil)))
	var marks []byte
	for _, d := range queued(bob, protocol.OpcodeMailAttach) {
		if d[2] == mailAttachMarks {
			marks = d[3:]
		}
	}
	if len(marks) != 10 || marks[0] != 2 || binary.LittleEndian.Uint32(marks[1:]) != g.id {
		t.Fatalf("the inbox's marks: % X", marks)
	}

	bob.SetMailAttach(false)
	rep = open()
	_, rest := readWideLine(rep[2+len(mailZZ(int64(g.id))):]) // the live To line
	text, _ := readWideLine(rest)
	if !strings.Contains(text, "Attached: 75 tunar and 2 Bone Arrow.") || strings.Contains(text, "|") {
		t.Fatalf("the older client's text %q", text)
	}
	if len(queued(bob, protocol.OpcodeMailAttach)) != 0 {
		t.Fatal("an older client was sent 0x7EA1")
	}
}

// zz is a zigzag number's bytes.
func mailZZ(v int64) []byte {
	b := make([]byte, 16)
	w := buffer.NewWriter(b)
	w.Write7BitEncodedInt64(v)
	return b[:w.Position()]
}

// The offer goes out with the mail forms' only from a server whose mail keeps
// grams and moves items; a client's answer is kept on its session.
func TestMailAttachOffer(t *testing.T) {
	mailAttachOn.Store(false)
	s := entitySlotsSession()
	SendMailOffer(s)
	if len(queued(s, protocol.OpcodeMailAttach)) != 0 {
		t.Fatal("offered without attachments")
	}
	r := newMailRig(t)
	t.Cleanup(func() { mailAttachOn.Store(false) })
	SetMailPostage(15)
	t.Cleanup(func() { SetMailPostage(0) })
	s = entitySlotsSession()
	SendMailOffer(s)
	got := queued(s, protocol.OpcodeMailAttach)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0xA1, 0x7E, 0, 1, 1, 15, 0, 0, 0}) {
		t.Fatalf("the offer % X", got)
	}
	bob := r.sess["Bob"]
	bob.SetMailAttach(false)
	r.m.HandleMailAttach(bob, incoming(protocol.OpcodeMailAttach, []byte{0, 1}))
	if !bob.MailAttach() {
		t.Fatal("the answer was not kept")
	}
	// the postage: taken from the sender, gone
	r.sess["Ann"].Queue().GetPending()
	r.send("Ann", "Bob", 100)
	if r.purse(21) != 885 || !strings.Contains(mailLines(r.sess["Ann"]), "postage was 15") {
		t.Fatalf("with postage Ann's purse %d", r.purse(21))
	}
}
