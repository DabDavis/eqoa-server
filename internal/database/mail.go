package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// mail.go — the tell-grams (handlers/mail.go) kept, and what they carry (the
// owner, 2026-11-19: "the mail system should let players mail each other
// armor, tunar, etc."). Until then a gram lived in the handler's memory and was
// gone at a restart; a gram that carries an item or tunar cannot be, so every
// gram is a row here and the handler reads them all at its start.
//
//   - mail_grams: one copy of a gram, in one character's box (owner: the name,
//     lower case, as the boxes are keyed; owner_id the character it was sent
//     to, so a new character of a freed name takes nothing attached to the old
//     one's mail). recipients is every name it went to, ';' between.
//   - mail_attachments: what a gram carries, held by the server from the send
//     to the take: tunar, or n of an item record with its condition and
//     charges (the item's own row is gone from the sender's pack: there is no
//     copy anywhere else to duplicate).
//   - mail_attachment_log: every send, take, return and drop, for the GMs.
//
// A send takes from the sender and files the gram and its attachments in one
// transaction (SendGramAttached); a take deletes the attachments and gives them
// in one (TakeGramAttachments), refused when they are no longer all there (a
// second take); a delete of a gram still carrying something returns it to its
// sender in one (ReturnGram). A crash or a keep restart between any two of a
// transaction's statements leaves the database as it was before it.

// MailSchema is the tables.
const MailSchema = `
	CREATE TABLE IF NOT EXISTS mail_grams (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		owner      TEXT NOT NULL,
		owner_id   INTEGER NOT NULL DEFAULT 0,
		sender     TEXT NOT NULL DEFAULT '',
		sender_id  INTEGER NOT NULL DEFAULT 0,
		recipients TEXT NOT NULL DEFAULT '',
		subject    TEXT NOT NULL DEFAULT '',
		body       TEXT NOT NULL DEFAULT '',
		sent       INTEGER NOT NULL DEFAULT 0,
		seen       INTEGER NOT NULL DEFAULT 0,
		returned   INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS mail_grams_owner ON mail_grams(owner);
	CREATE TABLE IF NOT EXISTS mail_attachments (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		gramid    INTEGER NOT NULL,
		tunar     INTEGER NOT NULL DEFAULT 0,
		patternid INTEGER NOT NULL DEFAULT 0,
		stack     INTEGER NOT NULL DEFAULT 0,
		remainhp  INTEGER NOT NULL DEFAULT 0,
		charges   INTEGER NOT NULL DEFAULT 0,
		itemname  TEXT NOT NULL DEFAULT '',
		from_id   INTEGER NOT NULL DEFAULT 0,
		from_name TEXT NOT NULL DEFAULT '',
		to_id     INTEGER NOT NULL DEFAULT 0,
		to_name   TEXT NOT NULL DEFAULT '',
		sent      INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS mail_attachments_gram ON mail_attachments(gramid);
	CREATE TABLE IF NOT EXISTS mail_attachment_log (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		at        INTEGER NOT NULL,
		event     TEXT NOT NULL,
		gramid    INTEGER NOT NULL DEFAULT 0,
		from_id   INTEGER NOT NULL DEFAULT 0,
		from_name TEXT NOT NULL DEFAULT '',
		to_id     INTEGER NOT NULL DEFAULT 0,
		to_name   TEXT NOT NULL DEFAULT '',
		tunar     INTEGER NOT NULL DEFAULT 0,
		patternid INTEGER NOT NULL DEFAULT 0,
		stack     INTEGER NOT NULL DEFAULT 0,
		itemname  TEXT NOT NULL DEFAULT ''
	);`

func (db *SQLiteDB) ensureMail() error {
	if _, err := db.Exec(MailSchema); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	return nil
}

// Gram is one copy of a gram, in Owner's box.
type Gram struct {
	ID       uint32
	Owner    string // the box: the recipient's name, lower case
	OwnerID  uint32 // the recipient character, 0 not known
	From     string
	FromID   uint32
	To       []string // every name it went to
	Subject  string
	Text     string
	Sent     time.Time
	Read     bool
	Returned bool // its attachments came back to their sender
}

