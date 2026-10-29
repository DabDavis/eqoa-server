package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A quest's events and its loot steps (quests.go; tools/quests/FORMAT.md).
//
// Events are what a quest does when it is taken (accept) and finished
// (finish), and what a step does when it becomes the step to do (start) and
// when it is done (done), in the order written, after the engine's own words
// at that moment:
//
//   - popup: a window the player closes. CLIENT's only box a server opens
//     that waits for the player is Msg0034's question (VIDialogBox, queued
//     while another is up), so a popup is one with a single answer ("OK"
//     unless the event names one), the text before its last 127 characters
//     said in 0x0046 boxes first, as the quests' questions are.
//   - say: an NPC's words, a 0x0046 box a line, its name before each.
//   - give: items into the inventory, as a reward's.
//   - spawn: a copy of an NPC a few steps in front of the player for the step
//     (World.SpawnForQuest), hostile or not; it goes when the step is done, the
//     quest given up or reset, the player away, or after questSpawnLife.
//   - visual: a spell visual (a SPELLFX.CSF DictID) played from the player on
//     itself, as a cast's 0x00B1 begins one for everyone who sees it.
//   - teleport: the player warped as a travel NPC warps it (a world its
//     client cannot load is refused).
//
// A loot step (kind loot) is a kill step whose kills drop its item: each kill
// of its creature credited to a player on the step, while the player has
// taken fewer than the step wants, drops one with the step's chance onto that
// player's own view of the corpse (questloot.go); the step counts what the
// player takes off corpses while it is the step to do (the character's
// quest count, as a kill step counts kills: an item held from before is not
// counted, as the content's chains want), and is done at its count.

// questSpawnLife is how long a quest's spawn stays at most.
const questSpawnLife = 10 * time.Minute

// questPopupAnswer is a popup's answer when its event names none.
const questPopupAnswer = "OK"

// questSpawn is an NPC a step's spawn event placed.
type questSpawn struct {
	id          entity.EntityID
	quest, step int
	until       time.Time
}

// questEventsLocked does a quest's own events of one time.
func (q *QuestEngine) questEventsLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, on string) {
	q.runEventsLocked(sess, p, c, qu, 0, database.EventsOn(qu.Events, on))
}

// stepEventsLocked does a step's events of one time.
func (q *QuestEngine) stepEventsLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, st *database.QuestStep, on string) {
	q.runEventsLocked(sess, p, c, qu, st.Step, database.EventsOn(st.Events, on))
}

// runEventsLocked does events in order for the quest (step its step, 0 the
// quest's own). Nothing is done for a character not in the world.
func (q *QuestEngine) runEventsLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, step int, evs []database.QuestEvent) {
	if sess == nil || p == nil || len(evs) == 0 {
		return
	}
	for _, e := range evs {
		switch e.Do {
		case database.EventPopup:
			q.popupLocked(sess, c, e.Text, e.Answer)
		case database.EventSay:
			q.tellLocked(c, sess, e.NPC, sayLines(e.Text)...)
		case database.EventGive:
			q.tellLocked(c, sess, "", q.giveItemsLocked(sess, p, database.ParseItemCounts(e.Items))...)
		case database.EventSpawn:
			q.spawnLocked(p, c, qu, step, e)
		case database.EventVisual:
			fx, ok := database.ParseVisualID(e.FX)
			if !ok || q.gw == nil {
				log.Printf("Quests: %q step %d: visual %q is no DictID", qu.Name, step, e.FX)
				continue
			}
			q.gw.PlayVisualOn(sess, fx, time.Duration(e.Seconds*float64(time.Second)))
		case database.EventTeleport:
			q.teleportLocked(sess, p, qu, e)
		default:
			log.Printf("Quests: %q step %d: no event %q", qu.Name, step, e.Do)
		}
	}
}

// popupLocked opens a window the player closes: text (its lines, each cut
// to boxes) in 0x0046 boxes, its last part the question of a 0x0034 with one
// answer, which changes nothing.
func (q *QuestEngine) popupLocked(sess *session.Session, c *questChar, text, answer string) {
	var parts []string
	for _, l := range sayLines(c.say(text)) {
		parts = append(parts, splitSay(l, questAskChars)...)
	}
	if len(parts) == 0 {
		return
	}
	q.tellLocked(c, sess, "", parts[:len(parts)-1]...)
	q.askLocked(sess, c, "", parts[len(parts)-1], questAskStep,
		[]questChoice{{kind: choiceNothing, label: orText(answer, questPopupAnswer)}})
}

