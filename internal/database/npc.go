package database

import (
	"database/sql"
	"errors"
	"log"
	"strings"
	"sync"
)

// NPCRecord represents a row from the npcs table.
type NPCRecord struct {
	ID              int
	NPCName         string
	Zone            int
	UnkA            int
	UnkB            int
	XCoord          float32
	YCoord          float32
	ZCoord          float32
	Facing          int
	World           int
	HP              int
	HPPercent       int
	ModelID         int
	Size            float32
	PrimaryWeapon   int
	SecondaryWeapon int
	Shield          int
	Torso           int
	Forearm         int
	Gloves          int
	Legs            int
	Feet            int
	Head            int
	Tint            int // unk_o: its armour's tint, $Tints' index 0-14 (0 none)
	HairColor       int
	HairLength      int
	HairStyle       int
	Face            int
	RobeStyle       string
	Race            string
	NPCLevel        int
	NPCType         int
	NPCID           int
}

// NPCStore queries the npcs table.
type NPCStore struct {
	db *SQLiteDB
}

// NewNPCStore creates a new NPC store.
func NewNPCStore(db *SQLiteDB) *NPCStore {
	return &NPCStore{db: db}
}

// npcColumns is the npcs columns NPCRecord holds, in scanNPC's order.
const npcColumns = `id, COALESCE(npc_name,''), COALESCE(zone,0), COALESCE(unk_a,0), COALESCE(unk_b,0),
	COALESCE(x_coord,0), COALESCE(y_coord,0), COALESCE(z_coord,0),
	COALESCE(facing,0), COALESCE(world,0),
	COALESCE(hp,0), COALESCE(hp_percent,0), COALESCE(modelid,0), COALESCE(size,1.0),
	COALESCE(primary_weapon,0), COALESCE(secondary_weapon,0), COALESCE(shield,0),
	COALESCE(torso,0), COALESCE(forearm,0), COALESCE(gloves,0),
	COALESCE(legs,0), COALESCE(feet,0), COALESCE(head,0),
	COALESCE(hair_color,0), COALESCE(hair_length,0), COALESCE(hair_style,0), COALESCE(face,0),
	COALESCE(robe_style,''), COALESCE(race,''),
	COALESCE(npc_level,1), COALESCE(npc_type,0), COALESCE(npc_id,0), COALESCE(unk_o,0)`

// scanNPC scans one row of npcColumns.
func scanNPC(scan func(...any) error) (*NPCRecord, error) {
	n := &NPCRecord{}
	err := scan(
		&n.ID, &n.NPCName, &n.Zone, &n.UnkA, &n.UnkB,
		&n.XCoord, &n.YCoord, &n.ZCoord,
		&n.Facing, &n.World,
		&n.HP, &n.HPPercent, &n.ModelID, &n.Size,
		&n.PrimaryWeapon, &n.SecondaryWeapon, &n.Shield,
		&n.Torso, &n.Forearm, &n.Gloves,
		&n.Legs, &n.Feet, &n.Head,
		&n.HairColor, &n.HairLength, &n.HairStyle, &n.Face,
		&n.RobeStyle, &n.Race,
		&n.NPCLevel, &n.NPCType, &n.NPCID, &n.Tint,
	)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// queryNPCs runs a SELECT of npcColumns and scans every row.
func (s *NPCStore) queryNPCs(query string, args ...any) ([]*NPCRecord, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var npcs []*NPCRecord
	for rows.Next() {
		n, err := scanNPC(rows.Scan)
		if err != nil {
			return nil, err
		}
		npcs = append(npcs, n)
	}
	return npcs, rows.Err()
}

// GetAllNPCs returns all NPCs of the shard: every row but a hidden one (npchidden.go: a GM's /delete).
func (s *NPCStore) GetAllNPCs() ([]*NPCRecord, error) {
	if err := s.db.ensureNPCHidden(); err != nil {
		return nil, err
	}
	return s.queryNPCs(`SELECT ` + npcColumns + ` FROM npcs WHERE ` + npcNotHidden + ` ORDER BY id`)
}

// GetNPC returns the NPC of that row id, or sql.ErrNoRows: for a hidden row too (npchidden.go), which is no row of the
// shard's.
func (s *NPCStore) GetNPC(id int) (*NPCRecord, error) {
	return scanNPC(s.db.QueryRow(`SELECT `+npcColumns+` FROM npcs WHERE id = ? AND `+npcNotHidden, id).Scan)
}

// GetNPCsByZone returns all NPCs in a given zone, a hidden row's aside (npchidden.go).
func (s *NPCStore) GetNPCsByZone(zone int) ([]*NPCRecord, error) {
	return s.queryNPCs(`SELECT `+npcColumns+` FROM npcs WHERE zone = ? AND `+npcNotHidden+` ORDER BY id`, zone)
}

// Count returns the total number of NPCs.
func (s *NPCStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&count)
	return count, err
}