// GramAttachment is what a gram carries: tunar, or Stack of an item record.
type GramAttachment struct {
	ID        int64
	GramID    uint32
	Tunar     int
	PatternID int // 0: tunar
	Stack     int
	RemainHP  int
	Charges   int
	ItemName  string
	FromID    uint32
	FromName  string
	ToID      uint32
	ToName    string
}

// Item is whether the attachment is an item (else tunar).
func (a GramAttachment) Item() bool { return a.PatternID > 0 }

// The refusals the transactions give.
var (
	// ErrGramItemGone is a send's item no longer in the sender's pack as the
	// send names it (sold, worn, moved, fewer than asked).
	ErrGramItemGone = errors.New("the item is no longer in the pack as it was")
	// ErrGramTaken is a take of attachments no longer all on the gram (taken
	// already, returned).
	ErrGramTaken = errors.New("what was attached is no longer there")
	// ErrGramGone is a gram not in the box named.
	ErrGramGone = errors.New("no such gram")
)

const gramColumns = `id, owner, owner_id, sender, sender_id, recipients, subject, body, sent, seen, returned`

func scanGram(sc interface{ Scan(...any) error }) (Gram, error) {
	var g Gram
	var to string
	var sent int64
	var seen, returned int
	if err := sc.Scan(&g.ID, &g.Owner, &g.OwnerID, &g.From, &g.FromID, &to, &g.Subject, &g.Text, &sent, &seen, &returned); err != nil {
		return g, err
	}
	if to != "" {
		g.To = strings.Split(to, ";")
	}
	g.Sent = time.Unix(sent, 0)
	g.Read, g.Returned = seen != 0, returned != 0
	return g, nil
}

