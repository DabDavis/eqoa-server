package database

import (
	"database/sql"
	"errors"
	"sync/atomic"
)

// accountbank.go — one bank for every character of an account (world.account_bank;
// the server owner, 2026-11-14: "make the heirloom server bank for players be account
// wide and increase capacity to 2000"). Off: every character has its own bank, as
// before.
//
// On:
//   - The bank's ITEMS are the charinventory rows at location 1 (LocBank) of every
//     character of the account, and of its characters deleted since (accountbank_holders:
//     a character's server id is AUTOINCREMENT, never made again). A row keeps the
//     serverid of the character that put it there (its owner before the merge, which is
//     how tools/account_bank_20261114.py undoes it); a withdrawal makes the row the
//     withdrawer's. So every query of a bank row names the ACCOUNT's rows (bankCond),
//     every query of a held row the character's pack and the account's bank (heldCond).
//   - The bank's TUNAR is accountbank.tunar (one row an account); characters.bankTunar
//     is not read (the merge summed it there and logged each).
//   - It holds AccountBankSlots records (2000 unless world.account_bank_slots says): no
//     client shows that many (CLIENT's VIItemList holds 40; vanilla up to
//     world.bank_slots, at most 255, its 0x1250 naming a record by a byte), so a session's
//     bank list is a PAGE of the account's bank (handlers/accountbank.go).
//   - The kept bank rows (LocBankKept) are not made: the page is the session's. Rows kept
//     before are brought back at a login (handlers/itemlists.go) or by the merge.

// AccountBankDefaultSlots is the records an account's bank holds unless
// world.account_bank_slots says: the server owner's 2000.
const AccountBankDefaultSlots = 2000

var (
	acctBankOn    atomic.Bool
	acctBankSlots atomic.Int32
)

func init() { acctBankSlots.Store(AccountBankDefaultSlots) }

// SetAccountBank turns the account bank on or off (world.account_bank) and
// sets the records it holds (world.account_bank_slots; 0 or less:
// AccountBankDefaultSlots), at the start, from config.
func SetAccountBank(on bool, slots int) {
	if slots <= 0 {
		slots = AccountBankDefaultSlots
	}
	acctBankSlots.Store(int32(slots))
	acctBankOn.Store(on)
}

// AccountBankOn is whether every character of an account shares one bank.
func AccountBankOn() bool { return acctBankOn.Load() }

// AccountBankSlots is the records an account's bank holds (AccountBankOn).
func AccountBankSlots() int { return int(acctBankSlots.Load()) }

// accountBankSchema is the account bank's tables: its tunar, and the deleted
// characters whose bank rows are still the account's.
const accountBankSchema = `
	CREATE TABLE IF NOT EXISTS accountbank (
		accountid INTEGER PRIMARY KEY,
		tunar     INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS accountbank_holders (
		serverid   INTEGER PRIMARY KEY,
		accountid  INTEGER NOT NULL,
		deleted_at INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_accountbank_holders_account ON accountbank_holders(accountid);
`

// bankOwnersSQL is the serverids whose bank rows are the bank of the
// character named by its two ?s (its server id, twice): its account's
// characters and those of them deleted since.
const bankOwnersSQL = `(SELECT o.serverid FROM characters o WHERE o.accountid = (SELECT a.accountid FROM characters a WHERE a.serverid = ?)
	UNION ALL SELECT h.serverid FROM accountbank_holders h WHERE h.accountid = (SELECT a.accountid FROM characters a WHERE a.serverid = ?))`

// bankCond is the SQL condition on a charinventory row (its columns prefixed
// p, "c." or "") being in the character's bank, and its arguments: its own
// rows at location 1, or with the account bank its account's.
func bankCond(p string, serverID uint32) (string, []any) {
	if !AccountBankOn() {
		return p + "serverid = ? AND " + p + "location = 1", []any{serverID}
	}
	return p + "location = 1 AND " + p + "serverid IN " + bankOwnersSQL, []any{serverID, serverID}
}

