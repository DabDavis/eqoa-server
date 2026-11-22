package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// relive.go — Relive (work board 1927 section 9): how a 60 goes back down to earn Heirloom Marks
// (marks.go). Every Rehabilitator offers, with world.heirloom_marks, "Relive" (a level 1-59 asked) and
// "Resume at N" (its table charrelive's row, kept after a Relive ends), at 60, out of a fight, alive,
// not in world 14; and to one reliving "Return to level 60". /relive end (#relive end, a PS2's
// /t relive end) ends it too.
//
//   - Refused while anything worn but an heirloom asks a level above the one picked: the line names it.
//   - While reliving the character plays as the Relive level for everything the level decides
//     (entity/marks.go): its experience fills the Relive level's bar, a death's debt is the Relive
//     level's, its masteries sleep, a spell above the Relive level is refused; every save writes the
//     true 60 (and the Relive's row beside it), and a login while reliving relives.
//   - The Dungeon Warden refuses one reliving (dungeons.go).
//
// A Relive's start and end send a MemoryDump where the player stands.

// reliveBoxMark marks the Relive's questions.
const reliveBoxMark = 0x77000000

// The Relive's questions (marksBox.kind).
const (
	reliveAskHalf = iota + 1
	reliveAskFive
	reliveAskLevel
)

// The rehabilitators' menu answers of the Relive (rehab.go's menu).
const (
	rehabRelive    = 100 + iota // asks a level
	rehabResume                 // its row's level again
	rehabReliveEnd              // back to 60
)

// The Relive's words (ours, but the Warden's and the cast's refusals, the spec's).
const (
	reliveAnswer       = "Relive"
	reliveResumeAnswer = "Resume at %d"
	reliveEndAnswer    = "Return to level 60"
	reliveAskLevelLine = "At which level will you relive?"
	reliveWorn         = "Take off what asks more than level %d first: %s."
	reliveStarted      = "You relive at level %d. Return to level 60 at any Rehabilitator, or with /relive end."
	reliveEnded        = "You return to level 60. A Rehabilitator can let you resume at level %d."
	reliveNotNow       = "You do not relive now."
	reliveStatus       = "You relive at level %d. /relive end returns you to level 60."
	reliveTrouble      = "Forgive me, my thoughts are elsewhere. Come back in a little while."
	WardenReliveLine   = "Return to your full strength first."
)

// reliveMenu is the rehabilitator's Relive answers for p now, with what each does.
func (m *HeirloomMarks) reliveMenu(p *entity.Player) (labels []string, opts []int) {
	if m == nil || !HeirloomMarksOn() || p == nil || p.GM {
		return nil, nil
	}
	if p.Reliving() {
		return []string{reliveEndAnswer}, []int{rehabReliveEnd}
	}
	if p.Level < entity.MaxLevel {
		return nil, nil
	}
	labels, opts = []string{reliveAnswer}, []int{rehabRelive}
	if r, ok, err := m.db.CharReliveOf(p.CharServerID); err == nil && ok && !r.Active && r.Level >= 1 {
		labels = append(labels, fmt.Sprintf(reliveResumeAnswer, r.Level))
		opts = append(opts, rehabResume)
	}
	return labels, opts
}

// relivePicked is a rehabilitator's Relive answer.
func (m *HeirloomMarks) relivePicked(sess *session.Session, p *entity.Player, speaker string, what int) {
	switch what {
	case rehabRelive:
		if why := m.gw.ReliveRefusal(sess); why != "" {
			marksSay(sess, speaker, why)
			return
		}
		m.ask(sess, p, reliveBoxMark, marksBox{kind: reliveAskHalf, speaker: speaker, opts: []int{1, 31}},
			reliveAskLevelLine, []string{"A level from 1 to 30", "A level from 31 to 59"})
	case rehabResume:
		r, ok, err := m.db.CharReliveOf(p.CharServerID)
		if err != nil || !ok {
			marksSay(sess, speaker, reliveTrouble)
			return
		}
		m.start(sess, p, speaker, r.Level, r.XPInLevel, r.Debt)
	case rehabReliveEnd:
		m.end(sess, p, speaker)
	}
}