// Grams is every gram kept, by id.
func (db *SQLiteDB) Grams() ([]Gram, error) {
	rows, err := db.Query(`SELECT ` + gramColumns + ` FROM mail_grams ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Gram
	for rows.Next() {
		g, err := scanGram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertGram(x execer, g *Gram) error {
	res, err := x.Exec(`INSERT INTO mail_grams (owner, owner_id, sender, sender_id, recipients, subject, body, sent, seen, returned)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(g.Owner), g.OwnerID, g.From, g.FromID, strings.Join(g.To, ";"), g.Subject, g.Text, g.Sent.Unix(),
		b2i(g.Read), b2i(g.Returned))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	g.ID = uint32(id)
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// AddGram files a gram (its ID set from the row).
func (db *SQLiteDB) AddGram(g *Gram) error { return insertGram(db, g) }

// SetGramRead marks a gram read.
func (db *SQLiteDB) SetGramRead(id uint32) error {
	_, err := db.Exec(`UPDATE mail_grams SET seen = 1 WHERE id = ?`, id)
	return err
}

// DeleteGram deletes a gram that carries nothing: one with attachments is
// returned (ReturnGram) or refused, never deleted with them.
func (db *SQLiteDB) DeleteGram(id uint32) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM mail_attachments WHERE gramid = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("gram %d carries %d attachments", id, n)
	}
	if _, err := tx.Exec(`DELETE FROM mail_grams WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

const attachmentColumns = `id, gramid, tunar, patternid, stack, remainhp, charges, itemname, from_id, from_name, to_id, to_name`

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func gramAttachments(q queryer, where string, args ...any) ([]GramAttachment, error) {
	rows, err := q.Query(`SELECT `+attachmentColumns+` FROM mail_attachments WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GramAttachment
	for rows.Next() {
		var a GramAttachment
		if err := rows.Scan(&a.ID, &a.GramID, &a.Tunar, &a.PatternID, &a.Stack, &a.RemainHP, &a.Charges, &a.ItemName,
			&a.FromID, &a.FromName, &a.ToID, &a.ToName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GramAttachments is what a gram carries.
func (db *SQLiteDB) GramAttachments(gramID uint32) ([]GramAttachment, error) {
	return gramAttachments(db, `gramid = ?`, gramID)
}

// AllGramAttachments is every attachment held, by gram.
func (db *SQLiteDB) AllGramAttachments() (map[uint32][]GramAttachment, error) {
	list, err := gramAttachments(db, `1`)
	if err != nil {
		return nil, err
	}
	out := map[uint32][]GramAttachment{}
	for _, a := range list {
		out[a.GramID] = append(out[a.GramID], a)
	}
	return out, nil
}

// GramRecipient is a character a gram may be addressed to, by name.
type GramRecipient struct {
	ServerID  uint32
	Name      string // as the character is named
	Hardcore  bool   // characters.hardcore (world.hardcore decides whether it binds)
	AccountID int
}

// GramRecipientNamed is the character of a name (any case); ok false none.
func (db *SQLiteDB) GramRecipientNamed(name string) (GramRecipient, bool, error) {
	var r GramRecipient
	var hc int
	err := db.QueryRow(`SELECT serverid, charName, COALESCE(hardcore, 0), accountid FROM characters WHERE LOWER(charName) = LOWER(?)`, name).
		Scan(&r.ServerID, &r.Name, &hc, &r.AccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	r.Hardcore = hc != 0
	return r, err == nil, err
}

// LoreHeldOrComing is whether a character holds one of an item (its pack, worn,
// kept, its bank or its account's) or has one waiting on a gram: a Lore item
// that cannot be sent it.
func (db *SQLiteDB) LoreHeldOrComing(serverID uint32, patternID int) (bool, error) {
	cond, args := heldCond("", serverID)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE patternid = ? AND (serverid = ? OR `+cond+`)`,
		append([]any{patternID, serverID}, args...)...).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM mail_attachments WHERE to_id = ? AND patternid = ?`, serverID, patternID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// GramItemTake is n of a pack row going onto a gram.
type GramItemTake struct {
	ItemID    int
	N         int
	PatternID int
	ItemName  string
}

// GramSendAttached is a gram with attachments, as SendGramAttached files it:
// the recipient's copy, the sender's purse once the tunar and the postage are
// out of it, and the item.
type GramSendAttached struct {
	Gram    Gram // the recipient's copy (Owner, OwnerID, From, FromID, ...)
	ToName  string
	Purse   int // the sender's purse after
	Tunar   int // tunar attached
	Postage int // tunar the post took (gone)
	Item    *GramItemTake
}

// SendGramAttached files x in one transaction: the item's n off the sender's
// pack row (the row deleted, and off its quick slot, when all of it goes), the
// sender's purse set, the gram and its attachments written, each logged.
// Nothing changes when the row is not in the pack as x names it
// (ErrGramItemGone). x.Gram.ID is set.
func (s *InventoryStore) SendGramAttached(x *GramSendAttached) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	g := &x.Gram
	now := g.Sent.Unix()
	var item GramAttachment
	if x.Item != nil {
		var stack int
		err := tx.QueryRow(`SELECT stackleft, remainHP, remaincharge FROM charinventory
			WHERE itemid = ? AND serverid = ? AND patternid = ? AND location = ? AND equiploc < 0`,
			x.Item.ItemID, g.FromID, x.Item.PatternID, LocPack).Scan(&stack, &item.RemainHP, &item.Charges)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (x.Item.N < 1 || stack < x.Item.N)) {
			return ErrGramItemGone
		}
		if err != nil {
			return err
		}
		if stack == x.Item.N {
			if _, err := tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, x.Item.ItemID, g.FromID); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, g.FromID, x.Item.ItemID); err != nil &&
				!isNoSuchTable(err) {
				return err
			}
		} else if _, err := tx.Exec(`UPDATE charinventory SET stackleft = ? WHERE itemid = ? AND serverid = ?`,
			stack-x.Item.N, x.Item.ItemID, g.FromID); err != nil {
			return err
		}
		item.PatternID, item.Stack, item.ItemName = x.Item.PatternID, x.Item.N, x.Item.ItemName
	}
	if x.Tunar > 0 || x.Postage > 0 {
		if x.Purse < 0 {
			return fmt.Errorf("a purse of %d", x.Purse)
		}
		if _, err := tx.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, x.Purse, g.FromID); err != nil {
			return err
		}
	}
	if err := insertGram(tx, g); err != nil {
		return err
	}
	var atts []GramAttachment
	if x.Tunar > 0 {
		atts = append(atts, GramAttachment{Tunar: x.Tunar})
	}
	if x.Item != nil {
		atts = append(atts, item)
	}
	for _, a := range atts {
		if _, err := tx.Exec(`INSERT INTO mail_attachments (gramid, tunar, patternid, stack, remainhp, charges, itemname,
				from_id, from_name, to_id, to_name, sent) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			g.ID, a.Tunar, a.PatternID, a.Stack, a.RemainHP, a.Charges, a.ItemName, g.FromID, g.From, g.OwnerID, x.ToName, now); err != nil {
			return err
		}
		if err := logGramAttachment(tx, now, "sent", g.ID, g.FromID, g.From, g.OwnerID, x.ToName, a); err != nil {
			return err
		}
	}
	if x.Postage > 0 {
		if err := logGramAttachment(tx, now, "postage", g.ID, g.FromID, g.From, 0, "", GramAttachment{Tunar: x.Postage}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func logGramAttachment(x execer, at int64, event string, gramID, fromID uint32, from string, toID uint32, to string, a GramAttachment) error {
	_, err := x.Exec(`INSERT INTO mail_attachment_log (at, event, gramid, from_id, from_name, to_id, to_name, tunar, patternid, stack, itemname)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, at, event, gramID, fromID, from, toID, to, a.Tunar, a.PatternID, a.Stack, a.ItemName)
	return err
}

// GramTake is a take of a gram's attachments: which (their ids, as read
// before), the taker, its purse before, and where an item goes: onto the
// stack of pack row Onto, or a new row of the pack (0).
type GramTake struct {
	GramID uint32
	IDs    []int64
	ToID   uint32
	ToName string
	Purse  int
	Onto   int
	At     time.Time
}

// GramTaken is what a take gave: the tunar, the item (its row: Onto, or the
// new one) and the purse after.
type GramTaken struct {
	Tunar  int
	Item   *GramAttachment
	ItemID int
	Purse  int
}

// TakeGramAttachments gives the taker the attachments t names in one
// transaction: they are deleted from the gram (all of them, or nothing: a
// second take, or one after a return, is ErrGramTaken), the tunar added to the
// purse, an item onto the stack of row Onto or as a new pack row, each logged.
// Only the gram's own recipient takes (ErrGramGone else).
func (s *InventoryStore) TakeGramAttachments(t GramTake) (GramTaken, error) {
	var out GramTaken
	if len(t.IDs) == 0 {
		return out, ErrGramTaken
	}
	tx, err := s.db.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var owner uint32
	if err := tx.QueryRow(`SELECT owner_id FROM mail_grams WHERE id = ?`, t.GramID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrGramGone
		}
		return out, err
	}
	if owner != t.ToID {
		return out, ErrGramGone
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(t.IDs)), ",")
	args := []any{t.GramID}
	for _, id := range t.IDs {
		args = append(args, id)
	}
	atts, err := gramAttachments(tx, `gramid = ? AND id IN (`+ph+`)`, args...)
	if err != nil {
		return out, err
	}
	if len(atts) != len(t.IDs) {
		return out, ErrGramTaken
	}
	res, err := tx.Exec(`DELETE FROM mail_attachments WHERE gramid = ? AND id IN (`+ph+`)`, args...)
	if err != nil {
		return out, err
	}
	if n, err := res.RowsAffected(); err != nil || n != int64(len(t.IDs)) {
		return out, ErrGramTaken
	}
	out.Purse = t.Purse
	now := t.At.Unix()
	for i := range atts {
		a := atts[i]
		if !a.Item() {
			out.Tunar += a.Tunar
		} else {
			if out.Item != nil {
				return out, fmt.Errorf("gram %d carries more than one item", t.GramID)
			}
			if t.Onto > 0 {
				r, err := tx.Exec(`UPDATE charinventory SET stackleft = stackleft + ? WHERE itemid = ? AND serverid = ? AND patternid = ? AND location = ?`,
					a.Stack, t.Onto, t.ToID, a.PatternID, LocPack)
				if err != nil {
					return out, err
				}
				if n, _ := r.RowsAffected(); n != 1 {
					return out, fmt.Errorf("row %d to stack onto is gone", t.Onto)
				}
				out.ItemID = t.Onto
			} else {
				r, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
					VALUES (?, ?, ?, ?, ?, -1, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))`,
					t.ToID, a.Stack, a.RemainHP, a.Charges, a.PatternID, LocPack, t.ToID, LocPack)
				if err != nil {
					return out, err
				}
				id, err := r.LastInsertId()
				if err != nil {
					return out, err
				}
				out.ItemID = int(id)
			}
			out.Item = &a
		}
		if err := logGramAttachment(tx, now, "taken", t.GramID, a.FromID, a.FromName, t.ToID, t.ToName, a); err != nil {
			return out, err
		}
	}
	if out.Tunar > 0 {
		out.Purse += out.Tunar
		if _, err := tx.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, out.Purse, t.ToID); err != nil {
			return out, err
		}
	}
	return out, tx.Commit()
}

// ReturnGram deletes gram id from owner's box and, in the same transaction,
// files back what it carries in a gram to its sender, back (its Returned set;
// back.ID is set): the attachments move to it. With no sender to return to
// (back nil: the character is gone) they are deleted, logged as dropped. A gram
// carrying nothing is simply deleted.
func (db *SQLiteDB) ReturnGram(id uint32, owner string, back *Gram) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM mail_grams WHERE id = ? AND owner = ?`, id, strings.ToLower(owner)).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrGramGone
	}
	atts, err := gramAttachments(tx, `gramid = ?`, id)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if len(atts) > 0 {
		if back != nil {
			back.Returned = true
			if err := insertGram(tx, back); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE mail_attachments SET gramid = ?, to_id = ?, to_name = ? WHERE gramid = ?`,
				back.ID, back.OwnerID, back.Owner, id); err != nil {
				return err
			}
		} else if _, err := tx.Exec(`DELETE FROM mail_attachments WHERE gramid = ?`, id); err != nil {
			return err
		}
		event := "returned"
		if back == nil {
			event = "dropped"
		}
		for _, a := range atts {
			// from the recipient who deleted it, back to its sender
			if err := logGramAttachment(tx, now, event, id, a.ToID, a.ToName, a.FromID, a.FromName, a); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM mail_grams WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// GramAttachmentLog is the log's rows, the newest first, at most limit (for
// the GM panel and the tests).
func (db *SQLiteDB) GramAttachmentLog(limit int) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT at, event, gramid, from_id, from_name, to_id, to_name, tunar, patternid, stack, itemname
		FROM mail_attachment_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var at int64
		var event, from, to, item string
		var gram, fromID, toID uint32
		var tunar, pid, stack int
		if err := rows.Scan(&at, &event, &gram, &fromID, &from, &toID, &to, &tunar, &pid, &stack, &item); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"at": at, "event": event, "gram": gram, "from_id": fromID, "from": from,
			"to_id": toID, "to": to, "tunar": tunar, "patternid": pid, "stack": stack, "item": item})
	}
	return out, rows.Err()
}

// VanishesAtLogout is whether an item record is one norentvanish names (No
// Rent by a GM's word: it vanishes at a logout), which no gram carries.
func (db *SQLiteDB) VanishesAtLogout(patternID int) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM norentvanish WHERE patternid = ?`, patternID).Scan(&n); err != nil {
		if isNoSuchTable(err) {
			return false, nil
		}
		return false, err
	}
	return n > 0, nil
}
