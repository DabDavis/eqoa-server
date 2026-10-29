package handlers

import (
	"errors"
	"fmt"
	"log"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Customer service, as CLIENT's VICSClient drives it (decomp
// modules/client/src/vicsclient.cpp, vicsrecords.cpp; the windows
// viwndcsrequest.cpp, viwndcsinbox.cpp, viwndreadcs.cpp: Support's two
// entries, the request form and the inbox). SOE's live CLIENT (eqoa-live-1500)
// speaks it unchanged, all 40 VICSClient functions byte-identical.
//
// A request is message 0x0AA1: a sub-op byte, then its fields. Every reply is
// 0x07B0: the sub-op, a status byte (0 is success; StatusToError maps the
// rest to the codes the client shows, "System is temporarily unavailable.
// (Code: -n)"), then the rest.
//
//	0 request  in: zigzag category, zigzag subcategory, wide text
//	1 reply    in: zigzag id, wide text; the client asks for the text again
//	2 inbox    out: status, u32 (unread by the client), zigzag count, each header
//	3 open     in: zigzag id; out: status, wide text
//	4 delete   in: zigzag id; out: status, zigzag id
//	5 (out only, nothing after): something new
//	6 lists    out: status, zigzag count, each category
//
// A header: u32 id, the date (u16 year, month, day, hour, minute, second),
// u32 category, u32 subcategory, an unread byte. A category: zigzag id, a
// hidden byte, a wide name (128 bytes), zigzag count, each subcategory
// (zigzag id, a hidden byte, a wide name).
//
// The tickets are the petitions table (database/petitions.go), by character:
// a new request is acknowledged with a line in its own ticket, a reply is
// added to the ticket's text, and a GM answers in the GM panel by writing
// into the text and setting it unread (no message answers a ticket), the
// player told in chat when on line. Once a GM closes a ticket, opening,
// replying to or deleting it is answered status 8: SOE's live client says
// then that "this petition has been addressed" (word 1888); the disc's only
// "(Code: -8)". A closed ticket stays listed database.PetitionClosedListed.

const (
	opcodeCSRequest protocol.GameOpcode = 0x0AA1
	opcodeCSReply   protocol.GameOpcode = 0x07B0
)

// The statuses this server answers (StatusToError: n is code -n).
const (
	csOK        byte = 0
	csBad       byte = 1 // a request that does not read
	csTooMany   byte = 2 // database.PetitionsListed listed
	csNotFound  byte = 3 // not this character's, or deleted
	csFull      byte = 4 // the ticket's text is full (database.PetitionMaxText)
	csFailed    byte = 5 // the database failed, or there is none
	csAddressed byte = 8 // closed by a GM: the live client's "addressed"
)

// csShownMax is the most of a ticket's text CLIENT keeps (its message
// buffer, 8192 wide characters with the end).
const csShownMax = 8191

type csSub struct {
	id     int64
	hidden bool
	name   string
}

type csCategory struct {
	id     int64
	hidden bool
	name   string
	subs   []csSub
}

// csCategories are what the request form offers (a hidden one is listed but
// not offered).
var csCategories = []csCategory{
	{id: 1, name: "Bug Report", subs: []csSub{{id: 10, name: "Crash"}, {id: 11, name: "Graphics"}, {id: 12, name: "Quest"}}},
	{id: 2, name: "Stuck", subs: []csSub{{id: 20, name: "In terrain"}, {id: 21, name: "Lost in a zone"}}},
	{id: 3, name: "Player Issue", subs: []csSub{{id: 30, name: "Harassment"}, {id: 31, name: "Other", hidden: true}}},
	{id: 9, name: "Internal", hidden: true, subs: []csSub{{id: 90, name: "Notes"}}},
}

// petitionStore keeps the tickets (database.SQLiteDB).
type petitionStore interface {
	PlayerPetitions(serverID uint32, now time.Time) ([]database.Petition, error)
	FilePetition(p database.Petition, now time.Time) (database.Petition, error)
	PlayerReply(serverID uint32, id int64, name, text string, now time.Time) error
	PlayerRead(serverID uint32, id int64) (database.Petition, error)
	PlayerDelete(serverID uint32, id int64, now time.Time) error
}

// SupportHandler answers 0x0AA1 on the petitions table.
type SupportHandler struct {
	gameWorld *entity.World
	store     petitionStore // nil: every request fails (csFailed)
}

// NewSupportHandler is the handler on store (nil: none kept).
func NewSupportHandler(gw *entity.World, store petitionStore) *SupportHandler {
	return &SupportHandler{gameWorld: gw, store: store}
}

func (h *SupportHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeCSRequest, h.handleRequest)
	if h.gameWorld != nil {
		h.gameWorld.OnPlayerEntered(h.announceAnswers)
	}
	log.Printf("Support: registered handlers")
}