// answerRelive is HandleDialogueOption's question for a Relive box. It reports whether the box was one.
func (m *HeirloomMarks) answerRelive(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != reliveBoxMark {
		return false
	}
	if m == nil || option == 255 {
		return true
	}
	b, ok := m.answered(p, box)
	if !ok || int(option) >= len(b.opts) {
		return true
	}
	first := b.opts[option]
	switch b.kind {
	case reliveAskHalf:
		// six ranges of five from first
		var opts []int
		var labels []string
		for lo := first; lo < first+30 && lo <= entity.ReliveTop; lo += 5 {
			hi := min(lo+4, entity.ReliveTop)
			opts = append(opts, lo)
			labels = append(labels, fmt.Sprintf("Levels %d to %d", lo, hi))
		}
		m.ask(sess, p, reliveBoxMark, marksBox{kind: reliveAskFive, speaker: b.speaker, opts: opts}, reliveAskLevelLine, labels)
	case reliveAskFive:
		var opts []int
		var labels []string
		for l := first; l <= min(first+4, entity.ReliveTop); l++ {
			opts = append(opts, l)
			labels = append(labels, fmt.Sprintf("Level %d", l))
		}
		m.ask(sess, p, reliveBoxMark, marksBox{kind: reliveAskLevel, speaker: b.speaker, opts: opts}, reliveAskLevelLine, labels)
	case reliveAskLevel:
		m.start(sess, p, b.speaker, first, 0, 0)
	}
	return true
}

// start is a Relive begun at level (a Resume's xp and debt), refused when anything worn but an
// heirloom asks more.
func (m *HeirloomMarks) start(sess *session.Session, p *entity.Player, speaker string, level, xp, debt int) {
	if !HeirloomMarksOn() {
		return
	}
	if names := m.wornAbove(p.CharServerID, level); len(names) > 0 {
		marksSay(sess, speaker, fmt.Sprintf(reliveWorn, level, strings.Join(names, ", ")))
		return
	}
	if why := m.gw.StartRelive(sess, level, xp, debt, m.now()); why != "" {
		marksSay(sess, speaker, why)
		return
	}
	if m.dump != nil {
		m.dump(sess)
	}
	SendClientMessage(sess, fmt.Sprintf(reliveStarted, min(max(level, 1), entity.ReliveTop)))
}

// end is a Relive ended: back to 60, its row kept for Resume.
func (m *HeirloomMarks) end(sess *session.Session, p *entity.Player, speaker string) {
	s, ok := m.gw.EndRelive(sess)
	if !ok {
		if speaker != "" {
			marksSay(sess, speaker, reliveNotNow)
		} else {
			SendClientMessage(sess, reliveNotNow)
		}
		return
	}
	if m.dump != nil {
		m.dump(sess)
	}
	SendClientMessage(sess, fmt.Sprintf(reliveEnded, s.Level))
}

// wornAbove is the names of what a character wears, heirlooms aside, that asks a level above level.
func (m *HeirloomMarks) wornAbove(serverID uint32, level int) []string {
	if m.inv == nil {
		return nil
	}
	items, err := m.inv.GetCharacterItems(serverID)
	if err != nil {
		return []string{"(your gear could not be read)"}
	}
	var out []string
	for _, it := range items {
		if it.EquipLoc < 0 || it.Pattern.LevelReq <= level {
			continue
		}
		if m.heir != nil {
			if _, heirloom := m.heir.def(it.Pattern.PatternID); heirloom {
				continue
			}
		}
		out = append(out, strings.TrimSpace(it.Pattern.ItemName))
	}
	return out
}

// reliveCommand is /relive [end] (#relive, a PS2's /t relive end).
func (m *HeirloomMarks) reliveCommand(sess *session.Session, p *entity.Player, rest string) {
	if m == nil || !HeirloomMarksOn() {
		SendClientMessage(sess, marksOffLine)
		return
	}
	if strings.EqualFold(strings.TrimSpace(rest), "end") {
		m.end(sess, p, "")
		return
	}
	if p.Reliving() {
		SendClientMessage(sess, fmt.Sprintf(reliveStatus, p.Level))
		return
	}
	SendClientMessage(sess, reliveNotNow)
}

// saveRelive writes a Relive's row (the World's hook, off its lock).
func (m *HeirloomMarks) saveRelive(s entity.ReliveSave) {
	if m == nil || m.db == nil || s.CharServerID == 0 {
		return
	}
	since := int64(0)
	if !s.Since.IsZero() {
		since = s.Since.Unix()
	}
	if err := m.db.SaveCharRelive(database.CharRelive{ServerID: s.CharServerID, Level: s.Level, XPInLevel: s.XPInLevel,
		Debt: s.Debt, Since: since, Active: s.Active}); err != nil {
		log.Printf("Relive: character %d's row: %v", s.CharServerID, err)
	}
}

// restoreRelive is a login's Relive: an active row and a 60, the player made its Relive level before
// its gear is read (character.go's enterFromRecord).
func (m *HeirloomMarks) restoreRelive(sess *session.Session, serverID uint32) {
	if m == nil || !HeirloomMarksOn() || m.db == nil {
		return
	}
	r, ok, err := m.db.CharReliveOf(serverID)
	if err != nil || !ok || !r.Active {
		return
	}
	since := time.Time{}
	if r.Since > 0 {
		since = time.Unix(r.Since, 0)
	}
	m.gw.RestoreRelive(sess, r.Level, r.XPInLevel, r.Debt, since)
}
