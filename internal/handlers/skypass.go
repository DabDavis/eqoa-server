package handlers

import (
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Plane of Sky's passages (petition 1608, the GM Jibrish, at the second
// isle of rain's essence: "This needs to ask for a key from the boss and then
// teleport people to the next island"; petition 1612, at Granavik: "after
// granavik dies he needs to drop 20 keys, and open a portal to the next island
// called Rain Key 3"). Each isle's essence takes one key of its isle and sends
// the player on to the next isle, the group standing by it along (moveGroup,
// as the Teleporter's trips were until petition 1938): one key, one trip for the giver and the
// group. The keys of the main isle's three essences are the ones Liztin
// Misseliania sells (rain key, sun key, key of the ancients); every other
// isle's is its boss's, twenty on its corpse (tools/sky_keys_20261111.py, the
// set sky-keys: the isle's boss is the one whose shard the guide's chain has
// it drop, Granavik for the third isle of rain as Jibrish named him, the Arch
// Bishop Lancel for the third of night, which has none). The order of the
// isles is EQOA-Data's Portal/plane-of-sky guide's (Rain 1-5, Day 1-4, Night
// 1-5, Snow 2-4 after Snow 1's jump, Desolation 1-4), each last isle's essence
// to the Isle of Wonder. A landing was measured over the ground and walls
// files of world 4 and its creatures (2026-11-11): on the isle's own open
// ground, no creature within 40 (DefaultAggroRadius is 30), out of the water,
// under the sky; where the guide says where one comes in, there ("Talk to
// Trixie Longwings", the aviaks' lower hut, the far side of the fourth isle of
// rain from its portal, "Chimareon spawns far ahead of where you ported in",
// Night 5's ledge above King Ruthra's pyramid), else the spot nearest the old
// quest trip's or farthest from the isle's own way out.
//
// The chains' quest steps at an essence (the steps the passage lists) are done
// by the passage alone, with the key: the quests' own trips were taken off
// them (the same set), so they no longer send a player on without one. A
// quest step at an essence the passages do not list (the snow chain's way
// back to the igloo village) goes as its quest has it. A player without the
// key is offered the essence's other trips (waysender.go: a finished chain's
// way back to its first isle), else told what it wants.

// skyPass is one essence's passage.
type skyPass struct {
	// NPC is its name; X, Z its spawn (a passage of a name with several of
	// it: the essence within skyPassReach of the spot); 0, 0 any of the name
	NPC  string
	X, Z float32
	// Key is the item it takes, by name (records' ids differ by shard)
	Key string
	// To is the place, At where it lands
	To string
	At wayPlace
	// Steps are the quest steps (quest, step) it does; StepsAs the NPC
	// name they are at (the portal does the essence's), "" its own
	Steps   [][2]int
	StepsAs string
}

// skyPassReach is how near its spot an essence of a name stands.
const skyPassReach = 20

// skyIsleOfWonder is where each chain's last essence lands: where the GM stood
// for the Teleporter's Plane of Sky (petition 1605).
var skyIsleOfWonder = wayPlace{World: 4, X: 5000.09, Y: 333.54, Z: 4998.23, Facing: 3.068}

var skyPasses = []skyPass{
	// the chain of rain: the main isle's essence, over the bridge north-west
	{NPC: "essence of rain", X: 4482.5, Z: 5293.2, Key: "rain key", To: "the first isle of rain",
		At: wayPlace{World: 4, X: 4236.2, Y: 233.05, Z: 5727.7, Facing: 3.38}, Steps: [][2]int{{101301, 2}}},
	// lands where Hoofbite stood (petition 2049, the owner: "you can do what they say")
	{NPC: "essence of rain", X: 4222.7, Z: 5714.6, Key: "Rain Key 1", To: "the second isle of rain",
		At: wayPlace{World: 4, X: 5643.74, Y: 326.63, Z: 4903.25, Facing: 3.068}, Steps: [][2]int{{101301, 4}}},
	// lands where Hoofbite stood (petition 2053, the owner: "you can do what they say"; was Kobe's 2042 spot)
	{NPC: "essence of rain", X: 4158.6, Z: 5122.5, Key: "Rain Key 2", To: "the third isle of rain",
		At: wayPlace{World: 4, X: 5731.73, Y: 404.03, Z: 4207.61, Facing: 5.007}, Steps: [][2]int{{101302, 4}}},
	{NPC: "essence of rain", X: 4638.6, Z: 4655.2, Key: "Rain Key 3", To: "the fourth isle of rain",
		At: wayPlace{World: 4, X: 4242.0, Y: 80.06, Z: 4305.3, Facing: 3.37}, Steps: [][2]int{{101303, 5}}},
	// Granavik's portal, standing where he fell (petition 1612)
	{NPC: "Rain 3 Portal", Key: "Rain Key 3", To: "the fourth isle of rain",
		At: wayPlace{World: 4, X: 4242.0, Y: 80.06, Z: 4305.3, Facing: 3.37}, Steps: [][2]int{{101303, 5}}, StepsAs: "essence of rain"},
	{NPC: "essence of rain", X: 4199.7, Z: 4148.3, Key: "Rain Key 4", To: "the fifth isle of rain",
		At: wayPlace{World: 4, X: 4869.0, Y: 26.75, Z: 4316.0, Facing: 4.67}, Steps: [][2]int{{101303, 9}}},
	{NPC: "essence of rain", X: 4516.5, Z: 4271.6, Key: "Rain Key 5", To: "the Isle of Wonder",
		At: skyIsleOfWonder, Steps: [][2]int{{101303, 11}}},

	// the chain of the sun
	// lands where Thorinoak stood, by the first isle's essence and Madam Watrya (petition 2048)
	{NPC: "essence of sun", X: 5137.3, Z: 5188.3, Key: "sun key", To: "the first isle of day",
		At: wayPlace{World: 4, X: 5773.76, Y: 280.0, Z: 5114.38, Facing: 4.639}, Steps: [][2]int{{101304, 2}}},
	{NPC: "essence of day", X: 5597.3, Z: 5343.1, Key: "Day Key 1", To: "the second isle of day",
		At: wayPlace{World: 4, X: 5608.0, Y: 374.06, Z: 4715.0, Facing: 3.66}, Steps: [][2]int{{101304, 4}}},
	{NPC: "essence of day", X: 5380.8, Z: 4755.9, Key: "Day Key 2", To: "the third isle of day",
		At: wayPlace{World: 4, X: 5463.3, Y: 380.19, Z: 4231.8, Facing: 1.76}, Steps: [][2]int{{101305, 4}}},
	// lands where Hoofbite stood (petition 2054)
	{NPC: "essence of day", X: 5459.6, Z: 4237.5, Key: "Day Key 3", To: "the fourth isle of day",
		At: wayPlace{World: 4, X: 5215.63, Y: 430.88, Z: 4216.92, Facing: 4.786}, Steps: [][2]int{{101305, 7}}},
	{NPC: "essence of day", X: 5054.4, Z: 4243.1, Key: "Day Key 4", To: "the Isle of Wonder",
		At: skyIsleOfWonder, Steps: [][2]int{{101305, 10}}},

	// the chain of night: the main isle's essence, south of Liztin's hut
	// lands where Hoofbite stood (petition 2055)
	{NPC: "essence of night", X: 4621.4, Z: 5729.6, Key: "key of the ancients", To: "the first isle of night",
		At: wayPlace{World: 4, X: 4160.69, Y: 612.38, Z: 6064.46, Facing: 0.025}, Steps: [][2]int{{101306, 2}}},
	{NPC: "essence of night", X: 4160.7, Z: 6404.1, Key: "Night Key 1", To: "the second isle of night",
		At: wayPlace{World: 4, X: 4425.0, Y: 448.75, Z: 6453.0, Facing: 2.98}, Steps: [][2]int{{101306, 8}}},
	{NPC: "essence of night", X: 4441.2, Z: 6436.7, Key: "Night Key 2", To: "the third isle of night",
		At: wayPlace{World: 4, X: 4122.0, Y: 569.38, Z: 6538.0, Facing: 0.89}, Steps: [][2]int{{101306, 10}}},
	{NPC: "essence of night", X: 4250.0, Z: 6806.4, Key: "Night Key 3", To: "the fourth isle of night",
		At: wayPlace{World: 4, X: 4122.6, Y: 353.12, Z: 7574.4, Facing: 0.84}},
	{NPC: "essence of night", X: 4322.6, Z: 7754.4, Key: "Night Key 4", To: "the fifth isle of night",
		At: wayPlace{World: 4, X: 4160.0, Y: 208.25, Z: 6980.0, Facing: 0.97}, Steps: [][2]int{{101306, 15}}},
	{NPC: "essence of night", X: 4400.0, Z: 7135.8, Key: "Night Key 5", To: "the Isle of Wonder",
		At: skyIsleOfWonder, Steps: [][2]int{{101306, 17}}},

	// the chain of snow, from the igloo village below the first isle's
	// mountain (the Mark of the Majestic's trip to it: waysender.go)
	{NPC: "essence of snow", X: 5766.8, Z: 6702.1, Key: "Snow Key 2", To: "the third isle of snow",
		At: wayPlace{World: 4, X: 5832.8, Y: 176.62, Z: 7079.7, Facing: 0.07}, Steps: [][2]int{{101307, 8}}},
	{NPC: "essence of snow", X: 5906.3, Z: 7570.5, Key: "Snow Key 3", To: "the fourth isle of snow",
		At: wayPlace{World: 4, X: 5527.8, Y: 95.00, Z: 7704.7, Facing: 3.69}, Steps: [][2]int{{101307, 11}}},
	{NPC: "essence of snow", X: 5525.1, Z: 7708.0, Key: "Snow Key 4", To: "the Isle of Wonder",
		At: skyIsleOfWonder, Steps: [][2]int{{101307, 17}}},

	// the chain of desolation (the Mark of Desolation's trip to its first
	// isle: waysender.go)
	{NPC: "essence of desolation", X: 4751.5, Z: 6418.5, Key: "Desolation Key 1", To: "the second isle of desolation",
		At: wayPlace{World: 4, X: 5127.7, Y: 615.75, Z: 6625.2, Facing: 4.77}, Steps: [][2]int{{101308, 4}}},
	{NPC: "essence of desolation", X: 5146.8, Z: 6620.1, Key: "Desolation Key 2", To: "the third isle of desolation",
		At: wayPlace{World: 4, X: 5199.3, Y: 709.38, Z: 6897.8, Facing: 6.12}, Steps: [][2]int{{101308, 9}}},
	{NPC: "essence of desolation", X: 5116.0, Z: 7174.8, Key: "Desolation Key 3", To: "the fourth isle of desolation",
		At: wayPlace{World: 4, X: 4798.0, Y: 858.7, Z: 7616.6, Facing: 3.77}, Steps: [][2]int{{101308, 12}}},
	{NPC: "essence of desolation", X: 4734.2, Z: 7688.3, Key: "Desolation Key 4", To: "the Isle of Wonder",
		At: skyIsleOfWonder, Steps: [][2]int{{101308, 14}}},
}

// The essence's words.
const (
	skyPassGo     = "%s: your %s turns in the light, and it takes you to %s."
	skyPassWants  = "The %s wants %s before it will take you on."
	skyPassDim    = "The %s is dim: it takes no one from here."
	skyGroupWords = "%s takes the group to %s."
)

// skyPassOf is the passage of that NPC, and whether its name is a passage's
// at all (an essence of the name with none of its own is dim).
func skyPassOf(npc *entity.NPC) (*skyPass, bool) {
	named := false
	for i := range skyPasses {
		p := &skyPasses[i]
		if !strings.EqualFold(strings.TrimSpace(npc.Name), p.NPC) {
			continue
		}
		named = true
		if p.X == 0 && p.Z == 0 || math.Hypot(float64(npc.SpawnX-p.X), float64(npc.SpawnZ-p.Z)) <= skyPassReach {
			return p, true
		}
	}
	return nil, named
}

// skyPassSteps is every passage's quest steps, which only a passage does.
func skyPassSteps() map[[2]int]bool {
	out := map[[2]int]bool{}
	for _, p := range skyPasses {
		for _, s := range p.Steps {
			out[s] = true
		}
	}
	return out
}

// withArticle is a thing with its article: "a Rain Key 2", "an essence".
func withArticle(s string) string {
	if s == "" {
		return s
	}
	switch strings.ToLower(s[:1]) {
	case "a", "e", "i", "o", "u":
		return "an " + s
	}
	return "a " + s
}

// skyPassTalk is HandleInteract's question for a Plane of Sky essence, before
// the quests: its passage with the key held, else its other trips, else what
// it wants. false for any other NPC, and for a player whose quest step at it
// is one the passages do not do (the quest's own, then).
func (h *InteractHandler) skyPassTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || h.gameWorld == nil {
		return false
	}
	pass, named := skyPassOf(npc)
	if !named {
		return false
	}
	q := Quests()
	if q.StepHereBesides(p, npc.Name, npc.ServerID, skyPassSteps()) {
		return false
	}
	if pass != nil && q.HeldNamed(p, pass.Key) > 0 {
		h.skyPass(sess, p, npc, pass)
		return true
	}
	forgetDialogs(p)
	// its trips the player may take (a finished chain's way back), and none
	// it may not: those are not this essence's word
	if dests, _, _ := wayDests(h.gameWorld, sess, npc.Name, p); len(dests) > 0 && h.offerWay(sess, p, npc) {
		return true
	}
	// one with no passage of its own: its trips' word (a trip a quest keeps
	// the player from), else dim
	if pass == nil && IsWaysender(npc.Name) && h.offerWay(sess, p, npc) {
		return true
	}
	if pass != nil {
		h.sendDialogueBox(sess, fmt.Sprintf(skyPassWants, npc.Name, withArticle(pass.Key)))
		log.Printf("SkyPass: %s has no %s for %s's passage to %s", p.Name, pass.Key, npc.Name, pass.To)
	} else {
		h.sendDialogueBox(sess, fmt.Sprintf(skyPassDim, npc.Name))
	}
	return true
}

