package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// marks.go — what the World knows of Heirloom Marks (work board 1927, a GM's spec; the
// Heirloom Test Server's alone, world.heirloom_marks; handlers/marks.go keeps the marks
// themselves) and of Relive (section 9).
//
// Who earns: a character below 60, or a 60 while it relives, never a GM's (MarksEarner).
// A kill's roll: each sharer of a kill that gives it experience (awardKillXPLocked) is
// handed to the hook (OnKillMarks) with the kill's weight (the experience it gives this
// character before any boon or reward, over an even-con solo kill's at its level, at most
// KillMarksWeightMost) and the earners of its group near it; the hook rolls and counts (its
// hour's cap, the Hot zones, the bonuses) off the lock. Only a creature's kill comes here: a
// player's death (PvP, a duel, the arenas) never gives experience and so never a mark.
//
// Relive: a 60 goes back down to a level it picks (StartRelive) and plays as that level for
// everything the level decides (Player.Level is the Relive level); its true level, experience
// and debt are kept on Player.Relive and every save writes those (SavedProgress), never the
// Relive level. Its masteries sleep (refreshCMLocked gives none of their stats; a mastery's
// spell, or any above the Relive level, is refused: reliveCastRefusal). Its experience fills the
// Relive level's bar, a full bar raising it a level, to ReliveTop; a death's debt is the Relive
// level's, paid from Relive experience. EndRelive puts the 60 back.

// KillMarksWeightMost is the most a kill's weight is.
const KillMarksWeightMost = 1.5

// MarksGroupRange is how near another earner of the group stands to count for the group bonus.
const MarksGroupRange = KillShareRange

// KillMarks is one sharer's chance of a mark at a kill.
type KillMarks struct {
	Sess     *session.Session
	ServerID uint32 // the character's
	Name     string
	Weight   float64
	Group    int // the other earners of its group in its world within MarksGroupRange
	NPCRow   EntityID
	NPCName  string
	World    uint8
	SpawnX   float32
	SpawnZ   float32
}

// KillMarksFunc hears a kill's chance of a mark. It is called with the World's lock held: it must
// not call back into the World, nor block.
type KillMarksFunc func(KillMarks)

// OnKillMarks sets the hook of kill marks (nil: none rolled).
func (w *World) OnKillMarks(fn KillMarksFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.killMarks = fn
}

// MarksEarner is whether p earns Heirloom Marks: below 60 or reliving, and no GM's.
func MarksEarner(p *Player) bool {
	return p != nil && MarksEarnerAt(p, p.Level)
}

// MarksEarnerAt is MarksEarner with p at level: a quest's, the level it was finished at before its
// experience (handlers/marks.go QuestDone).
func MarksEarnerAt(p *Player, level int) bool {
	return p != nil && !p.GM && (level < MaxLevel || p.Relive != nil)
}

// soloEvenExp is what an even-con solo kill gives a character of level: the weight's measure.
func soloEvenExp(level int) int {
	return groupExp(level, level, []int{level})
}

// killMarksWeight is a kill's weight for a character of level given xp by it.
func killMarksWeight(xp, level int) float64 {
	solo := soloEvenExp(level)
	if xp <= 0 || solo <= 0 {
		return 0
	}
	return min(float64(xp)/float64(solo), KillMarksWeightMost)
}

// marksGroupLocked is how many other earners of p's group stand in p's world within
// MarksGroupRange of it, alive. Must be called with the lock held.
func (w *World) marksGroupLocked(p *Player) int {
	n := 0
	for _, m := range w.groupMembersLocked(p) {
		if m == p || m.ServerID == p.ServerID || m.IsDead() || m.World != p.World || !MarksEarner(m) {
			continue
		}
		if dx, dz := m.X-p.X, m.Z-p.Z; dx*dx+dz*dz > MarksGroupRange*MarksGroupRange {
			continue
		}
		n++
	}
	return n
}

// MarksGroup is marksGroupLocked under the lock (a quest's award).
func (w *World) MarksGroup(p *Player) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p == nil {
		return 0
	}
	return w.marksGroupLocked(p)
}

