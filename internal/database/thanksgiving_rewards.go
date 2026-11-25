package database

import (
	"database/sql"
	"errors"
	"fmt"
)

const (
	DrunkUncleProtection = 1000813
	YellingAtCousins     = 1000814
	GibletRewardLimit    = 2
)

var ErrGibletRewardLimit = errors.New("the player already holds two copies of this reward")

func isGibletReward(id int) bool {
	return id == DrunkUncleProtection || id == YellingAtCousins
}

func (p ItemPatternRecord) GibletStack() bool {
	return isGibletReward(p.PatternID) && p.MaxStack == GibletRewardLimit
}

// Count the buyer's carried and kept pack rows and every page of the bank
// they actually use, including bank rows set aside for a smaller client.
func gibletRewardHeld(q sqlRunner, serverID uint32, patternID int) (int, error) {
	owner, bankArgs := bankOwnerCond("", serverID)
	var n int
	err := q.QueryRow(`SELECT coalesce(sum(max(stackleft,1)),0) FROM charinventory
		WHERE patternid = ? AND ((serverid = ? AND location IN (0,2)) OR (location IN (1,3) AND `+owner+`))`,
		append([]any{patternID, serverID}, bankArgs...)...).Scan(&n)
	return n, err
}

func (s *InventoryStore) GibletRewardHeld(serverID uint32, patternID int) (int, error) {
	return gibletRewardHeld(s.db, serverID, patternID)
}

// A two-copy shield stack uses the front shield first. The second is fresh
// unless a bank move joins two used shields. In that case remaincharge keeps
// the spare's charges in the tens digit and the front's in the units digit.
// Single copies and stacks with a fresh spare keep the original 1..3 value.
// This also survives sales, buybacks and restarts in their existing columns.
const fullShieldCharges = 3

func (e CharInventoryEntry) FrontCharges() int {
	if e.Pattern.PatternID == DrunkUncleProtection {
		return e.Charges % 10
	}
	return e.Charges
}

func (e CharInventoryEntry) spareCharges() int {
	if e.Pattern.PatternID == YellingAtCousins {
		return 1
	}
	if spare := e.Charges / 10; spare > 0 {
		return spare
	}
	return fullShieldCharges
}

// Moving or destroying one copy takes the spare, leaving the active copy's
// charge count (and the client's display of it) untouched.
func (e CharInventoryEntry) movedCharges(n int) int {
	if e.Pattern.GibletStack() && e.StackLeft == 2 && n == 1 {
		return e.spareCharges()
	}
	return e.Charges
}

func joinedRewardCharges(pattern, front, spare int) int {
	if pattern == DrunkUncleProtection && spare > 0 && spare < fullShieldCharges {
		return 10*spare + front
	}
	return front
}

// FinishGibletCopy spends the last charge of the front copy and promotes the
// spare. The count and charges change together or neither changes.
func (s *InventoryStore) FinishGibletCopy(serverID uint32, e CharInventoryEntry) (int, error) {
	if !e.Pattern.GibletStack() || e.StackLeft != 2 || e.FrontCharges() != 1 {
		return 0, errors.New("charged reward has no spent front copy to replace")
	}
	next := e.spareCharges()
	err := s.execOne(`UPDATE charinventory SET stackleft = 1, remaincharge = ? WHERE
		serverid = ? AND itemid = ? AND patternid = ? AND location = 0 AND stackleft = 2 AND remaincharge = ?`,
		next, serverID, e.ItemID, e.Pattern.PatternID, e.Charges)
	return next, err
}

// addGibletCopy joins exactly one copy to a single already in the destination.
func addGibletCopy(tx *sql.Tx, serverID uint32, pattern, location, onto, charges int) error {
	held, args := heldCond("", serverID)
	args = append([]any{onto, pattern, location}, args...)
	var front int
	err := tx.QueryRow(`SELECT remaincharge FROM charinventory WHERE itemid = ? AND patternid = ?
		AND location = ? AND stackleft = 1 AND `+held, args...).Scan(&front)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE charinventory SET stackleft = 2, remaincharge = ? WHERE itemid = ?`,
		joinedRewardCharges(pattern, front, charges), onto)
	return err
}

// GibletBankPart names an existing destination stack, or zero for a new one.
type GibletBankPart struct {
	ItemID int
	Count  int
}

// MoveGibletCopies splits or joins charged copies without refilling either.
// An entire stack moving to an empty slot uses the ordinary MoveCharItem.
func (s *InventoryStore) MoveGibletCopies(serverID uint32, src CharInventoryEntry, dstLoc int, parts []GibletBankPart) (CharInventoryEntry, error) {
	var moved CharInventoryEntry
	if !src.Pattern.GibletStack() || src.StackLeft < 1 || src.StackLeft > 2 || len(parts) == 0 ||
		(src.Location != LocPack && src.Location != LocBank) || dstLoc != 1-src.Location {
		return moved, errors.New("invalid charged reward move")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return moved, err
	}
	defer tx.Rollback()
	rest := src
	for _, part := range parts {
		if part.Count < 1 || part.Count > rest.StackLeft || (part.ItemID != 0 && part.Count != 1) {
			return moved, errors.New("invalid charged reward quantity")
		}
		moved = rest
		moved.ItemID, moved.Location, moved.StackLeft = part.ItemID, dstLoc, part.Count
		moved.Charges = rest.movedCharges(part.Count)
		if part.ItemID == 0 {
			moved.ItemID, err = addCharItem(tx.Exec, serverID, src.Pattern.PatternID, part.Count, src.RemainHP, moved.Charges, dstLoc)
		} else {
			err = addGibletCopy(tx, serverID, src.Pattern.PatternID, dstLoc, part.ItemID, moved.Charges)
		}
		if err != nil {
			return moved, err
		}
		rest.StackLeft -= part.Count
		rest.Charges = rest.FrontCharges()
	}
	held, args := heldCond("", serverID)
	guard := ` WHERE itemid = ? AND patternid = ? AND location = ? AND stackleft = ? AND remaincharge = ? AND ` + held
	args = append([]any{src.ItemID, src.Pattern.PatternID, src.Location, src.StackLeft, src.Charges}, args...)
	var result sql.Result
	if rest.StackLeft == 0 {
		result, err = tx.Exec(`DELETE FROM charinventory`+guard, args...)
		if err == nil {
			_, err = tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, src.ItemID)
		}
	} else {
		result, err = tx.Exec(`UPDATE charinventory SET stackleft = ?, remaincharge = ?`+guard,
			append([]any{rest.StackLeft, rest.Charges}, args...)...)
	}
	if err != nil {
		return moved, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return moved, fmt.Errorf("charged reward %d changed during bank move", src.ItemID)
	}
	return moved, tx.Commit()
}

func (s *InventoryStore) GibletPackCount(serverID uint32, pattern int) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT coalesce(sum(max(stackleft,1)),0) FROM charinventory
		WHERE serverid = ? AND patternid = ? AND location IN (0,2)`, serverID, pattern).Scan(&n)
	return n, err
}
