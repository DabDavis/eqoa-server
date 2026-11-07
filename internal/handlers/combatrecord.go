package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The combat record (the user, 2026-11-07: "lets add a combat record for pvp and duels
// to the menu for players, has lifetime kills, deaths, etc."): entity/combatrecord.go
// decides what counts, database/combatrecord.go keeps it (table charrecord), and this
// file writes the counts and shows the record three ways:
//
//   - /record (yours) and /record <name> (anyone's, in the world or not) from a client
//     that forwards its /commands; #record and /t record [name] from a PS2, which keeps
//     an unknown /command to itself and cannot type a # (chat.go). Lines of the PS2's
//     chat box, a row each where the names allow (recordLines).
//   - a target window: CLIENT's info window on a player (0x0010's guild line,
//     World.TargetInfoLines) shows "PvP 12-3" and "Duels 5-2" under the guild, on every
//     client.
//   - a window of its own, for a client that has one (vanilla's Character > Combat
//     Record): offered with protocol.OpcodeCombatRecord (0x7E20) at the world entry; a
//     client that answers 1 asks with OpcodeCombatRecordData (0x7E21) as its window
//     opens and is sent its record then, and again whenever a count changes it.
//
// Each shard keeps its own (its own database); a server without world.pvp shows the
// PvP half only to a character that has some.

// recordStore is the database's half.
type recordStore interface {
	RecordPvPKill(database.PvPKill) error
	RecordDuelEnd(database.DuelEnd) error
	CombatRecordOf(serverID uint32) (database.CombatRecord, error)
	CombatRecordByName(name string) (database.CombatRecord, string, bool, error)
}

// CombatRecordHandler writes the counts and answers the record's messages.
type CombatRecordHandler struct {
	world *entity.World
	db    recordStore
	now   func() time.Time

	// asked is when each character's client last had its ask answered
	// (recordAskEvery)
	askMu sync.Mutex
	asked map[uint32]time.Time
}

// recordAskEvery is how often a client's ask (0x7E21) is answered: a window opens
// once a press, and nothing a client sends faster reads the database again.
const recordAskEvery = 500 * time.Millisecond

// records is the handler a server runs, for /record (chat.go) and a keep restart's
// characters (keep.go); nil without a database.
var records atomic.Pointer[CombatRecordHandler]

// NewCombatRecordHandler creates the combat record's handler.
func NewCombatRecordHandler(w *entity.World, db recordStore) *CombatRecordHandler {
	return &CombatRecordHandler{world: w, db: db, now: time.Now, asked: map[uint32]time.Time{}}
}

// RegisterHandlers registers the record's opcodes, and has the world's counts written
// here and each login's summary read.
func (h *CombatRecordHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeCombatRecord, h.HandleAnswer)
	r.Register(protocol.OpcodeCombatRecordData, h.HandleAsk)
	if h.world == nil || h.db == nil {
		return
	}
	records.Store(h)
	h.world.OnCombatRecord(h.write)
	h.world.OnPlayerEntered(func(_ *session.Session, p *entity.Player) {
		h.readSummary(p.CharServerID)
	})
}

