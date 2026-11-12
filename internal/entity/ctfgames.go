package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// ctfgames.go — the arena games side by side: capture the flag (ctf.go, ctfflag.go) and the siege (siege.go, the
// owner, 2026-11-12: "lets add a pvp zone for pvp events. zentaurs keep castle, it is siege mode"). Both are one
// engine: the queue, the countdown, the split into two teams, the camps, the prep, the waves, the backfill, the
// forfeit, the time limit, the end and the way back, the strays, /stuck, the mercs and familiars waiting outside
// (ctf.go, ctfaway.go), each game in its own world with its own settings, layout and match. What differs is the
// game's objective: the banners and power-ups (ctfflag.go) or the King and the gates (siege.go), chosen by the
// layout's Game.
//
// The engine's functions act on w.ctf. The World keeps the game they act on there and the other in w.ctfOther
// (w.ctfCur says which is which), and an entry point that belongs to one game swaps it in for its call
// (ctfUseLocked): the tick runs each game in turn (ctfSystem), a player's hooks the game of its match, its arena's
// world or its sign-up (ctfGameOfLocked), the handlers' calls the game they name. A swap writes the World, so it is
// made under the write lock alone; a read under the read lock goes to the game it wants by ctfGameLocked's pointer
// and never swaps. Match ids are the World's (ctfMatchSeq): a player's ctf.match names its game's match alone.

// ArenaGame is one of the arena games.
type ArenaGame int

// The arena games.
const (
	GameCTF   ArenaGame = 0 // capture the flag (world.ctf)
	GameSiege ArenaGame = 1 // the siege (world.siege)
)

// arenaGames is every arena game, in the tick's order.
var arenaGames = [2]ArenaGame{GameCTF, GameSiege}

// ctfUseLocked makes g the game the engine's functions act on (w.ctf) until the returned function runs, which puts
// the World back as it was. Calls nest. Must be called with the write lock held.
func (w *World) ctfUseLocked(g ArenaGame) (restore func()) {
	if g == w.ctfCur {
		return func() {}
	}
	w.ctf, w.ctfOther = w.ctfOther, w.ctf
	w.ctfCur = g
	return func() {
		w.ctf, w.ctfOther = w.ctfOther, w.ctf
		w.ctfCur = 1 - g
	}
}

// ctfGameLocked is game g's state, where it is now: read through it, never swap. Must be called with the lock held
// (read will do).
func (w *World) ctfGameLocked(g ArenaGame) *ctfState {
	if g == w.ctfCur {
		return &w.ctf
	}
	return &w.ctfOther
}

// ctfGameOfLocked is the game p belongs to: the game of its match, else of the arena world it stands in, else of
// its sign-up; GameCTF when none. Must be called with the lock held (read will do).
func (w *World) ctfGameOfLocked(p *Player) ArenaGame {
	if p == nil {
		return GameCTF
	}
	if p.ctf.match != 0 {
		for _, g := range arenaGames {
			if m := w.ctfGameLocked(g).m; m != nil && m.id == p.ctf.match {
				return g
			}
		}
	}
	for _, g := range arenaGames {
		if st := w.ctfGameLocked(g); st.on && p.World == st.set.World {
			return g
		}
	}
	for _, g := range arenaGames {
		for _, s := range w.ctfGameLocked(g).queue {
			if s.id == p.ServerID {
				return g
			}
		}
	}
	return GameCTF
}

// ctfForLocked swaps in the game of p (ctfGameOfLocked) until the returned function runs. Must be called with the
// write lock held.
func (w *World) ctfForLocked(p *Player) (restore func()) {
	return w.ctfUseLocked(w.ctfGameOfLocked(p))
}

// ctfAnyLocked reports f true in either game, each swapped in for its call. Must be called with the write lock
// held.
func (w *World) ctfAnyLocked(f func() bool) bool {
	for _, g := range arenaGames {
		restore := w.ctfUseLocked(g)
		ok := f()
		restore()
		if ok {
			return true
		}
	}
	return false
}

// ctfEachLocked runs f in each game, swapped in. Must be called with the write lock held.
func (w *World) ctfEachLocked(f func()) {
	for _, g := range arenaGames {
		restore := w.ctfUseLocked(g)
		f()
		restore()
	}
}

// ctfArenaAnyLocked reports world either game's arena. Must be called with the lock held (read will do).
func (w *World) ctfArenaAnyLocked(world uint8) bool {
	for _, g := range arenaGames {
		if st := w.ctfGameLocked(g); st.on && world == st.set.World {
			return true
		}
	}
	return false
}

