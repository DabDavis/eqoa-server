package entity

import (
	"fmt"
	"log"
	"math"
	"slices"

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
// master classes and racial masteries raise their own stats further.
//
// They are BOUGHT since F3 pass 17 (cmbuy.go; the user's choice for the
// general ones: "Make them buyable too"): a stat's cap is the level's base
// plus the caps its character's masteries raise (CMEffects' STR MAX..CHA MAX),
// nothing more. Until then every character counted the general ones its level
// allowed as held (550 at 60 for all).
//
// A raised cap holds the training too (petition 1375, Beef: "stat maxes dont
// work, not extending maximums"; a player's claim is taken as given): a stat's
// base takes training points up to its cap, raises and all. SOE's game did
// not: Corsten's state holds m_p44 400 beside m_bonus [150 150 45 220 25 25
// 25] (his masteries' raises), and CLIENT's trainer stops a base at m_p44
// alone, so a raise reached only items, spells and the masteries' own stats.
// Until 2026-11-09 this server did the same: a level 60 trained to 400 and
// bought Brawn 1-5 and could train no further, the window refusing the point.
// CLIENT has one m_p44 for the seven stats, so the client is told the highest
// cap as m_p44 (TrainerCap) and each stat's cap less it as m_bonus
// (TrainerBonuses): GetP44Total(stat), the trainer's "Current", is the stat's
// own cap, and GetP44Total(-1), where the trainer stops a base, the highest.
// The server holds each stat's base to its own (FitTraining).

// MasteryCap is the most mastery points a character holds, spent and
// unspent together (F3 20261025-165747: 1500; the owner, 2026-11-13: "lets
// raise CM cap to 2000"). CMPointXP's table goes on past it unchanged.
const MasteryCap = 2000

// CMPointXP is the experience the character's nth mastery point takes (n
// counts every point it has earned: spent, unspent and this one): the live
// game's table, eqoa.fandom.com's "CM Point XP Required" (125,000 for the
// first, 13,426,215 for the 770th), which is exactly round(125000 *
// 1.0061^(n-1)) at all 151 points it lists.
func CMPointXP(n int) int {
	return int(math.Round(125000 * math.Pow(1.0061, float64(max(n, 1)-1))))
}

// StatCapBase is the stat cap before masteries at a level (CLIENT's m_p44):
// 350, and 400 past level 45.
func StatCapBase(level int) int {
	if level > 45 {
		return 400
	}
	return 350
}

// StatCapOf is the most a stat (StatStr to StatCha) may be at the player's
// level: the level's base and what its masteries raise that stat's cap by.
func (p *Player) StatCapOf(st Stat) int {
	n := StatCapBase(p.Level)
	if i := int(st - StatStr); i >= 0 && i < 7 {
		n += p.CMEffects[cmSlotCap+i]
	}
	return n
}

// capStat is v within the stat's cap.
func (p *Player) capStat(st Stat, v int) int {
	return min(v, p.StatCapOf(st))
}

// StatCaps are the seven stats' caps, STR..CHA (StatCapOf).
func (p *Player) StatCaps() [7]int {
	var c [7]int
	for i := range c {
		c[i] = p.StatCapOf(StatStr + Stat(i))
	}
	return c
}

// TrainerCap is the m_p44 the client is told (the MemoryDump's player record,
// 0x001C): the highest of the seven stats' caps, which CLIENT's trainer stops
// a base at (GetP44Total(-1); the file's doc). A character holding no
// mastery that raises a cap is told the level's, as SOE's server told it.
func (p *Player) TrainerCap() int {
	c := p.StatCaps()
	return slices.Max(c[:])
}

// TrainerBonuses are the seven m_bonus the client is told (the dump's mastery
// block, a purchase's and a removal's replies): each stat's cap less
// TrainerCap, 0 or below, so that m_p44 + m_bonus[stat] (GetP44Total(stat):
// what the trainer holds a stat's "Current" to) is the stat's own cap.
func (p *Player) TrainerBonuses() [7]int {
	c := p.StatCaps()
	top := slices.Max(c[:])
	for i := range c {
		c[i] -= top
	}
	return c
}

// FitTraining is what a request to the trainer may take (adds, STR..CHA) of
// the stats' bases with those caps: each stat's points held where its base
// would pass its cap (never below none), and how many points each was held
// back by. The cap is the stat's own, raises and all (the file's doc); a base
// already at it, or past it (a mastery that raised it removed), takes none.
func FitTraining(base, caps [7]int, adds [7]int64) (fit, held [7]int64) {
	for i, a := range adds {
		room := max(int64(caps[i])-int64(base[i]), 0)
		fit[i] = max(min(a, room), 0)
		held[i] = max(a, 0) - fit[i]
	}
	return fit, held
}

// Mastery is a character's class mastery points: unspent, spent, the share
// of experience that goes to them (0-100, the mastery points window's) and
// the experience gathered toward the next point.
type Mastery struct {
	Points, Spent, Percent, XP int
}

// xpShares is an award of experience divided: what goes to the level, to
// mastery points and to paying the experience debt.
type xpShares struct {
	Level, Mastery, Debt int
}

// splitXP divides an award of xp for a player at level owing debt, as jib has
// the live game's (F3 20261026-042328): while a debt is owed, half of the
// award pays it, never more than is owed; the rest goes to mastery points
// alone at the level cap (MaxLevel), and below it as the mastery points
// window's share divides it. Holding MasteryCap points, the masteries take
// nothing: their part pays what is left of the debt, then goes to the level
// (at the cap nothing comes of it).
func splitXP(m Mastery, level, debt, xp int) xpShares {
	var s xpShares
	if xp <= 0 {
		return s
	}
	debt = max(debt, 0)
	s.Debt = min(debt, xp/2)
	rest := xp - s.Debt
	pct := max(0, min(100, m.Percent))
	if level >= MaxLevel {
		pct = 100
	}
	s.Mastery = rest * pct / 100
	s.Level = rest - s.Mastery
	if m.Points+m.Spent >= MasteryCap {
		pay := min(debt-s.Debt, s.Mastery)
		s.Debt += pay
		s.Level += s.Mastery - pay
		s.Mastery = 0
	}
	return s
}

// gainMastery puts xp of mastery experience on m, each point made when the
// experience reaches its cost (CMPointXP of the point it would be), never
// past MasteryCap points: the points it made.
func gainMastery(m *Mastery, xp int) int {
	if xp <= 0 {
		return 0
	}
	m.XP += xp
	made := 0
	for m.Points+m.Spent < MasteryCap {
		cost := CMPointXP(m.Points + m.Spent + 1)
		if m.XP < cost {
			break
		}
		m.XP -= cost
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

// BarXP is the experience the HUD's bar is drawn from, 0x0020's second figure
// and the MemoryDump's: CLIENT draws exp / ExpForLevel(level+1), and the fifths
// of it under it, and reads exp for nothing else. Below the level cap, with
// some of the experience going to the level, it is the level's (XPInLevel). At
// the cap, or with all of it going to the masteries (the window's 100%), it is
// the way to the next mastery point, as the same share of the bar: the bar
// fills as that point comes (the GM Jibrish's petition 1271, 2026-11-08: "When
// on 100% CM xp commitment, or when level 60, the XP bar should show progress
// toward your next CM ... Same fashion, yellow and pink and when yellow bars
// fill you get your point"). Holding MasteryCap points it is empty: no point is
// to come. Held under 2^31, the client's signed int.
func (p *Player) BarXP() int {
	if p.Level < MaxLevel && p.Mastery.Percent < 100 {
		return p.XPInLevel
	}
	m := p.Mastery
	if m.Points+m.Spent >= MasteryCap {
		return 0
	}
	cost := CMPointXP(m.Points + m.Spent + 1)
	if cost <= 0 {
		return 0
	}
	need := int64(ExpForLevel(p.Level + 1))
	return int(min(need*int64(max(0, min(m.XP, cost)))/int64(cost), math.MaxInt32))
}

// LevelBar is the session's player's level and the experience its HUD's bar
// is drawn from (BarXP).
func (w *World) LevelBar(sess *session.Session) (level, bar int, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, 0, false
	}
	return p.Level, p.BarXP(), true
}

// CharacterBar is a character's (its serverid) HUD bar experience (BarXP), 0
// for one not in the world.
func (w *World) CharacterBar(serverID uint32) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p := w.playerByCharacterLocked(serverID); p != nil {
		return p.BarXP()
	}
	return 0
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
	if give > 0 {
		w.sendMasteryBarLocked(p)
	}
	return give, p.Mastery, true
}

// sendMasteryBarLocked tells p's client its HUD bar (0x0020, BarXP) when the
// bar is the way to the next mastery point: the points held and spent decide
// what that point costs (CMPointXP), so a rank removed (its points gone), a
// GM's #grantcm or the cap reached move the bar as no award does. Must be
// called with the lock held.
func (w *World) sendMasteryBarLocked(p *Player) {
	if p.Level < MaxLevel && p.Mastery.Percent < 100 {
		return // the level's bar: no mastery point moves it
	}
	if w.SessionManager == nil {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendGrantXP(sess, p.Level, p.BarXP())
	}
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

// SendStatCap is 0x001C: VIPlayer's m_p44, raw: the player's TrainerCap, for
// a level or a mastery that changed it.
func SendStatCap(sess *session.Session, trainerCap int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerP44)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodePlayerP44))
	w.WriteUint32(uint32(trainerCap))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// CharacterTrainerCap is a character's (its serverid) TrainerCap, the level's
// cap for one not in the world.
func (w *World) CharacterTrainerCap(serverID uint32, level int) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p := w.playerByCharacterLocked(serverID); p != nil {
		return p.TrainerCap()
	}
	return StatCapBase(level)
}

// masteryLine is what a GM is told of a character's points.
func masteryLine(m Mastery) string {
	next := "none past the most"
	if m.Points+m.Spent < MasteryCap {
		next = fmt.Sprintf("%d of %d toward the next", m.XP, CMPointXP(m.Points+m.Spent+1))
	}
	return fmt.Sprintf("%d unspent, %d spent (at most %d), %d%% of experience to masteries, %s", m.Points, m.Spent, MasteryCap, m.Percent, next)
}

// MasteryLine is masteryLine for the GM commands.
func MasteryLine(m Mastery) string { return masteryLine(m) }

// logMastery notes a gain of points.
func logMastery(name string, made int, m Mastery) {
	log.Printf("Mastery: %s earned %d point(s): %s", name, made, masteryLine(m))
}
