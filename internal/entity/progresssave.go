package entity

import (
	"sync"
	"time"
)

// progresssave.go — a character's level and experience written to its row while
// it plays, not only at its logout (the logout's savePlayerPosition) and at a
// restart (SaveAll). Before this a crash, a kill or anything else that ended the
// process between a level gained and the next logout lost the level: a Live
// Server character showed level 2 in the game while its row still said 1 (the
// owner, 2026-11-11, of a player who lost a level over a restart).
//
//   - A level gained (awardXP) queues the character, and the next tick writes
//     it (runProgressSaves, after the world lock is let go, as the combat
//     records are: combatrecord.go).
//   - Every ProgressSaveEvery each player in the world whose figures changed
//     since they were last written is written too, ProgressSavesPerTick at most a
//     tick, so the experience between two dings is at most that old after a
//     crash, and no tick writes the whole world at once.
//
// What is written is the figures savePlayerPosition writes of the level: the
// level, the total experience (TotalXP: the level's and the experience into
// it) and the experience debt, nothing else. The training points a level gives
// are made up from the row's level at the next login (handlers/character.go:
// TopUpTrainingBonus), so they need no write here, and writing them would race
// the trainer's own (handlers/training.go takes the points spent off the row's
// count). Mastery points are not written either: a mastery bought writes them
// with the rank in one transaction (classmastery.go), and a write here of
// figures read before the purchase could land after it; they stay the
// logout's, as before. The place is still the logout's (its guards against a
// fall or a world that does not hold it).
//
// The figures are read from the world's live Player of the character at the
// write, so a session a newer login replaced writes the newer login's, never
// the old window's (IsLivePlayer's rule). The logout's and the restart's saves
// hold ProgressHold while they write, so a write here never lands after theirs
// with older figures.

// ProgressSaveEvery is how often a player's experience is written while it plays.
const ProgressSaveEvery = 3 * time.Minute

// ProgressSavesPerTick is how many players a tick writes at most for the
// periodic save (a level gained is written at once, whatever this is).
const ProgressSavesPerTick = 4

// progressScanEvery is how often the periodic save looks for players due.
const progressScanEvery = time.Second

// ProgressSave is a character's figures to write: its row's level, totalXP
// and debt.
type ProgressSave struct {
	CharServerID   uint32
	Name           string
	Level, TotalXP int
	Debt           int
	Ding           bool // a level changed (else the periodic save)
}

// ProgressFunc writes a character's figures (the listener's: the character
// store), called in order with the world lock let go.
type ProgressFunc func(s ProgressSave)

// progressMark is what was last written of a player, and when.
type progressMark struct {
	at   time.Time
	save ProgressSave
}

// progressSaves is the characters waiting to be written.
type progressSaves struct {
	fn       ProgressFunc
	mu       sync.Mutex // the queue
	run      sync.Mutex // a write, and the logout's and the restart's saves (ProgressHold)
	queue    []*Player
	lastScan time.Time
}

// OnProgressSave sets the hook that writes the figures.
func (w *World) OnProgressSave(fn ProgressFunc) {
	w.progress.mu.Lock()
	defer w.progress.mu.Unlock()
	w.progress.fn = fn
}

// ProgressHold holds off the writes here until the function it returns is
// called: the logout's and the restart's saves, so neither is overwritten by a
// write of figures read before theirs. Never called with the world lock held.
func (w *World) ProgressHold() func() {
	w.progress.run.Lock()
	return w.progress.run.Unlock
}

// queueProgressLocked queues p's figures for the next tick's write (a level
// changed). Must be called with the lock held.
func (w *World) queueProgressLocked(p *Player) {
	w.progress.mu.Lock()
	defer w.progress.mu.Unlock()
	if w.progress.fn != nil && p != nil {
		w.progress.queue = append(w.progress.queue, p)
	}
}

// progressOf is p's figures. Must be called with the lock held.
func progressOf(p *Player) ProgressSave {
	return ProgressSave{CharServerID: p.CharServerID, Name: p.Name, Level: p.Level,
		TotalXP: TotalXP(p.Level, p.XPInLevel), Debt: p.Debt}
}

// liveOfLocked is the world's live Player of p's character: p, or the newer
// login's that replaced it; nil when the character is no longer in the world
// (its logout wrote it). Must be called with the lock held.
func (w *World) liveOfLocked(p *Player) *Player {
	if cur, ok := w.Players[p.ServerID]; ok && cur == p {
		return p
	}
	return w.playerByCharacterLocked(p.CharServerID)
}

// runProgressSaves writes the characters a level changed for, and those the
// periodic save finds due, outside the world lock (every tick).
func (w *World) runProgressSaves(now time.Time) {
	w.progress.mu.Lock()
	fn := w.progress.fn
	queued := w.progress.queue
	w.progress.queue = nil
	scan := fn != nil && now.Sub(w.progress.lastScan) >= progressScanEvery
	if scan {
		w.progress.lastScan = now
	}
	w.progress.mu.Unlock()
	if fn == nil || (len(queued) == 0 && !scan) {
		return
	}

	w.progress.run.Lock()
	defer w.progress.run.Unlock()
	var out []ProgressSave
	w.mu.Lock()
	seen := make(map[uint32]bool, len(queued))
	for _, p := range queued {
		live := w.liveOfLocked(p)
		if live == nil || seen[live.CharServerID] {
			continue
		}
		seen[live.CharServerID] = true
		s := progressOf(live)
		s.Ding = true
		live.progressSaved = progressMark{at: now, save: s}
		out = append(out, s)
	}
	if scan {
		n := 0
		for _, p := range w.Players {
			if n >= ProgressSavesPerTick {
				break
			}
			if seen[p.CharServerID] || p.CharServerID == 0 {
				continue
			}
			if p.progressSaved.at.IsZero() {
				// just in: its figures are its row's (a write would change nothing)
				p.progressSaved = progressMark{at: now, save: progressOf(p)}
				continue
			}
			if now.Sub(p.progressSaved.at) < ProgressSaveEvery {
				continue
			}
			s := progressOf(p)
			p.progressSaved.at = now
			if s == p.progressSaved.save {
				continue // nothing gained since the last write
			}
			p.progressSaved.save = s
			out = append(out, s)
			n++
		}
	}
	w.mu.Unlock()
	for _, s := range out {
		fn(s)
	}
}
