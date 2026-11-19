package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Tell-grams, the in-game mail, as CLIENT's gram client drives it
// (VIClientPart2EFD0, decomp modules/client/src/viclientpart2efd0.cpp), and as
// SOE's live CLIENT (eqoa-live-1500, the Corsten state) drives it: several
// recipients a gram, Reply All, and who else a gram went to shown with it.
//
// A request is message 0x0D01: a sub-op byte, then its fields. The disc's
// client puts an envelope after the sub-op of every request (a zigzag length
// and that many bytes, an inner 0x0D41 carrying the sub-op again: the bytes
// 06 41 0D <sub>); the live one leaves it out of sub-ops 1 and 3 and keeps it
// in 2 and 4. A request's form is read off its own bytes (gramEnvelope) and
// answered in that form, so a PS2 on the Frontiers disc is answered as before.
// Every reply is 0x0D41: the sub-op, a status, the rest.
//
//	1 send    disc in: string to (8-bit), wide subject, wide text
//	          disc out: status 1 sent, 9 unknown recipient, 10 mailbox full,
//	          11 too many unread from you, 12 the system is busy
//	          live in: zigzag n, n names (8-bit), wide subject, wide text
//	          live out: 1 sent to all; 9, zigzag n, the names no character
//	          has (sent to nobody, the compose window kept to fix them);
//	          4, zigzag n, the names it did not go to, zigzag n, a reason
//	          byte each (10 mailbox full, 11 too many unread from you, any
//	          other "Unknown error."), a byte 1 when someone got it; 12 busy.
//	          The live client takes 10 and 11 alone as "Unknown error.", so a
//	          live send that failed is always 4, one name or many.
//	2 inbox   out: status 2, zigzag count, each header
//	3 open    in: zigzag id; out: status 3, zigzag id, (live) wide To, the
//	          names it was sent to joined with ";" (Reply All parses it),
//	          wide text
//	4 delete  in: zigzag id; out: status 13 deleted (anything else fails)
//	5 (out only, no status): new mail
//
// A header (operator>>(VIMemStream &, VIGramHeader &)): u32 id, u32, u32,
// string from (24), wide subject (128 bytes), the date (u16 year, month,
// day, hour, minute, second) and a read byte.
//
// Each recipient gets a copy of its own (its own id and read flag), and every
// copy keeps the whole list of names the gram went to. The boxes live in
// memory, keyed by the recipient's name, and (with a database) every gram is a
// row of mail_grams too (database/mail.go), read back at the start: a gram
// survives a restart and a keep restart (it was gone at either until
// 2026-11-19, when grams began to carry items and tunar: mailattach.go).
//
// A Hardcore character (world.hardcore) uses no mail at all (the owner,
// 2026-11-19: "[HC] can't use mail"): it sends nothing, and a gram to one is
// refused at the send, the sender told why. A gram it holds from before stays
// readable, and can be deleted; nothing attached to one can be taken.

const (
	opcodeGramRequest protocol.GameOpcode = 0x0D01
	opcodeGramReply   protocol.GameOpcode = 0x0D41

	mailboxMax      = 100 // the client keeps 100 rows
	unreadPerSender = 10

	// gramRecipientsMax is how many names one gram goes to. The live
	// client's To line holds 511 characters and counts nothing itself
	// (ParseRecipients), so the cap is this server's; a name past it is
	// answered as one the gram did not go to, "Unknown error.".
	gramRecipientsMax = 20
)

// A send's statuses, and the reasons a live send gives for a name.
const (
	gramSent      byte = 1
	gramPartly    byte = 4 // live: the names it did not go to, their reasons, delivered
	gramUnknown   byte = 9
	gramBoxFull   byte = 10
	gramTooMany   byte = 11
	gramFailOther byte = 0 // a reason neither 10 nor 11: the live client's "Unknown error."
)

type gram struct {
	id      uint32
	from    string
	to      []string // every name the gram went to, as the sender wrote them
	subject string
	text    string
	date    time.Time
	read    bool

	ownerID  uint32 // the character it was sent to, 0 not known
	fromID   uint32 // the character that sent it, 0 not known
	returned bool   // its attachments came back to their sender
	// atts is what it carries (mailattach.go), as the database holds it
	atts []database.GramAttachment
}

