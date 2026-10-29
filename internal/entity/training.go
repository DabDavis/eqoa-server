package entity

import (
	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Training points (eqoa.fandom.com's Formulas, "TPs per Level": 1-14 3 a
// level, 15-29 5, 30-44 9, 45-60 14), read as the points a level gives when
// it is LEFT: going from level l to l+1 gives l's bracket's, so reaching 2 to
// 15 gives 3 each, 16 to 30 5, 31 to 45 9, 46 to 60 14. SOE's live server
// agrees with that reading better than with "reaching level l gives l's":
// Corsten (level 60) and Lostologist (45), with no point unspent, have base
// stats 465 and 256 over their creation (60 a stat, the race's and class's
// bonuses, the 20 points the create screen gives); left, 462 and 252;
// reached, 473 and 263. A character also has the create screen's 20.

// CreationTrainingPoints are the points the create screen gives (characters'
// totalTP starts there).
const CreationTrainingPoints = 20

// TrainingPerLevel is what leaving level l gives.
func TrainingPerLevel(l int) int {
	switch {
	case l < 1 || l >= 60:
		return 0
	case l <= 14:
		return 3
	case l <= 29:
		return 5
	case l <= 44:
		return 9
	}
	return 14
}

// TrainingPointsDue is every training point a character of that level has
// been given: the create screen's and each level's below it.
func TrainingPointsDue(level int) int {
	n := CreationTrainingPoints
	for l := 1; l < min(level, 60); l++ {
		n += TrainingPerLevel(l)
	}
	return n
}

// TopUpTraining is a character's training points made up to what its level
// is due, never twice for the same levels: total is every point it has been
// given (a lower level keeps what it had; a row holding none counts as having
// had the create screen's), unused the ones not spent. It gives the new
// unused and total and how many were given.
func TopUpTraining(level, unused, total int) (int, int, int) {
	have := max(total, CreationTrainingPoints)
	give := max(0, TrainingPointsDue(level)-have)
	return unused + give, have + give, give
}

// TopUpTrainingPoints makes the player's training points up to its level's
// (TopUpTraining): how many were given.
func (p *Player) TopUpTrainingPoints() int {
	var give int
	p.UnusedTP, p.TotalTP, give = TopUpTraining(p.Level, p.UnusedTP, p.TotalTP)
	return give
}

// SendTrainingPoints is 0x0036: a byte (1: sound 7) and the unspent training
// points (VIPlayer m_p40, the trainer's to spend), zigzag. CLIENT says "You
// have received more training points!" (word 625) when they rise.
func SendTrainingPoints(sess *session.Session, unused int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTrainingPoints)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodeTrainingPoints))
	w.WriteByte(1)
	w.Write7BitEncodedInt64(int64(unused))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// LiveTrainingPoints is a live player's unspent and total training points,
// for a save; false when it is not in the world.
func (w *World) LiveTrainingPoints(id EntityID) (unused, total int, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p, found := w.Players[id]
	if !found {
		return 0, 0, false
	}
	return p.UnusedTP, p.TotalTP, true
}

// TakeBackTraining undoes the training of an online character (a GM's
// training reset, handlers/gmresets.go): each stat's base lowered by the
// points it had taken (spent, STR..CHA, as the character's tp columns held
// them: its base is its race's and class's plus those), never below its
// race's and class's (what its next login makes of the columns at 0, should a
// hand's edit of them have left the game's base behind), the points added to
// its unspent ones, the stats, maxima, hit points and power worked out again
// and the stat channel sent again. It gives the unspent points before and
// after and the total; false when the character is not in the world.
func (w *World) TakeBackTraining(serverID uint32, spent [7]int) (before, after, total int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil {
		return 0, 0, 0, false
	}
	bs := LookupBaseStats(p.Race, p.Class)
	floor := [7]int{bs.Str, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha}
	base := []*int{&p.BaseStr, &p.BaseSta, &p.BaseAgi, &p.BaseDex, &p.BaseWis, &p.BaseInt, &p.BaseCha}
	refund := 0
	for i, v := range spent {
		*base[i] = max(*base[i]-v, floor[i])
		refund += v
	}
	before = p.UnusedTP
	p.UnusedTP += refund
	p.TotalTP = max(p.TotalTP, p.UnusedTP)
	p.RecalcStats(false)
	p.StatsDirty = true
	return before, p.UnusedTP, p.TotalTP, true
}
