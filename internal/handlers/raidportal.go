package handlers

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A raid's portal that the last one through closes (the owner, 2026-11-18, of
// eqoa.forumotion.com t218 "Tallon Zek": "When you talk to the portal, this is the
// dialog: (The energy of the portal is waning, use it quickly) CHOOSE: (use
// portal). Next dialog: Yes, I am the last / No, there are more that need to use
// it ... As the one with the key passes through last, and chooses 'Yes, I am the
// last', the portal will port the questor and close"). Generic: any portal NPC
// listed in raidPortals, each a spawn a trigger's run stands for a while (its
// stay is the "waning"; table npcspawnonly holds its row out of the world).
//
// Anyone of its level is asked the two questions and sent through on either
// answer to the second, the portal left open; only the bearer of its key closes it,
// and only on "Yes, I am the last": the bearer is sent through, its quest's step at
// the portal done (a step that has the key shown), and the portal taken away at
// once. The bearer's "No" sends no one ("the person with the key should go
// through last"). A non-bearer's "Yes" sends it through and says the portal stays.
// Nothing of a crowd closes it early (the post's "some have noticed glitches with
// the portal ... If it closes before all are through"): it closes on the bearer's
// word or its stay's end alone.
//
// A bearer whose quest has not counted its earlier steps (Heirloom 2026-11-18: the
// key held before the kills were counted, and the portal "has nothing to say") is
// put on the portal's step at once, and one who has not taken the quest is given it
// there, so the key and the portal are always enough.
//
// Before the quests (interact.go): the portal's quest step is the portal's to do.

// raidPortal is one portal that its key's bearer closes behind the raid.
type raidPortal struct {
	// NPC is its name (any case)
	NPC string
	// Key is the item, by name (records' ids may differ by shard), whose bearer
	// closes it
	Key string
	// To is the place, At where it lands
	To string
	At wayPlace
	// MinLevel is the level it takes (0: any)
	MinLevel int
	// Quest and Step are the quest step at it its bearer's "Yes" does (0: none)
	Quest, Step int
}

var raidPortals = []raidPortal{
	// Tallon Zek (t218): the Zek camp's portal, to Zaoran Plateau (S) by the ZP
	// armour's headstones, where the quest's old trip landed (the Teleporter's
	// "Tallon Zek's fortress" too), facing south up the mountain; quest 101327
	// "Tallon Zek" step 3 (tools/tallon_zek_20261118.py)
	{NPC: "Portal of Tallon", Key: "Tallon's Key", To: "Zaoran Plateau",
		At: wayPlace{World: 5, X: 4750, Y: 147.62, Z: 7380, Facing: 0}, MinLevel: 60, Quest: 101327, Step: 3},
}

// The portal's words: the post's, and ours for the rest.
const (
	raidPortalWaning  = "The energy of the portal is waning, use it quickly."
	raidPortalUse     = "Use the portal"
	raidPortalLeave   = "Leave it"
	raidPortalLastAsk = "Are you the last?"
	raidPortalLast    = "Yes, I am the last"
	raidPortalMore    = "No, there are more that need to use it"
	// the bearer's "No": it goes last
	raidPortalWaitLast = "Then let the others through first. The bearer of %s goes last, and the portal closes behind it."
	// a non-bearer's "Yes"
	raidPortalStays = "The portal holds open: only the bearer of %s can close it."
	// the bearer's "Yes"
	raidPortalCloses = "The portal flares as %s passes through, and closes."
	raidPortalGone   = "The portal is gone."
	raidPortalFar    = "You are too far from the portal."
)

// raidPortalBoxMark marks a raid portal's question's id (beside the summoning
// portal's 0x5A and the rest: interact.go's answers); raidPortalLastBit tells the
// second question from the first.
const (
	raidPortalBoxMark = 0x71000000
	raidPortalLastBit = 0x00800000
	// raidPortalReach is how far from the portal an answer may be given
	raidPortalReach = 40
	// raidPortalWait is how long a question stands
	raidPortalWait = 5 * time.Minute
)

// raidPortalAsk is a question put to a player: its box, the portal asked about and
// when it was asked.
type raidPortalAsk struct {
	box    uint32
	portal entity.EntityID
	which  int
	asked  time.Time
}

// raidPortalAsks are the questions out, by the player's id.
var raidPortalAsks = struct {
	sync.Mutex
	seq uint32
	by  map[entity.EntityID]raidPortalAsk
}{by: map[entity.EntityID]raidPortalAsk{}}

// IsRaidPortalBox is whether a box id is a raid portal's question.
func IsRaidPortalBox(id uint32) bool { return id&0xFF000000 == raidPortalBoxMark }