// gramNames says whether a character of a name exists
// (database.SQLiteCharacterStore).
type gramNames interface {
	NameExists(name string) (bool, error)
}

// MailHandler keeps every mailbox.
type MailHandler struct {
	gameWorld *entity.World
	names     gramNames // nil: every name is someone's
	// db keeps the grams and what they carry (database/mail.go); nil (a
	// test's handler): the boxes are memory alone and nothing is attached
	db *database.SQLiteDB
	// items is the windows' handler an attachment moves through (its
	// inventory store and the sessions' item lists); nil: none attached
	items *NPCWindowHandler
	// hardcore is whether the character of a name is a Hardcore character
	// world.hardcore binds (no mail to it); nil: hardcoreNamed
	hardcore func(name string) bool

	mu     sync.Mutex
	boxes  map[string][]*gram
	nextID uint32
}

func NewMailHandler(gw *entity.World, cs *database.SQLiteCharacterStore) *MailHandler {
	h := &MailHandler{gameWorld: gw, boxes: map[string][]*gram{}, nextID: 1}
	if cs != nil {
		h.names = cs
		h.db = cs.DB()
		h.load()
	}
	return h
}

// SetItems is the windows' handler attachments move through (registry.go).
func (h *MailHandler) SetItems(items *NPCWindowHandler) { h.items = items }

// load fills the boxes from the database: every gram and what it carries.
func (h *MailHandler) load() {
	if h.db == nil {
		return
	}
	grams, err := h.db.Grams()
	if err != nil {
		log.Printf("Mail: reading the grams: %v", err)
		return
	}
	atts, err := h.db.AllGramAttachments()
	if err != nil {
		log.Printf("Mail: reading what the grams carry: %v", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range grams {
		g := gramOf(r)
		g.atts = atts[g.id]
		h.boxes[r.Owner] = append(h.boxes[r.Owner], g)
		if g.id >= h.nextID {
			h.nextID = g.id + 1
		}
	}
	if len(grams) > 0 {
		log.Printf("Mail: %d grams in %d boxes, %d carrying something", len(grams), len(h.boxes), len(atts))
	}
}

// gramOf is a kept row as the boxes hold it.
func gramOf(r database.Gram) *gram {
	return &gram{id: r.ID, from: r.From, to: r.To, subject: r.Subject, text: r.Text, date: r.Sent.In(mailNow().Location()),
		read: r.Read, ownerID: r.OwnerID, fromID: r.FromID, returned: r.Returned}
}

// row is the gram as a kept row, in owner's box.
func (g *gram) row(owner string) database.Gram {
	return database.Gram{ID: g.id, Owner: strings.ToLower(owner), OwnerID: g.ownerID, From: g.from, FromID: g.fromID,
		To: g.to, Subject: g.subject, Text: g.text, Sent: g.date, Read: g.read, Returned: g.returned}
}

// file puts a new gram in owner's box: kept first (its id the row's), else
// numbered here. With mu held. False when it could not be kept.
func (h *MailHandler) fileLocked(owner string, g *gram) bool {
	key := strings.ToLower(owner)
	if h.db != nil {
		r := g.row(key)
		if err := h.db.AddGram(&r); err != nil {
			log.Printf("Mail: keeping a gram from %s to %s: %v", g.from, owner, err)
			return false
		}
		g.id = r.ID
	} else {
		g.id = h.nextID
		h.nextID++
	}
	h.boxes[key] = append(h.boxes[key], g)
	return true
}

// hardcoreNamed is whether the character of a name is bound by Hardcore:
// one in the world by its player, else by its row.
func (h *MailHandler) hardcoreNamed(name string) bool {
	if h.hardcore != nil {
		return h.hardcore(name)
	}
	if !entity.HardcoreOn() {
		return false
	}
	if p := h.playerNamed(name); p != nil {
		return p.HardcoreBound()
	}
	if h.db != nil {
		r, ok, err := h.db.GramRecipientNamed(name)
		if err != nil {
			log.Printf("Mail: is %s Hardcore: %v", name, err)
		}
		return ok && r.Hardcore
	}
	return false
}

// playerNamed is the player in the world of a name (any case), or nil.
func (h *MailHandler) playerNamed(name string) *entity.Player {
	if h.gameWorld == nil {
		return nil
	}
	var target *entity.Player
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		if strings.EqualFold(p.Name, name) {
			target = p
		}
	})
	return target
}

