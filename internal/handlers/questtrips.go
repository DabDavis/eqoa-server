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
// bars no one: no player could finish it). The rest of the NPC's routes are
// offered as ever. An unlock names its route by the NPC's name and the
// route's label, matched as routeKey has them; one naming no route is a
// problem the GM panel and tools/import_quests.py name, and opens nothing.

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
// to a place: a quest in the game unlocks it and the player has finished none
// of those. It names the quest to finish: the first the player's class and
// race may take, else the first.
func (q *QuestEngine) TripBarred(p *entity.Player, npc, to string) (quest string, barred bool) {
	if q == nil || p == nil {
		return "", false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	gate := q.cat.gates[tripKey(npc, to)]
	if len(gate) == 0 {
		return "", false
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	for _, qu := range gate {
		if cq := c.quests[qu.ID]; cq != nil && cq.Done > 0 {
			return "", false
		}
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
	NPC string `json:"npc"`
	To  string `json:"to"`
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
			out = append(out, TravelRoute{NPC: n, To: r.To})
		}
	}
	return out
}

// RouteOf is the route an unlock names (its NPC and place, matched as
// routeKey has them), and whether there is one.
func RouteOf(npc, to string) (TravelRoute, bool) {
	k := tripKey(npc, to)
	for n, rs := range wayRoutes {
		for _, r := range rs {
			if tripKey(n, r.To) == k {
				return TravelRoute{NPC: n, To: r.To}, true
			}
		}
	}
	return TravelRoute{}, false
}
