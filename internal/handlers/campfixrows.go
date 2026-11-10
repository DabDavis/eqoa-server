package handlers

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A camp report read off the database alone (a petition's filing,
// campfixpetition.go; the GM panel's review, campfixreview.go): a world's NPC
// rows as the sending sees the world's NPCs (entity/campfix.go's campfixRow:
// the NPC of a row, never one of a name npcspawnonly keeps out of the world),
// and what the review rules read of the other tables. The database has one
// connection: every read here is whole before the next query.

// campfixRowNPC is an NPC row as a report reads it: entity.CampfixNPC (its
// spawn, and its facing in radians as the loader turns the row's byte), the
// row's own facing byte and its npc_type.
type campfixRowNPC struct {
	entity.CampfixNPC
	FacingByte int
	Type       int
	// RawX, RawY and RawZ are the row's own coordinates, as the panel's row
	// edit reads them back (a change's expected values), and RawNull the
	// columns of its place and facing that hold NULL (read as 0)
	RawX, RawY, RawZ float64
	RawNull          []string
}

// campfixRows is a world's NPC rows a report reads: those round its marks
// (list: one query, every row within campfixRowsReach of them across), and
// every row of a name it asks about (named: a query a name, kept in names,
// which a reviewer shares between its reports of the world). err is the
// first read that failed in scan, which cannot return one.
type campfixRows struct {
	db        *database.SQLiteDB
	world     uint8
	spawnOnly map[string]bool
	list      []campfixRowNPC
	byID      map[entity.EntityID]int
	names     map[string][]campfixRowNPC
	err       error
}

// campfixRowsReach is how far round a report's marks its rows are read,
// across: the rules' farthest reach (a camp's campfixSpan, a stacked spawn's
// campfixStacked round a move's row) and some.
const campfixRowsReach float32 = 400

const campfixRowCols = `id, COALESCE(npc_name, ''), COALESCE(npc_level, 1), COALESCE(world, 0), COALESCE(x_coord, 0),
	COALESCE(y_coord, 0), COALESCE(z_coord, 0), COALESCE(facing, 0), COALESCE(npc_type, 0),
	x_coord IS NULL, y_coord IS NULL, z_coord IS NULL, facing IS NULL`

// campfixPlaceCols are the npcs columns of a row's place and facing, in
// campfixRowCols' NULL tests' order.
var campfixPlaceCols = []string{"x_coord", "y_coord", "z_coord", "facing"}

func scanCampfixRow(sc interface{ Scan(...any) error }) (campfixRowNPC, error) {
	var r campfixRowNPC
	var id int64
	var world int
	var x, y, z float64
	var null [4]bool
	if err := sc.Scan(&id, &r.Name, &r.Level, &world, &x, &y, &z, &r.FacingByte, &r.Type, &null[0], &null[1], &null[2], &null[3]); err != nil {
		return r, err
	}
	for i, n := range null {
		if n {
			r.RawNull = append(r.RawNull, campfixPlaceCols[i])
		}
	}
	r.ID, r.World = entity.EntityID(id), uint8(world)
	r.X, r.Y, r.Z = float32(x), float32(y), float32(z)
	r.RawX, r.RawY, r.RawZ = x, y, z
	r.Facing = float32(r.FacingByte) / 256.0 * 2.0 * math.Pi // as the loader turns it (entity's fromRecord)
	return r, nil
}

// campfixQueryRows is the rows a query of campfixRowCols gives, none of a name
// spawnOnly holds (by database.NPCClassKey, as database.NPCSpawnOnly keys it).
func campfixQueryRows(db *database.SQLiteDB, spawnOnly map[string]bool, query string, args ...any) ([]campfixRowNPC, error) {
	var out []campfixRowNPC
	err := campfixEach(db, `SELECT `+campfixRowCols+` FROM npcs `+query, func(sc func(...any) error) error {
		r, err := scanCampfixRow(scanFunc(sc))
		if err != nil {
			return err
		}
		if !spawnOnly[database.NPCClassKey(r.Name)] {
			out = append(out, r)
		}
		return nil
	}, args...)
	if err != nil {
		return nil, fmt.Errorf("npcs: %w", err)
	}
	return out, nil
}

// scanFunc is a Scan function as scanCampfixRow takes one.
type scanFunc func(...any) error

func (f scanFunc) Scan(dest ...any) error { return f(dest...) }

// loadCampfixRows is world's rows within reach of pts across (none when pts
// is empty), with names the rows of a name already read (nil: none).
func loadCampfixRows(db *database.SQLiteDB, world uint8, spawnOnly map[string]bool, pts [][3]float32, reach float32,
	names map[string][]campfixRowNPC) (*campfixRows, error) {
	if names == nil {
		names = map[string][]campfixRowNPC{}
	}
	out := &campfixRows{db: db, world: world, spawnOnly: spawnOnly, byID: map[entity.EntityID]int{}, names: names}
	if len(pts) == 0 {
		return out, nil
	}
	minX, maxX, minZ, maxZ := pts[0][0], pts[0][0], pts[0][2], pts[0][2]
	for _, p := range pts {
		minX, maxX, minZ, maxZ = min(minX, p[0]), max(maxX, p[0]), min(minZ, p[2]), max(maxZ, p[2])
	}
	list, err := campfixQueryRows(db, spawnOnly, `WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ? ORDER BY id`,
		int(world), float64(minX-reach), float64(maxX+reach), float64(minZ-reach), float64(maxZ+reach))
	if err != nil {
		return nil, err
	}
	for i, r := range list {
		out.byID[r.ID] = i
	}
	out.list = list
	return out, nil
}

// campfixRowByID is the NPC row id whatever its world and name, false when
// there is none.
func campfixRowByID(db *database.SQLiteDB, id int) (campfixRowNPC, bool, error) {
	l, err := campfixQueryRows(db, nil, `WHERE id = ?`, id)
	if err != nil || len(l) == 0 {
		return campfixRowNPC{}, false, err
	}
	return l[0], true, nil
}

// row is the row of id among those round the marks, false when it is not one.
func (rs *campfixRows) row(id entity.EntityID) (campfixRowNPC, bool) {
	i, ok := rs.byID[id]
	if !ok {
		return campfixRowNPC{}, false
	}
	return rs.list[i], true
}

// named is every row of a name in the world (compared as campfixKey compares
// two), read once.
func (rs *campfixRows) named(name string) ([]campfixRowNPC, error) {
	k := campfixKey(name)
	if l, ok := rs.names[k]; ok {
		return l, nil
	}
	query, args := `WHERE world = ? AND lower(trim(npc_name)) = ? ORDER BY id`, []any{int(rs.world), k}
	for _, r := range k {
		if r > unicode.MaxASCII { // SQLite's lower folds ASCII alone: the world's rows, compared here
			query, args = `WHERE world = ? ORDER BY id`, []any{int(rs.world)}
			break
		}
	}
	got, err := campfixQueryRows(rs.db, rs.spawnOnly, query, args...)
	if err != nil {
		return nil, err
	}
	l := []campfixRowNPC{}
	for _, r := range got {
		if campfixKey(r.Name) == k {
			l = append(l, r)
		}
	}
	rs.names[k] = l
	return l, nil
}

// nearestNamed is the row of name whose spawn is nearest (x, y, z), the lower
// id of two as near (its Dist that distance), false when the world has none.
func (rs *campfixRows) nearestNamed(name string, x, y, z float32) (campfixRowNPC, bool, error) {
	rows, err := rs.named(name)
	if err != nil {
		return campfixRowNPC{}, false, err
	}
	var best campfixRowNPC
	found := false
	for _, r := range rows {
		d := campfixDist(r.X, r.Y, r.Z, x, y, z)
		if !found || d < best.Dist || d == best.Dist && r.ID < best.ID {
			best, found = r, true
			best.Dist = d
		}
	}
	return best, found, nil
}

// sortCampfixNPCs puts the nearest first, the lower id of two as near
// (entity's sortCampfix).
func sortCampfixNPCs(l []entity.CampfixNPC) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Dist != l[j].Dist {
			return l[i].Dist < l[j].Dist
		}
		return l[i].ID < l[j].ID
	})
}

