package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questplaceoffer.go — a quest offered at a place, not only by talking to its
// giver. Heirloom petitions 3322 and 3325 (Underwater, 2026-11-27): in
// "Envaric Ascension" (101323) the Northeast and Southwest Altars of Envar are
// no creatures in the original game; standing on the altar gives a pop-up.
// Steps 5 and 10 became reach steps at the altars' spots (reachTakes, a popup
// event) and the Southwest Altar unselectable (npctitle.noselect), but the
// Northeast Altar had to stay a creature one talks to: it is the quest's
// giver, and a quest was offered only by a talk (Talk, offerAtLocked) or a
// shop's window (AtWindow). The owner, 2026-11-27: "yes build the place-based
// quest offer".
//
//   - A quest's place is its offer columns (database.Quest.OfferAt: offerworld,
//     offerx, offery, offerz, offerradius, offerheight; radius 0 none). Its
//     giver may stay (the journals, the GM panel's lists, a talk to it still
//     offers it) or be empty.
//   - Once a second (checkPlaceOffers, beside checkReach) each character in
//     the world and alive standing within the place's radius (and within its
//     height of its y, when it has one) is offered the quest when it may take
//     it: mayTakeLocked, the very rules of a giver's offer (its levels,
//     classes and races, the quests it requires, its twins, its day group, its
//     standings, a hold, neither under way nor finished unless repeatable).
//   - The offer is the giver's (offerLocked): its words in 0x0046 boxes and its
//     question in a 0x0034 box with the answers taking it and refusing it, the
//     box the reach steps' pop-ups (popupLocked) and every quest offer already
//     open with no NPC in it, on a PS2 (CLIENT's Msg0034, Msg0046) and in
//     vanilla alike. A place has no voice: the offer, the questions at it and
//     the words once taken are said with no name before them (questVoice), as
//     the original's pop-up was ("The altar's voice: ...").
//   - Once each time the character comes in: offered, it is not offered again
//     while it stays (atPlace), refused or not; it steps out and in again to
//     be offered again. A player that may not take it on coming in is offered
//     it the second it may (a level gained standing there). A login starts
//     afresh (LoggedIn), so does a restart. A refusal here is not the giver's
//     refusal (c.declined moves a giver's talk on to its next quest; the place
//     asks again at the next coming in whatever it holds).
//   - Taking it is the giver's taking (Answer's choiceAccept, startLocked): the
//     charquests row, the journal's line, the quest's accept events, the
//     marks; refusing it is the giver's refusal (its declined words).
//   - The map's "!" for it stands at the place (marksLocked; its giver's name,
//     or its own, the mark's name), not at its giver's spawns. The marks over
//     the NPCs' heads are by NPC and stay its giver's.
//
// No more is gated than a giver's offer is: a talk to a quest's giver asks
// nothing of the quality-of-life settings or of Hardcore, and the place asks
// nothing either.

// questVoice is who says a quest's offer, its questions and its words once
// taken: its giver, or no one for a quest offered at a place, a pop-up.
func questVoice(qu *database.Quest) string {
	if qu.OfferAt != nil {
		return ""
	}
	return qu.Giver
}

// placeQuestsOf is the enabled quests offered at a place, by id (the catalog's
// order).
func placeQuestsOf(all []*database.Quest) []*database.Quest {
	var out []*database.Quest
	for _, qu := range all {
		if qu.Enabled && qu.OfferAt != nil && qu.OfferAt.Radius > 0 {
			out = append(out, qu)
		}
	}
	return out
}

// checkPlaceOffers offers each character standing at a quest's place the
// quest, once each time it comes in, when it may take it. Once a second, off
// the engine's own goroutine.
func (q *QuestEngine) checkPlaceOffers() {
	if q.sessionOf == nil || q.playerOf == nil || q.gw == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.cat.places) == 0 {
		for _, c := range q.chars {
			c.atPlace = nil
		}
		return
	}
	for id, c := range q.chars {
		sess := q.sessionOf(id)
		if sess == nil {
			c.atPlace = nil // out of the world: its next coming in is one
			continue
		}
		p := q.playerOf(sess)
		world, x, y, z, ok := q.gw.PlayerPlace(p)
		if !ok {
			continue // dead or loading: where it stood stands until it is seen again
		}
		// where it stands: a place it stepped out of is forgotten first, so
		// that an offer left open there holds back no other place's
		var here []*database.Quest
		for _, qu := range q.cat.places {
			if qu.OfferAt.Within(int(world), x, y, z) {
				here = append(here, qu)
			} else {
				delete(c.atPlace, qu.ID)
			}
		}
		// at most one open offer of a place at a time: another place's (two
		// places that overlap) waits for its answer while the player stays at
		// the first
		pending := c.box != nil && c.placeBox != 0 && c.box.id == c.placeBox && c.atPlace[c.placeQuest]
		var look *factionLook
		for _, qu := range here {
			if c.atPlace[qu.ID] || pending {
				continue
			}
			if look == nil {
				look = q.lookAt(sess)
			}
			if q.mayTakeLocked(look, p, c, qu) != "" {
				continue // asked again next second: offered the second it may
			}
			if c.atPlace == nil {
				c.atPlace = map[int]bool{}
			}
			c.atPlace[qu.ID] = true
			c.box = nil
			log.Printf("Quests: %s stands at %q's place (world %d, %.1f, %.1f): offered", p.Name, qu.Name, world, x, z)
			q.offerLocked(sess, p, c, qu)
			if c.box != nil {
				c.placeBox, c.placeQuest, pending = c.box.id, qu.ID, true
			}
		}
	}
}

// placeMarksLocked is the map's "!" of the quests offered at a place the
// player may take now (as the givers' in marksLocked: a name once, none while
// a quest of its name is under way or finished, only near the player's level):
// each at its place, its giver's name (or its own) the mark's. byName is the
// names the givers mark already, which a place's does not mark again. Must be
// called with q.mu held.
func (q *QuestEngine) placeMarksLocked(look *factionLook, p *entity.Player, c *questChar, taken map[string]bool, marked func(string) bool,
	add func(kind byte, s entity.NPCSpot, mq markQuest)) {
	for _, qu := range q.cat.places {
		k := q.cat.foldedName(qu)
		pl := qu.OfferAt
		if taken[k] || marked(k) || !markLevelNear(p.Level, qu.MinLevel) || pl.World < 0 || pl.World > 255 {
			continue
		}
		if q.mayTakeLocked(look, p, c, qu) != "" {
			continue
		}
		name := strings.TrimSpace(qu.Giver)
		if name == "" {
			name = qu.Name
		}
		t := q.texts[qu.ID]
		if t == nil {
			t = &questText{}
		}
		add(markTake, entity.NPCSpot{World: uint8(pl.World), X: pl.X, Z: pl.Z, Name: name},
			markQuest{name: qu.Name, min: qu.MinLevel, max: qu.MaxLevel, line: t.offer, detail: t.rewardsFor(p.Class), ability: t.abilityFor(p.Class)})
		taken[k] = true // a name once
	}
}