// heldCond is the SQL condition on a row being one the character holds: in its
// pack (worn or carried) or in its bank (bankCond), never one kept for a larger
// client (itemlists.go).
func heldCond(p string, serverID uint32) (string, []any) {
	if !AccountBankOn() {
		return p + "serverid = ? AND " + p + "location IN (0, 1)", []any{serverID}
	}
	bank, args := bankCond(p, serverID)
	return "((" + p + "serverid = ? AND " + p + "location = 0) OR (" + bank + "))", append([]any{serverID}, args...)
}

// nextListSQL is the listnumber a new row at location takes (the end of its
// list) and its arguments: the end of the account's bank for the bank with
// the account bank, else of the character's own list.
func nextListSQL(serverID uint32, location int) (string, []any) {
	if location == LocBank && AccountBankOn() {
		cond, args := bankCond("", serverID)
		return `(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE ` + cond + `)`, args
	}
	return `(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?)`, []any{serverID, location}
}

// BankRowCount is how many records the character's bank holds (its account's
// with the account bank).
func BankRowCount(q sqlRunner, serverID uint32) (int, error) {
	cond, args := bankCond("", serverID)
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE `+cond, args...).Scan(&n)
	return n, err
}

// BankRowCount is BankRowCount on the store's database.
func (s *InventoryStore) BankRowCount(serverID uint32) (int, error) {
	return BankRowCount(s.db, serverID)
}

// accountOf is the character's account, and whether it is there.
func accountOfChar(q sqlRunner, serverID uint32) (int, bool, error) {
	var acct int
	err := q.QueryRow(`SELECT accountid FROM characters WHERE serverid = ?`, serverID).Scan(&acct)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return acct, err == nil, err
}

// AccountOf is the character's account, and whether it is there.
func (s *InventoryStore) AccountOf(serverID uint32) (int, bool, error) {
	return accountOfChar(s.db, serverID)
}

// accountTunarTx is an account's bank tunar (0 for one with no row yet).
func accountTunarTx(q sqlRunner, accountID int) (int, error) {
	var n int
	err := q.QueryRow(`SELECT COALESCE((SELECT tunar FROM accountbank WHERE accountid = ?), 0)`, accountID).Scan(&n)
	return n, err
}

// addBankTunarTx adds n tunar (negative: takes away) to the character's bank
// (its account's with the account bank); with floor, only while the bank holds
// at least -n. ok false: the character is not there, or (floor) the bank holds
// too little, and nothing changed.
func addBankTunarTx(q sqlRunner, serverID uint32, n int, floor bool) (bool, error) {
	if !AccountBankOn() {
		query := `UPDATE characters SET bankTunar = bankTunar + ? WHERE serverid = ?`
		args := []any{n, serverID}
		if floor {
			query += ` AND bankTunar + ? >= 0`
			args = append(args, n)
		}
		res, err := q.Exec(query, args...)
		if err != nil {
			return false, err
		}
		k, err := res.RowsAffected()
		return k == 1, err
	}
	acct, there, err := accountOfChar(q, serverID)
	if err != nil || !there {
		return false, err
	}
	if _, err := q.Exec(`INSERT OR IGNORE INTO accountbank (accountid, tunar) VALUES (?, 0)`, acct); err != nil {
		return false, err
	}
	query := `UPDATE accountbank SET tunar = tunar + ? WHERE accountid = ?`
	args := []any{n, acct}
	if floor {
		query += ` AND tunar + ? >= 0`
		args = append(args, n)
	}
	res, err := q.Exec(query, args...)
	if err != nil {
		return false, err
	}
	k, err := res.RowsAffected()
	return k == 1, err
}

// setBankTunarTx sets the character's bank tunar (its account's with the
// account bank). ok false: the character is not there.
func setBankTunarTx(q sqlRunner, serverID uint32, bank int) (bool, error) {
	if !AccountBankOn() {
		res, err := q.Exec(`UPDATE characters SET bankTunar = ? WHERE serverid = ?`, bank, serverID)
		if err != nil {
			return false, err
		}
		k, err := res.RowsAffected()
		return k == 1, err
	}
	acct, there, err := accountOfChar(q, serverID)
	if err != nil || !there {
		return false, err
	}
	_, err = q.Exec(`INSERT INTO accountbank (accountid, tunar) VALUES (?, ?)
		ON CONFLICT(accountid) DO UPDATE SET tunar = excluded.tunar`, acct, bank)
	return err == nil, err
}

// AddBankTunarOf adds n to the character's bank tunar (floor: only while it
// holds at least -n) and returns what the bank holds after it; ok false: no
// change (no character, or too little with floor).
func (s *InventoryStore) AddBankTunarOf(serverID uint32, n int, floor bool) (after int, ok bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	if ok, err = addBankTunarTx(tx, serverID, n, floor); err != nil || !ok {
		return 0, false, err
	}
	if after, _, err = bankTunarTx(tx, serverID); err != nil {
		return 0, false, err
	}
	return after, true, tx.Commit()
}

// fixBankTunar puts the account's bank tunar in each record, with the account
// bank (a character's own bankTunar is not read then).
func (s *SQLiteCharacterStore) fixBankTunar(cs ...*CharacterRecord) {
	if !AccountBankOn() {
		return
	}
	byAcct := map[int]int{}
	for _, c := range cs {
		if c == nil {
			continue
		}
		c.OwnBankTunar = c.BankTunar
		n, ok := byAcct[c.AccountID]
		if !ok {
			var err error
			if n, err = accountTunarTx(s.db, c.AccountID); err != nil {
				continue
			}
			byAcct[c.AccountID] = n
		}
		c.BankTunar = n
	}
}

// keepBankRowsTx keeps a character's bank rows as its account's when it is
// deleted, with the account bank: it is named a holder (accountbank_holders),
// so its rows stay in the bank of the characters left and of those made later.
func keepBankRowsTx(q sqlRunner, serverID uint32, now int64) error {
	if !AccountBankOn() {
		return nil
	}
	_, err := q.Exec(`INSERT OR IGNORE INTO accountbank_holders (serverid, accountid, deleted_at)
		SELECT c.serverid, c.accountid, ? FROM characters c WHERE c.serverid = ?
			AND EXISTS (SELECT 1 FROM charinventory i WHERE i.serverid = c.serverid AND i.location IN (1, 3))`, now, serverID)
	return err
}

// heldOf is heldCond on unprefixed columns (an UPDATE or DELETE of
// charinventory), heldOfC on "c."; heldArgsOf their arguments.
func heldOf(serverID uint32) string {
	c, _ := heldCond("", serverID)
	return c
}

func heldOfC(serverID uint32) string {
	c, _ := heldCond("c.", serverID)
	return c
}

func heldArgsOf(serverID uint32) []any {
	_, a := heldCond("", serverID)
	return a
}

// GetBankItems is the character's bank (its account's with the account bank),
// joined with the patterns as the character views them, in list order
// (listnumber, itemid): what a page of it is cut from (handlers/accountbank.go).
func (s *InventoryStore) GetBankItems(serverID uint32) ([]CharInventoryEntry, error) {
	cond, args := bankCond("c.", serverID)
	rows, err := s.db.Query(`SELECT c.itemid, c.stackleft, c.remainHP, c.remaincharge, c.equiploc, c.location, c.listnumber,
		`+itemPatternColumns+`
		FROM charinventory c JOIN itempattern p ON c.patternid = p.patternid
		WHERE `+cond+` ORDER BY c.listnumber, c.itemid`, args...)
	if err != nil {
		return nil, err
	}
	var out []CharInventoryEntry
	for rows.Next() {
		e, err := scanItemRow(rows.Scan)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		s.db.ViewPattern(serverID, &out[i].Pattern) // heirlooms.go: the character's own view
	}
	return out, nil
}

// BankTunarOf is the character's bank tunar (its account's with the account
// bank), and whether it is there.
func (s *InventoryStore) BankTunarOf(serverID uint32) (int, bool, error) {
	return bankTunarTx(s.db, serverID)
}