// scan is World.CampfixScan over the rows: those round the marks whose spawn
// lies within reach of (x, y, z), nearest first, and what the world holds of
// each of names (compared without case, trimmed); a failed read is rs.err.
func (rs *campfixRows) scan(x, y, z, reach float32, names []string) ([]entity.CampfixNPC, []entity.CampfixName) {
	var near []entity.CampfixNPC
	for _, r := range rs.list {
		if d := campfixDist(r.X, r.Y, r.Z, x, y, z); reach > 0 && d <= reach {
			c := r.CampfixNPC
			c.Dist = d
			near = append(near, c)
		}
	}
	sortCampfixNPCs(near)
	of := make([]entity.CampfixName, len(names))
	for i, n := range names {
		if campfixKey(n) == "" {
			continue
		}
		rows, err := rs.named(n)
		if err != nil {
			if rs.err == nil {
				rs.err = err
			}
			continue
		}
		for _, r := range rows {
			d := campfixDist(r.X, r.Y, r.Z, x, y, z)
			c := r.CampfixNPC
			c.Dist = d
			o := &of[i]
			o.Count++
			if o.Count == 1 || d < o.Nearest.Dist || d == o.Nearest.Dist && r.ID < o.Nearest.ID {
				o.Nearest = c
			}
		}
	}
	return near, of
}

// near is World.CampfixNear over the rows: those whose spawn lies within reach
// of any of pts, nearest (to its nearest point) first, at most most of them
// (0: all), each its Dist from its nearest point.
func (rs *campfixRows) near(pts [][3]float32, reach float32, most int) []campfixRowNPC {
	var out []campfixRowNPC
	for _, r := range rs.list {
		best := float32(-1)
		for _, p := range pts {
			if d := campfixDist(r.X, r.Y, r.Z, p[0], p[1], p[2]); d <= reach && (best < 0 || d < best) {
				best = d
			}
		}
		if best >= 0 {
			r.Dist = best
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dist != out[j].Dist {
			return out[i].Dist < out[j].Dist
		}
		return out[i].ID < out[j].ID
	})
	if most > 0 && len(out) > most {
		out = out[:most]
	}
	return out
}

