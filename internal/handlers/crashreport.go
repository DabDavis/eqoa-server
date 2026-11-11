package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// crashreport.go — crash reports, back as petitions (the owner, 2026-11-11,
// after two raid members' clients stopped at once: "we may need to add crash
// reports back"). Vanilla sent its crashes to a report server of their own
// until 2026-10-31; now a client keeps its crash where it runs (natively a
// file in its config directory, in a page the browser's storage) and at its
// next login hands it to the server it plays on, which files it as a petition
// of its character's: Bug Report / Crash (csCategories' hidden topic), open,
// its text the crash's first lines and its server view (petitionview.go) the
// report whole, the client's build, platform and graphics with it, beside the
// server's own record of the client stopping when it kept one
// (session.ClientStopOf: the session silent in the world, no logout). It asks
// the player nothing, so it is filed not waiting on a GM (database.Petition's
// NoAnswer): it is listed open on the GM panel's Petitions page, under its
// topic, until a GM closes it; a GM may still answer it, and the player reads
// it in Support's inbox. Online GMs are told of it as of any petition.
//
// The client is offered them (OpcodeCrashReportOffer) once the player is in
// the world; one that kept a report sends it (OpcodeCrashReport) and lets it
// go at the answer (OpcodeCrashReportFiled), which names the petition, or 0
// when it was not kept (empty, the account's limit, the store's refusal): a
// report is never sent twice. A PS2 ignores the offer and sends none.

// CrashReportMost is the most bytes of a report's text the server takes (the
// offer says it): a panic's message and a backtrace of a few dozen frames, in
// pieces of DRDP's 1,300 (session.FragmentedMost holds 16 KB).
const CrashReportMost = 12000

// crashReportsPerHour is how many crash reports an account may file in an
// hour (a GM's any): a client that crashes at every start sends one a start.
const crashReportsPerHour = 6

// crashSummaryMost is the most characters of the report's summary in the
// petition's text, which the player reads too.
const crashSummaryMost = 400

// buildCrashReportOffer is OpcodeCrashReportOffer: a u32 version (1), then a
// u32, the most bytes of a report's text.
func buildCrashReportOffer() []byte {
	return build(protocol.OpcodeCrashReportOffer, 10, func(w *buffer.Writer) {
		w.WriteUint32(1)
		w.WriteUint32(CrashReportMost)
	})
}

// SendCrashReportOffer offers the session's client to take its crash reports.
// A PS2 ignores it.
func SendCrashReportOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeCrashReportOffer, buildCrashReportOffer())
}

// sendCrashReportFiled answers a report: its number, and its petition (0 not
// kept).
func sendCrashReportFiled(sess *session.Session, number uint32, ticket int64) {
	queueReliable(sess, protocol.OpcodeCrashReportFiled, build(protocol.OpcodeCrashReportFiled, 10, func(w *buffer.Writer) {
		w.WriteUint32(number)
		w.WriteUint32(uint32(max(ticket, 0)))
	}))
}

// readCrashReport reads OpcodeCrashReport's body: its number and its text,
// false when it is short or its count is past CrashReportMost.
func readCrashReport(data []byte) (number uint32, text string, ok bool) {
	r := buffer.NewReader(data)
	if _, err := r.ReadUint32(); err != nil {
		return 0, "", false
	}
	number, err := r.ReadUint32()
	if err != nil {
		return 0, "", false
	}
	n, err := r.ReadUint32()
	if err != nil || n > CrashReportMost {
		return number, "", false
	}
	b, err := r.ReadBytes(int(n))
	if err != nil {
		return number, "", false
	}
	return number, string(b), true
}

