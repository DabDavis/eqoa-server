package gmweb

import (
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The World map's two layers beside the NPCs (F3 pass 18, the user: "add the
// camps and coaches to the GM panel map too"): the XP camps of #tele
// (handlers/camps.go, campinfo.go) and the coach stops (entity
// CoachMarkers). Both are the server's own: the camps' places and names as
// tools/gen_camp_teles.py wrote them from EQOA-Data's map, the coachmen as the
// world has them; nothing of the disc.

// lootSample is how many of a table's items a camp's detail names.
const lootSample = 8

func (s *Server) registerMapLayerOps() {
	s.addOp(&Op{
		Name: "map_layers", Title: "Map layers: XP camps and coaches", Scope: scopeRead,
		Description: "The World map's XP camps and coach stops of one world. camps: each #tele camp (key, the " +
			"aliases #tele also takes, label, levels: its creatures' here, guide: EQOA-Data's levels, note: " +
			"EQOA-Data's note on it, x/y/z: its anchor creature's spawn, fallback: the anchor is not loaded and " +
			"the place is as written, creatures: the names that make it, rare: EQOA-Data's rare mobs near it with " +
			"their level here (0: none of the name) and the drops EQOA-Data names). coaches: each coachman (id: " +
			"its NPC's, name, town, x/y/z: its spawn, routes: the towns its coach menu here offers, every other " +
			"coach town of the world; live: the towns the live game's coach ran to, EQOA-Data's coach map). towns: " +
			"the world's towns to name (Tunaria's coach towns, the others by EQOA-Data's map). you: " +
			"the GM's own character in the game (serverid, name, world), or null: what a camp's Teleport moves. " +
			"map_camp has a camp's loot tables.",
		Input: obj(map[string]any{"world": integer("the world index")}, "world"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ World int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			camps := []map[string]any{}
			for _, m := range handlers.CampMarkers(c.s.d.World, req.World) {
				camps = append(camps, campJSON(m))
			}
			coaches := []map[string]any{}
			if c.s.d.World != nil {
				for _, m := range c.s.d.World.CoachMarkers(req.World) {
					coaches = append(coaches, map[string]any{
						"id": m.ID, "name": m.Name, "town": m.Town, "named": m.Named, "world": m.World,
						"x": round1(m.X), "y": round1(m.Y), "z": round1(m.Z),
						"routes": orEmpty(m.Routes), "live": orEmpty(m.Live),
					})
				}
			}
			towns := []map[string]any{}
			for _, t := range handlers.TownMarks(req.World) {
				towns = append(towns, map[string]any{"name": t.Name, "world": t.World, "x": round1(t.X), "z": round1(t.Z)})
			}
			var you any
			if sess := c.s.gmSession(c.a); sess != nil {
				if p := c.s.d.World.SessionPlayer(sess); p != nil {
					you = map[string]any{"serverid": p.CharServerID, "name": p.Name, "world": p.World}
				}
			}
			return map[string]any{"camps": camps, "coaches": coaches, "towns": towns, "you": you}, nil
		},
	})
	s.addOp(&Op{
		Name: "map_quests", Title: "Map layer: quest givers", Scope: scopeRead,
		Description: "The World map's quest givers of one world: every NPC that gives a quest (quests.giver, matched " +
			"as the engine matches NPC names: trimmed, any case) where it spawns. givers is each such NPC (id, name, " +
			"x/y/z, status: \"in the game\" when any of its quests is, else \"draft\") with its quests (questid, name, " +
			"classes, minlevel, maxlevel, status: in the game, ready or draft, as quest_list says) and targets: the " +
			"NPCs its quests' talk, show and give steps send players to. missing names the givers no NPC of any " +
			"world is (a GM places one: quest_list's missing says which quests wait on it).",
		Input: obj(map[string]any{"world": integer("the world index")}, "world"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ World int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.mapQuests(req.World)
		},
	})
	s.addOp(&Op{
		Name: "map_camp", Title: "An XP camp", Scope: scopeRead,
		Description: "One #tele camp (its key or an alias) as map_layers has it, with the loot its creatures roll " +
			"(Loot tables, Creature loot: database/loottables.go) for a creature of the camp's middle level: loot " +
			"is each table bound to any of its creatures, its chance a roll (percent), rolls a kill, the creatures " +
			"bound to it, how many items it has at that level and a few of them by name.",
		Input: obj(map[string]any{"camp": str("the camp's #tele name (dfc) or an alias (deathfist)")}, "camp"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Camp string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			m, ok := handlers.CampMarkerOf(c.s.d.World, req.Camp)
			if !ok {
				return nil, notFound("no camp %q (map_layers lists them)", req.Camp)
			}
			out := campJSON(m)
			level := midLevel(m.Levels)
			out["level"] = level
			loot, err := c.s.campLoot(m.Creatures, level)
			if err != nil {
				return nil, err
			}
			out["loot"] = loot
			return out, nil
		},
	})
}

// mapQuests is map_quests: the world's quest givers where they stand, each
// with its quests as the quest index has them now (questindex.go).
func (s *Server) mapQuests(world int) (any, error) {
	idx, err := s.loadQuestIndex()
	if err != nil {
		return nil, err
	}
	key := func(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
	byGiver := map[string][]*questRow{}
	for _, r := range idx.rows {
		if k := key(r.Giver); k != "" {
			byGiver[k] = append(byGiver[k], r)
		}
	}
	type spot struct {
		id      any
		name    string
		x, y, z float64
	}
	here := map[string][]spot{}
	anywhere := map[string]bool{}
	if s.d.World != nil {
		for _, n := range s.d.World.NPCStates(-1) {
			k := key(n.Name)
			if _, gives := byGiver[k]; !gives || n.Temporary {
				continue
			}
			anywhere[k] = true
			if int(n.World) == world {
				here[k] = append(here[k], spot{n.ID, strings.TrimSpace(n.Name), round1(n.SpawnX), round1(n.SpawnY), round1(n.SpawnZ)})
			}
		}
	}
	givers := []map[string]any{}
	var missing []string
	keys := make([]string, 0, len(byGiver))
	for k := range byGiver {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows := byGiver[k]
		if !anywhere[k] {
			missing = append(missing, rows[0].Giver)
			continue
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].MinLevel != rows[j].MinLevel {
				return rows[i].MinLevel < rows[j].MinLevel
			}
			return rows[i].ID < rows[j].ID
		})
		status := questDraft
		var quests []map[string]any
		var targets []string
		for _, r := range rows {
			if r.Enabled {
				status = questInGame
			}
			quests = append(quests, map[string]any{"questid": r.ID, "name": r.Name, "classes": r.Classes,
				"minlevel": r.MinLevel, "maxlevel": r.MaxLevel, "status": r.Status})
			for _, t := range r.targets {
				if key(t) != k && !containsString(targets, t) {
					targets = append(targets, t)
				}
			}
		}
		for _, sp := range here[k] {
			givers = append(givers, map[string]any{"id": sp.id, "name": sp.name, "x": sp.x, "y": sp.y, "z": sp.z,
				"status": status, "quests": quests, "targets": orEmpty(targets)})
		}
	}
	return map[string]any{"givers": givers, "missing": orEmpty(missing)}, nil
}