// killMarksLocked hands a sharer's kill of npc, xp the experience it gives before any boon, to
// the hook. Must be called with the lock held.
func (w *World) killMarksLocked(sess *session.Session, p *Player, npc *NPC, xp int) {
	if w.killMarks == nil || !MarksEarner(p) || p.CharServerID == 0 || npc == nil {
		return
	}
	weight := killMarksWeight(xp, p.Level)
	if weight <= 0 {
		return
	}
	w.killMarks(KillMarks{Sess: sess, ServerID: p.CharServerID, Name: p.Name, Weight: weight, Group: w.marksGroupLocked(p),
		NPCRow: npc.ServerID, NPCName: npc.Name, World: npc.World, SpawnX: npc.SpawnX, SpawnZ: npc.SpawnZ})
}

// --- Relive ---

// ReliveTop is the highest Relive level: a full bar at it raises it no further.
const ReliveTop = MaxLevel - 1

// ReliveState is a reliving player's true figures, written by every save.
type ReliveState struct {
	Level, XPInLevel, Debt int
	Since                  time.Time
}

// ReliveSave is what a Relive's row (charrelive) holds now.
type ReliveSave struct {
	CharServerID           uint32
	Level, XPInLevel, Debt int
	Since                  time.Time
	Active                 bool
}

// ReliveSaveFunc writes a Relive's row, called with the lock let go.
type ReliveSaveFunc func(ReliveSave)

// OnReliveSave sets the hook that writes a Relive's row.
func (w *World) OnReliveSave(fn ReliveSaveFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reliveSave = fn
}

// Reliving is whether p relives.
func (p *Player) Reliving() bool { return p != nil && p.Relive != nil }

// SavedProgress is what a save writes of p's level: its true level, total experience and debt
// (a Relive's own are its row's).
func (p *Player) SavedProgress() (level, totalXP, debt int) {
	if r := p.Relive; r != nil {
		return r.Level, TotalXP(r.Level, r.XPInLevel), r.Debt
	}
	return p.Level, TotalXP(p.Level, p.XPInLevel), p.Debt
}

// reliveSaveOf is p's Relive's row now.
func reliveSaveOf(p *Player) ReliveSave {
	return ReliveSave{CharServerID: p.CharServerID, Level: p.Level, XPInLevel: p.XPInLevel, Debt: p.Debt,
		Since: p.Relive.Since, Active: true}
}

// FlushRelive writes p's Relive's row now (a logout's and a restart's save).
func (w *World) FlushRelive(p *Player) {
	if p == nil {
		return
	}
	w.mu.RLock()
	fn := w.reliveSave
	var s ReliveSave
	ok := p.Relive != nil
	if ok {
		s = reliveSaveOf(p)
	}
	w.mu.RUnlock()
	if ok && fn != nil {
		fn(s)
	}
}

// The Relive's refusals.
const (
	ReliveNot60     = "Only one at the height of its strength, level 60, may relive."
	ReliveInFight   = "Not while you fight."
	ReliveDead      = "Not while you are dead."
	ReliveInDungeon = "Not inside a dungeon."
	ReliveAlready   = "You relive already."
	ReliveNotNow    = "You do not relive now."
	ReliveCastLine  = "You cannot call on that power while you relive."
)

// DungeonWorld is the instanced dungeons' world, where no Relive starts.
const reliveNoWorld = 14

// ReliveRefusal is why p cannot start (or resume) a Relive now, "" when it can.
func (w *World) ReliveRefusal(sess *session.Session) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.reliveRefusalLocked(w.sessionPlayerLocked(sess))
}

func (w *World) reliveRefusalLocked(p *Player) string {
	switch {
	case p == nil:
		return ReliveNot60
	case p.Relive != nil:
		return ReliveAlready
	case p.Level < MaxLevel:
		return ReliveNot60
	case p.IsDead():
		return ReliveDead
	case p.World == reliveNoWorld:
		return ReliveInDungeon
	case w.fightingLocked(p) > 0:
		return ReliveInFight
	}
	return ""
}

