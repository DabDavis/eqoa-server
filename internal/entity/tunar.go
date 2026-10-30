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

// TakeTunar takes n tunar from the purse of a character in the world, when
// it holds that much, under the World's lock: a rehabilitator's price
// (handlers/rehab.go). It returns the purse after it, and false (nothing
// taken) when the purse is short or the character is not in the world. A
// negative n gives tunar back.
func (w *World) TakeTunar(serverID uint32, n int) (int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil || p.Tunar < n {
		return 0, false
	}
	p.Tunar -= n
	return p.Tunar, true
}