// campJSON is a camp as the map's ops send it.
func campJSON(m handlers.CampMarker) map[string]any {
	rare := []map[string]any{}
	for _, r := range m.Rare {
		rare = append(rare, map[string]any{"name": r.Name, "level": r.Level, "drops": orEmpty(r.Drops)})
	}
	return map[string]any{
		"key": m.Key, "aliases": orEmpty(m.Aliases), "label": m.Label, "levels": m.Levels, "guide": m.Guide,
		"note": m.Note, "world": m.World, "x": round1(m.X), "y": round1(m.Y), "z": round1(m.Z),
		"fallback": m.Fallback, "creatures": orEmpty(m.Creatures), "rare": rare,
	}
}

// campLoot is the loot tables bound to any of a camp's creatures, each with
// the creatures it is bound to and a few of its items at level.
func (s *Server) campLoot(creatures []string, level int) ([]map[string]any, error) {
	type table struct {
		chance, rolls int
		creatures     []string
		items         []string
		count         int
	}
	inv := database.NewInventoryStore(s.d.DB)
	byName := map[string]*table{}
	var order []string
	for _, cr := range creatures {
		rolls, err := inv.NPCLootRolls(cr, level)
		if err != nil {
			return nil, err
		}
		for _, r := range rolls {
			t, ok := byName[r.Table]
			if !ok {
				t = &table{chance: r.Chance, rolls: r.Rolls, count: len(r.Choices)}
				for _, ch := range r.Choices {
					if len(t.items) == lootSample {
						break
					}
					t.items = append(t.items, ch.Item.Pattern.ItemName)
				}
				byName[r.Table] = t
				order = append(order, r.Table)
			}
			t.creatures = append(t.creatures, cr)
		}
	}
	sort.Strings(order)
	out := []map[string]any{}
	for _, name := range order {
		t := byName[name]
		out = append(out, map[string]any{"table": name, "chance": t.chance, "rolls": t.rolls,
			"creatures": t.creatures, "count": t.count, "items": orEmpty(t.items)})
	}
	return out, nil
}

// midLevel is the middle of a level range as campDests writes one ("33-40",
// "45"), 1 for none.
func midLevel(levels string) int {
	lo, hi, found := strings.Cut(levels, "-")
	a, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return 1
	}
	if !found {
		return a
	}
	b, err := strconv.Atoi(strings.TrimSpace(hi))
	if err != nil || b < a {
		return a
	}
	return (a + b) / 2
}

// orEmpty is a list as JSON's [] rather than null.
func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
