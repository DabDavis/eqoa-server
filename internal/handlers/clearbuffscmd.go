package handlers

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// clearBuffsCommand is #clearbuffs (#nukebuffs too, EQEmu's name for it), a
// GM's: every effect on the GM's own character ended now, as its time
// running out ends it (entity.World.ClearBuffs: off the effects channel, the
// stats summed again, the run speed told, an illusion's body and
// invisibility gone with theirs), the harmful with the rest, and the slots
// #effect put on; answered with what went. F3 20261029-011146, jib: "Add a
// GM Test command to clear all buffs on the character issuing it". It takes
// no word after it: it is the GM's own buffs, never a target's.
func (h *ChatHandler) clearBuffsCommand(sess *session.Session) {
	names, slots, ok := h.gameWorld.ClearBuffs(sess)
	if !ok {
		return
	}
	if len(names) == 0 && slots == 0 {
		h.gmReply(sess, "No buffs on you")
		return
	}
	if len(names) > 0 {
		h.gmReply(sess, fmt.Sprintf("Buffs cleared (%d): %s", len(names), strings.Join(names, ", ")))
	}
	if slots > 0 {
		h.gmReply(sess, fmt.Sprintf("#effect's slots cleared: %d", slots))
	}
}