// StartRelive takes sess's 60 to level (1..ReliveTop) with xp into it and debt owed (a Resume's
// row; 0, 0 a new Relive), from since: the refusal, "" when done. The caller sends the dump.
func (w *World) StartRelive(sess *session.Session, level, xp, debt int, since time.Time) string {
	w.mu.Lock()
	p := w.sessionPlayerLocked(sess)
	if why := w.reliveRefusalLocked(p); why != "" {
		w.mu.Unlock()
		return why
	}
	level = min(max(level, 1), ReliveTop)
	if t := CharXPDict[level]; t > 0 {
		xp = min(max(xp, 0), t-1)
	}
	p.Relive = &ReliveState{Level: p.Level, XPInLevel: p.XPInLevel, Debt: p.Debt, Since: since}
	w.applyReliveLocked(sess, p, level, xp, max(debt, 0), true)
	s := reliveSaveOf(p)
	fn := w.reliveSave
	w.mu.Unlock()
	log.Printf("Relive: %s relives at level %d (%d xp into it, %d debt)", p.Name, level, xp, debt)
	if fn != nil {
		fn(s)
	}
	return ""
}

// RestoreRelive is a login's Relive (its row active): the player, its true level read off its
// row, made the Relive level before its gear is read. It reports whether it took.
func (w *World) RestoreRelive(sess *session.Session, level, xp, debt int, since time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.Relive != nil || p.Level < MaxLevel {
		return false
	}
	level = min(max(level, 1), ReliveTop)
	p.Relive = &ReliveState{Level: p.Level, XPInLevel: p.XPInLevel, Debt: p.Debt, Since: since}
	if t := CharXPDict[level]; t > 0 {
		xp = min(max(xp, 0), t-1)
	}
	w.applyReliveLocked(sess, p, level, xp, max(debt, 0), false) // the login reads its gear after
	log.Printf("Relive: %s logs in reliving at level %d", p.Name, level)
	return true
}

// applyReliveLocked makes p the Relive level, its level's hearers told when hear (the heirlooms'
// figures and the worn gear). Must be called with the lock held.
func (w *World) applyReliveLocked(sess *session.Session, p *Player, level, xp, debt int, hear bool) {
	p.Level, p.XPInLevel, p.Debt = level, xp, debt
	p.TotalXP = TotalXP(level, xp)
	p.refreshCMLocked(w.cms) // its masteries sleep
	p.RecalcStats(false)
	p.StatsDirty = true
	if hear {
		for _, fn := range w.onLevelUp { // the heirlooms' figures and the worn gear (handlers/heirloom.go)
			fn(sess, p)
		}
	}
}

// EndRelive puts sess's player back at its true level, the Relive's row kept (Resume): the row
// as it ends, ok false when it did not relive. The caller sends the dump.
func (w *World) EndRelive(sess *session.Session) (ReliveSave, bool) {
	w.mu.Lock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.Relive == nil {
		w.mu.Unlock()
		return ReliveSave{}, false
	}
	s := reliveSaveOf(p)
	s.Active = false
	r := p.Relive
	p.Relive = nil
	p.Level, p.XPInLevel, p.Debt = r.Level, r.XPInLevel, r.Debt
	p.TotalXP = TotalXP(r.Level, r.XPInLevel)
	p.refreshCMLocked(w.cms)
	p.RecalcStats(false)
	p.StatsDirty = true
	for _, fn := range w.onLevelUp {
		fn(sess, p)
	}
	fn := w.reliveSave
	w.mu.Unlock()
	log.Printf("Relive: %s returns to level %d (relived at %d)", p.Name, r.Level, s.Level)
	if fn != nil {
		fn(s)
	}
	return s, true
}

// reliveCastRefusal is why a reliving p may not cast spell: one above its Relive level, or one a
// mastery rank adds; "" otherwise (and always for one not reliving, or an item's cast). Must be
// called with the lock held.
func (w *World) reliveCastRefusalLocked(p *Player, spell *CastSpell) string {
	if p == nil || p.Relive == nil || spell.ItemCast || spell.Travel != nil || spell.Familiar != nil {
		return ""
	}
	if spell.Level > p.Level || w.masterySpellLocked(spell.SpellID) {
		return ReliveCastLine
	}
	return ""
}