// buildCombatRecordOffer is OpcodeCombatRecord from the server: a u32, 1.
func buildCombatRecordOffer() []byte {
	return build(protocol.OpcodeCombatRecord, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendCombatRecordOffer offers the session's client the record, on a server that
// keeps them (a database).
func SendCombatRecordOffer(sess *session.Session) {
	if records.Load() == nil {
		return
	}
	queueReliable(sess, protocol.OpcodeCombatRecord, buildCombatRecordOffer())
}

// write is the world's hook: one count in one transaction, then each character it
// names has its summary read again and, with the window, its record sent.
func (h *CombatRecordHandler) write(op entity.RecordOp) {
	var err error
	switch {
	case op.Kill != nil:
		err = h.db.RecordPvPKill(*op.Kill)
	case op.Duel != nil:
		err = h.db.RecordDuelEnd(*op.Duel)
	default:
		return
	}
	if err != nil {
		log.Printf("Record: not written: %v", err)
		return
	}
	var chars []uint32
	for _, f := range op.Fighters() {
		chars = append(chars, f.ServerID)
	}
	sessions := h.world.RecordWindows(chars)
	for _, c := range chars {
		r, ok := h.readSummary(c)
		if !ok {
			continue
		}
		if s := sessions[c]; s != nil {
			h.send(s, r)
		}
		if p := h.world.PlayerOfCharacter(c); p != nil {
			retellTargeters(h.world, p)
		}
	}
}

// retellTargeters sends p's target window again to everyone whose client has p
// targeted: CLIENT asks for one only when its target changes, and takes a reply it did
// not ask for under the token it holds (pvp.go's face told the same way), so the
// record's lines change under their eyes.
func retellTargeters(gw *entity.World, p *entity.Player) {
	for _, v := range gw.TargetersOf(p) {
		info := playerTargetInfo(gw, v.Viewer, p, v.ObjectID, v.Token)
		info.conColor = calcConColor(v.Viewer.Level, p.Level)
		sendTargetInfo(v.Sess, &info)
	}
}

// readSummary reads char's record and puts its summary on its player in the world.
func (h *CombatRecordHandler) readSummary(char uint32) (database.CombatRecord, bool) {
	r, err := h.db.CombatRecordOf(char)
	if err != nil {
		log.Printf("Record: character %d: %v", char, err)
		return r, false
	}
	h.world.SetRecordSummary(char, entity.SummaryOf(r))
	return r, true
}

// HandleAnswer makes the client's answer to the offer its player's: a u32, nonzero to
// be sent its record when it changes, 0 to stop. Nothing is answered.
func (h *CombatRecordHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("CombatRecord: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetRecordWindow(sess, on) {
		log.Printf("CombatRecord: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("CombatRecord: %s %s", sess.RemoteAddr(), map[bool]string{true: "has the window", false: "stopped"}[on])
	return nil
}

// HandleAsk is a client's 0x7E21: its record, sent now.
func (h *CombatRecordHandler) HandleAsk(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil || h.db == nil {
		return nil
	}
	p := h.world.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	now := h.now()
	h.askMu.Lock()
	last, seen := h.asked[p.CharServerID]
	if seen && now.Sub(last) >= 0 && now.Sub(last) < recordAskEvery {
		h.askMu.Unlock()
		return nil
	}
	h.asked[p.CharServerID] = now
	h.askMu.Unlock()
	r, ok := h.readSummary(p.CharServerID)
	if !ok {
		return nil
	}
	h.send(sess, r)
	return nil
}

// send is the record to sess's client, under its player's name.
func (h *CombatRecordHandler) send(sess *session.Session, r database.CombatRecord) {
	p := h.world.SessionPlayer(sess)
	if p == nil {
		return
	}
	rows := recordRows(r, h.world.PvPAllowed(), h.now())
	queueReliable(sess, protocol.OpcodeCombatRecordData, buildCombatRecordData(p.Name, rows))
	log.Printf("CombatRecord: %s's sent (%d rows)", p.Name, len(rows))
}

// recordRow is a row of the record's window: a heading (style 0) or a label and its
// value (1).
type recordRow struct {
	style        byte
	label, value string
}

const (
	recordHeading byte = 0
	recordLine    byte = 1
)

// recordRows is the record as its window shows it: the PvP half (on a server with
// world.pvp, or for a record that has some), then the duels.
func recordRows(r database.CombatRecord, pvp bool, now time.Time) []recordRow {
	var rows []recordRow
	line := func(label, value string) { rows = append(rows, recordRow{recordLine, label, value}) }
	if pvp || r.HasPvP() {
		rows = append(rows, recordRow{style: recordHeading, label: "PvP"})
		line("Kills", fmt.Sprint(r.PvPKills))
		line("Deaths", fmt.Sprint(r.PvPDeaths))
		line("Assists", fmt.Sprint(r.Assists))
		line("Kill streak", fmt.Sprintf("%d (best %d)", r.Streak, r.BestStreak))
		line("Best kill", whoAt(r.BestKill, r.BestKillLevel, time.Time{}, now, false))
		line("Last kill", whoAt(r.LastKill, 0, r.LastKillAt, now, false))
		line("Last killed by", whoAt(r.LastDeath, 0, r.LastDeathAt, now, false))
	}
	rows = append(rows, recordRow{style: recordHeading, label: "Duels"})
	line("Won", fmt.Sprint(r.DuelWins))
	lost := fmt.Sprint(r.DuelLosses)
	if r.DuelSurrenders > 0 {
		lost += fmt.Sprintf(" (%d surrendered)", r.DuelSurrenders)
	}
	line("Lost", lost)
	line("Drawn", fmt.Sprint(r.DuelDraws))
	line("Tunar won", fmt.Sprint(r.WagerWon))
	line("Tunar lost", fmt.Sprint(r.WagerLost))
	return rows
}

// whoAt is a name with its level ("Bob, level 45") or the time since ("Bob, 2 hours
// ago"; short: "Bob, 2h ago"), "none" for no name.
func whoAt(name string, level int, at, now time.Time, short bool) string {
	if name == "" {
		return "none"
	}
	switch {
	case level > 0:
		if short {
			return fmt.Sprintf("%s (%d)", name, level)
		}
		return fmt.Sprintf("%s, level %d", name, level)
	case !at.IsZero():
		return name + ", " + ago(now.Sub(at), short)
	}
	return name
}

// ago is how long ago d was: "just now", "5 minutes ago", "2 hours ago", "3 days ago"
// (short: "5m ago", "2h ago", "3d ago").
func ago(d time.Duration, short bool) string {
	n, unit, abbr := 0, "", ""
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n, unit, abbr = int(d/time.Minute), "minute", "m"
	case d < 48*time.Hour:
		n, unit, abbr = int(d/time.Hour), "hour", "h"
	default:
		n, unit, abbr = int(d/(24*time.Hour)), "day", "d"
	}
	if short {
		return fmt.Sprintf("%d%s ago", n, abbr)
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s ago", n, unit)
}

// recordString8 is a string as the record's message carries it: a u8 byte count and
// the UTF-8, cut to 255 bytes at a character's end.
func recordString8(b []byte, s string) []byte {
	for len(s) > 255 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	b = append(b, byte(len(s)))
	return append(b, s...)
}

// buildCombatRecordData is OpcodeCombatRecordData from the server.
func buildCombatRecordData(name string, rows []recordRow) []byte {
	if len(rows) > 255 {
		rows = rows[:255]
	}
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeCombatRecordData))
	b = append(b, 1)
	b = recordString8(b, name)
	b = append(b, byte(len(rows)))
	for _, r := range rows {
		b = append(b, r.style)
		b = recordString8(b, r.label)
		b = recordString8(b, r.value)
	}
	return b
}

// recordChatMost is the most characters a line of the record in chat packs: a PS2's
// chat box wraps its text at 417 pixels (VIWndChat: 462 less 45), about 48 characters
// of these lines in its font ("Duels: 5 won, 2 lost (1 surrendered), 1 drawn." is 409).
const recordChatMost = 48

// recordLines is the record in chat: a line for the PvP counts, the streak and the
// best, last kill and last death packed into lines of recordChatMost (a name of twelve
// letters takes a line of its own), the duels and the wagers; none of a part with
// nothing in it, and one line for no record at all. A typical full record is five
// lines, the rows a PS2's chat box shows at once.
func recordLines(name string, r database.CombatRecord, pvp bool, now time.Time) []string {
	if r.Empty() {
		return []string{fmt.Sprintf("%s has no combat record yet.", name)}
	}
	var lines []string
	pack := func(parts ...string) {
		cur := ""
		for _, p := range parts {
			if p == "" {
				continue
			}
			switch {
			case cur == "":
				cur = p
			case len(cur)+1+len(p) <= recordChatMost:
				cur += " " + p
			default:
				lines = append(lines, cur)
				cur = p
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	head := name + ": "
	if pvp || r.HasPvP() {
		lines = append(lines, fmt.Sprintf("%s%d PvP %s, %d %s, %d %s.", head, r.PvPKills, plural(r.PvPKills, "kill", "kills"),
			r.PvPDeaths, plural(r.PvPDeaths, "death", "deaths"), r.Assists, plural(r.Assists, "assist", "assists")))
		head = ""
		streak, best, last, by := "", "", "", ""
		if r.PvPKills > 0 {
			streak = fmt.Sprintf("Kill streak %d (best %d).", r.Streak, r.BestStreak)
			best = fmt.Sprintf("Best kill: %s.", whoAt(r.BestKill, r.BestKillLevel, time.Time{}, now, true))
			last = fmt.Sprintf("Last kill: %s.", whoAt(r.LastKill, 0, r.LastKillAt, now, true))
		}
		if r.PvPDeaths > 0 {
			by = fmt.Sprintf("Killed by: %s.", whoAt(r.LastDeath, 0, r.LastDeathAt, now, true))
		}
		pack(streak, best)
		pack(last, by)
	}
	if r.HasDuels() {
		d := fmt.Sprintf("%d won, %d lost", r.DuelWins, r.DuelLosses)
		if r.DuelSurrenders > 0 {
			d += fmt.Sprintf(" (%d surrendered)", r.DuelSurrenders)
		}
		if r.DuelDraws > 0 {
			d += fmt.Sprintf(", %d drawn", r.DuelDraws)
		}
		if head != "" {
			lines = append(lines, fmt.Sprintf("%s's duels: %s.", name, d))
		} else {
			lines = append(lines, fmt.Sprintf("Duels: %s.", d))
		}
		if r.WagerWon > 0 || r.WagerLost > 0 {
			lines = append(lines, fmt.Sprintf("Wagers: %d tunar won, %d lost.", r.WagerWon, r.WagerLost))
		}
	}
	return lines
}

// recordCommand is /record [name] from sess's player: the record of the character of
// that name, its own without one, in chat.
func recordCommand(gw *entity.World, sess *session.Session, me *entity.Player, name string) {
	h := records.Load()
	if h == nil || gw == nil {
		SendClientMessage(sess, "This server keeps no combat records.")
		return
	}
	name = strings.TrimSpace(name)
	var r database.CombatRecord
	var err error
	spelt := me.Name
	if name == "" || strings.EqualFold(name, me.Name) {
		r, err = h.db.CombatRecordOf(me.CharServerID)
	} else {
		var found bool
		r, spelt, found, err = h.db.CombatRecordByName(name)
		if err == nil && !found {
			SendClientMessage(sess, fmt.Sprintf("No character is named %s.", consoleLine(name, 24)))
			return
		}
	}
	if err != nil {
		log.Printf("Record: %s's /record %q: %v", me.Name, name, err)
		SendClientMessage(sess, "The combat record could not be read.")
		return
	}
	for _, l := range recordLines(spelt, r, gw.PvPAllowed(), h.now()) {
		SendClientMessage(sess, l)
	}
	log.Printf("Record: %s asked for %s's", me.Name, spelt)
}

// resumeRecord reads a character carried through a keep restart's summary again (a
// count may have come in the hold), keep.go's.
func resumeRecord(char uint32) {
	if h := records.Load(); h != nil {
		h.readSummary(char)
	}
}
