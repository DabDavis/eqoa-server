package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// snowlauncher.go — the Automatic Snowball Launcher (the owner, 2026-11-16:
// "lets give [a player] an automatic snowball launcher, infinite ammo"; what it
// does is entity/snowlauncher.go's).
//
// The launcher is an item record of this name (tools/snowball_launcher_20261116.py:
// CLIENT's usable type, procanim 2, so the item menu offers Use and it goes on a
// potion quick slot; LORE, NO TRADE, worth nothing), worn nowhere, never spent
// (its ammunition never runs out). It is bound to one character by table
// itemowner (database/itemowner.go): any other who holds a copy is refused its
// Use in red. A record of the name with no binding works for whoever holds it
// (the Barrel of Monkeys' rule).
//
// Each Use switches it: on (automatic fire at the target), or off.

// SnowLauncherName is the launcher's record's name.
const SnowLauncherName = "Automatic Snowball Launcher"

const (
	launcherOnLine    = "Your Automatic Snowball Launcher whirs to life: automatic fire ON. Target someone!"
	launcherOffLine   = "Your Automatic Snowball Launcher clicks off: automatic fire OFF."
	launcherNotYours  = "The launcher's gears lock up: it was built for someone else."
	launcherDeadLine  = "The launcher will not start: you are dead."
	launcherNoWorldLg = "Snowball Launcher: %s's use: not in the world"
)

// IsSnowLauncher is whether an item record is the launcher: worn nowhere and
// so named.
func IsSnowLauncher(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), SnowLauncherName)
}

// useSnowLauncher is the launcher's Use (equip.go): automatic fire on or off,
// told to the player.
func (h *EquipHandler) useSnowLauncher(sess *session.Session, player *entity.Player, p *database.ItemPatternRecord) {
	if h.gameWorld == nil {
		return
	}
	if owner, ok := itemOwnerOf(p.PatternID); ok && owner != player.CharServerID {
		SendColoredChat(sess, launcherNotYours, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		log.Printf("Snowball Launcher: %s (char %d) is not its owner (char %d): refused", player.Name, player.CharServerID, owner)
		return
	}
	if line := snowLauncherLine(h.gameWorld.ToggleSnowLauncher(sess, time.Now()), player.Name); line != "" {
		SendClientMessage(sess, line)
	}
}

// snowLauncherLine is what a use of the launcher tells its user ("" for none:
// logged).
func snowLauncherLine(u entity.LauncherUse, who string) string {
	switch u {
	case entity.LauncherOn:
		return launcherOnLine
	case entity.LauncherOff:
		return launcherOffLine
	case entity.LauncherDead:
		return launcherDeadLine
	}
	log.Printf(launcherNoWorldLg, who)
	return ""
}
