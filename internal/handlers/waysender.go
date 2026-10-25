package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A waysender sends a player to another town for a fee (F3 20261025-173746:
// Waysender Brindel's trip to Lavastorm, for level 55 and up). The routes are
// the ones EQOA-Data's quest guides (github.com/Jadiction/EQOA-Data, Quests/)
// document, each landing just in front of the NPC the guides meet there: the
// other town's waysender, else its coachman, else (Lavastorm) the outpost a
// level-53 guide walks to. Fees are the guides' where they give one (Neriak to
// Freeport and back 100 tunar, Neriak to Grobb 225), else none. A waysender
// with no route here answers nothing, as any NPC with nothing to say.

// wayRoute is one of a waysender's trips.
type wayRoute struct {
	// To is the place, as the menu lists it.
	To string
	// Arrive is the NPC whose spot the player lands on.
	Arrive string
	// Fee is its tunar, MinLevel the level it needs (0: none).
	Fee, MinLevel int
}

// wayRoutes are each waysender's trips, by name; the guides' words beside.
var wayRoutes = map[string][]wayRoute{
	// "Talk to Waysender T'Thox, who is outside of the Blind Fish Tavern (you
	// will need 100 tunar to get to Freeport ...)" (neriak-nektulos/enchanter);
	// "Waysender Thi'thox ports you to Grobb" (neriak-nektulos/necromancer);
	// "To return to Grobb, pay 225 tunar to the Dark Elf Waysender"
	// (grobb/warrior)
	"Waysender Ti`Thox": {
		{To: "Freeport", Arrive: "Waysender Abodai", Fee: 100},
		{To: "Grobb", Arrive: "Waysender Zapgit", Fee: 225},
	},
	// "hire Waysender Abodai to teleport you back to Neriak" (... and another
	// 100 tunar to hire the waysender to get back)
	"Waysender Abodai": {
		{To: "Neriak", Arrive: "Waysender Ti`Thox", Fee: 100},
	},
	// "sends you to Scornblade D'Eleth in Neriak via Waysender Zapit";
	// "Waysender Zapgit for a trip to Honjour" (grobb/shadowknight, shaman)
	"Waysender Zapgit": {
		{To: "Neriak", Arrive: "Waysender Ti`Thox"},
		{To: "Honjour", Arrive: "Coachman Norial"},
	},
	// "Waysender Glowbap who will teleport you to Rivervale" (klick-anon/wizard)
	"Waysender Glowbap": {
		{To: "Rivervale", Arrive: "Waysender Windfoot"},
	},
	// "Waysender Windfoot to teleport back to Klick'Anon" (klick-anon/wizard)
	"Waysender Windfoot": {
		{To: "Klik'Anon", Arrive: "Waysender Glowbap"},
	},
	// "speak with Waysender Carowyn in Fayspires ... Carowyn sends you to
	// Moradhim (sign the coach)" (klick-anon/magician)
	"Waysender Carowyn": {
		{To: "Moradhim", Arrive: "Coachman Doba"},
	},
	// "Go to Waysender Naelarie, near the well. Once in Surefall Glade, sign
	// the coach"; "To get to Rivervale, see Waysender Naelarie in Qeynos"
	// (qeynos/bard)
	"Waysender Naelarie": {
		{To: "Surefall Glade", Arrive: "Coachman Frender"},
		{To: "Rivervale", Arrive: "Waysender Windfoot"},
	},
	// "coach to Klick'Anon, from coach, run out the west gate, follow
	// mountains to the west. Soon you will see a shelter of sorts; there is a
	// waysender there, talk to him. Minastickie is in a hut to your right"
	// (53-59/53); for level 55 and up (F3 20261025-173746). Lavastorm is
	// world 3, the outpost its gnomes' camp.
	"Waysender Brindel": {
		{To: "Lavastorm", Arrive: "Explorer Springa", MinLevel: 55},
	},
}

// The waysender's words. A trip is asked as Sandstorm's dockmasters ask one
// (pcaps/sandstorm-warp.pcap: "Where do you wish to go?", then "Very
// well."); one place alone as Yes or No.
const (
	wayAskMany = "Where do you wish to go?"
	wayGo      = "Very well."
)

// IsWaysender is whether the NPC of that name is a waysender.
func IsWaysender(name string) bool {
	return strings.HasPrefix(name, "Waysender ")
}

// wayAskOne is the question for a waysender's one trip.
func wayAskOne(d entity.CoachDest) string {
	if d.Fee > 0 {
		return fmt.Sprintf("Do you wish to travel to %s for %d tunar?", d.Name, d.Fee)
	}
	return fmt.Sprintf("Do you wish to travel to %s?", d.Name)
}

// tooWeak is a waysender's answer to a player below every trip's level.
func tooWeak(place string) string {
	return fmt.Sprintf("You are too weak to travel to %s. Come back when you are stronger.", place)
}

// wayDests are the waysender's trips this session can take: each landing on
// its NPC's spot, in a world the game loads; the ones its level allows, and
// the first place a level kept it from (or "" if none did).
func wayDests(gw *entity.World, sess *session.Session, waysender string, level int) (dests []entity.CoachDest, barred string) {
	for _, r := range wayRoutes[waysender] {
		x, y, z, facing, world, ok := gw.SpawnOfNPC(r.Arrive)
		if !ok {
			log.Printf("Waysender: %s's trip to %s lands on %s, who is not here", waysender, r.To, r.Arrive)
			continue
		}
		if !CanLoadWorld(sess, world) {
			continue
		}
		if r.MinLevel > 0 && level < r.MinLevel {
			if barred == "" {
				barred = r.To
			}
			continue
		}
		// in front of them, not inside them: a body is solid
		fx, fz, face := inFrontOf(x, z, facing)
		dests = append(dests, entity.CoachDest{Name: r.To, X: fx, Y: y, Z: fz, Facing: face,
			World: world, Fee: r.Fee, MinLevel: r.MinLevel, Via: waysender})
	}
	return dests, barred
}

// destLabel is a trip as a menu of several lists it: the place, and its fee.
func destLabel(d entity.CoachDest) string {
	if d.Fee > 0 {
		return fmt.Sprintf("%s (%d tunar)", d.Name, d.Fee)
	}
	return d.Name
}

// offerWay opens a waysender's trips, or says the player is too weak for
// them; it reports whether the waysender had anything to say.
func (h *InteractHandler) offerWay(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	dests, barred := wayDests(h.gameWorld, sess, npc.Name, player.Level)
	switch {
	case len(dests) == 0 && barred != "":
		forgetDialogs(player)
		h.sendDialogueBox(sess, tooWeak(barred))
		log.Printf("Waysender: %s is too weak (%d) for %s's trip to %s", player.Name, player.Level, npc.Name, barred)
		return true
	case len(dests) == 0:
		return false
	}
	forgetDialogs(player)
	player.PendingCoachDests = dests
	if len(dests) == 1 {
		player.PendingYesNo = true
		h.sendOptionBox(sess, wayAskOne(dests[0]), []string{"Yes", "No"})
	} else {
		h.sendCoachPage(sess, player)
	}
	log.Printf("Waysender: %s offers %s %d trips", npc.Name, player.Name, len(dests))
	return true
}

// forgetDialogs closes whatever box an NPC had open for the player: a new one
// replaces it, and a pick must answer the box on the screen.
func forgetDialogs(player *entity.Player) {
	player.PendingCoachDests = nil
	player.CoachPage = 0
	player.PendingYesNo = false
	player.PendingBind = entity.Bind{}
	player.PendingSpar = entity.SparPick{}
	if player.PendingHDDMenu != HDDMenuNone {
		player.PendingHDDMenu = HDDMenuNone
		clearFormatArm(player.ServerID)
	}
}