// giveItemsLocked gives items as a reward does and returns the lines that say so.
func (q *QuestEngine) giveItemsLocked(sess *session.Session, p *entity.Player, items []database.ItemCount) []string {
	var lines []string
	for _, it := range items {
		pat, err := q.items.GetItemPattern(it.PatternID)
		if err != nil || pat == nil {
			log.Printf("Quests: item %d: %v", it.PatternID, err)
			continue
		}
		n, err := GiveItemNow(sess, q.gw, q.inv, *pat, it.Count)
		if err != nil {
			log.Printf("Quests: giving %s %q: %v", p.Name, pat.ItemName, err)
		}
		if n > 0 {
			lines = append(lines, "You have received "+countOf(n, pat.ItemName)+".")
		}
		if n < it.Count {
			lines = append(lines, fmt.Sprintf("Your inventory is full: %s could not be given.", countOf(it.Count-n, pat.ItemName)))
		}
	}
	return lines
}

// spawnLocked places a step's spawn and remembers it.
func (q *QuestEngine) spawnLocked(p *entity.Player, c *questChar, qu *database.Quest, step int, e database.QuestEvent) {
	if q.gw == nil || step == 0 {
		return
	}
	id, name, ok := q.gw.SpawnForQuest(strings.TrimSpace(e.NPC), p, e.Hostile)
	if !ok {
		log.Printf("Quests: %q step %d: no NPC %q to spawn", qu.Name, step, e.NPC)
		return
	}
	if q.spawns == nil {
		q.spawns = map[uint32][]questSpawn{}
	}
	q.spawns[c.serverID] = append(q.spawns[c.serverID], questSpawn{id: id, quest: qu.ID, step: step, until: time.Now().Add(questSpawnLife)})
	log.Printf("Quests: %q step %d spawned %q (%d) for %s", qu.Name, step, name, id, p.Name)
}

// clearSpawnsLocked takes away a character's spawns of the quest (step 0:
// every step's). A spawn that died is left to its corpse.
func (q *QuestEngine) clearSpawnsLocked(serverID uint32, quest, step int) {
	keep := q.spawns[serverID][:0]
	for _, s := range q.spawns[serverID] {
		if s.quest == quest && (step == 0 || s.step == step) {
			q.despawn(s)
			continue
		}
		keep = append(keep, s)
	}
	if len(keep) == 0 {
		delete(q.spawns, serverID)
	} else {
		q.spawns[serverID] = keep
	}
}

// despawn takes a live spawn out of the world.
func (q *QuestEngine) despawn(s questSpawn) {
	if q.gw != nil && q.gw.NPCAlive(s.id) {
		q.gw.DespawnNPC(s.id)
	}
}

// expireSpawns takes away the spawns past their life and those of
// characters no longer in the world (once a second).
func (q *QuestEngine) expireSpawns() {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for id, list := range q.spawns {
		away := q.sessionOf == nil || q.sessionOf(id) == nil
		keep := list[:0]
		for _, s := range list {
			if away || now.After(s.until) {
				q.despawn(s)
				continue
			}
			keep = append(keep, s)
		}
		if len(keep) == 0 {
			delete(q.spawns, id)
		} else {
			q.spawns[id] = keep
		}
	}
}

// teleportLocked warps the player as a travel NPC does.
func (q *QuestEngine) teleportLocked(sess *session.Session, p *entity.Player, qu *database.Quest, e database.QuestEvent) {
	if q.gw == nil || e.World < 0 || e.World > 255 {
		return
	}
	world := byte(e.World)
	if !CanLoadWorld(sess, world) {
		log.Printf("Quests: %q: %s's client cannot load world %d", qu.Name, p.Name, world)
		return
	}
	if _, err := ValidateTeleport(e.X, e.Y, e.Z, world); err != nil {
		log.Printf("Quests: %q: teleport refused: %v", qu.Name, err)
		return
	}
	facing, counter, changed, ok := q.gw.BeginWarp(p.ServerID, world)
	if !ok {
		return
	}
	sendTeleportWarp(sess, world, e.X, e.Y, e.Z, facing, counter)
	beginWorldLoad(sess, changed)
	q.gw.UpdatePosition(p.ServerID, e.X, e.Y, e.Z, facing)
	log.Printf("Quests: %q teleported %s to (%.1f, %.1f, %.1f) world=%d", qu.Name, p.Name, e.X, e.Y, e.Z, world)
}