// raidPortalOf is the raid portal of that NPC's name, or -1.
func raidPortalOf(name string) int {
	for i := range raidPortals {
		if strings.EqualFold(strings.TrimSpace(name), raidPortals[i].NPC) {
			return i
		}
	}
	return -1
}

// raidPortalTalk is HandleInteract's question for a raid's portal, before the
// quests: the first question to anyone of its level. false for any other NPC.
func (h *InteractHandler) raidPortalTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || p == nil {
		return false
	}
	i := raidPortalOf(npc.Name)
	if i < 0 {
		return false
	}
	rp := &raidPortals[i]
	forgetDialogs(p)
	if rp.MinLevel > 0 && p.Level < rp.MinLevel {
		h.sendDialogueBox(sess, tooWeak(rp.To))
		return true
	}
	h.askRaidPortal(sess, p, npc.ServerID, i, 0, raidPortalWaning, []string{raidPortalUse, raidPortalLeave})
	return true
}

// askRaidPortal puts a question (kind 0: the first, raidPortalLastBit: the last)
// to the player and keeps it.
func (h *InteractHandler) askRaidPortal(sess *session.Session, p *entity.Player, portal entity.EntityID, which int, kind uint32, text string, options []string) {
	raidPortalAsks.Lock()
	raidPortalAsks.seq++
	box := raidPortalBoxMark | kind | (raidPortalAsks.seq & (raidPortalLastBit - 1))
	raidPortalAsks.by[p.ServerID] = raidPortalAsk{box: box, portal: portal, which: which, asked: time.Now()}
	raidPortalAsks.Unlock()
	SendQuestPopupMulti(sess, box, text, options)
}

// answerRaidPortal is HandleDialogueOption's question for a raid portal's box: it
// reports whether the box was one (an old one does nothing).
func (h *InteractHandler) answerRaidPortal(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsRaidPortalBox(box) {
		return false
	}
	if p == nil || option == 255 { // a close: the question stays (interact.go's rule)
		return true
	}
	raidPortalAsks.Lock()
	a, ok := raidPortalAsks.by[p.ServerID]
	if ok && a.box == box {
		delete(raidPortalAsks.by, p.ServerID)
	}
	raidPortalAsks.Unlock()
	if !ok || a.box != box || time.Since(a.asked) > raidPortalWait || a.which < 0 || a.which >= len(raidPortals) {
		return true
	}
	rp := &raidPortals[a.which]
	world, x, _, z, alive := h.gameWorld.NPCSpot(a.portal)
	if !alive || !h.gameWorld.NPCAlive(a.portal) {
		h.sendDialogueBox(sess, raidPortalGone)
		return true
	}
	if pw, px, _, pz, dead, ok := h.gameWorld.PlayerSpot(p); !ok || dead || pw != world || math.Hypot(float64(px-x), float64(pz-z)) > raidPortalReach {
		h.sendDialogueBox(sess, raidPortalFar)
		return true
	}
	if box&raidPortalLastBit == 0 {
		if option == 0 { // use the portal
			h.askRaidPortal(sess, p, a.portal, a.which, raidPortalLastBit, raidPortalLastAsk, []string{raidPortalLast, raidPortalMore})
		}
		return true
	}
	last := option == 0
	bearer := Quests().HeldNamed(p, rp.Key) > 0
	switch {
	case bearer && !last:
		h.sendDialogueBox(sess, fmt.Sprintf(raidPortalWaitLast, rp.Key))
		log.Printf("RaidPortal: %s, bearing %s, waits to go last through %s", p.Name, rp.Key, rp.NPC)
		return true
	case bearer && last:
		if rp.Quest > 0 && rp.Step > 0 {
			q := Quests()
			if why := q.CatchUpTo(sess, p, rp.Quest, rp.Step); why != "" {
				log.Printf("RaidPortal: %s's quest %d not put on step %d: %s", p.Name, rp.Quest, rp.Step, why)
			}
			q.PassSteps(sess, p, rp.NPC, a.portal, [][2]int{{rp.Quest, rp.Step}})
		}
		if !h.raidPortalSend(sess, p, rp) {
			return true
		}
		h.gameWorld.DespawnNPC(a.portal)
		SendClientMessage(sess, fmt.Sprintf(raidPortalCloses, p.Name))
		log.Printf("RaidPortal: %s, bearing %s, went last through %s: closed", p.Name, rp.Key, rp.NPC)
	default:
		if !h.raidPortalSend(sess, p, rp) {
			return true
		}
		if last {
			SendClientMessage(sess, fmt.Sprintf(raidPortalStays, rp.Key))
		}
		log.Printf("RaidPortal: %s went through %s (last: %v, no key)", p.Name, rp.NPC, last)
	}
	return true
}

