package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Bug reports, the players' way to this server's GMs (the user's choice of
// 2026-11-05: a petition goes over the game's own connection to the server
// the player is on, so a server hears only its own players). Three ways in,
// all one petition (support.go's tickets, the GM panel's Petitions page):
//
//   - Support's request form, Bug Report and a topic, on every client (the
//     disc's own window);
//   - /bug <what happened> (vanilla forwards an unknown /command) or #bug (a
//     PS2 keeps an unknown /command to itself but sends a # line as chat):
//     filed on the spot under Bug Report / Sent from chat; /feature or
//     #feature <what you would like> the same, under Bug Report / Feature
//     Request (petition 1841), apart from the bugs;
//   - vanilla's F3, which opens Support's form on Bug Report, from a client
//     that answered this server's offer (protocol.OpcodeBugReportOffer).
//
// Each petition is filed with the server's own view of the moment
// (petitionview.go: where the player stands, its target, #stats' lines, what
// it was shown lately, its quests, the build) and with the client's note
// (protocol.OpcodeBugReportNote) when its client sent one; the GMs read both
// in the panel, the player never does. Online GMs are told of each new one in
// chat. A login is told once how to report (SetReportNotice).

// BugNoteMax is the most of a client's note kept, in bytes.
const BugNoteMax = 4000

// The filings an account may make: one every reportGap, reportsPerHour in an
// hour, whichever way they come (a form refused is answered csFailed and told
// why in chat). A GM's account may make any (isGM).
const (
	reportGap      = 15 * time.Second
	reportsPerHour = 12
)

// reportNoticeAfter is how long after a login its line on reporting goes out
// to a client that has not answered the offer (a PS2): a client that answers
// is told at its answer (F3's line), which comes once it has loaded its world.
const reportNoticeAfter = 15 * time.Second

var reportNotice atomic.Bool

func init() { reportNotice.Store(true) }

// SetReportNotice turns the login's line on how to report a bug on or off
// (world.report_notice; on unless set false).
func SetReportNotice(on bool) { reportNotice.Store(on) }

// The login's lines on reporting: one for a client with the F3 key (it
// answered the offer), one for everyone else.
const (
	reportNoticeF3   = "Found a bug? Press F3, or type /bug and what went wrong, right where it happened. A GM answers in Support's inbox (start menu)."
	reportNoticeText = "Found a bug? Type #bug and what went wrong, right where it happened, or use Support in the start menu. A GM answers in Support's inbox."
)

// bugSubcategory is the hidden topic a chat report is filed under (support.go
// csCategories).
const bugSubcategory = 19

// featureSubcategory is Bug Report's Feature Request topic (support.go
// csCategories; petition 1841): offered on the form to every client, and
// /feature's (or #feature's). Bug Report's own ids, 10-19, are all taken, so
// it is the first of a second hundred: no category's tens (20s Stuck, 30s
// Player Issue, 90s Internal), and CLIENT keeps no table of its own (its
// lists are the server's, VICSClient::RecvCategories). Vanilla's /feature
// opens the form on it by this id (cs::FEATURE_REQUEST): never renumber it.
const featureSubcategory = 100

// reportLimiter counts each account's filings (off: none refused, for tests
// that file many).
type reportLimiter struct {
	mu   sync.Mutex
	seen map[int][]time.Time
	off  bool
}

// allow is whether account may file at now, kept as a filing when it may.
func (l *reportLimiter) allow(account int, now time.Time) bool {
	return l.allowN(account, now, reportsPerHour)
}

// allowN is allow with perHour filings an hour.
func (l *reportLimiter) allowN(account int, now time.Time, perHour int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.off {
		return true
	}
	if l.seen == nil {
		l.seen = map[int][]time.Time{}
	}
	var kept []time.Time
	for _, t := range l.seen[account] {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perHour || (len(kept) > 0 && now.Sub(kept[len(kept)-1]) < reportGap) {
		l.seen[account] = kept
		return false
	}
	l.seen[account] = append(kept, now)
	return true
}

// buildBugReportOffer is OpcodeBugReportOffer from the server: a u32, 1, then
// a u32, the most characters of a request's or reply's text it takes
// (CSTextMost). A client of before the second reads the first alone, and a
// client keeps its form at a PS2's 511 for a server that sends no second (one
// from before it put a long message's pieces back together).
func buildBugReportOffer() []byte {
	return build(protocol.OpcodeBugReportOffer, 10, func(w *buffer.Writer) {
		w.WriteUint32(1)
		w.WriteUint32(CSTextMost)
	})
}

// SendBugReportOffer offers the session's client the bug report key and note.
// A PS2 ignores it.
func SendBugReportOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeBugReportOffer, buildBugReportOffer())
}

