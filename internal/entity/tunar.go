package entity

import "github.com/eqoa/server/internal/database"

// DefaultTunar is the purse a character starts with when its row has never
// held one. Every login used to set it, whatever the row said; the merchant,
// bank and blacksmith now store the purse (characters.tunar / bankTunar).
const DefaultTunar = 5000

// StartingTunar is the purse a login takes from the character's row: what it
// stores, or DefaultTunar for a row that has never stored any (both columns
// still at the schema's 0).
func StartingTunar(record *database.CharacterRecord) int {
	if record.Tunar != 0 || record.BankTunar != 0 {
		return record.Tunar
	}
	return DefaultTunar
}
