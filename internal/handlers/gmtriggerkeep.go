package handlers

// gmtriggerkeep.go — the GM triggers' runs through a restart that keeps the
// sessions (the owner, 2026-11-13, after Lonewulf lost two Rallos Zek fights to
// server updates; the dungeons' instances are kept the same way since da0857d7,
// entity/keepdungeon.go).
//
// Until this a run lived in memory alone: a keep restart ended every one, its
// spawns (copies no row makes again) with it, so a raid half way through Rallos
// Zek came back to an empty isle, its lock free and its quest waiting on a kill
// that could no longer happen.
//
// Now each run that is not stopped is written down (KeepTriggers): its trigger
// as it fired (the record and so its steps: a GM's edit since does not change a
// run under way), who fired it, its place and reach, the step it is on, its
// wait's end, its await (awaittalk, awaitdead) and what has died during it, its
// clock (its end, its empty and when a player was last near), its lock, its
// resummons and its protected NPC, and its spawns (id, name, stay); the living
// ones' state is entity/keeptrigger.go's (place, hit points, flags, a sic's
// enemy), a dead one's corpse keepcopy.go's. The new process makes the spawns
// again under their ids when free (RestoreTriggers, after the creatures, the
// copies' corpses and the dungeons, before the sessions) and the runs with them,
// each spawn's old id mapped to its new; a spawn that could not be made again
// counts as gone, as a spawn gone does at a tick (an awaitdead on it goes on).
// Every time a run keeps (its wait's end, its clock's end, its spawns' stays,
// when a player was last near, a resummon's fall) moves on by the restart's
// length, so a clock has the time it had left when the server stopped.
//
// A cold start (a crash, the swap with no one in, #servdown, the GM panel's
// restart) carries nothing: every run ends with the old process, as before, and
// the quests waiting on an encounter's kill are put back (questencounter.go).

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// keepTriggerGapMax is the longest a restart's length is taken to be when the
// runs' times move on by it (a file older than this is refused by the keep).
const keepTriggerGapMax = 2 * time.Minute

// KeptTriggers is the triggers' runs as a restart carries them.
type KeptTriggers struct {
	Written time.Time             `json:"written"`
	Seq     int                   `json:"seq"`
	Runs    []KeptTriggerRun      `json:"runs,omitempty"`
	NPCs    []entity.KeptEventNPC `json:"npcs,omitempty"` // the runs' living spawns
}

// KeptTriggerRun is one run (triggerRun's fields).
type KeptTriggerRun struct {
	ID     int                   `json:"id"`
	Trig   database.GMTrigger    `json:"trig"`
	By     string                `json:"by,omitempty"`
	GMChar uint32                `json:"gm_char,omitempty"`
	Quiet  bool                  `json:"quiet,omitempty"`
	World  uint8                 `json:"world"`
	X      float32               `json:"x"`
	Y      float32               `json:"y"`
	Z      float32               `json:"z"`
	Next   int                   `json:"next"`
	At     time.Time             `json:"at,omitzero"`
	Start  time.Time             `json:"start"`
	Spawns []KeptTriggerSpawn    `json:"spawns,omitempty"`
	Lock   string                `json:"lock,omitempty"`
	Reach  float32               `json:"reach,omitempty"`
	Await  *database.TriggerStep `json:"await,omitempty"`
	// Deadline is its clock's end (zero: none), EmptyAfter its empty, LastNear
	// when a player was last within its reach
	Deadline   time.Time      `json:"deadline,omitzero"`
	EmptyAfter time.Duration  `json:"empty_after,omitempty"`
	LastNear   time.Time      `json:"last_near,omitzero"`
	Died       []string       `json:"died,omitempty"`
	Dead       *KeptTrigPlace `json:"dead,omitempty"`
	Resummons  []KeptResummon `json:"resummons,omitempty"`
	Protect    *KeptProtect   `json:"protect,omitempty"`
}

// KeptTriggerSpawn is one of a run's spawns.
type KeptTriggerSpawn struct {
	ID    uint32    `json:"id"`
	Name  string    `json:"name"`
	Until time.Time `json:"until"`
}

// KeptTrigPlace is where a run's last spawn died.
type KeptTrigPlace struct {
	World uint8   `json:"world"`
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	Z     float32 `json:"z"`
}

// KeptResummon is one of a run's resummon steps.
type KeptResummon struct {
	Step       database.TriggerStep `json:"step"`
	Fell       time.Time            `json:"fell,omitzero"`
	CallerSeen bool                 `json:"caller_seen,omitempty"`
}

// KeptProtect is a run's protected NPC.
type KeptProtect struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
	Text string `json:"text,omitempty"`
}

// KeepTriggers is the running engine's runs at now (the file's doc); nil with
// none, or no engine. The listener is paused.
func KeepTriggers(now time.Time) *KeptTriggers {
	e := Triggers()
	if e == nil || e.gw == nil {
		return nil
	}
	return e.keep(now)
}