// The Hardcore refusals (ours).
const (
	hcNoMailWords     = "Mail is not available to Hardcore characters."
	hcNoMailThemWords = "%s is a Hardcore character and cannot receive mail."
)

func (h *MailHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeGramRequest, h.handleRequest)
	r.Register(protocol.OpcodeMailForm, h.HandleMailForm)
	r.Register(protocol.OpcodeMailAttach, h.HandleMailAttach)
	if h.db != nil && h.items != nil && h.items.inv != nil {
		mailAttachOn.Store(true) // the offer goes out (mailattach.go)
	}
	if h.gameWorld != nil {
		h.gameWorld.OnPlayerEntered(h.announceUnread)
	}
	log.Printf("Mail: registered handlers")
}

// The mail form offer (protocol.OpcodeMailForm, not retail): a u32, both
// ways. The server offers it (1) with the other offers at the world entry; a
// client that sends the live forms (vanilla, once it has them) answers 1 and
// from then sends them, 0 to go back to the disc's. Every request is still
// answered by its own form (gramEnvelope), so the answer is kept on the
// session (MailLiveForms) only to tell a form the client did not announce in
// the log; a client that never answers (a PS2, an S2-era client) is
// answered as ever.

// buildMailFormOffer is OpcodeMailForm from the server: a u32, 1.
func buildMailFormOffer() []byte {
	return build(protocol.OpcodeMailForm, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendMailOffer offers the session's client the live mail forms. A PS2
// ignores it.
func SendMailOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeMailForm, buildMailFormOffer())
	sendMailAttachOffer(sess) // what a gram carries, when this server's mail carries anything
}

// HandleMailForm keeps the client's answer to the offer on its session: a
// u32, nonzero for the live forms.
func (h *MailHandler) HandleMailForm(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("MailForm: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	sess.SetMailLiveForms(on)
	log.Printf("MailForm: %s %s", sess.RemoteAddr(), map[bool]string{true: "sends the live mail forms", false: "sends the disc's"}[on])
	return nil
}

// newMailText is WORDS.TXT's word 1192. CLIENT shows nothing for 0x0D41's
// sub-op 5 (Recv0D41 only sets the gram client's m_x08, which nothing
// reads) and no code of the game's shows the word, so the server says it:
// a gram arrived and nobody knew (F3 20261021-001609).
const newMailText = "You have new mail."

// notifyNewMail is sub-op 5 and the line that says it.
func notifyNewMail(sess *session.Session) {
	gramReply(sess, 5, nil)
	sess.Queue().AddWithSequence(buildClientMessage(newMailText))
}

// announceUnread tells a player entering the world of grams it has not read.
func (h *MailHandler) announceUnread(sess *session.Session, p *entity.Player) {
	if h.unread(p.Name) > 0 {
		notifyNewMail(sess)
	}
}

// unread is how many grams in name's box are unread.
func (h *MailHandler) unread(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, g := range h.boxes[strings.ToLower(name)] {
		if !g.read {
			n++
		}
	}
	return n
}

// gramReply queues 0x0D41 with sub, then fill.
func gramReply(sess *session.Session, sub byte, fill func(w *buffer.Writer)) {
	queueReliable(sess, opcodeGramReply, build(opcodeGramReply, 32768, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	}))
}

// The dates are shown "PST".
func mailNow() time.Time {
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		return time.Now().In(loc)
	}
	return time.Now()
}

