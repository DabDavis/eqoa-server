package main

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

// npcRow is an npcs row as the plan sees it: its place and what the server
// makes of it.
type npcRow struct {
	id        int64
	name      string
	key       string // database.NPCClassKey
	world     int
	x, y, z   float64
	level     int
	typ       int
	model     uint32
	movement  string // entity's verdict (MoveWhyText)
	wanders   bool   // the server wanders it
	wild      bool   // as wildlife: an animal by its name, or by its body (npcmove's "an animal by its body")
	spawnOnly bool
	open      bool // on the open ground: within onGround of the highest floor
	wet       bool // in water, slime, lava or mud
}

// input is the database as the plan needs it.
type input struct {
	rows      []*npcRow
	byID      map[int64]*npcRow
	players   map[uint32]bool   // the players' race bodies (charactermodel)
	pets      map[string]bool   // a pet's body: a spell's name (entity/pet.go)
	water     map[string]string // npcmove's water word by name
	moveNote  map[string]string // npcmove's note by name
	undead    map[string]bool   // npcundead's word by name
	protected map[int64]string  // rows another tool or table places, and why
	places    []placeRule       // npcfaction's rules bound to a place
}

type placeRule struct {
	key     string
	world   int
	x, z, r float64
}

// load reads the NPCs and builds the server's world from them, as
// cmd/gameserver does at its start, for its verdict on each: which wander.
func load(db *database.SQLiteDB, g *ground.Ground, env *ground.Env) (*input, error) {
	store := database.NewNPCStore(db)
	players, err := store.PlayerModels()
	if err != nil {
		return nil, fmt.Errorf("charactermodel: %w", err)
	}
	w := entity.NewWorld()
	var loadErr error
	quiet(func() {
		w.SetPlayerModels(players)
		w.LoadNPCs(store)
		w.SetGround(g)
		w.SetEnv(env)
		if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
			w.LoadNPCPaths(recs)
		}
		if _, _, err := w.LoadUndeadTags(db); err != nil {
			loadErr = err
			return
		}
		if _, err := w.LoadMoveRules(db); err != nil {
			loadErr = err
			return
		}
		if _, _, err := w.LoadSpawnOnly(db); err != nil {
			loadErr = err
		}
	})
	if loadErr != nil {
		return nil, loadErr
	}
	recs, err := store.GetAllNPCs()
	if err != nil {
		return nil, err
	}
	in := &input{byID: map[int64]*npcRow{}, players: players, pets: map[string]bool{},
		water: map[string]string{}, moveNote: map[string]string{}, undead: map[string]bool{}, protected: map[int64]string{}}
	if err := in.readTables(db); err != nil {
		return nil, err
	}
	for _, rec := range recs {
		r := &npcRow{id: int64(rec.ID), name: rec.NPCName, key: database.NPCClassKey(rec.NPCName), world: rec.World,
			x: float64(rec.XCoord), y: float64(rec.YCoord), z: float64(rec.ZCoord), level: rec.NPCLevel,
			typ: rec.NPCType, model: uint32(rec.ModelID)}
		if s, ok := w.NPCStateOf(entity.EntityID(rec.ID)); ok {
			r.movement = s.Movement
			r.wanders = strings.HasPrefix(s.Movement, "it wanders")
			r.wild = strings.HasPrefix(s.Movement, "it wanders: wildlife") ||
				(r.wanders && strings.HasPrefix(in.moveNote[r.key], "wanders: an animal by its body"))
			r.spawnOnly = s.SpawnOnly
		}
		if h, ok := g.Floor(uint8(r.world), float32(r.x), 1e6, float32(r.z)); ok {
			r.open = math.Abs(float64(h)-r.y) <= onGround
		}
		r.wet = badAt(env, r.world, r.x, r.y, r.z)
		in.rows = append(in.rows, r)
		in.byID[r.id] = r
	}
	return in, nil
}

