package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// norent.go: a login takes what a character held of the records that vanish at logout
// (database/norent.go), before the login reads its items. A character still in the world (a
// login that takes it over as it stands) has not left, so it keeps them. The player is told once
// in the world.
//
// A login within NoRentAway of the character's last logout keeps them too (jib's PR #1, bf7400b,
// the GM Jibrish's ruling of 2026-10-29: No Rent "means it vanishes if the player logs out for more
// than 5 minutes"): the logout is table charlogout's, written as a session ends and at a
// shutdown's save (network/udp/norentsave.go) and taken away at the next login. With no row (a
// crash, the first login after this came) the items go, as before it: only a logout known to be
// that recent spares them.

// NoRentAway is how long a character may be out of the world and keep its No Rent items.
const NoRentAway = 5 * time.Minute

// noRentNow is the clock a login's time away is read by (a test's to turn).
var noRentNow = time.Now

// vanished is what a login took, until its player is in the world to be told.
var (
	vanishedMu sync.Mutex
	vanished   = map[uint32][]string{}
)

// vanishNoRent takes the character's vanishing items at its login.
func (h *CharacterHandler) vanishNoRent(serverID uint32) {
	if h.store == nil {
		return
	}
	if h.gameWorld != nil {
		if _, live := h.gameWorld.PlayerByCharacter(serverID); live {
			return
		}
	}
	if at, ok, err := h.store.DB().TakeLogout(serverID); err != nil {
		log.Printf("NoRent: character %d's logout: %v", serverID, err)
	} else if away := noRentNow().Sub(at); ok && away < NoRentAway {
		log.Printf("NoRent: character %d was away %v, under %v: its No Rent items stay", serverID, away.Round(time.Second), NoRentAway)
		return
	}
	names, err := h.store.DB().VanishNoRent(serverID)
	if err != nil {
		log.Printf("NoRent: character %d: %v", serverID, err)
		return
	}
	if len(names) == 0 {
		return
	}
	log.Printf("NoRent: character %d's %q vanished at the login", serverID, names)
	vanishedMu.Lock()
	vanished[serverID] = names
	vanishedMu.Unlock()
}

// tellVanished tells a player entering the world what its login took.
func tellVanished(sess *session.Session, p *entity.Player) {
	vanishedMu.Lock()
	names := vanished[p.CharServerID]
	delete(vanished, p.CharServerID)
	vanishedMu.Unlock()
	if len(names) > 0 {
		SendColoredChat(sess, fmt.Sprintf("Your No Rent items vanished when you logged out: %s.", strings.Join(names, ", ")), 255, 255, 0)
	}
}