// NPCClassKey is an NPC's name as the npcclass table keys it: trimmed, any
// case, without the trailing '$' some sources mark names with.
//
// It is asked of every wandering creature near a player every tick (its move
// rule, movement.go) and of every NPC update written (its dyes, npcdyes.go):
// a profile of a full shard (2026-11-20, 38 players, 63,000 NPCs) had it at
// 4% of the CPU, nearly all of it lowering the names with capitals into a new
// string each time, which the collector then had to sweep. A name that is
// already its own key comes back as it is; any other's key is made once and
// kept (a shard has a few thousand names).
func NPCClassKey(name string) string {
	if isOwnClassKey(name) {
		return name
	}
	classKeys.RLock()
	k, ok := classKeys.m[name]
	classKeys.RUnlock()
	if ok {
		return k
	}
	k = npcClassKeyOf(name)
	classKeys.Lock()
	if len(classKeys.m) < maxClassKeys {
		classKeys.m[name] = k
	}
	classKeys.Unlock()
	return k
}

// classKeys holds the keys of the names that are not their own (capitals,
// spaces at an end, the '$'). It is bounded so that names made without end
// (a GM's, a player's pet's) cannot grow it past maxClassKeys.
var classKeys = struct {
	sync.RWMutex
	m map[string]string
}{m: map[string]string{}}

const maxClassKeys = 1 << 16

// npcClassKeyOf is NPCClassKey made from the name.
func npcClassKeyOf(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSpace(strings.TrimSuffix(n, "$"))
}

// RuleKeySQL is NPCClassKey in SQLite on column col (lower case, trimmed, the
// trailing '$' off), for a rule table's stored name to meet a key whatever
// case it was written in (board 743).
func RuleKeySQL(col string) string {
	return "trim(rtrim(lower(trim(" + col + ")), '$'))"
}

// isOwnClassKey says if a name is already its key: all ASCII, no capital, no
// space at either end and no '$' at its end (anything else, non-ASCII
// included, goes the long way).
func isOwnClassKey(name string) bool {
	if name == "" {
		return true
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 0x80 || ('A' <= c && c <= 'Z') {
			return false
		}
	}
	switch name[0] {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return false
	}
	switch name[len(name)-1] {
	case ' ', '\t', '\n', '\v', '\f', '\r', '$':
		return false
	}
	return true
}