func writeGramHeader(w *buffer.Writer, g *gram) {
	w.WriteUint32(g.id)
	w.WriteUint32(0)
	w.WriteUint32(0)
	writeString8(w, g.from, 24)
	writeWideString(w, g.subject, 128)
	w.WriteUint16(uint16(g.date.Year()))
	w.WriteByte(byte(g.date.Month()))
	w.WriteByte(byte(g.date.Day()))
	w.WriteByte(byte(g.date.Hour()))
	w.WriteByte(byte(g.date.Minute()))
	w.WriteByte(byte(g.date.Second()))
	if g.read {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func readString8(r *buffer.Reader) (string, bool) {
	n, err := r.ReadUint32()
	if err != nil || n > 1024 {
		return "", false
	}
	s, err := r.ReadStringUTF8(int(n))
	return s, err == nil
}

func readWide(r *buffer.Reader) (string, bool) {
	n, err := r.ReadUint32()
	if err != nil || n > 1024 {
		return "", false
	}
	s, err := r.ReadStringUnicode(int(n))
	return s, err == nil
}

// gramEnvelope is whether a request carries the disc client's envelope after
// its sub-op: 06 41 0D, a zigzag 3 and the inner 0x0D41. A live request
// cannot begin so: its send starts with a recipient count and a name of at
// most 23 bytes (a u32 count, never 0x0D41...), its open with a one-byte id.
func gramEnvelope(data []byte) bool {
	return len(data) >= 4 && data[1] == 0x06 && data[2] == 0x41 && data[3] == 0x0D
}

// gramSend is a send, read: the names it is for, the subject, the text.
type gramSend struct {
	to            []string
	subject, text string
}

// readDiscSend is the disc's send after its envelope: one name.
func readDiscSend(r *buffer.Reader) (gramSend, bool) {
	to, ok1 := readString8(r)
	subject, ok2 := readWide(r)
	text, ok3 := readWide(r)
	return gramSend{to: []string{to}, subject: subject, text: text}, ok1 && ok2 && ok3
}

// readLiveSend is the live client's send (SendGram 0x012F5390): a zigzag
// count of names, each an 8-bit string, then the subject and the text.
func readLiveSend(r *buffer.Reader) (gramSend, bool) {
	n, err := r.Read7BitEncodedInt64()
	// the To line is 511 characters: no more names than that fit in it
	if err != nil || n < 0 || n > 512 {
		return gramSend{}, false
	}
	var req gramSend
	for i := 0; i < int(n); i++ {
		name, ok := readString8(r)
		if !ok {
			return gramSend{}, false
		}
		req.to = append(req.to, name)
	}
	var ok1, ok2 bool
	req.subject, ok1 = readWide(r)
	req.text, ok2 = readWide(r)
	return req, ok1 && ok2
}

func (h *MailHandler) handleRequest(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		return nil
	}
	sub := data[0]
	disc := gramEnvelope(data)
	r := buffer.NewReader(data[1:])
	if disc {
		// the inner 0x0D41 as a blob: skipped
		n, err := r.Read7BitEncodedInt64()
		if err != nil || n < 0 || int(n) > r.Remaining() {
			return nil
		}
		if err := r.Advance(int(n)); err != nil {
			return nil
		}
	}
	if h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	if me == nil {
		return nil
	}
	if (sub == 1 || sub == 3) && disc == sess.MailLiveForms() {
		// answered by its own form all the same
		log.Printf("Mail: %s sent sub-op %d in the %s form, having answered the offer %v",
			me.Name, sub, map[bool]string{true: "disc's", false: "live"}[disc], sess.MailLiveForms())
	}
	mine := strings.ToLower(me.Name)
	switch sub {
	case 1:
		if me.HardcoreBound() {
			SendErrorMessage(sess, hcNoMailWords)
			log.Printf("Hardcore: %s's gram refused", me.Name)
			// 12, "busy", in both forms: the compose stays
			gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(12) })
			return nil
		}
		var req gramSend
		var ok bool
		if disc {
			req, ok = readDiscSend(r)
		} else {
			req, ok = readLiveSend(r)
		}
		if !ok {
			gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(0) })
			return nil
		}
		out := h.deliverFrom(me.Name, me.CharServerID, req.to, consoleLine(req.subject, gramSubjectMost), consoleBlock(req.text, gramTextMost), sess)
		gramReply(sess, 1, func(w *buffer.Writer) {
			if disc {
				w.WriteByte(out.discStatus())
			} else {
				out.writeLive(w)
			}
		})
	case 2:
		if me.HardcoreBound() {
			// what it held from before stays readable
			SendErrorMessage(sess, hcNoMailWords)
		}
		h.mu.Lock()
		box := append([]*gram(nil), h.boxes[mine]...)
		h.mu.Unlock()
		gramReply(sess, 2, func(w *buffer.Writer) {
			w.WriteByte(2)
			w.Write7BitEncodedInt64(int64(len(box)))
			for _, g := range box {
				writeGramHeader(w, g)
			}
		})
		h.sendMarks(sess, box)
	case 3:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		h.mu.Lock()
		var found *gram
		for _, g := range h.boxes[mine] {
			if g.id == uint32(id) {
				if !g.read && h.db != nil {
					if err := h.db.SetGramRead(g.id); err != nil {
						log.Printf("Mail: gram %d read: %v", g.id, err)
					}
				}
				g.read = true
				found = g
			}
		}
		var to []string
		var text string
		var atts []database.GramAttachment
		if found != nil {
			to, text, atts = found.to, found.text, append([]database.GramAttachment(nil), found.atts...)
		}
		h.mu.Unlock()
		if len(atts) > 0 && !sess.MailAttach() {
			// a client that cannot take them is told they are there
			text = withAttachNote(text, atts)
		}
		gramReply(sess, 3, func(w *buffer.Writer) {
			if found == nil {
				w.WriteByte(0)
				return
			}
			w.WriteByte(3)
			w.Write7BitEncodedInt64(id)
			if !disc {
				// the live read window's To line (msg+0xB0), then the text
				writeWideString(w, strings.Join(to, ";"), 1024)
			}
			writeWideString(w, text, 1024)
		})
		if found != nil {
			h.sendAttachments(sess, found.id, atts)
		}
	case 4:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		status, said, back := h.remove(me, uint32(id))
		gramReply(sess, 4, func(w *buffer.Writer) { w.WriteByte(status) })
		if said != "" {
			SendColoredChat(sess, said, 255, 255, 0)
		}
		if back != "" {
			h.tellArrived(back)
		}
	}
	return nil
}

