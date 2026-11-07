package entity

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// combatrecord.go — each character's combat record (the user, 2026-11-07: "lets add a
// combat record for pvp and duels to the menu for players, has lifetime kills, deaths,
// etc."), lifetime, in table charrecord (database/combatrecord.go). What counts is
// decided at the two places a fight between players ends, and nowhere else:
//
//   - a PvP death (pvpDeathLocked, pvp.go): the killer's kill (a pet's, a shield's and
//     a damage tick's are its owner's, as their hits are), the victim's death, and an
//     assist for each other player whose PvP attack landed on the victim in the
//     AssistWindow before it died (a hit, its pet's or a shield's, a harmful spell, a
//     damage tick: everything pvpAttackedLocked hears). A death to a creature, a fall or
//     anything else is no PvP death and counts nothing, flagged or not. The kill streak
//     is kills since the last PvP death.
//   - a duel's end (endDuelLocked, duel.go): the winner's win and the wager it took (the
//     loser's stake), the loser's loss and its stake; a surrender is also counted as one,
//     and so is a loss by fleeing, a logout or a death to something else, which the
//     server announces as forfeits. A draw counts for both. A duel called off before its
//     fight, or cut by a restart (its stakes go back), counts nothing.
//
// The counts are written after the world lock is let go, as a duel's wager is
// (duelpurse.go): queued here and handed in order to the RecordFunc (handlers/
// combatrecord.go: one transaction each), which then puts the summary a target window
// shows back on each player it names who is in the world (SetRecordSummary) and sends a
// client with the record window its record. SaveAll writes what is still queued before a
// restart or a shutdown (FlushCombatRecords). Each shard counts in its own database.

// AssistWindow is how long before a PvP death a player's attack landing on the one
// killed makes it a helper (the user's "30 s").
const AssistWindow = 30 * time.Second

// pvpHurt is a player's PvP attack landing on another: the attacker's character, its
// name and level then, and when.
type pvpHurt struct {
	char  uint32
	name  string
	level int
	at    time.Time
}

// noteHurt keeps attacker's attack landing on p now: the latest of each character,
// those past the AssistWindow let go.
func (p *Player) noteHurt(attacker *Player, now time.Time) {
	if attacker == nil || attacker == p {
		return
	}
	kept := p.pvp.hurt[:0]
	for _, h := range p.pvp.hurt {
		if h.char != attacker.CharServerID && now.Sub(h.at) <= AssistWindow {
			kept = append(kept, h)
		}
	}
	p.pvp.hurt = append(kept, pvpHurt{char: attacker.CharServerID, name: attacker.Name, level: attacker.Level, at: now})
}

// recordFighter is p as a count names it.
func recordFighter(p *Player) database.RecordFighter {
	return database.RecordFighter{ServerID: p.CharServerID, Name: p.Name, Level: p.Level}
}

// recordKillLocked counts t's PvP death at killer's hand (pvpDeathLocked, once a death):
// the helpers are those who hurt t in the AssistWindow before it, other than the killer.
// Must be called with the lock held.
func (w *World) recordKillLocked(killer, t *Player, now time.Time) {
	k := database.PvPKill{Killer: recordFighter(killer), Victim: recordFighter(t), At: now}
	var names []string
	for _, h := range t.pvp.hurt {
		if h.char == killer.CharServerID || h.char == t.CharServerID || now.Sub(h.at) > AssistWindow {
			continue
		}
		k.Assists = append(k.Assists, database.RecordFighter{ServerID: h.char, Name: h.name, Level: h.level})
		names = append(names, h.name)
	}
	t.pvp.hurt = nil
	log.Printf("Record: %s kills %s (assists: %s)", killer.Name, t.Name, strings.Join(names, ", "))
	w.queueRecordLocked(RecordOp{Kill: &k})
}

// recordDuelLocked counts a duel's fight ending, loser losing it (0: a draw), before its
// stakes are paid (endDuelLocked). Must be called with the lock held.
func (w *World) recordDuelLocked(d *Duel, loser EntityID, how duelEnd, now time.Time) {
	if d.pvp {
		return // two flagged players' fight: its end is a death (recordKillLocked)
	}
	e := database.DuelEnd{A: recordFighter(d.who[0]), B: recordFighter(d.who[1]), At: now}
	if how != duelDraw && loser != 0 && (loser == d.A || loser == d.B) {
		side := d.side(loser)
		e.Loser = d.who[side].CharServerID
		e.Surrendered = how == duelSurrendered
		if d.held {
			e.Wager = d.Stake[side]
		}
	}
	w.queueRecordLocked(RecordOp{Duel: &e})
}

// RecordOp is one count of the combat records: a PvP death or a duel's end.
type RecordOp struct {
	Kill *database.PvPKill
	Duel *database.DuelEnd
}

// Fighters are the characters the count names.
func (op RecordOp) Fighters() []database.RecordFighter {
	var out []database.RecordFighter
	if op.Kill != nil {
		out = append(out, op.Kill.Killer, op.Kill.Victim)
		out = append(out, op.Kill.Assists...)
	}
	if op.Duel != nil {
		out = append(out, op.Duel.A, op.Duel.B)
	}
	return out
}

// RecordFunc writes a count (handlers': database/combatrecord.go), run in order after
// the world lock is let go.
type RecordFunc func(op RecordOp)