// skyPass takes the player's key and sends it, and its group by the essence,
// on: the passage's quest steps first (a step that gives the key takes it),
// then the key if none did.
func (h *InteractHandler) skyPass(sess *session.Session, p *entity.Player, npc *entity.NPC, pass *skyPass) {
	forgetDialogs(p)
	dest := entity.CoachDest{Name: pass.To, X: pass.At.X, Y: pass.At.Y, Z: pass.At.Z, Facing: pass.At.Facing, World: pass.At.World}
	if !CanLoadWorld(sess, dest.World) {
		return
	}
	if _, err := ValidateTeleport(dest.X, dest.Y, dest.Z, dest.World); err != nil {
		log.Printf("SkyPass: %s's passage to %s refused: %v", npc.Name, pass.To, err)
		return
	}
	q := Quests()
	stepsAs := npc.Name
	if pass.StepsAs != "" {
		stepsAs = pass.StepsAs
	}
	held := q.HeldNamed(p, pass.Key)
	q.PassSteps(sess, p, stepsAs, npc.ServerID, pass.Steps)
	if q.HeldNamed(p, pass.Key) >= held && !q.TakeNamed(sess, p, pass.Key) {
		h.sendDialogueBox(sess, fmt.Sprintf(skyPassWants, npc.Name, withArticle(pass.Key)))
		return
	}
	SendClientMessage(sess, fmt.Sprintf(skyPassGo, npc.Name, pass.Key, pass.To))
	log.Printf("SkyPass: %s gave %s a %s: to %s", p.Name, npc.Name, pass.Key, pass.To)
	h.warp(sess, p, dest)
	h.moveGroup(p, npc.ServerID, dest, fmt.Sprintf(skyGroupWords, npc.Name, pass.To), "SkyPass",
		func(ms *session.Session, m *entity.Player) {
			// a member's own steps of the passage are done too (its own key
			// taken by a step that gives one)
			q.PassSteps(ms, m, stepsAs, npc.ServerID, pass.Steps)
		})
}

