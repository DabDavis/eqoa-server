package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// norent.go: a login takes what a character held of the records that vanish at logout
// (database/norent.go), before the login reads its items. A character still in the world (a
// login that takes it over as it stands) has not left, so it keeps them. The player is told once
// in the world.

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
