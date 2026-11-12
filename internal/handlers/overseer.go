package handlers

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Overseers: an NPC who, talked to, says which bosses are up (petition 1699,
// Skywalker: "Overseer Yurgulch can be used to check whether Bertoxxulous,
// Uzrathixius, or Poxathixius spawns are up and available to kill"; the wiki's
// Plane of Disease: Bertoxxulous Quest). Table npcoverseer (database/npcoverseer.go)
// names the overseers and the bosses each tells of, with his own words for each
// state; this file reads the world for each boss:
//
//   - up: one of the name alive in its world (its row, or a trigger's or a
//     quest's spawn of it);
//   - down: its row's creature dead, back by its respawn (npcrespawn, corpse.go)
//     at a time said roughly ("soon", "in about 5 hours");
//   - for a boss a trigger brings (Bertoxxulous at his throne, trigger
//     "Bertoxxulous Rises"), not up: ready while the trigger's lock is free,
//     resting while a run of it lives, until that run's waits are done;
//   - unknown: none up and no time known (a GM's Kill Spawner holds it, no row
//     of it stands, its trigger is off or broken).
//
// He answers after the quests (a quest's step at him still comes first) and
// before the town's lines; one dialogue box, or more when the lines do not fit.

// overseerNow is the clock an overseer reads (a test's own).
var overseerNow = time.Now

// overseerSoon is how near a time is said as "soon".
const overseerSoon = 15 * time.Minute

// The plain lines of each state, for a row that has none of its own.
const (
	overseerUp      = "%b is up."
	overseerDown    = "%b is dead. Back %t."
	overseerReady   = "%b is not up. It can be called now."
	overseerResting = "%b is not up. It can be called again %t."
	overseerUnknown = "%b is not up, and no one knows when."
)

// overseerTalk answers an overseer: a line for each of his bosses. It reports
// whether the NPC was one.
func (h *InteractHandler) overseerTalk(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	if h.overseers == nil || npc == nil || npc.Dead || h.gameWorld == nil {
		return false
	}
	all, err := h.overseers()
	if err != nil {
		log.Printf("Interact: npcoverseer: %v", err)
		return false
	}
	rows := all[database.NPCClassKey(npc.Name)]
	if len(rows) == 0 {
		return false
	}
	forgetDialogs(player)
	lines := OverseerLines(h.gameWorld, Triggers(), npc, player.Name, rows, overseerNow())
	sayPacked(sess, npc.Name, lines)
	log.Printf("Interact: %s asked %s which bosses are up: %q", player.Name, npc.Name, lines)
	return true
}

// OverseerLines is what the overseer npc says to the player called name: the
// greeting of his first row that has one, then a line for each row's boss.
func OverseerLines(gw *entity.World, e *TriggerEngine, npc *entity.NPC, name string, rows []database.NPCOverseerRow, now time.Time) []string {
	var out []string
	for _, r := range rows {
		if g := strings.TrimSpace(r.Greeting); g != "" {
			out = append(out, overseerText(g, r.Boss, "", name))
			break
		}
	}
	for _, r := range rows {
		world := npc.World
		if r.World >= 0 {
			world = uint8(r.World)
		}
		st := gw.BossState(r.Boss, world, now)
		var line string
		switch {
		case st.Up:
			line = overseerText(orText(r.Up, overseerUp), r.Boss, "", name)
		case strings.TrimSpace(r.Trigger) != "":
			rest := e.RestOf(r.Trigger, now)
			switch {
			case !rest.Known:
				line = overseerText(orText(r.Unknown, overseerUnknown), r.Boss, "", name)
			case !rest.Held:
				line = overseerText(orText(r.Ready, overseerReady), r.Boss, "", name)
			case rest.Free.IsZero():
				line = overseerText(orText(r.Unknown, overseerUnknown), r.Boss, "", name)
			default:
				line = overseerText(orText(r.Resting, overseerResting), r.Boss, overseerWhen(rest.Free.Sub(now)), name)
			}
		case !st.Back.IsZero():
			line = overseerText(orText(r.Down, overseerDown), r.Boss, overseerWhen(st.Back.Sub(now)), name)
		default:
			line = overseerText(orText(r.Unknown, overseerUnknown), r.Boss, "", name)
		}
		out = append(out, line)
	}
	return out
}

// overseerText is a row's line: %b the boss, %t when, %p the player.
func overseerText(text, boss, when, player string) string {
	return strings.NewReplacer("%b", strings.TrimSpace(boss), "%t", when, "%p", player).Replace(strings.TrimSpace(text))
}

// overseerWhen is a time to come, roughly: "soon" within overseerSoon, then
// minutes to the nearest ten, hours, and days with their hours.
func overseerWhen(d time.Duration) string {
	switch {
	case d <= overseerSoon:
		return "soon"
	case d < 50*time.Minute:
		return fmt.Sprintf("in about %d minutes", int(math.Round(d.Minutes()/10))*10)
	case d < 90*time.Minute:
		return "in about an hour"
	case d < 36*time.Hour:
		return fmt.Sprintf("in about %d hours", int(math.Round(d.Hours())))
	}
	h := int(math.Round(d.Hours()))
	days, hours := h/24, h%24
	switch {
	case hours == 0:
		return fmt.Sprintf("in about %d days", days)
	case hours == 1:
		return fmt.Sprintf("in about %d days and 1 hour", days)
	}
	return fmt.Sprintf("in about %d days and %d hours", days, hours)
}

// sayPacked says lines in as few dialogue boxes (0x0046) as hold them, a line a
// row of the box, the speaker's name before the first; a '%' doubled, which
// CLIENT's text expander shows as one (signs.go).
func sayPacked(sess *session.Session, speaker string, lines []string) {
	box := ""
	flush := func() {
		if box != "" {
			SendDialogueBox(sess, 0, strings.ReplaceAll(box, "%", "%%"))
			box = ""
		}
	}
	for i, l := range lines {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if i == 0 && speaker != "" {
			l = strings.TrimSpace(speaker) + ": " + l
		}
		for _, part := range splitSay(l, questBoxChars) {
			if box != "" && len([]rune(box))+1+len([]rune(part)) > questBoxChars {
				flush()
			}
			if box == "" {
				box = part
			} else {
				box += "\n" + part
			}
		}
	}
	flush()
}

// TriggerRest is a trigger's place as an overseer reads it: Known, the trigger
// is there, on and sound; Held, a run of its lock lives (it cannot fire now);
// Free, when that run is done by its waits (zero: not known, a wait on a talk).
type TriggerRest struct {
	Known, Held bool
	Free        time.Time
}

// RestOf is the trigger called name's rest at now. A trigger with no lock is
// never held.
func (e *TriggerEngine) RestOf(name string, now time.Time) TriggerRest {
	if e == nil || e.db == nil {
		return TriggerRest{}
	}
	slot, ok := e.slotNamed(name)
	if !ok {
		return TriggerRest{}
	}
	t, ok, err := e.db.GMTriggerBySlot(slot)
	if err != nil || !ok || !t.Enabled || len(database.TriggerProblems(t)) > 0 {
		return TriggerRest{}
	}
	steps, err := database.ParseTriggerSteps(t.Steps)
	if err != nil {
		return TriggerRest{}
	}
	lock := database.TriggerLockName(steps)
	rest := TriggerRest{Known: true}
	if lock == "" {
		return rest
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.runs {
		if r.lock != lock || r.stopped {
			continue
		}
		free, ok := r.freeAt(now)
		if !ok {
			// one run's end unknown: the whole rest is
			rest.Held, rest.Free = true, time.Time{}
			return rest
		}
		rest.Held = true
		if free.After(rest.Free) {
			rest.Free = free
		}
	}
	return rest
}

// freeAt is when the run's steps are done by its waits, false when a wait on a
// talk with no time stands in the way. A random wait counts its middle; a wait
// on deaths nothing (its spawns are what is asked about); the run's spawns'
// stay is not counted. Must be called with e.mu held.
func (r *triggerRun) freeAt(now time.Time) (time.Time, bool) {
	at := now
	switch {
	case r.await != nil:
		if r.await.Do == database.TriggerAwaitTalk {
			if r.at.IsZero() {
				return time.Time{}, false
			}
			at = r.at
		}
	case r.next > 0 && r.next <= len(r.steps) && r.steps[r.next-1].Do == database.TriggerWait && r.at.After(now):
		at = r.at
	}
	for _, s := range r.steps[min(r.next, len(r.steps)):] {
		switch s.Do {
		case database.TriggerWait:
			d := s.Seconds
			if s.Upto > s.Seconds {
				d += (s.Upto - s.Seconds) / 2
			}
			at = at.Add(time.Duration(d * float64(time.Second)))
		case database.TriggerAwaitTalk:
			if s.Seconds <= 0 {
				return time.Time{}, false
			}
			at = at.Add(time.Duration(s.Seconds * float64(time.Second)))
		}
	}
	return at, true
}
