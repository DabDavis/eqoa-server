package handlers

import (
	"sort"
	"strings"
	"unicode"

	"github.com/eqoa/server/internal/entity"
)

// The trips quests unlock (database.QuestUnlock, table questunlocks; the
// content's "unlocks", tools/quests/FORMAT.md): a travel NPC's route
// (waysender.go's wayRoutes: waysenders, portals, the Plane of Disease's
// necrologist) is offered only to a player who has finished one of the
// quests that unlock it, while one of those quests is in the game (a draft's
// bars no one: no player could finish it); a route only a quest opens
// (wayRoute.QuestOnly: the Portal guides' trips) is offered to no one else
// even then. Having finished any quest that unlocks a trip, in the game or
// not, opens it. The rest of the NPC's routes are offered as ever. An unlock
// names its route by the NPC's name and the route's label, matched as
// routeKey has them; one naming no route is a problem the GM panel and
// tools/import_quests.py name, and opens nothing.

// tripKey is a travel NPC's trip as unlocks are matched to routes: the NPC's
// folded name and the place's routeKey.
func tripKey(npc, to string) string { return foldName(npc) + "\x00" + routeKey(to) }

// routeKey is a place's name as an unlock's matches a route's label: case, a
// leading "the" and everything but letters and digits aside ("the Plane of
// Disease" is "Plane of Disease", "Klik'Anon" is "KlikAnon"); a spelling that
// differs in a letter is another place.
func routeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "the ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TripBarred is whether the quests keep the player from a travel NPC's trip
// to a place: a quest in the game unlocks it (or the trip is one only a quest
// opens) and the player has finished none of the quests that unlock it. It
// names the quest to finish: of those in the game, the first the player's
// class and race may take, else the first ("" when none is in the game).
func (q *QuestEngine) TripBarred(p *entity.Player, npc, to string) (quest string, barred bool) {
	if p == nil {
		return "", false
	}
	only := false
	if _, r, ok := wayRouteOf(npc, to); ok {
		only = r.QuestOnly
	}
	if q == nil {
		return "", only // no quests run: a trip only a quest opens is shut
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	k := tripKey(npc, to)
	gate := q.cat.gates[k]
	if len(gate) == 0 && !only {
		return "", false
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	for _, qu := range q.cat.unlockers[k] {
		if cq := c.quests[qu.ID]; cq != nil && cq.Done > 0 {
			return "", false
		}
	}
	if len(gate) == 0 {
		return "", true
	}
	name := strings.TrimSpace(gate[0].Name)
	for _, qu := range gate {
		if (len(qu.Classes) == 0 || containsInt(qu.Classes, p.Class)) && (len(qu.Races) == 0 || containsInt(qu.Races, p.Race)) {
			name = strings.TrimSpace(qu.Name)
			break
		}
	}
	return name, true
}

// TravelRoute is one travel NPC's trip as waysender.go has it, for the GM
// panel and the tools that check unlocks against the routes.
type TravelRoute struct {
	NPC       string `json:"npc"`
	To        string `json:"to"`
	QuestOnly bool   `json:"questOnly,omitempty"` // only a quest opens it
}

// TravelRoutes is every waysender's, portal's and necrologist's trip
// (wayRoutes), by NPC and then as each lists them.
func TravelRoutes() []TravelRoute {
	npcs := make([]string, 0, len(wayRoutes))
	for n := range wayRoutes {
		npcs = append(npcs, n)
	}
	sort.Strings(npcs)
	var out []TravelRoute
	for _, n := range npcs {
		for _, r := range wayRoutes[n] {
			out = append(out, TravelRoute{NPC: n, To: r.To, QuestOnly: r.QuestOnly})
		}
	}
	return out
}

// RouteOf is the route an unlock names (its NPC and place, matched as
// routeKey has them), and whether there is one.
func RouteOf(npc, to string) (TravelRoute, bool) {
	n, r, ok := wayRouteOf(npc, to)
	if !ok {
		return TravelRoute{}, false
	}
	return TravelRoute{NPC: n, To: r.To, QuestOnly: r.QuestOnly}, true
}

// wayRouteOf is the waysender (its name as wayRoutes has it) and its route to
// that place an unlock of npc's trip to `to` names.
func wayRouteOf(npc, to string) (string, wayRoute, bool) {
	k := tripKey(npc, to)
	for n, rs := range wayRoutes {
		for _, r := range rs {
			if tripKey(n, r.To) == k {
				return n, r, true
			}
		}
	}
	return "", wayRoute{}, false
}
