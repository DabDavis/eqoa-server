package handlers

import (
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// charslots.go — more characters than CLIENT's 8 (the server owner, 2026-11-11:
// "lets let player create more than 8 chars, also allow client to scroll for the
// additional slots. lets make max 20 for now"; world.max_characters,
// entity/charslots.go).
//
// CLIENT's select page holds 8 (VIClient::CharacterList keeps `m_charCount` at 8
// and reads that many records into `m_chars[8]`, the rest unread), so:
//
//   - A client whose page lists every character and scrolls them (vanilla) says
//     so with bit 1 of its answer to the live look's offer (protocol.OpcodeLiveLook,
//     livelook.go). The look in force (OpcodeLiveLookInForce) then carries a
//     second u32, the characters the account may hold, and the list is sent
//     again, whole.
//   - Every other client (a PS2, an older vanilla) is listed the 8 most recently
//     played (charplayed, database/charplayed.go), in the list's own order, and
//     may not make a ninth: creation past 8 is refused as CLIENT refuses one
//     (0x002E, word 269 "Character create failed"), which a PS2 cannot reach in
//     any case: its page shows no empty slot with 8 listed. The rest are kept,
//     untouched, and listed again on a client that shows them.

// charCap is the characters sess's client is listed and may make.
func charCap(sess *session.Session) int {
	return entity.ClientCharCap(sess != nil && sess.ManyChars())
}

// listedChars is what of records (in their list order) a client of n slots is
// sent: all where they fit, else the n most recently played (played: Unix
// seconds by server id; none played go after, the newest made first), still in
// the list's order.
func listedChars(records []*database.CharacterRecord, n int, played map[uint32]int64) []*database.CharacterRecord {
	if len(records) <= n {
		return records
	}
	byPlay := append([]*database.CharacterRecord(nil), records...)
	sort.SliceStable(byPlay, func(i, j int) bool {
		a, b := played[uint32(byPlay[i].ServerID)], played[uint32(byPlay[j].ServerID)]
		if a != b {
			return a > b
		}
		return byPlay[i].ServerID > byPlay[j].ServerID
	})
	keep := map[uint32]bool{}
	for _, r := range byPlay[:max(n, 0)] {
		keep[r.ServerID] = true
	}
	out := make([]*database.CharacterRecord, 0, n)
	for _, r := range records {
		if keep[r.ServerID] {
			out = append(out, r)
		}
	}
	return out
}

// sessionChars is the account's characters as sess's client is listed them.
func (h *CharacterHandler) sessionChars(sess *session.Session, records []*database.CharacterRecord) []*database.CharacterRecord {
	n := charCap(sess)
	if len(records) <= n {
		return records
	}
	var played map[uint32]int64
	if h.store != nil && h.store.DB() != nil {
		var err error
		if played, err = h.store.DB().AccountPlayed(int(sess.AccountID())); err != nil {
			log.Printf("SendCharacterSelectEQOA: account %d's last played: %v", sess.AccountID(), err)
		}
	}
	out := listedChars(records, n, played)
	log.Printf("SendCharacterSelectEQOA: account %d has %d characters; its client lists %d: the %d most recently played", sess.AccountID(), len(records), n, len(out))
	return out
}

// stampPlayed keeps that a character entered the world now (charplayed).
func (h *CharacterHandler) stampPlayed(serverID uint32) {
	if h.store == nil || h.store.DB() == nil {
		return
	}
	if err := h.store.DB().StampPlayed(serverID, time.Now()); err != nil {
		log.Printf("SelectedChar: character %d's login time: %v", serverID, err)
	}
}
