package handlers

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// #dye: a character's dyes (table chardyes, entity/livelook.go), the GM's
// target when that is a player, else the GM's own:
//
//	#dye                         the dyes, slot by slot
//	#dye <slot> <colour>         one slot: chest bracer gloves legs boots helm
//	                             primary secondary shield robe
//	#dye armor|all <colour>      the six armour slots and the robe / every slot
//	#dye <slot|armor|all> none   the dye taken off
//
// A colour is RRGGBB or RRGGBBAA in hex (ff0000), tN (the disc's tint N) or
// a name (red, green, blue, ...). Each change is written with its gm_audit
// line, as the GM panel writes a row (database.SetCharDye), and put on the
// character where it stands: every client of the live game's look draws it
// from the next tick, a PS2 never sees it.

// dyeArmorSlots are #dye armor's: the six armour slots and the robe.
var dyeArmorSlots = []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs,
	database.DyeBoots, database.DyeHelm, database.DyeRobe}

// SetDyeStore gives #dye the table it writes (database.SQLiteDB).
func (h *ChatHandler) SetDyeStore(s dyeStore) { h.dyes = s }

func (h *ChatHandler) dyeCommand(sess *session.Session, args []string) {
	if h.gameWorld == nil || h.dyes == nil {
		h.gmReply(sess, "#dye: dyes are not kept here")
		return
	}
	serverID, name, cur, ok := h.gameWorld.DyeTarget(sess)
	if !ok || serverID == 0 {
		h.gmReply(sess, "#dye: no character in the world")
		return
	}
	if len(args) == 0 {
		h.gmReply(sess, fmt.Sprintf("%s's dyes (drawn by clients of the live game's look):", name))
		for i, n := range database.DyeSlotNames {
			h.gmReply(sess, fmt.Sprintf("  %-9s %s", n, database.FormatDye(cur[i])))
		}
		return
	}
	if len(args) != 2 {
		h.gmReply(sess, "#dye [<slot|armor|all> <RRGGBB[AA] | tN | name | none>]: slots "+strings.Join(database.DyeSlotNames[:], " "))
		return
	}
	var slots []int
	switch w := strings.ToLower(args[0]); w {
	case "armor", "armour":
		slots = dyeArmorSlots
	case "all":
		for i := 0; i < database.DyeSlots; i++ {
			slots = append(slots, i)
		}
	default:
		slot, found := database.DyeSlotByName(w)
		if !found {
			h.gmReply(sess, fmt.Sprintf("#dye: no slot %q; slots %s, armor or all", args[0], strings.Join(database.DyeSlotNames[:], " ")))
			return
		}
		slots = []int{slot}
	}
	color, err := database.ParseDyeColor(args[1])
	if err != nil {
		h.gmReply(sess, "#dye: "+err.Error())
		return
	}
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if a := sess.RemoteAddr(); a != nil {
		who.IP = a.IP.String()
	}
	next := cur
	changed := 0
	for _, slot := range slots {
		if _, audit, err := h.dyes.SetCharDye(serverID, slot, color, who, "#dye in the game"); err != nil {
			h.gmReply(sess, fmt.Sprintf("#dye %s: %v", database.DyeSlotNames[slot], err))
			return
		} else if audit != 0 {
			changed++
		}
		next[slot] = color
	}
	h.gameWorld.SetDyes(serverID, entity.Dyes(next))
	what := database.FormatDye(color)
	if color == 0 {
		what = "no dye"
	}
	names := make([]string, len(slots))
	for i, s := range slots {
		names[i] = database.DyeSlotNames[s]
	}
	h.gmReply(sess, fmt.Sprintf("%s's %s: %s (%d changed; seen by clients of the live game's look)", name, strings.Join(names, ", "), what, changed))
}