// gramOutcome is what a send did: whether it named anyone, the names no
// character has (then it went to nobody), the names it did not go to with
// each one's reason, and how many got it.
type gramOutcome struct {
	noNames   bool
	unknown   []string
	failed    []string
	reasons   []byte
	delivered int
}

// discStatus is the outcome as the disc's one-name send is answered: 9 for
// no name or an unknown one, the reason it did not go (10, 11), else 1.
func (o gramOutcome) discStatus() byte {
	switch {
	case o.noNames || len(o.unknown) > 0:
		return gramUnknown
	case len(o.reasons) > 0:
		if r := o.reasons[0]; r == gramBoxFull || r == gramTooMany {
			return r
		}
		return 12 // "busy": the disc's one status left for "not sent"
	}
	return gramSent
}

// writeLive writes the outcome as the live client's RecvSend reads it
// (0x012F56D0): 9 and the unknown names, 4 and the names it did not go to,
// as many reasons and whether anyone got it, or 1.
func (o gramOutcome) writeLive(w *buffer.Writer) {
	names := func(list []string) {
		w.Write7BitEncodedInt64(int64(len(list)))
		for _, n := range list {
			writeString8(w, n, 24)
		}
	}
	switch {
	case o.noNames || len(o.unknown) > 0:
		w.WriteByte(gramUnknown)
		names(o.unknown)
	case len(o.failed) > 0:
		w.WriteByte(gramPartly)
		names(o.failed)
		// as many reasons as names: the display reads one a name, unbounded
		w.Write7BitEncodedInt64(int64(len(o.failed)))
		for i := range o.failed {
			w.WriteByte(o.reasons[i])
		}
		w.WriteByte(b2u8(o.delivered > 0))
	default:
		w.WriteByte(gramSent)
	}
}

// send files a gram for one name, as the disc's client sends one: the
// disc's status (deliver).
func (h *MailHandler) send(from, to, subject, text string) byte {
	return h.deliver(from, []string{to}, subject, text).discStatus()
}

// deliver is deliverFrom with no sender's id and no session to tell.
func (h *MailHandler) deliver(from string, to []string, subject, text string) gramOutcome {
	return h.deliverFrom(from, 0, to, subject, text, nil)
}

