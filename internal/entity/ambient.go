package entity

// Ambient lines: what an NPC says aloud, unprompted, to the players near it
// (a GM's ask of 2026-11-11 from live footage: a researcher in Highbourne's
// library says her lines to the room with no one talking to her, and they show
// in the chat as a say, "Name: line"). The lines are npcdialog rows of use
// ambient (database/npcdialog.go), whose selectors pick the speakers as a
// talk's lines do (useLineLocked); the timing and reach are ours:
//
//   - who: an NPC with an ambient line that fits it, able to speak (alive, not
//     a pet or a dummy, not fighting: dialogAble), with a player in its world
//     within AmbientNear;
//   - when: AmbientMin to AmbientMax after its last line, or after a player
//     first came near; never the same line twice running when another fits;
//   - how: a say from the NPC (its name before the line, white, as a player's
//     say goes) to every player within a say's reach (MetalSayHeard), never to
//     one loading a world, never in a dialogue box.
//
// A keep restart keeps none of it: the speakers' times are memory alone and a
// new image schedules each afresh when a player is near (at most AmbientMax).

import (
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

const (
	// AmbientNear is how close a player brings an NPC's ambient lines on.
	AmbientNear float32 = 60
	// AmbientMin and AmbientMax bound the time between an NPC's ambient lines.
	AmbientMin = 90 * time.Second
	AmbientMax = 240 * time.Second
	// ambientEvery is how often the speakers are looked at.
	ambientEvery = 5 * time.Second
	// ambientLoadMost is how long a world load is taken to be underway at
	// most (a player whose client is never seen at the destination hears
	// again after it).
	ambientLoadMost = time.Minute
)

// ambientSpeaker is one NPC's ambient time: when it speaks next and the line
// it said last.
type ambientSpeaker struct {
	next time.Time
	last int
}

// ambientState is the ambient lines' state: the table's reader (read with no
// lock held), when the speakers were last looked at, and each speaker's time.
type ambientState struct {
	lines    func() ([]database.NPCDialogLine, error)
	at       time.Time
	speakers map[EntityID]*ambientSpeaker
}

// SetAmbientLines gives the World npcdialog's reader (database.NPCDialog) for
// the ambient lines; nil: none are said.
func (w *World) SetAmbientLines(f func() ([]database.NPCDialogLine, error)) {
	w.mu.Lock()
	w.ambient.lines = f
	w.mu.Unlock()
}

// runAmbient is the ambient lines' turn after the tick's lock (Tick), every
// ambientEvery: npcdialog read with no lock held (it may query), then
// AmbientSay.
func (w *World) runAmbient(now time.Time) {
	w.mu.RLock()
	f, at := w.ambient.lines, w.ambient.at
	w.mu.RUnlock()
	if f == nil || now.Sub(at) < ambientEvery {
		return
	}
	w.mu.Lock()
	w.ambient.at = now
	w.mu.Unlock()
	lines, err := f()
	if err != nil {
		log.Printf("Ambient: npcdialog: %v", err)
		return
	}
	w.AmbientSay(lines, now)
}

// AmbientSay is one look at the speakers (the file's comment): each NPC near a
// player with an ambient line that fits it is given its time, and says a line
// when its time has come. It returns how many lines were said.
func (w *World) AmbientSay(lines []database.NPCDialogLine, now time.Time) int {
	ambient := false
	for _, l := range lines {
		if l.Use == database.DialogUseAmbient && l.Line != "" && l.Weight > 0 {
			ambient = true
			break
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !ambient {
		w.ambient.speakers = nil
		return 0
	}
	if w.ambient.speakers == nil {
		w.ambient.speakers = make(map[EntityID]*ambientSpeaker)
	}
	seen := make(map[EntityID]bool)
	said := 0
	for _, p := range w.Players {
		if !w.ambientHearsLocked(p, now) {
			continue
		}
		for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, AmbientNear) {
			npc, ok := w.NPCs[id]
			if !ok || seen[id] || npc.World != p.World || !dialogAble(npc) {
				continue
			}
			seen[id] = true
			sp := w.ambient.speakers[id]
			if sp == nil {
				sp = &ambientSpeaker{next: now.Add(ambientWait())}
				w.ambient.speakers[id] = sp
			}
			if now.Before(sp.next) {
				continue
			}
			// its line, picked as a talk's is, to the player who brought it on;
			// each hearer gets the words filled in for them
			if _, _, lid, ok := w.useLineLocked(p, npc, lines, sp.last, database.DialogUseAmbient); ok {
				w.ambientSayLocked(npc, lines, lid, now)
				sp.last = lid
				said++
			}
			sp.next = now.Add(ambientWait())
		}
	}
	for id := range w.ambient.speakers {
		if !seen[id] {
			delete(w.ambient.speakers, id) // no one near: its time starts again when someone comes
		}
	}
	return said
}

// ambientWait is a time between AmbientMin and AmbientMax.
func ambientWait() time.Duration {
	return AmbientMin + time.Duration(rand.Int64N(int64(AmbientMax-AmbientMin)+1))
}

// ambientHearsLocked is whether p hears ambient lines: in the world and not
// loading one. Must be called with the lock held.
func (w *World) ambientHearsLocked(p *Player, now time.Time) bool {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	return sess != nil && sess.CharacterInWorld() && !sess.WorldLoadUnderway(now, ambientLoadMost)
}

// ambientSayLocked sends npc's line id as a say to every player within a say's
// reach in its world who hears ambient lines. Must be called with the lock
// held.
func (w *World) ambientSayLocked(npc *NPC, lines []database.NPCDialogLine, id int, now time.Time) {
	var text string
	for _, l := range lines {
		if l.ID == id {
			text = l.Line
			break
		}
	}
	speaker := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(npc.Name), "$"))
	town := DialogTown(w.npcFactionLocked(npc))
	n := 0
	for _, p := range w.Players {
		if p.World != npc.World || distXZ(p.X, p.Z, npc.X, npc.Z) > MetalSayHeard || !w.ambientHearsLocked(p, now) {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendChatToSession(sess, speaker+": "+DialogSay(text, p, speaker, town))
			n++
		}
	}
	log.Printf("Ambient: %s says line %d to %d players", speaker, id, n)
}
