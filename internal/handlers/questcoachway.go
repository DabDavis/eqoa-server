package handlers

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The coaches as the quests' ways (the owner, 2026-11-25, Heirloom petition
// 3014: "the mini map and quest finder should direct you to the coachman for
// this", "maybe even put a yellow questionmark next to Goldfeather Eyrie in
// the coach list while on the quest"). Packleader Varg's quests are done in
// the Homelands' Goldfeather Eyrie, a home of its own that only a coach
// reaches; in Freeport the map had no mark (no waysender's trip leads into
// the Homelands), and at Varg's camp it marked the badgers 5,800 off, in a
// zone no road joins.
//
//   - Every coachman's stops are trips of the way marks (wayHopsLocked,
//     entity.CoachTrips), between places told apart by world and, in a world
//     of races' homes, by home (wayNode): a step in another home of the
//     Homelands has its way mark at the coachman, "Ride Coachman Skulk's
//     coach to Goldfeather Eyrie", as a step in another world has.
//   - A coach stop counts only as his menu would sell it to the player
//     (wayAllowLocked): a world its client holds, and on a network a ledger
//     it has signed (charcoaches, read at most every signedEvery).
//   - His menu puts " ?" after the stop of a step under way (CoachStops,
//     coachMenu): the stop in the step's place nearest the step's mark (when
//     that is nearer it than the coachman is by coachStopGain, or in another
//     place), else the stop the way there goes on by. The menu is the
//     server's text, so a PS2 shows it too.

// questStopTag is what a coach menu's stop of a quest's step says at the end
// of its line, after any fee or period (petition 3191: "...Aviak Village. ?";
// vanilla draws a trailing " ?" yellow).
const questStopTag = " ?"

// optionChars is the most of an answer a 0x0034 carries (writeWideString's 63).
const optionChars = 63

// tagged is line with tag at its end, kept inside optionChars: a line that
// would run past them with it loses its closing period, then its tail, so the
// cut (writeWideString's) never takes the mark.
func tagged(line, tag string) string {
	if tag == "" {
		return line
	}
	r, t := []rune(line), []rune(tag)
	if len(r)+len(t) > optionChars && len(r) > 0 && r[len(r)-1] == '.' {
		r = r[:len(r)-1]
	}
	if len(r)+len(t) > optionChars {
		r = []rune(strings.TrimRight(string(r[:max(optionChars-len(t), 0)]), " "))
	}
	return string(r) + tag
}

// signedEvery is how long a character's signed ledgers are kept for the way
// marks before they are read again.
const signedEvery = 30 * time.Second

// coachStopGain is how much nearer a step's mark a stop in the coachman's own
// place must set the player down for its " ?".
const coachStopGain = 500

// signedLedgers is a character's signed ledgers as read at.
type signedLedgers struct {
	towns map[string]bool
	at    time.Time
}

// signedLocked is the character's signed ledgers (nil: unknown, every stop
// counts). Must be called with q.mu held.
func (q *QuestEngine) signedLocked(serverID uint32, now time.Time) map[string]bool {
	if q.coachSigned == nil || serverID == 0 {
		return nil
	}
	if s, ok := q.signed[serverID]; ok && now.Sub(s.at) < signedEvery {
		return s.towns
	}
	towns, err := q.coachSigned(serverID)
	if err != nil {
		log.Printf("Quests: the way marks reading %d's ledgers: %v", serverID, err)
		towns = nil
	}
	if q.signed == nil {
		q.signed = map[uint32]signedLedgers{}
	}
	q.signed[serverID] = signedLedgers{towns, now}
	return towns
}

// wayAllowLocked is which trips the character may take: every travel NPC's
// (its own rules are its menu's), and a coach's stop only in a world its
// client holds (holdsWorld) and, on a network, with its ledger signed. Must
// be called with q.mu held.
func (q *QuestEngine) wayAllowLocked(serverID uint32) func(wayHop) bool {
	signed := q.signedLocked(serverID, time.Now())
	var sess *session.Session
	if q.sessionOf != nil {
		sess = q.sessionOf(serverID)
	}
	return func(h wayHop) bool {
		if !h.coach {
			return true
		}
		if sess != nil && !holdsWorld(sess, h.to) {
			return false
		}
		return !h.ledger || signed == nil || signed[h.name]
	}
}

// CoachStops marks the stops of a coachman's menu (dests, the player's) where a
// step of its quests under way is done, or the way there goes on by
// (entity.CoachDest.QuestStop); the file's doc.
func (q *QuestEngine) CoachStops(p *entity.Player, coach *entity.NPC, dests []entity.CoachDest) {
	if q == nil || p == nil || coach == nil || len(dests) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	c := q.namedLocked(p.CharServerID, p.Name)
	spots := q.markSpotsLocked(now)
	here := wayNodeAt(coach.World, coach.SpawnX, coach.SpawnZ)
	var hops []wayHop // the trips the player may take, made at the first step that needs them
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		for _, st := range openSteps(qu, cq) {
			_, at := q.stepSpotsLocked(spots, st, c.serverID, coach.World, coach.SpawnX, coach.SpawnZ)
			if len(at) == 0 {
				continue
			}
			mark := at[0]
			there := wayNodeOf(mark)
			// a stop in the step's place: the nearest its mark
			best, bestD := -1, 0.0
			for i, d := range dests {
				if d.Ledgers || wayNodeAt(d.World, d.X, d.Z) != there {
					continue
				}
				if dd := math.Hypot(float64(d.X-mark.X), float64(d.Z-mark.Z)); best < 0 || dd < bestD {
					best, bestD = i, dd
				}
			}
			if best >= 0 {
				own := math.Hypot(float64(coach.SpawnX-mark.X), float64(coach.SpawnZ-mark.Z))
				if there != here || bestD+coachStopGain < own {
					dests[best].QuestStop = true
				}
				continue
			}
			if there == here {
				continue
			}
			// else the stop the way there goes on by: the fewest trips on
			if hops == nil {
				allow := q.wayAllowLocked(c.serverID)
				hops = []wayHop{}
				for _, h := range q.wayHopsLocked(spots) {
					if allow(h) {
						hops = append(hops, h)
					}
				}
			}
			far := wayFar(hops, there)
			limit, reach := far[here]
			best, bestN := -1, 0
			for i, d := range dests {
				if d.Ledgers {
					continue
				}
				n, ok := far[wayNodeAt(d.World, d.X, d.Z)]
				if !ok || reach && n >= limit {
					continue
				}
				if best < 0 || n < bestN {
					best, bestN = i, n
				}
			}
			if best >= 0 {
				dests[best].QuestStop = true
			}
		}
	}
}