// stepStartedLocked is a step become the step to do: its start events.
func (q *QuestEngine) stepStartedLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) {
	q.stepEventsLocked(sess, p, c, qu, st, database.EventStart)
}

// lootKillLocked is a kill on a loot step: its item dropped for the player
// with the step's chance while it has taken fewer than the step wants.
func (q *QuestEngine) lootKillLocked(k entity.KillCredit, kp entity.KillPlayer, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) {
	pid, need, ok := st.LootItem()
	if !ok || q.drop == nil || k.NPCID == 0 || cq.Count >= need {
		return
	}
	if q.roll() >= database.LootChance(st.Chance) {
		return
	}
	pat, err := q.items.GetItemPattern(pid)
	if err != nil || pat == nil {
		log.Printf("Quests: %q step %d's item %d: %v", qu.Name, st.Step, pid, err)
		return
	}
	q.drop(k.NPCID, k.Died, kp.ServerID, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat})
	log.Printf("Quests: %q dropped %q for %s (step %d of %q)", k.NPCName, pat.ItemName, kp.Name, st.Step, qu.Name)
}

// Looted is n of an item a player took off a corpse: counted on a loot step
// of that item, which is done at its count, else the count is said.
func (q *QuestEngine) Looted(sess *session.Session, p *entity.Player, pattern, n int) {
	if q == nil || p == nil || n < 1 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil || st.Kind != database.StepLoot {
			continue
		}
		pid, need, ok := st.LootItem()
		if !ok || pid != pattern {
			continue
		}
		cq.Count += n
		if cq.Count < need {
			q.saveLocked(cq)
			if sess != nil {
				name := fmt.Sprintf("item %d", pid)
				if pat, err := q.items.GetItemPattern(pid); err == nil && pat != nil {
					name = pat.ItemName
				}
				SendClientMessage(sess, fmt.Sprintf("%s: %d of %d.", name, cq.Count, need))
			}
			continue
		}
		q.completeStepLocked(sess, p, c, qu, cq, st, nil, nil)
	}
}

// AtWindow is a merchant's, banker's or blacksmith's window asked for: CLIENT
// never talks to such an NPC (InteractWithTarget asks for its window
// instead), so a talk, show or give step to do at it is done here, first, when
// it can be done now (a show or give step's items and tunar held); else the
// window opens as ever. A quest it gives is offered first too, as a talk offers
// one (the user's "have shop NPCs offer the quest first"), once a session a
// quest: declined or left unanswered, the next request opens the window. It
// reports whether the quests answered (the window is not opened then).
func (q *QuestEngine) AtWindow(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	look := q.lookAt(sess)
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil || !database.AtNPC(st.Kind) || !stepNPC(st, npc.Name, npc.ServerID) {
			continue
		}
		if (st.Kind == database.StepGive || st.Kind == database.StepShow) && q.lackingLocked(p, st) != "" {
			continue
		}
		// a giver the player's standing keeps from its steps: as items that
		// lack, the window opens as ever
		if giverWontDeal(look, qu, st) != "" {
			continue
		}
		c.box = nil
		q.atNPCLocked(sess, p, c, qu, cq, st, false, -1)
		log.Printf("Quests: %s's window answered for %q step %d", npc.Name, qu.Name, st.Step)
		return true
	}
	for _, qu := range q.cat.givers[foldName(npc.Name)] {
		if c.windowOffered[qu.ID] || q.mayTakeLocked(look, p, c, qu) != "" {
			continue
		}
		if c.windowOffered == nil {
			c.windowOffered = make(map[int]bool)
		}
		c.windowOffered[qu.ID] = true
		c.box = nil
		q.offerLocked(sess, p, c, qu)
		log.Printf("Quests: %s's window offered %q first", npc.Name, qu.Name)
		return true
	}
	return false
}

// questsAtWindow is AtWindow on the running engine, the client's pending
// request cleared first (0x00D7) so its boxes show: true when the quests
// answered and the window is not to open.
func questsAtWindow(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	q := Quests()
	if q == nil || !q.AtWindow(sess, p, npc) {
		return false
	}
	refuse(sess)
	return true
}