// readTables reads what else decides a row: the pets' names, npcmove's
// water words, the rows other tools and tables place, the place-bound
// faction rules. A table this database lacks is skipped.
func (in *input) readTables(db *database.SQLiteDB) error {
	strs := func(q string, f func(a, b string)) {
		rows, err := db.Query(q)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var a, b string
			if rows.Scan(&a, &b) == nil {
				f(a, b)
			}
		}
	}
	strs(`SELECT COALESCE(spellname,''), '' FROM spellpattern`, func(a, _ string) {
		in.pets[database.NPCClassKey(a)] = true
	})
	strs(`SELECT name, water FROM npcmove`, func(a, b string) {
		in.water[database.NPCClassKey(a)] = b
	})
	strs(`SELECT name, note FROM npcmove`, func(a, b string) {
		in.moveNote[database.NPCClassKey(a)] = b
	})
	strs(`SELECT name, CAST(undead AS TEXT) FROM npcundead`, func(a, b string) {
		in.undead[database.NPCClassKey(a)] = b != "0"
	})
	ids := func(why, q string) {
		rows, err := db.Query(q)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil && id > 0 {
				if _, ok := in.protected[id]; !ok {
					in.protected[id] = why
				}
			}
		}
	}
	ids("placed by tools/seed_wildlife.py", `SELECT npc_row FROM wildlife_seed_npcs`)
	ids("placed by tools/seed_town_wildlife.py", `SELECT npc_row FROM town_wildlife_seed_npcs`)
	ids("placed by tools/seed_pod.py", `SELECT npc_row FROM pod_seed_npcs`)
	ids("placed by tools/seed_quest_npcs.py", `SELECT npc_row FROM quest_npc_seed_rows`)
	ids("placed by the beach crab seed", `SELECT npc FROM beach_crab_seed`)
	ids("walks a path (npcpaths)", `SELECT npcid FROM npcpaths`)
	ids("its path's seed (npcpaths_seed)", `SELECT npcid FROM npcpaths_seed`)
	ids("a sign (signtext)", `SELECT npcid FROM signtext`)
	ids("a quest step names it (queststeps)", `SELECT npcid FROM queststeps WHERE npcid > 0`)
	// a tagged run's fills (-tag) stay where it put them
	var tagged []string
	if tabs, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'wildspread\_adds\_%' ESCAPE '\'`); err == nil {
		for tabs.Next() {
			var n string
			if tabs.Scan(&n) == nil {
				tagged = append(tagged, n)
			}
		}
		tabs.Close()
	}
	for _, n := range tagged {
		ids("placed by wildspread -tag "+strings.TrimPrefix(n, "wildspread_adds_"), `SELECT npc_row FROM `+n)
	}
	rows, err := db.Query(`SELECT name, world, x, z, radius FROM npcfaction WHERE radius > 0 AND world >= 0`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p placeRule
			var name string
			if rows.Scan(&name, &p.world, &p.x, &p.z, &p.r) == nil {
				p.key = database.NPCClassKey(name)
				in.places = append(in.places, p)
			}
		}
	}
	return nil
}

// readQuestGivers is, for each kind a quest's kill or loot step names (its
// target and queststeptargets' others), where the quest's givers stand (the
// NPCs of the giver's name, those with the NPC bit when any has it: a town's
// people may have none), and the steps themselves (-quest-weight, -quest-min).
func readQuestGivers(db *database.SQLiteDB, in *input) (map[string][]place, []questStep, error) {
	type row struct {
		qid, step, minLevel int
		giver, target       string
	}
	var rs []row
	// a step's own target first, then its others
	rows, err := db.Query(`SELECT q.questid, s.step, COALESCE(q.minlevel, 1), q.giver, s.target, 0 FROM queststeps s
		JOIN quests q ON q.questid = s.questid WHERE s.kind IN ('kill', 'loot') AND q.enabled = 1
		UNION SELECT q.questid, s.step, COALESCE(q.minlevel, 1), q.giver, t.target, 1 FROM queststeptargets t
		JOIN queststeps s ON s.questid = t.questid AND s.step = t.step JOIN quests q ON q.questid = t.questid
		WHERE s.kind IN ('kill', 'loot') AND q.enabled = 1
		ORDER BY 1, 2, 6, 5`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var r row
		var g, t sql.NullString
		var other int
		if rows.Scan(&r.qid, &r.step, &r.minLevel, &g, &t, &other) == nil && g.Valid && t.Valid && t.String != "" {
			r.giver, r.target = database.NPCClassKey(g.String), database.NPCClassKey(t.String)
			rs = append(rs, r)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	at, bit := map[string][]place{}, map[string][]place{}
	for _, r := range in.rows {
		if r.key == "" {
			continue
		}
		at[r.key] = append(at[r.key], place{r.world, r.x, r.z})
		if r.typ&0x80 != 0 {
			bit[r.key] = append(bit[r.key], place{r.world, r.x, r.z})
		}
	}
	giverAt := func(k string) []place {
		if ps := bit[k]; len(ps) > 0 {
			return ps
		}
		return at[k]
	}
	out := map[string][]place{}
	var steps []questStep
	for i, r := range rs {
		out[r.target] = append(out[r.target], giverAt(r.giver)...)
		if i == 0 || rs[i-1].qid != r.qid || rs[i-1].step != r.step {
			steps = append(steps, questStep{minLevel: r.minLevel, givers: giverAt(r.giver)})
		}
		st := &steps[len(steps)-1]
		st.names = append(st.names, r.target)
	}
	return out, steps, nil
}
