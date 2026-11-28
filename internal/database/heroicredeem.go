package database

import (
	"errors"
	"fmt"
)

type HeroicReward int

const (
	HeroicUnlock HeroicReward = iota + 1
	HeroicRelic
	HeroicTeleport
)

func (r HeroicReward) MarkCost() int {
	switch r {
	case HeroicUnlock:
		return 3
	case HeroicRelic:
		return 5
	case HeroicTeleport:
		return 1
	}
	return 0
}

var (
	ErrHeroicShort    = errors.New("not enough matching marks in the pack")
	ErrHeroicClaimed  = errors.New("this character already redeemed that reward")
	ErrHeroicPackFull = errors.New("the pack is full")
)

type HeroicMarkDebit struct {
	ItemID, Amount int
}

type HeroicRedemption struct {
	Taken []HeroicMarkDebit
	Item  CharInventoryEntry
}

// RedeemHeroicMarks spends only the buyer's matching marks in the pack. A
// permanent unlock or relic claim and its cost commit together with any item
// given. The claim is per boss and character and survives destroying the item.
func (s *InventoryStore) RedeemHeroicMarks(serverID uint32, class, packLimit int, boss string, reward HeroicReward) (HeroicRedemption, error) {
	var out HeroicRedemption
	set := HeroicMarksFor(boss)
	if set == nil || set.RoleFor(class) == nil || reward.MarkCost() == 0 {
		return out, fmt.Errorf("heroic reward %d unavailable for %q and class %d", reward, boss, class)
	}
	role := set.RoleFor(class)
	patternID, charges := 0, 0
	switch reward {
	case HeroicUnlock:
		if set.Mastery.ID == 0 {
			return out, fmt.Errorf("no mastery reward for %q", boss)
		}
	case HeroicRelic:
		patternID = role.RelicID
	case HeroicTeleport:
		patternID, charges = set.Teleport.ID, HeroicTeleportCharges
		if patternID == 0 {
			return out, fmt.Errorf("no teleport reward for %q", boss)
		}
	}
	if patternID != 0 {
		pattern, err := NewItemPatternStore(s.db).GetItemPattern(patternID)
		if err != nil {
			return out, err
		}
		out.Item = CharInventoryEntry{Pattern: *pattern, StackLeft: 1, Charges: charges, EquipLoc: -1}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if reward != HeroicTeleport {
		res, err := tx.Exec(`INSERT INTO charheroicrewards (serverid,boss,reward) VALUES (?,?,?)
			ON CONFLICT DO NOTHING`, serverID, boss, reward)
		if err != nil {
			return out, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return out, err
		}
		if n == 0 {
			return out, ErrHeroicClaimed
		}
	}
	rows, err := tx.Query(`SELECT itemid,stackleft FROM charinventory
		WHERE serverid=? AND location=0 AND equiploc=-1 AND patternid=? AND stackleft>0
		ORDER BY stackleft,itemid`, serverID, role.MarkID)
	if err != nil {
		return out, err
	}
	type stack struct{ id, qty int }
	var stacks []stack
	need := reward.MarkCost()
	for need > 0 && rows.Next() {
		var st stack
		if err := rows.Scan(&st.id, &st.qty); err != nil {
			rows.Close()
			return out, err
		}
		stacks = append(stacks, st)
		need -= min(need, st.qty)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	if need > 0 {
		return out, ErrHeroicShort
	}
	left := reward.MarkCost()
	for _, st := range stacks {
		n := min(left, st.qty)
		if n == st.qty {
			_, err = tx.Exec(`DELETE FROM charinventory WHERE itemid=?`, st.id)
		} else {
			_, err = tx.Exec(`UPDATE charinventory SET stackleft=stackleft-? WHERE itemid=?`, n, st.id)
		}
		if err != nil {
			return out, err
		}
		out.Taken = append(out.Taken, HeroicMarkDebit{ItemID: st.id, Amount: n})
		left -= n
	}
	if patternID != 0 {
		var pack int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid=? AND location=0`, serverID).Scan(&pack); err != nil {
			return out, err
		}
		if pack >= packLimit {
			return out, ErrHeroicPackFull
		}
		out.Item.ItemID, err = addCharItem(tx.Exec, serverID, patternID, 1, out.Item.Pattern.MaxHP, charges, 0)
		if err != nil {
			return out, err
		}
		out.Item.RemainHP = out.Item.Pattern.MaxHP
	}
	return out, tx.Commit()
}

// HeroicMasteryUnlocks returns the boss-specific unlocks this character bought.
func (s *SQLiteCharacterStore) HeroicMasteryUnlocks(serverID uint32) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT boss FROM charheroicrewards WHERE serverid=? AND reward=?`, serverID, HeroicUnlock)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	unlocked := make(map[string]bool)
	for rows.Next() {
		var boss string
		if err := rows.Scan(&boss); err != nil {
			return nil, err
		}
		unlocked[boss] = true
	}
	return unlocked, rows.Err()
}