// --- the quests' side ---

// StepHereBesides is whether the player has a quest step at the NPC called
// name (spawn, its spawn) that is not one of those.
func (q *QuestEngine) StepHereBesides(p *entity.Player, name string, spawn entity.EntityID, those map[[2]int]bool) bool {
	if q == nil || p == nil {
		return false
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
		if st == nil || !database.AtNPC(st.Kind) || !stepNPC(st, name, spawn) {
			continue
		}
		if !those[[2]int{qu.ID, st.Step}] {
			return true
		}
	}
	return false
}

// PassSteps does each of the player's quest steps among those at the NPC
// called name that can be done now (a give step's items held), its first
// answer taken (the passage asks nothing). It reports how many it did.
func (q *QuestEngine) PassSteps(sess *session.Session, p *entity.Player, name string, spawn entity.EntityID, those [][2]int) int {
	if q == nil || p == nil || len(those) == 0 {
		return 0
	}
	want := map[[2]int]bool{}
	for _, s := range those {
		want[s] = true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	done := 0
	tried := map[[2]int]bool{}
	for {
		var qu *database.Quest
		var cq *database.CharQuest
		var st *database.QuestStep
		for _, a := range c.active() {
			b := q.cat.byID[a.QuestID]
			if b == nil {
				continue
			}
			s := b.Step(a.Step)
			k := [2]int{b.ID, a.Step}
			if s == nil || !want[k] || tried[k] || !database.AtNPC(s.Kind) || !stepNPC(s, name, spawn) {
				continue
			}
			qu, cq, st = b, a, s
			break
		}
		if st == nil {
			return done
		}
		tried[[2]int{qu.ID, st.Step}] = true
		if giverWontDeal(look, qu, st) != "" {
			continue
		}
		if (st.Kind == database.StepGive || st.Kind == database.StepShow) && q.lackingLocked(p, st) != "" {
			continue
		}
		pick := -1
		if len(st.Choices.Options) > 0 {
			pick = 0
		}
		q.atNPCLocked(sess, p, c, qu, cq, st, true, pick)
		done++
	}
}

// HeldNamed is how many of the item of that name (any case) the player
// carries in its pack.
func (q *QuestEngine) HeldNamed(p *entity.Player, name string) int {
	if q == nil || p == nil {
		return 0
	}
	n := 0
	for _, r := range q.heldItems(p) {
		if strings.EqualFold(strings.TrimSpace(r.Pattern.ItemName), strings.TrimSpace(name)) {
			n += r.StackLeft
		}
	}
	return n
}

// TakeNamed takes one of the item of that name from the player's pack, in
// the database and the client's list (as a give step takes it); false when
// it holds none.
func (q *QuestEngine) TakeNamed(sess *session.Session, p *entity.Player, name string) bool {
	if q == nil || p == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, r := range q.heldItems(p) {
		if strings.EqualFold(strings.TrimSpace(r.Pattern.ItemName), strings.TrimSpace(name)) {
			st := &database.QuestStep{Kind: database.StepGive, Items: []database.ItemCount{{PatternID: r.Pattern.PatternID, Count: 1}}}
			_, ok := q.takeLocked(sess, p, st)
			return ok
		}
	}
	return false
}