// RangedModels is the item catalog's ranged weapons by model: a model every
// one of whose weapons (itempattern rows in a hand's wear slot) is a bow (7),
// a crossbow (9) or a thrown weapon (10), with that attack type (the
// commonest, if several). An NPC holding one swings it with that type, the
// one the PS2 shoots it with (entity.npcHandTypes).
func (s *NPCStore) RangedModels() (map[uint32]byte, error) {
	rows, err := s.db.Query(`SELECT model, attacktype, COUNT(*) FROM itempattern
		WHERE model <> 0 AND equipslot IN (12, 14, 15, 16, 17, 18, 20, 21) GROUP BY model, attacktype`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[uint32]map[int]int{}
	for rows.Next() {
		var model int64
		var t, n int
		if err := rows.Scan(&model, &t, &n); err != nil {
			return nil, err
		}
		m := uint32(model)
		if counts[m] == nil {
			counts[m] = map[int]int{}
		}
		counts[m][t] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ranged := map[uint32]byte{}
	for m, types := range counts {
		best, most, melee := 0, 0, false
		for t, n := range types {
			if t != 7 && t != 9 && t != 10 {
				melee = true
				break
			}
			if n > most || n == most && t < best {
				best, most = t, n
			}
		}
		if !melee {
			ranged[m] = byte(best)
		}
	}
	return ranged, nil
}

// PlayerModels is the players' race models (charactermodel: each race's
// two), as the wire carries a model id: an NPC with one of them is one of
// the townsfolk (entity/roam.go).
func (s *NPCStore) PlayerModels() (map[uint32]bool, error) {
	rows, err := s.db.Query(`SELECT modelid FROM charactermodel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint32]bool)
	for rows.Next() {
		var m int64
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out[uint32(m)] = true
	}
	return out, rows.Err()
}

// RaceModels is each player race's two bodies by race and sex
// (charactermodel), as the wire carries a model id: the bodies an illusion
// of a race shows (entity/illusion.go).
func (s *NPCStore) RaceModels() (map[[2]int]uint32, error) {
	rows, err := s.db.Query(`SELECT race, sex, modelid FROM charactermodel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[[2]int]uint32)
	for rows.Next() {
		var race, sex int
		var m int64
		if err := rows.Scan(&race, &sex, &m); err != nil {
			return nil, err
		}
		out[[2]int{race, sex}] = uint32(m)
	}
	return out, rows.Err()
}

// ModelSexes is each players' race body's sex (charactermodel: 0 male, 1
// female), by its model id as the wire carries it: whom a line of town
// dialog for a husband or a wife fits (entity/dialog.go).
func (s *NPCStore) ModelSexes() (map[uint32]int, error) {
	rows, err := s.db.Query(`SELECT modelid, sex FROM charactermodel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint32]int)
	for rows.Next() {
		var m int64
		var sex int
		if err := rows.Scan(&m, &sex); err != nil {
			return nil, err
		}
		out[uint32(m)] = sex
	}
	return out, rows.Err()
}

// ModelRaces is each players' race body's race (charactermodel), by its model
// id as the wire carries it: the voice an NPC on it speaks in (entity/dialog.go).
func (s *NPCStore) ModelRaces() (map[uint32]int, error) {
	rows, err := s.db.Query(`SELECT modelid, race FROM charactermodel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint32]int)
	for rows.Next() {
		var m int64
		var race int
		if err := rows.Scan(&m, &race); err != nil {
			return nil, err
		}
		out[uint32(m)] = race
	}
	return out, rows.Err()
}

// NPCClasses is the npcclass table (tools/import_npc_classes.py): each NPC
// name's class, by NPCClassKey. A name it does not hold is a warrior's.
func (s *NPCStore) NPCClasses() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT name, class FROM npcclass`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// keyed by NPCClassKey, as every reader asks (board 743: a row stored with
	// capitals, 207 of the census's, classed no one); where two rows meet on
	// one key, the one stored as its key wins and the other is logged
	out := make(map[string]int)
	own := make(map[string]bool)
	for rows.Next() {
		var name string
		var class int
		if err := rows.Scan(&name, &class); err != nil {
			return nil, err
		}
		key := NPCClassKey(name)
		if _, dup := out[key]; dup {
			if own[key] || key != name {
				log.Printf("npcclass: row %q is %q's too; the row %q counts", name, key, key)
				continue
			}
			log.Printf("npcclass: a row under another case is %q's too; the row %q counts", key, key)
		}
		out[key], own[key] = class, key == name
	}
	return out, rows.Err()
}

// NPCClassOf is the npcclass table's class for an NPC name (by NPCClassKey),
// and 0 (a warrior's) when it holds none.
func (s *NPCStore) NPCClassOf(name string) (int, error) {
	var class int
	key := NPCClassKey(name)
	err := s.db.QueryRow(`SELECT class FROM npcclass WHERE `+RuleKeySQL("name")+` = ?
		ORDER BY name = ? DESC LIMIT 1`, key, key).Scan(&class)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return class, err
}