// raidPortalSend sends the player through the portal to its place.
func (h *InteractHandler) raidPortalSend(sess *session.Session, p *entity.Player, rp *raidPortal) bool {
	dest := entity.CoachDest{Name: rp.To, X: rp.At.X, Y: rp.At.Y, Z: rp.At.Z, Facing: rp.At.Facing, World: rp.At.World}
	if !CanLoadWorld(sess, dest.World) {
		h.sendDialogueBox(sess, fmt.Sprintf("Your client cannot travel to %s.", rp.To))
		return false
	}
	if _, err := ValidateTeleport(dest.X, dest.Y, dest.Z, dest.World); err != nil {
		log.Printf("RaidPortal: %s's trip to %s refused: %v", rp.NPC, rp.To, err)
		return false
	}
	h.warp(sess, p, dest)
	return true
}

// CatchUpTo puts the player on the quest's step: one under way on an earlier step
// is moved on to it (the steps before counted as done), one who has not taken the
// quest (or finished a repeatable one) is given it there when it may take it. A
// player past the step, or on it, is left. It reports why nothing was done ("" when
// it is on the step).
func (q *QuestEngine) CatchUpTo(sess *session.Session, p *entity.Player, questID, step int) string {
	if q == nil || p == nil {
		return "the quests are not running"
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[questID]
	if qu == nil {
		return fmt.Sprintf("no quest %d", questID)
	}
	st := qu.Step(step)
	if st == nil {
		return fmt.Sprintf("%q has no step %d", qu.Name, step)
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	cq := c.quests[questID]
	if cq == nil || !cq.Active() {
		if why := q.mayTakeLocked(q.lookAt(sess), p, c, qu); why != "" {
			return why
		}
		if why := q.startLocked(sess, p, c, qu, step, false); why != "" {
			return why
		}
		log.Printf("Quests: %s given %q at step %d by its portal's key", p.Name, qu.Name, step)
		return ""
	}
	if cq.Step >= step {
		return ""
	}
	idx := c.index(questID)
	old := q.lineLocked(cq)
	q.clearSpawnsLocked(c.serverID, questID, cq.Step)
	from := cq.Step
	cq.Step, cq.Count = st.Step, 0
	if text := q.lineLocked(cq); text != old {
		q.dropLineLocked(sess, c, idx)
		cq.Slot = c.nextSlot()
		q.addLineLocked(sess, c, text)
	}
	q.saveLocked(cq)
	q.stepStartedLocked(sess, p, c, qu, cq, st)
	log.Printf("Quests: %s's %q caught up from step %d to %d by its portal's key", p.Name, qu.Name, from, step)
	return ""
}

// A raid's flag at a kill (the owner, 2026-11-18, of Tallon Zek's Zek Sentry: "the
// zek sentry step should place a bbox in front of the castle door until defeated
// and gives the flag for players"): the death of a creature of a listed name puts
// every player the kill counts for (the killer's group or raid within
// entity.KillShareRange, entity's creditKillLocked) on the quest's step after it,
// whatever step of it they were on or none: one on the kill's own step is moved on
// by its kill as ever; one on an earlier step, or without the quest, is put on the
// next (CatchUpTo, given the quest when it may take it). A player past it is left.
type raidKillFlag struct {
	// NPC is the creature's name (any case)
	NPC string
	// Quest and Step are the quest and the step the kill puts its players on
	Quest, Step int
}

var raidKillFlags = []raidKillFlag{
	// quest 101327 "Tallon Zek": step 5 kills the sentry, step 6 Tallon
	{NPC: "Zek sentry", Quest: 101327, Step: 6},
}

// killFlags puts a kill's players on its flag's step (raidKillFlags). It is called
// on the quest engine's goroutine after the kill is counted, without q.mu.
func (q *QuestEngine) killFlags(k entity.KillCredit) {
	for _, f := range raidKillFlags {
		if !strings.EqualFold(strings.TrimSpace(k.NPCName), f.NPC) {
			continue
		}
		for _, kp := range k.Players {
			var p *entity.Player
			if kp.Session != nil && q.playerOf != nil {
				p = q.playerOf(kp.Session)
			}
			if p == nil {
				continue
			}
			if why := q.CatchUpTo(kp.Session, p, f.Quest, f.Step); why != "" {
				log.Printf("Quests: %s not flagged by %s's death (quest %d step %d): %s", kp.Name, k.NPCName, f.Quest, f.Step, why)
			}
		}
	}
}
