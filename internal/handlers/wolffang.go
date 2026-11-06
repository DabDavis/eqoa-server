package handlers

import (
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// wolffang.go — the Heirloom Fang of the Wolf (the user, 2026-11-06: "lets add
// a speed heir loom item like the lamp they save it to the item toolbar").
//
// The fang is an item record named WolfFangName (tools/seed_wolf_fang.py makes
// it and puts it among every combo's starting items; a character with none is
// given one at its login, giveWolfFang, as the lamp is). It is CLIENT's gem
// type (procanim 7: VIInfoText::IsUsable, the item menu's Use), which the HUD's
// third quick slot takes (Msg000A: a type 7 on slot 2; the live client's on 2
// or 3: gearsets.go), so the two potion slots stay the potions' and the lamp's;
// a quick slot's press is UseItem(i, 1), the same 0x003E as the menu's Use. Its
// Use is never spent: it puts the boards' Spirit of the Wolf on its holder
// (entity/boardwolf.go: Spirit of the Wolf's run speed for an hour of play, in
// place of one on it, ended by the holder's own hostile act: a swing, auto
// attack engaged, a harmful spell, a potion thrown), so after a fight it is
// used again. One effect whatever gave it: a board's talk renews a fang's hour
// and the fang a board's.

// WolfFangName is the fang's record's name.
const WolfFangName = "Heirloom Fang of the Wolf"

// IsWolfFang is whether an item record is the fang: worn nowhere and so named.
func IsWolfFang(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), WolfFangName)
}

// giveWolfFang gives a character with no fang one at its login, as
// giveGenieLamp gives the lamp.
func (h *CharacterHandler) giveWolfFang(serverID uint32) {
	if h.store == nil || h.inventoryStore == nil {
		return
	}
	h.giveKeptItem(serverID, keptItemID(h.store.DB(), WolfFangName, "Fang"), "Fang", "fang")
}

// useWolfFang puts the boards' Spirit of the Wolf on the session's player and
// tells it so. A dead player is given nothing and told nothing.
func (h *EquipHandler) useWolfFang(sess *session.Session) {
	if h.gameWorld == nil {
		return
	}
	if given, renewed := h.gameWorld.GiveWolf(sess, time.Now(), "the fang's"); given {
		SendClientMessage(sess, wolfFangLine(renewed))
	}
}

// wolfFangLine is the player's line for the fang's Spirit of the Wolf (ours).
func wolfFangLine(renewed bool) string {
	if renewed {
		return "The fang renews your Spirit of the Wolf for an hour of play. A hostile act ends it."
	}
	return "The spirit of the wolf quickens your step for an hour of play. A hostile act ends it."
}
