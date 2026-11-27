package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// heroicraids.go — the Heroic Raids NPC (work board 2429, a GM's request approved as submitted: "publish Mythic
// Nagafen's Island through the northwest Heroic Raids NPC"): one board by Tunaria's north-west mountains, north of
// Permafrost, that sends a party into the hidden dungeons it names (entity.DungeonDef.Via "Heroic Raids": today
// Mythic Lord Nagafen alone, entity/mythicnagafen.go). Ordinary players use it; the party's leader (or a player
// alone) opens its instance and members near come along, others enter theirs later from the same board, as the
// Dungeon Warden's door does (dungeons.go: the same DungeonEnterVia, the same takeGroup, the way out by the warden
// standing in the instance, a login back into an open instance).
//
// Nothing else names it: the wardens', the Heroic Dungeons board's and /dungeon's menus and help leave hidden
// dungeons out, /dungeon enter and reset by its key answer as though there were none, a summon into its instance
// and a Scroll of Returning kept in it are refused, and the public map has no mark of it (a GM's mapmarks row would
// be the only way: none is written). A talk is answered only within reach of the board (InteractRange), and an
// answer only within heroicRaidsReach of it still (a dialogue left open and answered from afar does nothing).
// Questions under box mark 0x6C000000.

// heroicRaidsBoxMark marks the board's questions' ids (boxmarks_test.go keeps every feature's mark its own).
const heroicRaidsBoxMark = 0x6C000000

// HeroicRaidsName is the board's name (the data tool's row, a copy of the Heroic Dungeons board's body).
const HeroicRaidsName = "Heroic Raids"

// heroicRaidsReach is how near the board an answer is taken.
const heroicRaidsReach float32 = 15

// The board's words (ours).
const (
	heroicRaidsAsk    = "Raids of legend for level 60 heroes. Each party gets its own copy. Where to?"
	heroicRaidsClosed = "No raid stands open beyond these mountains now."
	heroicRaidsNoOpen = "No raid of %s is open for your party, and only your group's leader may open one."
	heroicRaidsTooFar = "Come back to the Heroic Raids board to answer."
)

type heroicRaidsAsked struct {
	box  uint32
	keys []string
	npc  entity.EntityID
}

// heroicRaids is the board's questions asked.
var heroicRaids = struct {
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]heroicRaidsAsked
}{asked: map[uint32]heroicRaidsAsked{}}

// isHeroicRaids reports an NPC the Heroic Raids board.
func isHeroicRaids(npc *entity.NPC) bool {
	return npc != nil && strings.EqualFold(strings.TrimSpace(npc.Name), HeroicRaidsName)
}

// heroicRaidsNear reports p within reach of the board npc in its world.
func heroicRaidsNear(gw *entity.World, p *entity.Player, npc entity.EntityID, reach float32) bool {
	w, x, _, z, here := gw.NPCSpot(npc)
	pw, px, _, pz, dead, ok := gw.PlayerSpot(p)
	return here && ok && !dead && w == pw && (x-px)*(x-px)+(z-pz)*(z-pz) <= reach*reach
}

// heroicRaidsTalk is a talk to the Heroic Raids board: its question, else false for any other NPC.
func (h *InteractHandler) heroicRaidsTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if !isHeroicRaids(npc) || npc.Dead {
		return false
	}
	forgetDialogs(p)
	gw := h.gameWorld
	as := HeroicRaidsName + ": "
	if gw == nil || dungeonsH.Load() == nil {
		dungeonsSay(sess, as, heroicRaidsClosed)
		return true
	}
	if !heroicRaidsNear(gw, p, npc.ServerID, entity.InteractRange) {
		SendColoredChat(sess, talkOutOfReachLine, 255, 63, 63)
		return true
	}
	if p.Reliving() {
		dungeonsSay(sess, as, WardenReliveLine) // relive.go
		return true
	}
	choices := gw.DungeonViaChoices(sess, HeroicRaidsName)
	var labels, keys []string
	for _, c := range choices {
		if !c.Open && !c.MayOpen {
			continue
		}
		labels = append(labels, c.Name)
		keys = append(keys, c.Key)
	}
	switch {
	case len(choices) == 0:
		dungeonsSay(sess, as, heroicRaidsClosed)
		return true
	case len(keys) == 0:
		dungeonsSay(sess, as, fmt.Sprintf(heroicRaidsNoOpen, choices[0].Name))
		return true
	}
	heroicRaids.mu.Lock()
	heroicRaids.seq++
	id := uint32(heroicRaidsBoxMark) | (heroicRaids.seq & 0xFFFFFF)
	heroicRaids.asked[p.CharServerID] = heroicRaidsAsked{box: id, keys: keys, npc: npc.ServerID}
	heroicRaids.mu.Unlock()
	for i := range labels {
		labels[i] = clipRunes(labels[i], questAnswerChars)
	}
	SendQuestPopupMulti(sess, id, clipRunes(as+heroicRaidsAsk, questAskChars), labels)
	log.Printf("Heroic Raids: %s asks at the board (%s)", p.Name, strings.Join(keys, ", "))
	return true
}

// answerHeroicRaids is HandleDialogueOption's question for the board's box. It reports whether the box was its.
func (h *InteractHandler) answerHeroicRaids(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != heroicRaidsBoxMark {
		return false
	}
	heroicRaids.mu.Lock()
	asked, ok := heroicRaids.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(heroicRaids.asked, p.CharServerID)
	}
	heroicRaids.mu.Unlock()
	ds := dungeonsH.Load()
	gw := h.gameWorld
	if option == 255 || !ok || asked.box != box || int(option) >= len(asked.keys) || gw == nil || ds == nil {
		return true
	}
	if !heroicRaidsNear(gw, p, asked.npc, heroicRaidsReach) {
		dungeonsTell(sess, heroicRaidsTooFar)
		log.Printf("Heroic Raids: %s answered from afar: refused", p.Name)
		return true
	}
	ds.enter(sess, p, asked.keys[option], HeroicRaidsName) // its world held, the party's instance (dungeons.go)
	return true
}

// mythicValveTalk is a talk to one of a fight's valves (entity/mythicnagafen.go): its turn. False for any other NPC.
func (h *InteractHandler) mythicValveTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	gw := h.gameWorld
	if gw == nil || npc == nil || npc.Instance == 0 ||
		!strings.EqualFold(npc.Name, entity.MythicEastValveName) && !strings.EqualFold(npc.Name, entity.MythicWestValveName) {
		return false
	}
	line, ok := gw.MythicTurnValve(p, npc.ServerID, time.Now())
	if !ok {
		return false
	}
	if line != "" {
		dungeonsTell(sess, line)
	}
	return true
}