// cleanCrash is a report as kept: valid UTF-8, no control character but a
// line end and a tab, at most CrashReportMost bytes.
func cleanCrash(s string) string {
	s = strings.ToValidUTF8(s, "?")
	var b strings.Builder
	for _, r := range s {
		if r == '\r' {
			continue
		}
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7F {
			r = ' '
		}
		if b.Len()+len(string(r)) > CrashReportMost {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// crashSummary is the report's line the player reads in the petition: its
// "panic:" line (vanilla's report says the panic's place and message there),
// else its first, at most crashSummaryMost characters.
func crashSummary(report string) string {
	lines := strings.Split(report, "\n")
	pick := strings.TrimSpace(lines[0])
	for _, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "panic:") {
			pick = strings.TrimSpace(strings.TrimPrefix(t, "panic:"))
			break
		}
	}
	if r := []rune(pick); len(r) > crashSummaryMost {
		pick = string(r[:crashSummaryMost]) + "..."
	}
	return pick
}

// crashFiled is the line a player is told when its client's report is filed.
const crashFiled = "Your game client stopped last time; its crash report went to the GMs as Ticket #%d. Thank you: no answer is needed."

// handleCrashReport files a client's crash report as a petition of its
// character's (the file's doc) and answers it.
func (h *SupportHandler) handleCrashReport(sess *session.Session, msg *protocol.Message) error {
	number, raw, ok := readCrashReport(msg.Data())
	if !ok {
		log.Printf("CrashReport: %s sent one that does not read (%d bytes)", sess.Label(), len(msg.Data()))
		sendCrashReportFiled(sess, number, 0)
		return nil
	}
	report := cleanCrash(raw)
	me := h.sessionPlayer(sess)
	if report == "" || me == nil || h.store == nil {
		log.Printf("CrashReport: %s's report %d not kept (empty %v, in the world %v, a store %v)",
			sess.Label(), number, report == "", me != nil, h.store != nil)
		sendCrashReportFiled(sess, number, 0)
		return nil
	}
	now := time.Now()
	p, err := h.fileCrash(sess, me, report, now)
	if err != nil {
		log.Printf("CrashReport: %s's report %d (%d bytes) not kept: %v", me.Name, number, len(report), err)
		sendCrashReportFiled(sess, number, 0)
		return nil
	}
	sendCrashReportFiled(sess, number, p.ID)
	SendColoredChat(sess, fmt.Sprintf(crashFiled, p.ID), 255, 255, 0)
	return nil
}

// sessionPlayer is sess's character in the world, nil with none.
func (h *SupportHandler) sessionPlayer(sess *session.Session) *entity.Player {
	if h.gameWorld == nil {
		return nil
	}
	return h.gameWorld.SessionPlayer(sess)
}

// fileCrash files report for the session's character: Bug Report / Crash, not
// waiting on a GM, with the server's view and its record of the client
// stopping; online GMs told. Refused past the account's crashReportsPerHour.
func (h *SupportHandler) fileCrash(sess *session.Session, me *entity.Player, report string, now time.Time) (database.Petition, error) {
	if !h.isGM(sess) && !h.crashLimit.allowN(sess.AccountID(), now, crashReportsPerHour) {
		return database.Petition{}, errReportLimit
	}
	const cat, sub = 1, crashSubcategory
	c, s := CSNames(cat, sub)
	stop, stopped := session.ClientStopOf(me.CharServerID)
	view := h.serverViewWith(sess, me, "crash report", now, func(v *serverView) {
		v.Crash = report
		if stopped {
			v.Stopped = &stop
		}
	})
	text := fmt.Sprintf("Your crash report (%s / %s) has been received:\n\nThe game client stopped: %s\n\nNo answer is needed: the GMs read the client's report with it.",
		c, s, crashSummary(report))
	p, err := h.store.FilePetition(database.Petition{ServerID: me.CharServerID, CharName: me.Name,
		AccountID: sess.AccountID(), Category: cat, Subcategory: sub, ServerView: view, NoAnswer: true,
		Text: database.PetitionText(text)}, now)
	if err != nil {
		return p, err
	}
	stopLine := "none kept"
	if stopped {
		stopLine = stop.String()
	}
	log.Printf("CrashReport: %s filed ticket %d (%d bytes; the server's record: %s): %s", me.Name, p.ID, len(report), stopLine, crashSummary(report))
	h.tellGMs(sess, p, c, s, crashSummary(report))
	return p, nil
}

// crashSubcategory is Bug Report's hidden Crash topic (csCategories).
const crashSubcategory = 10
