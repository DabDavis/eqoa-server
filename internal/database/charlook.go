package database

import (
	"database/sql"
	"fmt"
)

// charlook: a part of a character's body as it chose it at creation, for a race
// of the server's own with options (entity/raceoptions.go): a colour laid on a
// part of its body or a texture on it (its face, its hair). part is the option
// row's key, kind its kind (2 a colour, 3 a texture), value the colour (the
// entity package's Recolour: a mode in the high byte and three bytes) or the
// texture's CHAR.ESF/CHARCUST.CSF DictID. A part with no row is the body's own,
// and a character with none is drawn as it always was; only a client that asked
// is told (entity/raceoptions.go's OpcodeRaceLooks), a PS2 never. The body a
// character chose is characters.modelid, not a row here.
const charLookSchema = `
	CREATE TABLE IF NOT EXISTS charlook (
		serverid INTEGER NOT NULL,
		part     TEXT NOT NULL,
		kind     INTEGER NOT NULL,
		value    INTEGER NOT NULL,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'creation',
		PRIMARY KEY (serverid, part)
	);`

// Kinds of a charlook row (entity.KindColour, entity.KindTexture).
const (
	LookColour  = 2
	LookTexture = 3
)

// CharLook is one row of charlook.
type CharLook struct {
	Part  string
	Kind  int
	Value uint32
}

// ensureCharLook makes table charlook (initSchema).
func (db *SQLiteDB) ensureCharLook() error {
	if _, err := db.Exec(charLookSchema); err != nil {
		return fmt.Errorf("charlook: %w", err)
	}
	return nil
}