// combatRecords is the counts waiting for the hook, in order.
type combatRecords struct {
	fn    RecordFunc
	mu    sync.Mutex // the queue
	run   sync.Mutex // one runner at a time, so the counts go in order
	queue []RecordOp
}

// OnCombatRecord sets the hook that writes the counts.
func (w *World) OnCombatRecord(fn RecordFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records.mu.Lock()
	defer w.records.mu.Unlock()
	w.records.fn = fn
}

func (w *World) queueRecordLocked(op RecordOp) {
	w.records.mu.Lock()
	defer w.records.mu.Unlock()
	if w.records.fn != nil {
		w.records.queue = append(w.records.queue, op)
	}
}

// runCombatRecords writes the counts waiting, outside the world lock (every tick).
func (w *World) runCombatRecords() {
	w.records.run.Lock()
	defer w.records.run.Unlock()
	for {
		w.records.mu.Lock()
		ops := w.records.queue
		w.records.queue = nil
		fn := w.records.fn
		w.records.mu.Unlock()
		if len(ops) == 0 || fn == nil {
			return
		}
		for _, op := range ops {
			fn(op)
		}
	}
}

// FlushCombatRecords writes every count still waiting: SaveAll's, before a restart or
// a shutdown, so a kill in the last tick is not lost with the process.
func (w *World) FlushCombatRecords() {
	w.runCombatRecords()
}

// PvPAllowed reports world.pvp: whether players may flag themselves here (pvp.go).
func (w *World) PvPAllowed() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.pvpAllowed
}

// RecordSummary is what a player's target window shows of its record.
type RecordSummary struct {
	PvPKills, PvPDeaths             int
	DuelWins, DuelLosses, DuelDraws int
}

// SummaryOf is a record's summary.
func SummaryOf(r database.CombatRecord) RecordSummary {
	return RecordSummary{PvPKills: r.PvPKills, PvPDeaths: r.PvPDeaths, DuelWins: r.DuelWins, DuelLosses: r.DuelLosses, DuelDraws: r.DuelDraws}
}

// Lines are the summary in a target window: "PvP 12-3" (kills and deaths) once there is
// any, "Duels 5-2" (won and lost, "-1" more for draws) once there is any; none for an
// empty record. CLIENT wraps that window's text at 155 pixels (DrawTargetInfo's
// CountLines), and in its font "PvP 12-3, Duels 5-2" is 180, so each is a line of its
// own ("PvP 1234-567" is 126).
func (s RecordSummary) Lines() []string {
	var out []string
	if s.PvPKills != 0 || s.PvPDeaths != 0 {
		out = append(out, "PvP "+strconv.Itoa(s.PvPKills)+"-"+strconv.Itoa(s.PvPDeaths))
	}
	if s.DuelWins != 0 || s.DuelLosses != 0 || s.DuelDraws != 0 {
		l := "Duels " + strconv.Itoa(s.DuelWins) + "-" + strconv.Itoa(s.DuelLosses)
		if s.DuelDraws != 0 {
			l += "-" + strconv.Itoa(s.DuelDraws)
		}
		out = append(out, l)
	}
	return out
}

// SetRecordSummary puts the record's summary on the player of character char in the
// world, if it is.
func (w *World) SetRecordSummary(char uint32, s RecordSummary) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.playerByCharacterLocked(char); p != nil {
		p.record = s
	}
}

// PlayerOfCharacter is the player of character char in the world, or nil.
func (w *World) PlayerOfCharacter(char uint32) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.playerByCharacterLocked(char)
}

// RecordSummaryOf is the summary a target window shows of p (tests, the handlers).
func (w *World) RecordSummaryOf(p *Player) RecordSummary {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return p.record
}

// infoLineMost is the most UTF-16 units 0x0010's guild line carries (Msg0010 reads 64,
// the last its end).
const infoLineMost = 63

// withRecord is a target window's guild line with the record's lines under it: CLIENT
// prints that line whole into its text (DrawTargetInfo: name, guild, race, class or
// title, level, a line each) and counts the text's lines to size the window, so each
// "\n" is a line of its own there, on a PS2 and on vanilla alike. A line that would not
// fit the 63 units, the guild's name first, is left out: the guild is the player's own.
func withRecord(guild string, lines []string) string {
	out := guild
	for _, l := range lines {
		next := l
		if out != "" {
			next = out + "\n" + l
		}
		if len(utf16.Encode([]rune(next))) > infoLineMost {
			break
		}
		out = next
	}
	return out
}

// SetRecordWindow makes the client's answer to the combat record offer (0x7E20) its
// player's: on, its record is sent to it when it changes. It reports false when the
// session has no player in the world.
func (w *World) SetRecordWindow(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.RecordWindow = on
	return true
}

// RecordWindows are the sessions of the characters in the world, of those named, whose
// clients have the record window (SetRecordWindow), by character.
func (w *World) RecordWindows(chars []uint32) map[uint32]*session.Session {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := map[uint32]*session.Session{}
	for _, c := range chars {
		p := w.playerByCharacterLocked(c)
		if p == nil || !p.RecordWindow || w.SessionManager == nil {
			continue
		}
		if s := w.findSessionByEntityIDLocked(p.ServerID); s != nil {
			out[c] = s
		}
	}
	return out
}
