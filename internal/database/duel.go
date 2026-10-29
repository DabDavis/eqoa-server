package database

import (
	"database/sql"
	"fmt"
	"time"
)

// A duel's wager (entity/duel.go): the tunar each of its two staked out of
// the purse (SOE's live client stakes the carried tunar: its wager's deposit
// window takes the purse as its limit, live VIWndDeposit::Setup mode 4),
// held in duel_stakes while the duel is fought, and paid out at its end. Every
// move is one transaction, so a stake is always in exactly one place: the
// purse, the table, or the winner's purse. A server that stops with stakes
// held gives each back to its owner at its next start (DuelRefundHeld).

const duelStakesSchema = `
	CREATE TABLE IF NOT EXISTS duel_stakes (
		duel     INTEGER NOT NULL,
		serverid INTEGER NOT NULL,
		tunar    INTEGER NOT NULL,
		held     INTEGER NOT NULL,
		PRIMARY KEY (duel, serverid)
	);`

// ensureDuelStakes makes table duel_stakes (initSchema).
func (db *SQLiteDB) ensureDuelStakes() error {
	if _, err := db.Exec(duelStakesSchema); err != nil {
		return fmt.Errorf("duel_stakes: %w", err)
	}
	return nil
}

// DuelStake is one side's stake as it is held: the character, the tunar
// staked and its purse once the stake is out of it (the server's own figure,
// written as the purse is everywhere else).
type DuelStake struct {
	ServerID uint32
	Tunar    int
	PurseNow int
}

// DuelPay is one side's share at a duel's end: the tunar it is paid and, for
// a character in the world, its purse once paid (the server's figure); a
// character no longer in the world (Online false) is paid onto its row.
type DuelPay struct {
	ServerID uint32
	Tunar    int
	Online   bool
	PurseNow int
}

// duelTx runs fn in one transaction, rolled back on any error.
func (db *SQLiteDB) duelTx(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func execOneDuel(tx *sql.Tx, query string, args ...any) error {
	res, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return fmt.Errorf("%d rows changed, want 1", n)
	}
	return nil
}

// DuelHold puts a duel's stakes in duel_stakes and the purses they came out
// of, in one transaction. A duel already holding stakes is refused.
func (db *SQLiteDB) DuelHold(duel uint32, stakes []DuelStake, now time.Time) error {
	return db.duelTx(func(tx *sql.Tx) error {
		for _, s := range stakes {
			if s.Tunar < 0 || s.PurseNow < 0 {
				return fmt.Errorf("duel %d: %d staked, %d left for %d", duel, s.Tunar, s.PurseNow, s.ServerID)
			}
			if err := execOneDuel(tx, `UPDATE characters SET tunar = ? WHERE serverid = ?`, s.PurseNow, s.ServerID); err != nil {
				return fmt.Errorf("duel %d: purse of %d: %w", duel, s.ServerID, err)
			}
			if err := execOneDuel(tx, `INSERT INTO duel_stakes (duel, serverid, tunar, held) VALUES (?, ?, ?, ?)`,
				duel, s.ServerID, s.Tunar, now.Unix()); err != nil {
				return fmt.Errorf("duel %d: stake of %d: %w", duel, s.ServerID, err)
			}
		}
		return nil
	})
}

// DuelSettle pays a duel's held stakes out and forgets them, in one
// transaction: what each is paid must be what the duel holds, and nothing is
// paid for a duel that holds nothing (settled already, or never held).
func (db *SQLiteDB) DuelSettle(duel uint32, pays []DuelPay) error {
	return db.duelTx(func(tx *sql.Tx) error {
		var held, paid int
		if err := tx.QueryRow(`SELECT COALESCE(SUM(tunar), 0) FROM duel_stakes WHERE duel = ?`, duel).Scan(&held); err != nil {
			return err
		}
		for _, p := range pays {
			paid += p.Tunar
		}
		if paid != held {
			return fmt.Errorf("duel %d holds %d, pays %d", duel, held, paid)
		}
		if held == 0 {
			_, err := tx.Exec(`DELETE FROM duel_stakes WHERE duel = ?`, duel)
			return err
		}
		for _, p := range pays {
			if p.Tunar == 0 {
				continue
			}
			var err error
			if p.Online {
				err = execOneDuel(tx, `UPDATE characters SET tunar = ? WHERE serverid = ?`, p.PurseNow, p.ServerID)
			} else {
				err = execOneDuel(tx, `UPDATE characters SET tunar = tunar + ? WHERE serverid = ?`, p.Tunar, p.ServerID)
			}
			if err != nil {
				return fmt.Errorf("duel %d: paying %d: %w", duel, p.ServerID, err)
			}
		}
		_, err := tx.Exec(`DELETE FROM duel_stakes WHERE duel = ?`, duel)
		return err
	})
}

// DuelForget drops a duel's held stakes without paying them: for a duel whose
// payment reached the purses another way (the last resort when DuelSettle
// failed after the server's own figures were paid; a refund at the next start
// would pay them twice).
func (db *SQLiteDB) DuelForget(duel uint32) error {
	_, err := db.Exec(`DELETE FROM duel_stakes WHERE duel = ?`, duel)
	return err
}

// DuelRefundHeld gives every stake still held back to its owner (a server
// that stopped mid-duel), in one transaction; it returns how many.
func (db *SQLiteDB) DuelRefundHeld() (int, error) {
	n := 0
	err := db.duelTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT duel, serverid, tunar FROM duel_stakes`)
		if err != nil {
			return err
		}
		type stake struct {
			duel, id uint32
			tunar    int
		}
		var all []stake
		for rows.Next() {
			var s stake
			if err := rows.Scan(&s.duel, &s.id, &s.tunar); err != nil {
				rows.Close()
				return err
			}
			all = append(all, s)
		}
		rows.Close()
		for _, s := range all {
			// a character deleted since has no purse to go back to
			if _, err := tx.Exec(`UPDATE characters SET tunar = tunar + ? WHERE serverid = ?`, s.tunar, s.id); err != nil {
				return err
			}
			n++
		}
		_, err = tx.Exec(`DELETE FROM duel_stakes`)
		return err
	})
	return n, err
}

// DuelHeld is the tunar a duel holds now (for tests and the GM's eye).
func (db *SQLiteDB) DuelHeld(duel uint32) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COALESCE(SUM(tunar), 0) FROM duel_stakes WHERE duel = ?`, duel).Scan(&n)
	return n, err
}