// SetCharLooks makes looks serverID's whole look (its rows replaced, in one
// transaction): at creation.
func (db *SQLiteDB) SetCharLooks(serverID uint32, looks []CharLook) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM charlook WHERE serverid = ?`, serverID); err != nil {
		return err
	}
	for _, l := range looks {
		if _, err := tx.Exec(`INSERT INTO charlook (serverid, part, kind, value) VALUES (?, ?, ?, ?)`,
			serverID, l.Part, l.Kind, int64(l.Value)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetSelectedRaceLook saves the selected body and part changes together. Zero
// leaves the body as stored. Reset parts return to the body's default look.
func (db *SQLiteDB) SetSelectedRaceLook(serverID uint32, model int64, reset []string, looks []CharLook) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	changed := false
	exec := func(query string, args ...any) error {
		r, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		changed = changed || n > 0
		return err
	}
	if model != 0 {
		if err := exec(`UPDATE characters SET modelid = ? WHERE serverid = ? AND modelid <> ?`, model, serverID, model); err != nil {
			return false, err
		}
	}
	for _, part := range reset {
		if err := exec(`DELETE FROM charlook WHERE serverid = ? AND part = ?`, serverID, part); err != nil {
			return false, err
		}
	}
	for _, l := range looks {
		if err := exec(`INSERT INTO charlook (serverid, part, kind, value, source) VALUES (?, ?, ?, ?, 'customize')
			ON CONFLICT (serverid, part) DO UPDATE SET kind = excluded.kind, value = excluded.value, source = excluded.source, note = ''
			WHERE charlook.kind <> excluded.kind OR charlook.value <> excluded.value`, serverID, l.Part, l.Kind, int64(l.Value)); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

// CharLooks is serverID's rows, by part.
func (db *SQLiteDB) CharLooks(serverID uint32) ([]CharLook, error) {
	rows, err := db.Query(`SELECT part, kind, value FROM charlook WHERE serverid = ? ORDER BY part`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharLook
	for rows.Next() {
		l, err := scanCharLook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// scanCharLook reads a row's part, kind and value.
func scanCharLook(rows *sql.Rows) (CharLook, error) {
	var l CharLook
	var v int64
	if err := rows.Scan(&l.Part, &l.Kind, &v); err != nil {
		return l, err
	}
	l.Value = uint32(v)
	return l, nil
}

// LookCharacter is a character with charlook rows, as the clients that ask are
// told of it: its name, whether a GM's (its channel name carries the tag), its
// race and sex and its body (characters.modelid), and its rows.
type LookCharacter struct {
	Name    string
	GM      bool
	Sub     bool // a sub GM's (subGMCharacters): its channel name may carry the [Test] tag
	OG      bool // an [OG] character (ogchar.go): its channel name may carry the tag
	HC      bool // a Hardcore character (hardcore.go): its channel name may carry the tag
	Race    int
	Sex     int
	ModelID int64
	Looks   []CharLook
}

// LookCharacters is every character with a charlook row, by name; with
// serverIDs, only those.
func (db *SQLiteDB) LookCharacters(serverIDs ...uint32) ([]LookCharacter, error) {
	q := `SELECT c.charName, COALESCE(a.acctlevel, 0) >= 1, COALESCE(c.og, 0) != 0, COALESCE(c.hardcore, 0) != 0, c.race, c.sex, c.modelid, l.part, l.kind, l.value
		FROM charlook l JOIN characters c ON c.serverid = l.serverid
		LEFT JOIN accountinfo a ON a.accountid = c.accountid`
	var args []any
	if len(serverIDs) > 0 {
		in := make([]byte, 0, 2*len(serverIDs))
		for i, id := range serverIDs {
			if i > 0 {
				in = append(in, ',')
			}
			in = append(in, '?')
			args = append(args, id)
		}
		q += ` WHERE l.serverid IN (` + string(in) + `)`
	}
	rows, err := db.Query(q+` ORDER BY c.charName, l.part`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LookCharacter
	for rows.Next() {
		var c LookCharacter
		var l CharLook
		var v int64
		if err := rows.Scan(&c.Name, &c.GM, &c.OG, &c.HC, &c.Race, &c.Sex, &c.ModelID, &l.Part, &l.Kind, &v); err != nil {
			return nil, err
		}
		l.Value = uint32(v)
		if n := len(out); n > 0 && out[n-1].Name == c.Name {
			out[n-1].Looks = append(out[n-1].Looks, l)
			continue
		}
		c.Looks = []CharLook{l}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // its one connection free for the next query
	subs := db.subGMCharacters()
	for i := range out {
		out[i].Sub = subs[out[i].Name]
	}
	return out, nil
}

// HasCharLook reports whether serverID has a charlook row: its login refreshes
// the clients' lists (an ordinary character's asks nothing more).
func (db *SQLiteDB) HasCharLook(serverID uint32) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charlook WHERE serverid = ?`, serverID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// SetCharLookAudited writes serverID's charlook row of part as a colour
// (LookColour) of value, value 0 deleting it, with a gm_audit line: a GM's
// /aura <colour> hair|gnomecap|glasses (entity/gmlooks.go). Source "gm".
func (db *SQLiteDB) SetCharLookAudited(serverID uint32, part string, value uint32, who GMEdit, detail string) error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	if err := db.ensureCharLook(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	key := map[string]any{"serverid": int64(serverID), "part": part}
	const sel = `SELECT * FROM charlook WHERE serverid = ? AND part = ?`
	before, err := fullRow(tx, sel, serverID, part)
	if err != nil {
		return err
	}
	action := "update"
	switch {
	case value == 0:
		if before == nil {
			return nil
		}
		action = "delete"
		_, err = tx.Exec(`DELETE FROM charlook WHERE serverid = ? AND part = ?`, serverID, part)
	case before == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO charlook (serverid, part, kind, value, source) VALUES (?, ?, ?, ?, 'gm')`, serverID, part, LookColour, int64(value))
	default:
		_, err = tx.Exec(`UPDATE charlook SET kind = ?, value = ?, source = 'gm' WHERE serverid = ? AND part = ?`, LookColour, int64(value), serverID, part)
	}
	if err != nil {
		return err
	}
	after, err := fullRow(tx, sel, serverID, part)
	if err != nil {
		return err
	}
	if _, err := auditRowChange(tx, who, action, "charlook", key, before, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}