// campfixFacts is what the review rules read of the other tables: the names
// kept out of the world, the camps' spawns, the paths, the respawn times, the
// loot bindings, the quests' NPCs, the start spots, and the rare mobs the
// camps' notes name.
type campfixFacts struct {
	spawnOnly map[string]bool               // npcspawnonly, by database.NPCClassKey
	camps     map[int]database.CampSpawnRow // campspawns, by npcs row
	paths     map[int]bool                  // npcpaths' rows
	respawn   map[uint32]float64            // npcrespawn: seconds, by npcs row
	loot      map[string][]string           // npcloot: tables, by database.NPCClassKey
	quests    map[string]string             // what a name is to a quest (its giver, a step's), by campfixKey
	pins      map[int]string                // a quest step's own spawn (queststeps.npcid): what
	givers    map[string]bool               // quest givers, by campfixKey
	spots     []database.StartSpot
	rare      map[string]string // a camp's rare mob (campNotes), by campfixKey: the camp's key
}

// loadCampfixFacts reads campfixFacts.
func loadCampfixFacts(db *database.SQLiteDB) (*campfixFacts, error) {
	f := &campfixFacts{camps: map[int]database.CampSpawnRow{}, paths: map[int]bool{}, loot: map[string][]string{},
		quests: map[string]string{}, pins: map[int]string{}, givers: map[string]bool{}, rare: map[string]string{}}
	var err error
	if f.spawnOnly, err = db.NPCSpawnOnly(); err != nil {
		return nil, fmt.Errorf("npcspawnonly: %w", err)
	}
	camps, err := db.CampSpawns()
	if err != nil {
		return nil, fmt.Errorf("campspawns: %w", err)
	}
	for _, c := range camps {
		f.camps[c.NPCID] = c
	}
	if f.respawn, err = db.NPCRespawns(); err != nil {
		return nil, fmt.Errorf("npcrespawn: %w", err)
	}
	if err := campfixEach(db, `SELECT npcid FROM npcpaths`, func(sc func(...any) error) error {
		var id int
		if err := sc(&id); err != nil {
			return err
		}
		f.paths[id] = true
		return nil
	}); err != nil {
		return nil, fmt.Errorf("npcpaths: %w", err)
	}
	if err := campfixEach(db, `SELECT npc_name, tablename FROM npcloot`, func(sc func(...any) error) error {
		var name, table string
		if err := sc(&name, &table); err != nil {
			return err
		}
		k := database.NPCClassKey(name)
		f.loot[k] = append(f.loot[k], strings.ToLower(strings.TrimSpace(table)))
		return nil
	}); err != nil {
		return nil, fmt.Errorf("npcloot: %w", err)
	}
	names := map[int64]string{}
	if err := campfixEach(db, `SELECT questid, name, giver FROM quests`, func(sc func(...any) error) error {
		var id int64
		var name, giver string
		if err := sc(&id, &name, &giver); err != nil {
			return err
		}
		names[id] = name
		if k := campfixKey(giver); k != "" {
			f.givers[k] = true
			if _, ok := f.quests[k]; !ok {
				f.quests[k] = fmt.Sprintf("the giver of quest %d (%s)", id, name)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("quests: %w", err)
	}
	step := func(id int64, n int) string { return fmt.Sprintf("step %d of quest %d (%s)", n, id, names[id]) }
	if err := campfixEach(db, `SELECT questid, step, target, npcid FROM queststeps`, func(sc func(...any) error) error {
		var id int64
		var n, pin int
		var target string
		if err := sc(&id, &n, &target, &pin); err != nil {
			return err
		}
		if k := campfixKey(target); k != "" {
			if _, ok := f.quests[k]; !ok {
				f.quests[k] = step(id, n)
			}
		}
		if pin != 0 {
			f.pins[pin] = step(id, n)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("queststeps: %w", err)
	}
	if err := campfixEach(db, `SELECT questid, step, target FROM queststeptargets`, func(sc func(...any) error) error {
		var id int64
		var n int
		var target string
		if err := sc(&id, &n, &target); err != nil {
			return err
		}
		if k := campfixKey(target); k != "" {
			if _, ok := f.quests[k]; !ok {
				f.quests[k] = step(id, n)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("queststeptargets: %w", err)
	}
	if f.spots, err = db.StartSpots(); err != nil {
		return nil, fmt.Errorf("defaultcharacter: %w", err)
	}
	for key, n := range campNotes {
		for _, r := range n.Rare {
			if k := campfixKey(r.Name); k != "" {
				if cur, ok := f.rare[k]; !ok || key < cur {
					f.rare[k] = key
				}
			}
		}
	}
	return f, nil
}

// campfixEach runs query and hands each row's Scan to fn, the rows whole
// before fn's caller queries again.
func campfixEach(db *database.SQLiteDB, query string, fn func(scan func(...any) error) error, args ...any) error {
	rs, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		if err := fn(rs.Scan); err != nil {
			return err
		}
	}
	return rs.Err()
}