// gramRecipients is the names a send is for, trimmed, the empty ones
// dropped and each name once (the first spelling kept; the client neither
// drops repeats nor counts them).
func gramRecipients(to []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range to {
		n = entity.PlainName(n) // a drawn name's tags (entity/ogtag.go)
		k := strings.ToLower(n)
		if n == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	return out
}

// deliverFrom files a gram in the box of each name it is for, and tells each
// recipient who is on line (sub-op 5 and newMailText). The names past
// gramRecipientsMax get none (gramFailOther) and are not looked up. Of the
// others, a name no character has stops the whole send: nothing goes, and
// the unknown names are said. Else each name gets a copy unless its box is
// full (10) or holds unreadPerSender unread grams from this sender (11), or
// it is a Hardcore character's (gramFailOther, and the sender's session, when
// given, told why). Every copy keeps the names the gram was for (up to the
// cap).
func (h *MailHandler) deliverFrom(from string, fromID uint32, to []string, subject, text string, sess *session.Session) gramOutcome {
	names := gramRecipients(to)
	if len(names) == 0 {
		return gramOutcome{noNames: true}
	}
	addressed, over := names, []string(nil)
	if len(names) > gramRecipientsMax {
		addressed, over = names[:gramRecipientsMax], names[gramRecipientsMax:]
	}
	var out gramOutcome
	if h.names != nil {
		for _, n := range addressed {
			if ok, err := h.names.NameExists(n); err != nil || !ok {
				out.unknown = append(out.unknown, n)
			}
		}
		if len(out.unknown) > 0 {
			return out
		}
	}
	now := mailNow()
	var got []string
	ownerIDs := map[string]uint32{}
	for _, n := range addressed {
		if h.hardcoreNamed(n) {
			out.failed, out.reasons = append(out.failed, n), append(out.reasons, gramFailOther)
			if sess != nil {
				SendErrorMessage(sess, fmt.Sprintf(hcNoMailThemWords, n))
			}
			log.Printf("Hardcore: a gram from %s to %s refused", from, n)
			continue
		}
		if h.db != nil {
			if r, ok, err := h.db.GramRecipientNamed(n); err == nil && ok {
				ownerIDs[strings.ToLower(n)] = r.ServerID
			}
		}
	}
	h.mu.Lock()
	for _, n := range addressed {
		key := strings.ToLower(n)
		if slices.Contains(out.failed, n) {
			continue
		}
		box := h.boxes[key]
		if len(box) >= mailboxMax {
			out.failed, out.reasons = append(out.failed, n), append(out.reasons, gramBoxFull)
			continue
		}
		unread := 0
		for _, g := range box {
			if !g.read && strings.EqualFold(g.from, from) {
				unread++
			}
		}
		if unread >= unreadPerSender {
			out.failed, out.reasons = append(out.failed, n), append(out.reasons, gramTooMany)
			continue
		}
		g := &gram{from: from, fromID: fromID, ownerID: ownerIDs[key], to: append([]string(nil), addressed...), subject: subject, text: text, date: now}
		if !h.fileLocked(n, g) {
			out.failed, out.reasons = append(out.failed, n), append(out.reasons, gramFailOther)
			continue
		}
		got = append(got, n)
	}
	h.mu.Unlock()
	for _, n := range over {
		out.failed, out.reasons = append(out.failed, n), append(out.reasons, gramFailOther)
	}
	out.delivered = len(got)
	if len(got) > 0 {
		log.Printf("Mail: %s -> %s: %q", from, strings.Join(got, ", "), subject)
	}
	if len(out.failed) > 0 {
		log.Printf("Mail: %s -> not delivered to %s (reasons %v)", from, strings.Join(out.failed, ", "), out.reasons)
	}
	for _, n := range got {
		h.tellArrived(n)
	}
	return out
}

// tellArrived tells the player of that name, when on line, that a gram came.
func (h *MailHandler) tellArrived(name string) {
	if h.gameWorld == nil {
		return
	}
	if target := h.playerNamed(name); target != nil {
		if s := h.gameWorld.FindSessionByEntityID(target.ServerID); s != nil {
			notifyNewMail(s)
		}
	}
}
