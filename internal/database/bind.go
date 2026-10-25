package database

import "database/sql"

// BindRow is a character's bind point: where a spiritmaster bound it.
type BindRow struct {
	World        int
	X, Y, Z      float32
	Facing       float32
	Spiritmaster string
}

// LoadBind is the character's bind point; ok is false when it never bound.
func (s *SQLiteCharacterStore) LoadBind(serverID uint32) (b BindRow, ok bool, err error) {
	err = s.db.QueryRow(`SELECT world, x, y, z, facing, spiritmaster FROM charbind WHERE serverid = ?`, serverID).
		Scan(&b.World, &b.X, &b.Y, &b.Z, &b.Facing, &b.Spiritmaster)
	if err == sql.ErrNoRows {
		return BindRow{}, false, nil
	}
	return b, err == nil, err
}

// SaveBind keeps the character's bind point, replacing the one before.
func (s *SQLiteCharacterStore) SaveBind(serverID uint32, b BindRow) error {
	_, err := s.db.Exec(`INSERT INTO charbind (serverid, world, x, y, z, facing, spiritmaster) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET world = excluded.world, x = excluded.x, y = excluded.y,
		z = excluded.z, facing = excluded.facing, spiritmaster = excluded.spiritmaster`,
		serverID, b.World, b.X, b.Y, b.Z, b.Facing, b.Spiritmaster)
	return err
}
