package entity

import (
	"github.com/eqoa/server/internal/database"
)

// StartingTunar is the purse a login takes from the character's row: what it
// stores, always. A new character's purse is stored when it is made
// (world.start_tunar, database/newchar.go); a login used to give 5000 to a row
// at 0 tunar and 0 banked, which refilled anyone who spent and banked down to
// exactly 0 (and, with the account bank, anyone who deposited it all).
func StartingTunar(record *database.CharacterRecord) int {
	return record.Tunar
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
