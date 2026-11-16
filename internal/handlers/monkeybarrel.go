package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// monkeybarrel.go — the Barrel of Monkeys (the owner, 2026-11-16: "give moose a
// barrel of monkeys unique item. it spawns a monkey that follows him around, up
// to 5"; the monkeys are entity/monkeys.go's).
//
// The barrel is an item record of this name (tools/barrel_of_monkeys_20261116.py:
// CLIENT's usable type, procanim 2, so the item menu offers Use and it goes on a
// potion quick slot; LORE, NO TRADE, worth nothing), worn nowhere, never spent.
// It is bound to one character by table itemowner (database/itemowner.go): any
// other who holds a copy (the account bank, a GM's give) is refused its Use. A
// record of the name with no binding works for whoever holds it.
//
// Each Use lets one more monkey out, up to entity.MonkeysMax, and says its name
// and how many are out; a Use with all five out sends them all back into the
// barrel, and the fifth's line says so beforehand.

// MonkeyBarrelName is the barrel's record's name.
const MonkeyBarrelName = "Barrel of Monkeys"

const (
	monkeyCameLine   = "%s climbs out of the barrel. (%d of %d)"
	monkeyFullLine   = "%s climbs out of the barrel. All %d of your monkeys are out: use the barrel again to send them home."
	monkeysBackHome  = "Your monkeys scramble back into the barrel."
	monkeyNotYours   = "The monkeys in this barrel answer only to the one it was made for."
	monkeyDeadLine   = "The barrel stays shut: you are dead."
	monkeyArenaLine  = "The monkeys will not come out here."
	monkeyNoWorldLog = "Barrel of Monkeys: %s's use: not in the world"
)

// IsMonkeyBarrel is whether an item record is the barrel: worn nowhere and so
// named.
func IsMonkeyBarrel(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), MonkeyBarrelName)
}

// useMonkeyBarrel is the barrel's Use (equip.go): one more monkey, or all of
// them home, told to the player.
func (h *EquipHandler) useMonkeyBarrel(sess *session.Session, player *entity.Player, p *database.ItemPatternRecord) {
	if h.gameWorld == nil {
		return
	}
	if owner, ok := itemOwnerOf(p.PatternID); ok && owner != player.CharServerID {
		SendColoredChat(sess, monkeyNotYours, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
		return
	}
	if line := monkeyBarrelLine(h.gameWorld.UseMonkeyBarrel(sess, time.Now()), player.Name); line != "" {
		SendClientMessage(sess, line)
	}
}

// monkeyBarrelLine is what a use of the barrel tells its user ("" for none:
// logged).
func monkeyBarrelLine(r entity.MonkeyResult, who string) string {
	switch r.Use {
	case entity.MonkeyCame:
		if r.Out >= entity.MonkeysMax {
			return fmt.Sprintf(monkeyFullLine, r.Name, entity.MonkeysMax)
		}
		return fmt.Sprintf(monkeyCameLine, r.Name, r.Out, entity.MonkeysMax)
	case entity.MonkeysBack:
		return monkeysBackHome
	case entity.MonkeyDead:
		return monkeyDeadLine
	case entity.MonkeyAway:
		return monkeyArenaLine
	}
	log.Printf(monkeyNoWorldLog, who)
	return ""
}