func (e *TriggerEngine) keep(now time.Time) *KeptTriggers {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := &KeptTriggers{Written: now, Seq: e.runSeq}
	var ids []entity.EntityID
	for _, r := range e.runs {
		if r.stopped {
			continue
		}
		kr := KeptTriggerRun{ID: r.id, Trig: r.trig, By: r.by, GMChar: r.gmChar, Quiet: r.quiet, World: r.world, X: r.x, Y: r.y, Z: r.z,
			Next: r.next, At: r.at, Start: r.start, Lock: r.lock, Reach: r.reach, Deadline: r.deadline, EmptyAfter: r.emptyAfter,
			LastNear: r.lastNear, Died: append([]string(nil), r.died...)}
		if r.await != nil {
			s := *r.await
			kr.Await = &s
		}
		if r.dead != nil {
			kr.Dead = &KeptTrigPlace{World: r.dead.world, X: r.dead.x, Y: r.dead.y, Z: r.dead.z}
		}
		for _, s := range r.spawns {
			kr.Spawns = append(kr.Spawns, KeptTriggerSpawn{ID: uint32(s.id), Name: s.name, Until: s.until})
			ids = append(ids, s.id)
		}
		for _, rs := range r.resummons {
			kr.Resummons = append(kr.Resummons, KeptResummon{Step: rs.step, Fell: rs.fell, CallerSeen: rs.callerSeen})
		}
		if r.protect != nil {
			kr.Protect = &KeptProtect{ID: uint32(r.protect.id), Name: r.protect.name, Text: r.protect.text}
		}
		k.Runs = append(k.Runs, kr)
	}
	if len(k.Runs) == 0 {
		return nil
	}
	k.NPCs = e.gw.KeepEventNPCs(ids, now)
	return k
}

// TriggersRestored says what RestoreTriggers did.
type TriggersRestored struct {
	Runs, RunsLost int // runs made again; not (their steps no longer read)
	Alive, Lost    int // living spawns made again; not (their rows gone): counted gone
	Corpses        int // spawns lying dead, their corpses made again (keepcopy.go)
}

// RestoreTriggers makes the kept runs again in the running engine (the file's
// doc), once the creatures, the copies' corpses (copies: their old ids' new ones)
// and the dungeons are restored, before the sessions are put back. It returns the
// living spawns' old ids' new ones.
func RestoreTriggers(k *KeptTriggers, copies map[entity.EntityID]entity.EntityID, now time.Time) (map[entity.EntityID]entity.EntityID, TriggersRestored) {
	var r TriggersRestored
	e := Triggers()
	if k == nil {
		return nil, r
	}
	if e == nil || e.gw == nil {
		r.RunsLost = len(k.Runs)
		log.Printf("Keep: %d trigger runs written, but the triggers are not running: none made again", len(k.Runs))
		return nil, r
	}
	return e.restore(k, copies, now, &r), r
}

func (e *TriggerEngine) restore(k *KeptTriggers, copies map[entity.EntityID]entity.EntityID, now time.Time, r *TriggersRestored) map[entity.EntityID]entity.EntityID {
	alive, lost := e.gw.RestoreEventNPCs(k.NPCs)
	r.Lost = lost
	gap := now.Sub(k.Written)
	if gap < 0 || k.Written.IsZero() {
		gap = 0
	}
	gap = min(gap, keepTriggerGapMax)
	later := func(t time.Time) time.Time {
		if t.IsZero() {
			return t
		}
		return t.Add(gap)
	}
	mapped := func(id uint32) (entity.EntityID, bool) {
		if to, ok := alive[entity.EntityID(id)]; ok {
			return to, true
		}
		to, ok := copies[entity.EntityID(id)]
		return to, ok
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runSeq = max(e.runSeq, k.Seq)
	for _, kr := range k.Runs {
		steps, err := database.ParseTriggerSteps(kr.Trig.Steps)
		if err != nil || len(steps) == 0 || kr.Next < 0 || kr.Next > len(steps) {
			log.Printf("Keep: %s's run %d not made again: its steps no longer read (%v)", kr.Trig.Title(), kr.ID, err)
			r.RunsLost++
			continue
		}
		run := &triggerRun{id: kr.ID, trig: kr.Trig, steps: steps, by: kr.By, gmChar: kr.GMChar, quiet: kr.Quiet, world: kr.World,
			x: kr.X, y: kr.Y, z: kr.Z, next: kr.Next, at: later(kr.At), start: kr.Start, lock: kr.Lock, reach: kr.Reach,
			deadline: later(kr.Deadline), emptyAfter: kr.EmptyAfter, lastNear: later(kr.LastNear), died: kr.Died}
		if kr.Await != nil {
			s := *kr.Await
			run.await = &s
		}
		if kr.Dead != nil {
			run.dead = &triggerPlaceAt{world: kr.Dead.World, x: kr.Dead.X, y: kr.Dead.Y, z: kr.Dead.Z}
		}
		for _, s := range kr.Spawns {
			id, ok := mapped(s.ID)
			if !ok {
				// not made again (dead with its corpse gone, or its row gone): gone, as a tick counts one gone
				run.died = append(run.died, s.Name)
				continue
			}
			if _, live := alive[entity.EntityID(s.ID)]; live {
				r.Alive++
			} else {
				r.Corpses++
			}
			run.spawns = append(run.spawns, triggerSpawn{id: id, name: s.Name, until: later(s.Until)})
		}
		for _, rs := range kr.Resummons {
			run.resummons = append(run.resummons, &triggerResummon{step: rs.Step, fell: later(rs.Fell), callerSeen: rs.CallerSeen})
		}
		if kr.Protect != nil {
			id := entity.EntityID(kr.Protect.ID)
			if to, ok := mapped(kr.Protect.ID); ok {
				id = to
			}
			run.protect = &triggerProtect{id: id, name: kr.Protect.Name, text: kr.Protect.Text}
		}
		e.runSeq = max(e.runSeq, run.id)
		e.runs = append(e.runs, run)
		r.Runs++
		log.Printf("Triggers: %s's run %d (fired by %s) made again at step %d of %d: %d spawns, lock %q, clock %s",
			run.trig.Title(), run.id, run.by, run.next+1, len(run.steps), len(run.spawns), run.lock, clockLeft(run.deadline, now))
	}
	e.syncNamesLocked()
	return alive
}

// clockLeft is a run's clock as a log line says it.
func clockLeft(deadline, now time.Time) string {
	if deadline.IsZero() {
		return "none"
	}
	return deadline.Sub(now).Round(time.Second).String() + " left"
}
