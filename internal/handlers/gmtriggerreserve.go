package handlers

import (
	"fmt"
	"log"
	"time"
)

// A trigger run reserved ahead of a quest hand-in (board entry 1263, the Plane of
// Sky's shard hand-ins whose doing fires an encounter): the run is checked as Fire
// checks one (definition, on, parsed, placed: prepareRun) and its lock taken BEFORE
// anything is handed in, so the hand-in and its encounter are one outcome. A
// reservation holds its lock against every other firing (a GM's, a death's, another
// quest's: lockHeldLocked) until it is activated (the run starts, once) or
// released (the hand-in was refused; nothing ran).
//
// Lock order: the quest engine's (QuestEngine.mu) before this engine's (e.mu), as a
// step's events already fire triggers; this engine never takes the quests' lock
// under its own. A reservation is never kept: a keep restart (or a cold start)
// finds the hand-in either committed and its run activated, or nothing done.

// triggerReserveTTL is how long a reservation holds its lock unactivated before it
// counts as given up (a hand-in takes milliseconds; this only frees a lock a fault
// left held).
const triggerReserveTTL = 30 * time.Second

// TriggerReservation is a run admitted but not yet started.
type TriggerReservation struct {
	id  int
	run *triggerRun
	at  time.Time
}

// Lock is the reservation's lock name ("" for a trigger with none).
func (r *TriggerReservation) Lock() string {
	if r == nil {
		return ""
	}
	return r.run.lock
}

// ReserveNamed reserves a run of the trigger called name for by: refused (lines and
// the error say why: none, off, broken, no place, busy) with nothing held.
func (e *TriggerEngine) ReserveNamed(name string, by TriggerFirer) (*TriggerReservation, []string, error) {
	if e == nil || e.db == nil || e.gw == nil {
		return nil, nil, ErrTriggerNone
	}
	slot, ok := e.slotNamed(name)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %q", ErrTriggerNone, name)
	}
	r, lines, err := e.prepareRun(slot, by)
	if err != nil {
		return nil, lines, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lockHeldLocked(r.lock) {
		return nil, []string{fmt.Sprintf("%s waits: a run of its lock %q lives", r.trig.Title(), r.lock)}, ErrTriggerBusy
	}
	if e.reserved == nil {
		e.reserved = map[int]*TriggerReservation{}
	}
	e.resSeq++
	res := &TriggerReservation{id: e.resSeq, run: r, at: e.now()}
	e.reserved[res.id] = res
	return res, nil, nil
}

// Activate starts a reserved run, once: its lines; ErrTriggerBusy when the
// reservation had lapsed and another run took its lock meanwhile (logged: the
// hand-in stands, its encounter does not run).
func (e *TriggerEngine) Activate(res *TriggerReservation) ([]string, error) {
	if e == nil || res == nil {
		return nil, ErrTriggerNone
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.reserved[res.id]; !ok {
		if res.run.id != 0 {
			return nil, fmt.Errorf("reservation %d activated already", res.id)
		}
		// lapsed and swept: it may start only if its lock is free
		if e.lockHeldLocked(res.run.lock) {
			log.Printf("Triggers: %s's reservation lapsed and its lock %q was taken: not run", res.run.trig.Title(), res.run.lock)
			return nil, ErrTriggerBusy
		}
	}
	delete(e.reserved, res.id)
	return e.admitLocked(res.run), nil
}

// Release gives a reservation up: nothing ran, its lock is free.
func (e *TriggerEngine) Release(res *TriggerReservation) {
	if e == nil || res == nil {
		return
	}
	e.mu.Lock()
	delete(e.reserved, res.id)
	e.mu.Unlock()
}

// reservedLocked is whether a live reservation holds lock; lapsed ones are swept.
// Must be called with e.mu held.
func (e *TriggerEngine) reservedLocked(lock string) bool {
	if lock == "" || len(e.reserved) == 0 {
		return false
	}
	now := e.now()
	held := false
	for id, r := range e.reserved {
		if now.Sub(r.at) > triggerReserveTTL {
			log.Printf("Triggers: %s's reservation %d lapsed unactivated", r.run.trig.Title(), id)
			delete(e.reserved, id)
			continue
		}
		if r.run.lock == lock {
			held = true
		}
	}
	return held
}