// ctfMatchOfLocked is the match p plays in, in whichever game, nil none. Must be called with the lock held (read
// will do).
func (w *World) ctfMatchOfLocked(p *Player) *ctfMatch {
	if p == nil || p.ctf.match == 0 {
		return nil
	}
	for _, g := range arenaGames {
		if m := w.ctfGameLocked(g).m; m != nil && m.id == p.ctf.match {
			return m
		}
	}
	return nil
}

// The GM's lines for running a game by hand (ours).
const (
	arenaGMClosed    = "%s is closed on this server."
	arenaGMStarted   = "Match %d begins: %d against %d."
	arenaGMTooFew    = "Too few are ready to start: %d signed up and ready, one a side at least."
	arenaGMOn        = "A match is on already: %s stop first."
	arenaGMStopped   = "Match %d is stopped: no victor."
	arenaGMNoMatch   = "No match is on."
	arenaGMCountdown = "The countdown is stopped; %d stay signed up."
	arenaGMTime      = "The time limit is %d minutes now%s."
	arenaGMTimeLeft  = " (this match ends in %d minutes)"
	arenaGMTeams     = "Teams hold up to %d now (a match begins at %d a side)."
)

// ArenaStartNow starts game g's match now with whoever is signed up and ready, one a side at least (a GM's
// #siege start or #ctf start). It returns the GM's line.
func (w *World) ArenaStartNow(g ArenaGame, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.ctfUseLocked(g)()
	switch {
	case !w.ctf.on:
		return fmt.Sprintf(arenaGMClosed, upFirstWord(w.ctfWords().event))
	case w.ctf.m != nil:
		return fmt.Sprintf(arenaGMOn, strings.ToLower(w.ctfTag()))
	}
	ready := len(w.ctfReadyLocked())
	if ready < 2 {
		return fmt.Sprintf(arenaGMTooFew, ready)
	}
	min := w.ctf.set.MinPerTeam
	w.ctf.set.MinPerTeam = 1
	w.ctfStartLocked(now)
	w.ctf.set.MinPerTeam = min
	m := w.ctf.m
	if m == nil {
		return fmt.Sprintf(arenaGMTooFew, ready)
	}
	log.Printf("%s: a GM starts match %d", w.ctfTag(), m.id)
	return fmt.Sprintf(arenaGMStarted, m.id, len(m.teams[0].members), len(m.teams[1].members))
}

// ArenaStop ends game g's match with no victor (everyone goes back after the end's hold), or stops its countdown
// (a GM's #siege stop or #ctf stop). It returns the GM's line.
func (w *World) ArenaStop(g ArenaGame, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.ctfUseLocked(g)()
	m := w.ctf.m
	switch {
	case m != nil && m.phase != ctfOver:
		w.ctfEndLocked(-1, "stopped by a GM", now)
		return fmt.Sprintf(arenaGMStopped, m.id)
	case m == nil && !w.ctf.startAt.IsZero():
		w.ctf.startAt = time.Time{}
		w.ctfTellQueueLocked(fmt.Sprintf(ctfLineCancel, w.ctf.set.MinPerTeam), ctfGold)
		return fmt.Sprintf(arenaGMCountdown, len(w.ctf.queue))
	}
	return arenaGMNoMatch
}

// ArenaSetTime sets game g's time limit (a GM's #siege time or #ctf time): the next match's, and a match on ends
// that long after it went live (at once if that is past). It returns the GM's line.
func (w *World) ArenaSetTime(g ArenaGame, d time.Duration, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.ctfUseLocked(g)()
	w.ctf.set.TimeLimit = d
	left := ""
	if m := w.ctf.m; m != nil && m.phase == ctfPlay {
		m.ends = m.opens.Add(d)
		m.minsTold = int(math.Ceil(m.ends.Sub(now).Minutes())) + 1
		left = fmt.Sprintf(arenaGMTimeLeft, max(int(math.Ceil(m.ends.Sub(now).Minutes())), 0))
	}
	log.Printf("%s: a GM sets the time limit to %v", w.ctfTag(), d)
	return fmt.Sprintf(arenaGMTime, int(d/time.Minute), left)
}

// ArenaSetTeamSize sets the most a team of game g holds (a GM's #siege teams or #ctf teams), the fewest it starts
// with no more than it. It returns the GM's line.
func (w *World) ArenaSetTeamSize(g ArenaGame, n int) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.ctfUseLocked(g)()
	w.ctf.set.TeamSize = n
	w.ctf.set.MinPerTeam = min(w.ctf.set.MinPerTeam, n)
	log.Printf("%s: a GM sets the teams to %d", w.ctfTag(), n)
	return fmt.Sprintf(arenaGMTeams, n, w.ctf.set.MinPerTeam)
}
