package database

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A rehabilitator's templates (handlers/rehabtemplates.go; petition 1322,
// jibrish: "save 2 different rehab state and CM templates ... If they come
// back with 705 cm's and pick that template, it still only spends the 500 and
// just unassigns the other points for them to spend as they wish"). Table
// rehabtemplates is a row a character's slot:
//
//   - serverid and slot (1 or 2, RehabTemplateSlots);
//   - the training points each stat had taken when it was saved, as the
//     characters' tp columns hold them (the create screen's twenty and the
//     trainer's), under the same names;
//   - cms, the Combat Mastery ranks it held, "id:cost" in the order bought
//     (cmcatalog's id, what it paid), comma separated;
//   - the level it was saved at, and when (RFC 3339, UTC).
//
// Using one spends a character's training points and mastery points as it
// keeps them, never more than it spent: what the character holds past that
// stays unspent.

// RehabTemplateSlots is how many templates a character keeps.
const RehabTemplateSlots = 2

// RehabTemplate is a slot's template.
type RehabTemplate struct {
	Slot  int
	Level int
	// TP is the training points each stat had taken, STR..CHA
	TP [7]int
	// CMs is the ranks it held and what each cost, in the order bought
	CMs   []OwnedCM
	Saved string
}

// TPTotal is the training points it spends.
func (t RehabTemplate) TPTotal() int {
	n := 0
	for _, v := range t.TP {
		n += max(v, 0)
	}
	return n
}

// CMTotal is the mastery points it spends.
func (t RehabTemplate) CMTotal() int {
	n := 0
	for _, o := range t.CMs {
		n += max(o.Cost, 0)
	}
	return n
}

// Empty is whether it spends nothing.
func (t RehabTemplate) Empty() bool { return t.TPTotal() == 0 && len(t.CMs) == 0 }

const rehabTemplateSchema = `
CREATE TABLE IF NOT EXISTS rehabtemplates (
	serverid    INTEGER NOT NULL,
	slot        INTEGER NOT NULL,
	level       INTEGER NOT NULL DEFAULT 0,
	tpstrength  INTEGER NOT NULL DEFAULT 0,
	tpstamina   INTEGER NOT NULL DEFAULT 0,
	tpagility   INTEGER NOT NULL DEFAULT 0,
	tpdexterity INTEGER NOT NULL DEFAULT 0,
	tpwisdom    INTEGER NOT NULL DEFAULT 0,
	tpintel     INTEGER NOT NULL DEFAULT 0,
	tpcharisma  INTEGER NOT NULL DEFAULT 0,
	cms         TEXT NOT NULL DEFAULT '',
	saved       TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (serverid, slot)
);`

// ensureRehabTemplates makes the table (ensureRehab).
func (db *SQLiteDB) ensureRehabTemplates() error {
	if _, err := db.Exec(rehabTemplateSchema); err != nil {
		return fmt.Errorf("rehabtemplates: %w", err)
	}
	return nil
}

// FormatTemplateCMs is a template's ranks as its cms column holds them.
func FormatTemplateCMs(cms []OwnedCM) string {
	parts := make([]string, 0, len(cms))
	for _, o := range cms {
		parts = append(parts, fmt.Sprintf("%d:%d", o.ID, o.Cost))
	}
	return strings.Join(parts, ",")
}

// ParseTemplateCMs reads a cms column ("id:cost,id:cost"), in its order: an
// id with no cost costs 0, what is not a positive id is skipped, and an id
// twice counts once (its first).
func ParseTemplateCMs(s string) []OwnedCM {
	var out []OwnedCM
	seen := map[int]bool{}
	for _, f := range strings.Split(s, ",") {
		id, cost, _ := strings.Cut(strings.TrimSpace(f), ":")
		n, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil || n <= 0 || seen[n] {
			continue
		}
		c, _ := strconv.Atoi(strings.TrimSpace(cost))
		seen[n] = true
		out = append(out, OwnedCM{ID: n, Cost: max(c, 0)})
	}
	return out
}

// CheckTemplateCMs is why a cms column is not one the game reads every entry
// of, nil when it is: comma-separated "id:cost", a positive id once each and a
// cost of 0 or more (an empty column is no rank). ParseTemplateCMs skips what
// does not read; a GM's edit is refused instead (gmweb), so no rank is lost.
func CheckTemplateCMs(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	seen := map[int]bool{}
	for _, f := range strings.Split(s, ",") {
		id, cost, ok := strings.Cut(strings.TrimSpace(f), ":")
		n, err := strconv.Atoi(strings.TrimSpace(id))
		if !ok || err != nil || n <= 0 {
			return fmt.Errorf("%q is not id:cost (cmcatalog's id, what it cost)", strings.TrimSpace(f))
		}
		c, err := strconv.Atoi(strings.TrimSpace(cost))
		if err != nil || c < 0 {
			return fmt.Errorf("%q: the cost is a whole number of 0 or more", strings.TrimSpace(f))
		}
		if seen[n] {
			return fmt.Errorf("rank %d is in it twice", n)
		}
		seen[n] = true
	}
	return nil
}

