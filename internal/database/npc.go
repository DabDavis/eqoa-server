package database

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

// GetAllNPCs returns all NPCs.
func (s *NPCStore) GetAllNPCs() ([]*NPCRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, COALESCE(npc_name,''), COALESCE(zone,0), COALESCE(unk_a,0), COALESCE(unk_b,0),
			COALESCE(x_coord,0), COALESCE(y_coord,0), COALESCE(z_coord,0),
			COALESCE(facing,0), COALESCE(world,0),
			COALESCE(hp,0), COALESCE(hp_percent,0), COALESCE(modelid,0), COALESCE(size,1.0),
			COALESCE(primary_weapon,0), COALESCE(secondary_weapon,0), COALESCE(shield,0),
			COALESCE(torso,0), COALESCE(forearm,0), COALESCE(gloves,0),
			COALESCE(legs,0), COALESCE(feet,0), COALESCE(head,0),
			COALESCE(hair_color,0), COALESCE(hair_length,0), COALESCE(hair_style,0), COALESCE(face,0),
			COALESCE(robe_style,''), COALESCE(race,''),
			COALESCE(npc_level,1), COALESCE(npc_type,0), COALESCE(npc_id,0)
		FROM npcs ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var npcs []*NPCRecord
	for rows.Next() {
		n := &NPCRecord{}
		err := rows.Scan(
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
		npcs = append(npcs, n)
	}
	return npcs, rows.Err()
}

// GetNPCsByZone returns all NPCs in a given zone.
func (s *NPCStore) GetNPCsByZone(zone int) ([]*NPCRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, COALESCE(npc_name,''), COALESCE(zone,0), COALESCE(unk_a,0), COALESCE(unk_b,0),
			COALESCE(x_coord,0), COALESCE(y_coord,0), COALESCE(z_coord,0),
			COALESCE(facing,0), COALESCE(world,0),
			COALESCE(hp,0), COALESCE(hp_percent,0), COALESCE(modelid,0), COALESCE(size,1.0),
			COALESCE(primary_weapon,0), COALESCE(secondary_weapon,0), COALESCE(shield,0),
			COALESCE(torso,0), COALESCE(forearm,0), COALESCE(gloves,0),
			COALESCE(legs,0), COALESCE(feet,0), COALESCE(head,0),
			COALESCE(hair_color,0), COALESCE(hair_length,0), COALESCE(hair_style,0), COALESCE(face,0),
			COALESCE(robe_style,''), COALESCE(race,''),
			COALESCE(npc_level,1), COALESCE(npc_type,0), COALESCE(npc_id,0)
		FROM npcs WHERE zone = ? ORDER BY id
	`, zone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var npcs []*NPCRecord
	for rows.Next() {
		n := &NPCRecord{}
		err := rows.Scan(
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
		npcs = append(npcs, n)
	}
	return npcs, rows.Err()
}

// Count returns the total number of NPCs.
func (s *NPCStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&count)
	return count, err
}