// handleBugReportOffer keeps the client's answer to the offer: a u32, nonzero
// when it has the key and sends its note.
func (h *SupportHandler) handleBugReportOffer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	sess.SetBugReports(on)
	sess.SetAckMasks() // any answer: one of ours, which takes our acks' masks
	log.Printf("BugReports: %s %s", sess.RemoteAddr(), map[bool]string{true: "reports with F3 and a note", false: "has no report key"}[on])
	// a client that answers is one of ours, not a PS2: it may be offered the
	// jump (jump.go), which no PS2 is ever sent
	if on {
		OfferJump(sess)
	}
	// the login's line on reporting, F3's, now (once a session)
	if on && reportNotice.Load() && sess.CharacterInWorld() && sess.TakeReportNotice() {
		SendClientMessage(sess, reportNoticeF3)
	}
	return nil
}

// handleBugReportNote keeps the client's note for the next petition it files:
// a u32 version, then a u32 byte count and the UTF-8 text.
func (h *SupportHandler) handleBugReportNote(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	if _, err := r.ReadUint32(); err != nil {
		return nil
	}
	n, err := r.ReadUint32()
	if err != nil || n > 1<<16 {
		return nil
	}
	b, err := r.ReadBytes(int(n))
	if err != nil {
		return nil
	}
	note := cleanNote(string(b))
	sess.SetBugNote(note, time.Now())
	return nil
}

