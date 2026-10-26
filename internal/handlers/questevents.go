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
// of its creature credited to a player on the step, while the player holds
// fewer than the step wants, drops one with the step's chance onto that
// player's own view of the corpse (questloot.go); the step is done when the
// player takes the last one it needs, or at once when it starts with them
// held.

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

// stepStartedLocked is a step become the step to do: its start events, then
// a loot step whose item is held already is done.
func (q *QuestEngine) stepStartedLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) {
	q.stepEventsLocked(sess, p, c, qu, st, database.EventStart)
	if st.Kind != database.StepLoot || p == nil {
		return
	}
	if pid, need, ok := st.LootItem(); ok && q.heldOf(p.CharServerID, pid) >= need {
		q.completeStepLocked(sess, p, c, qu, cq, st, nil)
	}
}

// heldOf is how many of an item the character holds unworn in its inventory.
func (q *QuestEngine) heldOf(serverID uint32, pattern int) int {
	if q.inv == nil {
		return 0
	}
	rows, err := q.inv.GetCharacterItems(serverID)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if r.Location == 0 && r.EquipLoc < 0 && r.Pattern.PatternID == pattern {
			n += r.StackLeft
		}
	}
	return n
}

// lootKillLocked is a kill on a loot step: its item dropped for the player
// with the step's chance while it holds fewer than the step wants.
func (q *QuestEngine) lootKillLocked(k entity.KillCredit, kp entity.KillPlayer, qu *database.Quest, st *database.QuestStep) {
	pid, need, ok := st.LootItem()
	if !ok || q.drop == nil || k.NPCID == 0 {
		return
	}
	if q.heldOf(kp.ServerID, pid) >= need {
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

// Looted is an item a player took off a corpse: a loot step of that item is
// done once the step's count of it is held, else the count is said.
func (q *QuestEngine) Looted(sess *session.Session, p *entity.Player, pattern int) {
	if q == nil || p == nil {
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
		have := q.heldOf(p.CharServerID, pid)
		if have < need {
			if sess != nil {
				name := fmt.Sprintf("item %d", pid)
				if pat, err := q.items.GetItemPattern(pid); err == nil && pat != nil {
					name = pat.ItemName
				}
				SendClientMessage(sess, fmt.Sprintf("%s: %d of %d.", name, have, need))
			}
			continue
		}
		q.completeStepLocked(sess, p, c, qu, cq, st, nil)
	}
}