// csReply queues 0x07B0 with sub and status, then fill.
func csReply(sess *session.Session, sub, status byte, fill func(w *buffer.Writer)) {
	queueReliable(sess, opcodeCSReply, build(opcodeCSReply, 32768, func(w *buffer.Writer) {
		w.WriteByte(sub)
		w.WriteByte(status)
		if fill != nil {
			fill(w)
		}
	}))
}

// csTicket is a ticket's header as the inbox lists it.
type csTicket struct {
	id          uint32
	date        time.Time
	category    int64
	subcategory int64
	unread      bool
}

// ticketOf is a petition's header, its date in the server's time zone
// (mailNow's: the game's dates are PST).
func ticketOf(p database.Petition) csTicket {
	date, err := time.Parse(time.RFC3339, p.Filed)
	if err != nil {
		date = time.Now()
	}
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		date = date.In(loc)
	}
	return csTicket{id: uint32(p.ID), date: date, category: p.Category, subcategory: p.Subcategory, unread: p.Unread}
}

func writeCSHeader(w *buffer.Writer, t *csTicket) {
	w.WriteUint32(t.id)
	w.WriteUint16(uint16(t.date.Year()))
	w.WriteByte(byte(t.date.Month()))
	w.WriteByte(byte(t.date.Day()))
	w.WriteByte(byte(t.date.Hour()))
	w.WriteByte(byte(t.date.Minute()))
	w.WriteByte(byte(t.date.Second()))
	w.WriteUint32(uint32(t.category))
	w.WriteUint32(uint32(t.subcategory))
	if t.unread {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func writeCSCategories(w *buffer.Writer) {
	flag := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	w.Write7BitEncodedInt64(int64(len(csCategories)))
	for _, c := range csCategories {
		w.Write7BitEncodedInt64(c.id)
		w.WriteByte(flag(c.hidden))
		writeWideString(w, c.name, 128)
		w.Write7BitEncodedInt64(int64(len(c.subs)))
		for _, s := range c.subs {
			w.Write7BitEncodedInt64(s.id)
			w.WriteByte(flag(s.hidden))
			writeWideString(w, s.name, 128)
		}
	}
}

// CSNames is a ticket's category and subcategory by their names ("" for an
// id the lists lack).
func CSNames(cat, sub int64) (string, string) {
	for _, c := range csCategories {
		if c.id == cat {
			for _, s := range c.subs {
				if s.id == sub {
					return c.name, s.name
				}
			}
			return c.name, ""
		}
	}
	return "", ""
}

// csShown is a ticket's text as it is sent: whole when CLIENT keeps it all,
// else its end (the latest said, an answer) after a line saying so.
func csShown(text string) string {
	u := utf16.Encode([]rune(text))
	if len(u) <= csShownMax {
		return text
	}
	const lead = "(the start of this ticket is not shown)\n\n"
	tail := u[len(u)-(csShownMax-len(lead)):]
	if tail[0] >= 0xDC00 && tail[0] < 0xE000 { // not the second half of a pair alone
		tail = tail[1:]
	}
	return lead + string(utf16.Decode(tail))
}

// csStatus is a refusal from the store as its status.
func csStatus(err error) byte {
	switch {
	case err == nil:
		return csOK
	case errors.Is(err, database.ErrPetitionNotFound):
		return csNotFound
	case errors.Is(err, database.ErrPetitionClosed):
		return csAddressed
	case errors.Is(err, database.ErrTooManyPetitions):
		return csTooMany
	case errors.Is(err, database.ErrPetitionTooLong):
		return csFull
	}
	return csFailed
}

func (h *SupportHandler) handleRequest(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil || h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	if me == nil {
		return nil
	}
	if sub == 6 {
		csReply(sess, 6, csOK, writeCSCategories)
		return nil
	}
	if h.store == nil {
		var fill func(w *buffer.Writer)
		if sub == 4 { // RecvDelete reads the id before it looks at the status
			id, _ := r.Read7BitEncodedInt64()
			fill = func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) }
		}
		csReply(sess, sub, csFailed, fill)
		return nil
	}
	now := time.Now()
	switch sub {
	case 0:
		cat, err1 := r.Read7BitEncodedInt64()
		subcat, err2 := r.Read7BitEncodedInt64()
		text, ok := readWide(r)
		if err1 != nil || err2 != nil || !ok {
			csReply(sess, 0, csBad, nil)
			return nil
		}
		c, s := CSNames(cat, subcat)
		p, err := h.store.FilePetition(database.Petition{ServerID: me.CharServerID, CharName: me.Name,
			AccountID: sess.AccountID(), Category: cat, Subcategory: subcat,
			Text: fmt.Sprintf("Your request (%s / %s) has been received:\n\n%s", c, s, database.PetitionText(text))}, now)
		if err != nil {
			log.Printf("Support: %s's request refused: %v", me.Name, err)
			csReply(sess, 0, csStatus(err), nil)
			return nil
		}
		log.Printf("Support: %s filed ticket %d (%d/%d): %q", me.Name, p.ID, cat, subcat, text)
		csReply(sess, 0, csOK, nil)
	case 1:
		id, err := r.Read7BitEncodedInt64()
		text, ok := readWide(r)
		if err != nil || !ok {
			csReply(sess, 1, csBad, nil)
			return nil
		}
		err = h.store.PlayerReply(me.CharServerID, id, me.Name, text, now)
		if err == nil {
			log.Printf("Support: %s replied to ticket %d", me.Name, id)
		}
		csReply(sess, 1, csStatus(err), nil)
	case 2:
		list, err := h.store.PlayerPetitions(me.CharServerID, now)
		if err != nil {
			log.Printf("Support: %s's inbox: %v", me.Name, err)
			csReply(sess, 2, csFailed, nil)
			return nil
		}
		csReply(sess, 2, csOK, func(w *buffer.Writer) {
			w.WriteUint32(0)
			w.Write7BitEncodedInt64(int64(len(list)))
			for _, p := range list {
				t := ticketOf(p)
				writeCSHeader(w, &t)
			}
		})
	case 3:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		p, err := h.store.PlayerRead(me.CharServerID, id)
		if err != nil {
			csReply(sess, 3, csStatus(err), nil)
			return nil
		}
		csReply(sess, 3, csOK, func(w *buffer.Writer) { writeWideString(w, csShown(p.Text), 2*(csShownMax+1)) })
	case 4:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		err = h.store.PlayerDelete(me.CharServerID, id, now)
		csReply(sess, 4, csStatus(err), func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) })
	}
	return nil
}

