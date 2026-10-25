package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Class masteries and the stat caps they raise.
//
// CLIENT keeps a stat cap of its own (decomp viclient.cpp GetP44Total): the
// player record's m_p44 (the MemoryDump's player record, and 0x001C) plus the
// mastery part's m_bonus[stat] (the dump's mastery block, ReadBonuses). Its
// trainer shows a stat as min(the stat channel's total, that cap) and spends
// no training point past m_p44 alone (viwndtrainer.cpp). SOE's live server
// sent m_p44 = 400 to a level 60 (Corsten, 2014) and 350 to a level 45
// (Lostologist, 2015): read out of both retail save states' VIPlayer. Return
// Home's server steps 350 to 400 at level 45, which the level 45 refutes by
// one; nothing measured lies between 46 and 59, so the step is put at 46.
//
// The masteries that raise a cap are EQOA-Data's (Information/databases/cms):
// each stat's general one (Brawn, Vigor, Nimbleness, Balance, Insight, Wit,
// Beauty) by 15, 20, 25, 30 and 35 at levels 10, 20, 30, 45 and 60, and
// Godborn every stat by 25 at 50: 400 + 125 + 25 = 550 at level 60, the cap
// the live game had "including the normal 5 ranks" (F3 20261025-163704);
// master classes ("tag") and racial masteries raise their own stats further.
//
// This server cannot sell a mastery yet (handlers/classmastery.go replays a
// captured tree), so every character counts the general ones its level
// allows, as if it had bought them; master class and racial ones none.

// MasteryCap is the most mastery points a character holds, spent and
// unspent together (F3 20261025-165747).
const MasteryCap = 1500

// DefaultCMXPPerPoint is how much experience makes a mastery point unless
// world.cm_xp_per_point says otherwise. No capture or source gives the live
// game's rate: it is this server's, sized to its experience (a kill gives a
// hundred a level of the creature).
const DefaultCMXPPerPoint = 10000

// StatCapBase is the stat cap before masteries at a level (CLIENT's m_p44):
// 350, and 400 past level 45.
func StatCapBase(level int) int {
	if level > 45 {
		return 400
	}
	return 350
}

// generalCapRanks are the general stat-cap masteries' ranks: the level each
// may be held from and what it raises a stat's cap by.
var generalCapRanks = [...]struct{ level, raise int }{
	{10, 15}, {20, 20}, {30, 25}, {45, 30}, {60, 35},
}

// GeneralCapRaise is what the general masteries a level allows raise each
// stat's cap by: the stat's own five ranks and Godborn (25 at level 50).
func GeneralCapRaise(level int) int {
	n := 0
	for _, r := range generalCapRanks {
		if level >= r.level {
			n += r.raise
		}
	}
	if level >= 50 {
		n += 25
	}
	return n
}

// StatCap is the most a stat (StatStr to StatCha) may be at the player's
// level, its masteries' raise included.
func (p *Player) StatCap() int {
	return StatCapBase(p.Level) + GeneralCapRaise(p.Level)
}

// capStat is v within the player's stat cap.
func (p *Player) capStat(v int) int {
	return min(v, p.StatCap())
}

// Mastery is a character's class mastery points: unspent, spent, the share
// of experience that goes to them (0-100, the mastery points window's) and
// the experience gathered toward the next point.
type Mastery struct {
	Points, Spent, Percent, XP int
}

// SetCMXPPerPoint sets how much experience makes a mastery point (at least 1).
func (w *World) SetCMXPPerPoint(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cmXPPerPoint = max(1, n)
}

// CMXPPerPoint is how much experience makes a mastery point.
func (w *World) CMXPPerPoint() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cmXPPerPointLocked()
}

func (w *World) cmXPPerPointLocked() int {
	if w.cmXPPerPoint <= 0 {
		return DefaultCMXPPerPoint
	}
	return w.cmXPPerPoint
}

// splitXP is an award of xp as the player's mastery share divides it: what
// goes to the level and what to mastery points. Holding MasteryCap points,
// all of it goes to the level.
func splitXP(m Mastery, xp int) (level, mastery int) {
	if xp <= 0 || m.Points+m.Spent >= MasteryCap {
		return xp, 0
	}
	pct := max(0, min(100, m.Percent))
	mastery = xp * pct / 100
	return xp - mastery, mastery
}

// gainMastery puts xp of mastery experience on m, a point for every perPoint
// of it, never past MasteryCap points: the points it made.
func gainMastery(m *Mastery, xp, perPoint int) int {
	if xp <= 0 || perPoint <= 0 {
		return 0
	}
	m.XP += xp
	made := 0
	for m.XP >= perPoint && m.Points+m.Spent < MasteryCap {
		m.XP -= perPoint
		m.Points++
		made++
	}
	if m.Points+m.Spent >= MasteryCap {
		m.XP = 0
	}
	return made
}

// grantMastery is #grantcm's: amount points more, never past MasteryCap
// spent and unspent together. It reports how many were given.
func grantMastery(m *Mastery, amount int) int {
	give := max(0, min(amount, MasteryCap-m.Points-m.Spent))
	m.Points += give
	return give
}

// MasteryOf is a copy of a player's mastery points, for the MemoryDump and
// the save at a logout; false when the player is not in the world.
func (w *World) MasteryOf(id EntityID) (Mastery, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p, ok := w.Players[id]
	if !ok {
		return Mastery{}, false
	}
	return p.Mastery, true
}

// RestoreMastery puts a character's kept mastery points on its session's
// player at the login.
func (w *World) RestoreMastery(sess *session.Session, m Mastery) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		m.Percent = max(0, min(100, m.Percent))
		p.Mastery = m
	}
}

// SetMasteryPercent is the mastery points window's share (0x1401 sub-op 1),
// kept within 0-100, as CLIENT's Select clamps it: the share now.
func (w *World) SetMasteryPercent(sess *session.Session, pct int) (Mastery, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return Mastery{}, false
	}
	p.Mastery.Percent = max(0, min(100, pct))
	return p.Mastery, true
}

// GrantMastery is #grantcm on the session's player: amount points more,
// never past MasteryCap; how many were given and the points now.
func (w *World) GrantMastery(sess *session.Session, amount int) (int, Mastery, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, Mastery{}, false
	}
	give := grantMastery(&p.Mastery, amount)
	return give, p.Mastery, true
}

// SendMasteryPoints is 0x1402 sub-op 3 (CLIENT's Recv03): the unspent and
// the spent points, raw. CLIENT says "You have earned %d mastery points."
// (word 1627, 1628 for one) when the unspent ones rose.
func SendMasteryPoints(sess *session.Session, m Mastery) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMasteryServer)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodeClassMasteryServer))
	w.WriteByte(3)
	w.WriteUint32(uint32(m.Points))
	w.WriteUint32(uint32(m.Spent))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendStatCap is 0x001C: the stat cap before masteries (VIPlayer's m_p44),
// raw, for a level that changed it.
func SendStatCap(sess *session.Session, level int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerP44)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodePlayerP44))
	w.WriteUint32(uint32(StatCapBase(level)))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// masteryLine is what a GM is told of a character's points.
func masteryLine(m Mastery) string {
	return fmt.Sprintf("%d unspent, %d spent (at most %d), %d%% of experience to masteries", m.Points, m.Spent, MasteryCap, m.Percent)
}

// MasteryLine is masteryLine for the GM commands.
func MasteryLine(m Mastery) string { return masteryLine(m) }

// logMastery notes a gain of points.
func logMastery(name string, made int, m Mastery) {
	log.Printf("Mastery: %s earned %d point(s): %s", name, made, masteryLine(m))
}
