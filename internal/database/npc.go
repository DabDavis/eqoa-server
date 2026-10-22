package database

import (
	"database/sql"
	"errors"
	"strings"
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
	COALESCE(npc_level,1), COALESCE(npc_type,0), COALESCE(npc_id,0)`

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
		&n.NPCLevel, &n.NPCType, &n.NPCID,
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

// GetAllNPCs returns all NPCs.
func (s *NPCStore) GetAllNPCs() ([]*NPCRecord, error) {
	return s.queryNPCs(`SELECT ` + npcColumns + ` FROM npcs ORDER BY id`)
}

// GetNPC returns the NPC of that row id, or sql.ErrNoRows.
func (s *NPCStore) GetNPC(id int) (*NPCRecord, error) {
	return scanNPC(s.db.QueryRow(`SELECT `+npcColumns+` FROM npcs WHERE id = ?`, id).Scan)
}

// GetNPCsByZone returns all NPCs in a given zone.
func (s *NPCStore) GetNPCsByZone(zone int) ([]*NPCRecord, error) {
	return s.queryNPCs(`SELECT `+npcColumns+` FROM npcs WHERE zone = ? ORDER BY id`, zone)
}

// Count returns the total number of NPCs.
func (s *NPCStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&count)
	return count, err
}

// NPCClassKey is an NPC's name as the npcclass table keys it: trimmed, any
// case, without the trailing '$' some sources mark names with.
func NPCClassKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSpace(strings.TrimSuffix(n, "$"))
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

// NPCClasses is the npcclass table (tools/import_npc_classes.py): each NPC
// name's class, by NPCClassKey. A name it does not hold is a warrior's.
func (s *NPCStore) NPCClasses() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT name, class FROM npcclass`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var name string
		var class int
		if err := rows.Scan(&name, &class); err != nil {
			return nil, err
		}
		out[name] = class
	}
	return out, rows.Err()
}

// NPCClassOf is the npcclass table's class for an NPC name (by NPCClassKey),
// and 0 (a warrior's) when it holds none.
func (s *NPCStore) NPCClassOf(name string) (int, error) {
	var class int
	err := s.db.QueryRow(`SELECT class FROM npcclass WHERE name = ?`, NPCClassKey(name)).Scan(&class)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return class, err
}