// PetitionAnsweredText is what a player is told in chat of a GM's answer to
// its petition: no message of the game's says it (the inbox shows the ticket
// unread at its next refresh).
func PetitionAnsweredText(id int64) string {
	return fmt.Sprintf("A GM has answered your petition, Ticket #%d: read it in Support's inbox (get new headers first).", id)
}

// TellPetitionAnswered tells the character, when it is in the world, that a GM
// answered its petition id (PetitionAnsweredText), and says whether it was.
func TellPetitionAnswered(gw *entity.World, sm *session.Manager, serverID uint32, id int64) bool {
	sess := SessionOfCharacter(gw, sm, serverID)
	if sess == nil {
		return false
	}
	SendColoredChat(sess, PetitionAnsweredText(id), 255, 255, 0)
	return true
}

// announceAnswers tells a player entering the world of each answer to its
// petitions it has not read.
func (h *SupportHandler) announceAnswers(sess *session.Session, p *entity.Player) {
	if h.store == nil {
		return
	}
	list, err := h.store.PlayerPetitions(p.CharServerID, time.Now())
	if err != nil {
		return
	}
	for _, t := range list {
		if t.Status == database.PetitionOpen && t.AnswerUnread() {
			SendColoredChat(sess, PetitionAnsweredText(t.ID), 255, 255, 0)
		}
	}
}
