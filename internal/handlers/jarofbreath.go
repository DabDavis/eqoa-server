package handlers

import (
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// jarofbreath.go — the Heirloom Jar of Breath (the user, 2026-11-11: "lets
// also add a heirloom jar of breath that buffs underwater breath for 4
// hours").
//
// The jar is an item record named JarOfBreathName (tools/jar_of_breath_20261111.py
// makes it, its itemeffect row and its startitems rows; a character with none
// is given one at its login, giveJarOfBreath, as the lamp, the fang and the
// portal are: world.heirlooms, so the Heirloom Test Server alone, whose
// database alone has the record). It is CLIENT's gem type (procanim 7, as the
// fang: the item menu's Use and the HUD's gem quick slot), NO TRADE, LORE,
// worth 0. Its Use is its itemeffect row's, as a potion's is drunk
// (potion.go): "Self, Enduring Breath, 4hr" (entity/spelltext.go reads the
// workbook's breath in a line), so it lands on its user as an effect of its
// own on the effects list (entity.PotionEffectID of the record: a second use
// renews the four hours), under water no breath is lost while it lasts
// (entity/workbook.go holdsBreath, the breath system), it is kept over a
// logout as every effect (entity/buffsave.go: its time runs only in the
// world) and ended by death as every effect but the towns' boon. Unlike a
// potion it is never spent (neverSpent). A use without its itemeffect row
// does nothing but say so.

// JarOfBreathName is the jar's record's name.
const JarOfBreathName = "Heirloom Jar of Breath"

// The jar's words (ours).
const (
	jarUsedLine = "The jar's cool air fills your lungs: you lose no breath under water for four hours."
	jarQuiet    = "The Heirloom Jar of Breath is empty here."
)

// IsJarOfBreath is whether an item record is the jar: worn nowhere and so
// named.
func IsJarOfBreath(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), JarOfBreathName)
}

// neverSpent is whether a potion's use takes nothing of the item (usePotion):
// the jar's.
func neverSpent(p *database.ItemPatternRecord) bool { return IsJarOfBreath(p) }

// giveJarOfBreath gives a character with no jar one at its login, as
// giveGenieLamp gives the lamp.
func (h *CharacterHandler) giveJarOfBreath(serverID uint32, room listSizes) {
	if h.store == nil || h.inventoryStore == nil {
		return
	}
	h.giveKeptItem(serverID, keptItemID(h.store.DB(), JarOfBreathName, "Jar"), "Jar", "jar", room)
}

// useJarOfBreath is the jar's Use at list index idx: its itemeffect row's use,
// as a potion's, nothing spent, said in a line; without the row its line
// alone.
func (h *EquipHandler) useJarOfBreath(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry, target uint32) {
	pe, ok := h.potionOf(&row.Pattern)
	if !ok {
		SendErrorMessage(sess, jarQuiet)
		return
	}
	if h.usePotion(sess, player, idx, row, pe, target) {
		SendClientMessage(sess, jarUsedLine)
	}
}
