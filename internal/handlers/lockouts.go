package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// restoreLockouts gives the character a login just built from its row (sess's
// player) the ability lockouts its last session kept that still run
// (entity/lockoutsave.go, table charlockouts), the time it was away counted:
// each on the display index its spell has in the book now, the book its
// MemoryDump sends (GetCharacterSpells). A spell the book has no more keeps
// its lockout shown on nothing, as a change of book keeps one
// (entity/bookchange.go); a book that cannot be read shows them all on
// nothing. The server refuses each either way. The badges go after the dump
// (SendLockouts).
func (h *CharacterHandler) restoreLockouts(sess *session.Session, record *database.CharacterRecord) {
	if h.store == nil || h.gameWorld == nil {
		return
	}
	now := time.Now()
	rows, err := h.store.LoadLockouts(record.ServerID, now)
	if err != nil {
		log.Printf("SelectedChar: '%s' lockouts: %v", record.CharName, err)
		return
	}
	if len(rows) == 0 {
		return
	}
	index := map[int]int{}
	if h.spellbookStore != nil {
		book, err := h.spellbookStore.GetCharacterSpells(record.ServerID)
		if err != nil {
			log.Printf("SelectedChar: '%s' book for its lockouts: %v", record.CharName, err)
		}
		for i := range book {
			if _, seen := index[book[i].Pattern.SpellID]; !seen {
				index[book[i].Pattern.SpellID] = book[i].AddedOrder
			}
		}
	}
	n := h.gameWorld.RestoreLockouts(sess, rows, index, now)
	if n == 0 {
		return
	}
	said := make([]string, 0, len(rows))
	for _, r := range rows {
		on := "on nothing (not in the book)"
		if at, ok := index[r.SpellID]; ok {
			on = fmt.Sprintf("on ability %d", at)
		}
		said = append(said, fmt.Sprintf("spell %d %v %s", r.SpellID, r.Until.Sub(now).Round(time.Second), on))
	}
	log.Printf("SelectedChar: '%s' keeps %d abilities locked out from its last session: %s", record.CharName, n, strings.Join(said, ", "))
}