// cleanNote is a client's note as kept: valid UTF-8, no control character but
// a line end and a tab, at most BugNoteMax bytes.
func cleanNote(s string) string {
	s = strings.ToValidUTF8(s, "?")
	var b strings.Builder
	for _, r := range s {
		if r == '\r' {
			continue
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			r = ' '
		}
		if b.Len()+utf8.RuneLen(r) > BugNoteMax {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// fileTicket files a petition for the session's character with the server's
// view of the moment, the client's note (when it sent one) and online GMs
// told; via says how it came ("support", "chat"). It is refused by the store,
// or by the account's limit (errReportLimit).
func (h *SupportHandler) fileTicket(sess *session.Session, me *entity.Player, cat, sub int64, text, via string, now time.Time) (database.Petition, error) {
	return h.fileTicketWith(sess, me, cat, sub, text, via, now, nil)
}

// fileTicketWith is fileTicket with more of the server view's fields set (more,
// when not nil: a portal's, gmportal.go).
func (h *SupportHandler) fileTicketWith(sess *session.Session, me *entity.Player, cat, sub int64, text, via string, now time.Time,
	more func(v *serverView)) (database.Petition, error) {
	if !h.isGM(sess) && !h.limit.allow(sess.AccountID(), now) {
		return database.Petition{}, errReportLimit
	}
	c, s := CSNames(cat, sub)
	view := h.serverViewWith(sess, me, via, now, more)
	p, err := h.store.FilePetition(database.Petition{ServerID: me.CharServerID, CharName: me.Name,
		AccountID: sess.AccountID(), Category: cat, Subcategory: sub, ServerView: view,
		Text: fmt.Sprintf("Your request (%s / %s) has been received:\n\n%s", c, s, database.PetitionText(text))}, now)
	if err != nil {
		return p, err
	}
	log.Printf("Support: %s filed ticket %d (%d/%d, %s, a server view of %d bytes): %q", me.Name, p.ID, cat, sub, via, len(view), text)
	h.tellGMs(sess, p, c, s, text)
	return p, nil
}

var errReportLimit = fmt.Errorf("too many reports lately")

// isGM is whether sess's account is a GM's (acctlevel 1 or more), whose
// reports the limit does not count (the GM Jibrish files many while testing,
// 2026-11-10: the owner, "lets raise limit").
func (h *SupportHandler) isGM(sess *session.Session) bool {
	if h.access == nil || sess.AccountID() == 0 {
		return false
	}
	level, err := h.access.AccessLevel(sess.AccountID())
	return err == nil && level >= 1
}

// reportLimitText is a refused filing's line.
const reportLimitText = "You have sent several reports lately: please wait a minute and send it again."

// TellGMs says a line in yellow to every GM in the world (acctlevel 1 or
// more) but from's session: a new petition, a new character transfer.
func TellGMs(sm *session.Manager, access accessLevels, from *session.Session, line string) {
	if sm == nil || access == nil {
		return
	}
	sm.ForEach(func(sess *session.Session) bool {
		if sess == from || !sess.CharacterInWorld() || sess.PendingTermination() || sess.AccountID() == 0 {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced {
			return true
		}
		if level, err := access.AccessLevel(sess.AccountID()); err == nil && level >= 1 {
			SendColoredChat(sess, line, 255, 255, 0)
		}
		return true
	})
}

// tellGMs says a new petition to every GM in the world but its own filer.
func (h *SupportHandler) tellGMs(from *session.Session, p database.Petition, cat, sub, text string) {
	if h.sessions == nil || h.access == nil || h.gameWorld == nil {
		return
	}
	short := strings.Join(strings.Fields(consoleLine(text, len(text))), " ")
	if r := []rune(short); len(r) > 100 {
		short = string(r[:100]) + "..."
	}
	TellGMs(h.sessions, h.access, from, fmt.Sprintf("[Petition] Ticket #%d from %s (%s / %s): %s", p.ID, p.CharName, cat, sub, short))
}

// ChatReport is /bug (or #bug) <what happened>: a petition on the spot under
// Bug Report / Sent from chat, the player told its ticket number.
func (h *SupportHandler) ChatReport(sess *session.Session, me *entity.Player, text string) {
	h.chatFile(sess, me, text, bugSubcategory, "/bug", "report",
		"Type /bug (or #bug) and what went wrong, right where it happened: it goes to the GMs with where you are and what you were doing. A GM answers in Support's inbox (start menu).")
}

// ChatFeature is /feature (or #feature) <what you would like>: a petition on
// the spot under Bug Report / Feature Request (petition 1841), apart from the
// bugs, the player told its ticket number. A PS2 can pick the topic on
// Support's form too; this is the typed way.
func (h *SupportHandler) ChatFeature(sess *session.Session, me *entity.Player, text string) {
	h.chatFile(sess, me, text, featureSubcategory, "/feature", "feature request",
		"Type /feature (or #feature) and what you would like added or changed: it goes to the GMs as a feature request, apart from the bug reports. A GM answers in Support's inbox (start menu).")
}

// chatFile files text typed after word under Bug Report / sub (kind is what
// the player is told it is); with no text, how is said.
func (h *SupportHandler) chatFile(sess *session.Session, me *entity.Player, text string, sub int64, word, kind, how string) {
	text = strings.TrimSpace(text)
	if text == "" {
		SendClientMessage(sess, how)
		return
	}
	if h.store == nil {
		SendErrorMessage(sess, "This server keeps no reports.")
		return
	}
	p, err := h.fileTicket(sess, me, 1, sub, text, "chat", time.Now())
	switch {
	case err == errReportLimit:
		SendErrorMessage(sess, reportLimitText)
	case err != nil:
		log.Printf("Support: %s's %s refused: %v", me.Name, word, err)
		SendErrorMessage(sess, "Your "+kind+" could not be kept: "+refusalText(err))
	default:
		SendColoredChat(sess, fmt.Sprintf("Thank you: your %s is Ticket #%d. A GM answers in Support's inbox (start menu).", kind, p.ID), 255, 255, 0)
	}
}

// refusalText is a refusal from the store as a player reads it.
func refusalText(err error) string {
	switch {
	case err == database.ErrTooManyPetitions:
		return "you have too many open tickets."
	case err == database.ErrPetitionTooLong:
		return "it is too long."
	}
	return "please try again later."
}

// noticeReporting tells a player entering the world, once a session, how to
// report a bug: a client that answers the offer is told F3's line at its
// answer (handleBugReportOffer); one that has not by reportNoticeAfter (a PS2)
// is told the line for every client.
func (h *SupportHandler) noticeReporting(sess *session.Session, p *entity.Player) {
	if !reportNotice.Load() {
		return
	}
	time.AfterFunc(reportNoticeAfter, func() {
		if !sess.CharacterInWorld() || sess.PendingTermination() || !sess.TakeReportNotice() {
			return
		}
		if _, replaced := sess.ReplacedFor(); replaced {
			return
		}
		if sess.BugReports() {
			SendClientMessage(sess, reportNoticeF3)
		} else {
			SendClientMessage(sess, reportNoticeText)
		}
	})
}
