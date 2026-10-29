package database

import (
	"database/sql"
	"errors"
	"math"
)

// A GM's resets of a character (the live CLIENT's /rst_both, the server's
// #rst_both; handlers/gmresets.go) and its bank's tunar (/banktunar), kept.

// TrainingReset is what a training reset gave back: the points each stat had
// taken (STR, STA, AGI, DEX, WIS, INT, CHA: the create screen's twenty and the
// trainer's), and the unspent and total points after it.
type TrainingReset struct {
	Spent         [7]int
	Unused, Total int
}

// Refund is how many points came back.
func (t TrainingReset) Refund() int {
	n := 0
	for _, v := range t.Spent {
		n += v
	}
	return n
}

// ResetTraining gives back every training point a character out of the world
// spent, in one transaction: each tp column lowered by what it held (a
// negative one, which only a hand's edit makes, is left as it is) and the
// points added to its unspent ones, the total never below them. A character
// in the world is TakeTraining's.
func (s *SQLiteCharacterStore) ResetTraining(serverID uint32) (TrainingReset, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return TrainingReset{}, err
	}
	defer tx.Rollback()
	var tp [7]int
	var unused, total int
	err = tx.QueryRow(`SELECT tpstrength, tpstamina, tpagility, tpdexterity, tpwisdom, tpintel, tpcharisma,
		unusedTP, totalTP FROM characters WHERE serverid = ?`, serverID).
		Scan(&tp[0], &tp[1], &tp[2], &tp[3], &tp[4], &tp[5], &tp[6], &unused, &total)
	if errors.Is(err, sql.ErrNoRows) {
		return TrainingReset{}, ErrCharacterNotFound
	}
	if err != nil {
		return TrainingReset{}, err
	}
	r := TrainingReset{Spent: TrainingSpent(tp)}
	r.Unused = unused + r.Refund()
	r.Total = max(total, r.Unused)
	if err := takeTrainingTx(tx, serverID, r); err != nil {
		return TrainingReset{}, err
	}
	return r, tx.Commit()
}

// TrainingSpent is what a reset gives back of a character's tp columns
// (STR..CHA): each one's points, a negative one none.
func TrainingSpent(tp [7]int) [7]int {
	var out [7]int
	for i, v := range tp {
		out[i] = max(0, v)
	}
	return out
}

// TakeTraining keeps the training reset of a character in the world, once
// the game has it (entity.World.TakeBackTraining): each tp column lowered by
// what r gave back of it, and its unspent and total points the game's now.
// The columns are lowered, not zeroed, so points the trainer took meanwhile
// stay theirs.
func (s *SQLiteCharacterStore) TakeTraining(serverID uint32, r TrainingReset) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := takeTrainingTx(tx, serverID, r); err != nil {
		return err
	}
	return tx.Commit()
}

func takeTrainingTx(tx *sql.Tx, serverID uint32, r TrainingReset) error {
	res, err := tx.Exec(`UPDATE characters SET tpstrength = tpstrength - ?, tpstamina = tpstamina - ?,
		tpagility = tpagility - ?, tpdexterity = tpdexterity - ?, tpwisdom = tpwisdom - ?, tpintel = tpintel - ?,
		tpcharisma = tpcharisma - ?, unusedTP = ?, totalTP = ? WHERE serverid = ?`,
		r.Spent[0], r.Spent[1], r.Spent[2], r.Spent[3], r.Spent[4], r.Spent[5], r.Spent[6],
		r.Unused, r.Total, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	return nil
}

// MaxBankTunar is the most a bank holds: CLIENT reads its tunar as a signed
// 32-bit number (0x1253's zigzag, VIBankControl::Recv).
const MaxBankTunar = math.MaxInt32

// AddBankTunar adds n tunar (negative: takes away) to a character's bank,
// never below 0 nor past MaxBankTunar, and returns what it holds now. For a
// character out of the world: one in it has its bank as the game holds it
// (Player.BankTunar, kept with InventoryStore.SetTunar).
func (s *SQLiteCharacterStore) AddBankTunar(serverID uint32, n int) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var bank int
	err = tx.QueryRow(`SELECT bankTunar FROM characters WHERE serverid = ?`, serverID).Scan(&bank)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCharacterNotFound
	}
	if err != nil {
		return 0, err
	}
	bank = AddTunar(bank, n)
	if _, err := tx.Exec(`UPDATE characters SET bankTunar = ? WHERE serverid = ?`, bank, serverID); err != nil {
		return 0, err
	}
	return bank, tx.Commit()
}

// AddTunar is have plus n, held within 0..MaxBankTunar.
func AddTunar(have, n int) int {
	v := int64(have) + int64(n)
	return int(min(max(v, 0), MaxBankTunar))
}