// RehabTemplates is a character's templates by slot; a slot with no row is
// not in the map (nor one outside 1..RehabTemplateSlots).
func (db *SQLiteDB) RehabTemplates(serverID uint32) (map[int]RehabTemplate, error) {
	rows, err := db.Query(`SELECT slot, level, tpstrength, tpstamina, tpagility, tpdexterity, tpwisdom, tpintel,
		tpcharisma, cms, saved FROM rehabtemplates WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]RehabTemplate{}
	for rows.Next() {
		var t RehabTemplate
		var cms string
		if err := rows.Scan(&t.Slot, &t.Level, &t.TP[0], &t.TP[1], &t.TP[2], &t.TP[3], &t.TP[4], &t.TP[5], &t.TP[6],
			&cms, &t.Saved); err != nil {
			return nil, err
		}
		if t.Slot < 1 || t.Slot > RehabTemplateSlots {
			continue
		}
		t.TP = TrainingSpent(t.TP)
		t.CMs = ParseTemplateCMs(cms)
		out[t.Slot] = t
	}
	return out, rows.Err()
}

// SaveRehabTemplate keeps t in its slot, in place of what the slot held.
func (db *SQLiteDB) SaveRehabTemplate(serverID uint32, t RehabTemplate, at time.Time) error {
	if t.Slot < 1 || t.Slot > RehabTemplateSlots {
		return fmt.Errorf("rehabtemplates: slot %d is none of 1..%d", t.Slot, RehabTemplateSlots)
	}
	tp := TrainingSpent(t.TP)
	_, err := db.Exec(`INSERT INTO rehabtemplates (serverid, slot, level, tpstrength, tpstamina, tpagility, tpdexterity,
		tpwisdom, tpintel, tpcharisma, cms, saved) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (serverid, slot) DO UPDATE SET level = excluded.level, tpstrength = excluded.tpstrength,
		tpstamina = excluded.tpstamina, tpagility = excluded.tpagility, tpdexterity = excluded.tpdexterity,
		tpwisdom = excluded.tpwisdom, tpintel = excluded.tpintel, tpcharisma = excluded.tpcharisma,
		cms = excluded.cms, saved = excluded.saved`,
		serverID, t.Slot, t.Level, tp[0], tp[1], tp[2], tp[3], tp[4], tp[5], tp[6], FormatTemplateCMs(t.CMs),
		at.UTC().Format(time.RFC3339))
	return err
}

// deleteRehabTemplates forgets a deleted character's templates: a server id
// made again must not inherit them.
func (db *SQLiteDB) deleteRehabTemplates(serverID uint32) error {
	_, err := db.Exec(`DELETE FROM rehabtemplates WHERE serverid = ?`, serverID)
	return err
}

// OwnedCMsInOrder is the ranks a character bought, in the order bought (a
// template keeps them so; a tie by id).
func (s *SQLiteCharacterStore) OwnedCMsInOrder(serverID uint32) ([]OwnedCM, error) {
	rows, err := s.db.Query(`SELECT cmid, cost FROM charcms WHERE serverid = ? ORDER BY bought, cmid`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OwnedCM
	for rows.Next() {
		var o OwnedCM
		if err := rows.Scan(&o.ID, &o.Cost); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SetOwnedCMs keeps a template used on a character in the world (entity.World.ApplyRespec)
// once the game has it, in one transaction: the ranks it no longer holds
// gone, each it holds now with what it cost (one it held before keeps when
// it was first bought), and its points as they are now.
func (s *SQLiteCharacterStore) SetOwnedCMs(serverID uint32, remove []int, put []OwnedCM, m MasteryRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range remove {
		if _, err := tx.Exec(`DELETE FROM charcms WHERE serverid = ? AND cmid = ?`, serverID, id); err != nil {
			return err
		}
	}
	now := time.Now().Unix()
	for _, o := range put {
		if _, err := tx.Exec(`INSERT INTO charcms (serverid, cmid, cost, bought) VALUES (?, ?, ?, ?)
			ON CONFLICT (serverid, cmid) DO UPDATE SET cost = excluded.cost`, serverID, o.ID, o.Cost, now); err != nil {
			return err
		}
	}
	if err := saveMasteryTx(tx, serverID, m); err != nil {
		return err
	}
	return tx.Commit()
}

// RespendTraining keeps a template's training on a character in the world
// once the game has it (entity.World.ApplyRespec), in one transaction: each tp
// column lowered by what the stat had taken (from, as the row held it) and
// raised by what it takes now (to), and its unspent and total points the
// game's now. The columns move by the difference, not to a value, so points
// a trainer took meanwhile stay theirs (TakeTraining's rule).
func (s *SQLiteCharacterStore) RespendTraining(serverID uint32, from, to [7]int, unused, total int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var d [7]int
	for i := range d {
		d[i] = from[i] - to[i]
	}
	if err := takeTrainingTx(tx, serverID, TrainingReset{Spent: d, Unused: unused, Total: total}); err != nil {
		return err
	}
	return tx.Commit()
}
