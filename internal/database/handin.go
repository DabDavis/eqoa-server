package database

import (
	"database/sql"
	"fmt"
	"time"
)

// HandInTake is one inventory row a quest hand-in takes from: N of the row,
// which held Left when the hand-in was planned.
type HandInTake struct {
	ItemID, Left, N int
}

// HandIn is a quest hand-in made one outcome (handlers/quests.go's atomic
// hand-in, board entry 1263): every row taken, the purse set and the quest's
// progress written in ONE transaction, or none of it. A row that no longer
// holds what was planned (taken, split or moved since) refuses the whole
// hand-in, as does any error; fault, when set (tests), is asked before each
// write by its index (the rows, then the purse, then the progress) and an
// error from it aborts there. Purse < 0 leaves the purse alone.
func (s *InventoryStore) HandIn(serverID uint32, takes []HandInTake, purse int, cq *CharQuest, fault func(i int) error) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	i := 0
	step := func() error {
		defer func() { i++ }()
		if fault != nil {
			return fault(i)
		}
		return nil
	}
	for _, t := range takes {
		if err = step(); err != nil {
			return err
		}
		var res sql.Result
		if t.N >= t.Left {
			res, err = tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND stackleft = ? AND `+heldOf(serverID),
				append([]any{t.ItemID, t.Left}, heldArgsOf(serverID)...)...)
		} else {
			res, err = tx.Exec(`UPDATE charinventory SET stackleft = ? WHERE itemid = ? AND stackleft = ? AND `+heldOf(serverID),
				append([]any{t.Left - t.N, t.ItemID, t.Left}, heldArgsOf(serverID)...)...)
		}
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("item %d no longer holds the %d planned", t.ItemID, t.Left)
		}
		if t.N >= t.Left {
			if _, err = tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, t.ItemID); err != nil {
				return err
			}
		}
	}
	if err = step(); err != nil {
		return err
	}
	if purse >= 0 {
		var res sql.Result
		if res, err = tx.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, purse, serverID); err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("character %d: no purse row", serverID)
		}
	}
	if err = step(); err != nil {
		return err
	}
	if cq != nil {
		row := *cq
		row.Updated = time.Now().UTC().Format(time.RFC3339)
		if _, err = tx.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(serverid, questid) DO UPDATE SET step = excluded.step, count = excluded.count,
				done = excluded.done, slot = excluded.slot, updated = excluded.updated, picks = excluded.picks`,
			row.ServerID, row.QuestID, row.Step, row.Count, row.Done, row.Slot, row.Updated, row.Picks); err != nil {
			return err
		}
	}
	return tx.Commit()
}
