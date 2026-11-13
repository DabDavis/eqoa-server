package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// /aura (#aura): a GM gives an aura to the NPCs of its target's name (table
// npctint's glow and aura columns, entity/npctint.go) or to an item's worn
// piece (table itemaura, entity/pieceaura.go), kept, audited and sent at once
// to every client that draws auras (the owner, 2026-11-13: Mr. Moogie's aura
// "can be a template to make any model have particle effects ... even pieces
// of armor like bracers"; claude-notes/auras-20261113.md):
//
//	/aura <kind> <RRGGBBAA> [source] [nodes]    the target NPC's name
//	/aura off                                   its name's glow and aura gone (its tint kept)
//	/aura item <id> <kind> <RRGGBBAA> [source] [nodes]
//	/aura item <id> off
//
// kind: default, none, shell, particles, both (or 0-4); the colour hex, 0
// none; source a DictID in hex (0 the Sword of Virtue's); nodes 0-32.

const auraUsage = "Usage: /aura <kind> <RRGGBBAA> [source] [nodes] on your target NPC's name, /aura off; " +
	"/aura item <id> <kind> <RRGGBBAA> [source] [nodes], /aura item <id> off. kind: default, none, shell, particles, both " +
	"(0-4); source: a DictID in hex (0 the Sword of Virtue's); nodes 0-32 (0 the default)"

// auraKinds are the kinds by name (database.AuraDefault ...).
var auraKinds = map[string]uint8{"default": database.AuraDefault, "none": database.AuraNone, "shell": database.AuraShell,
	"particles": database.AuraParticles, "both": database.AuraBoth}

// auraKindNames are the kinds' names by number.
var auraKindNames = [...]string{"default", "none", "shell", "particles", "both"}

// parseHex32 reads a 32-bit hex value, with or without 0x.
func parseHex32(s string) (uint32, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(s), "0x"), "#")
	v, err := strconv.ParseUint(s, 16, 32)
	return uint32(v), err == nil
}

// parseAura reads <kind> <RRGGBBAA> [source] [nodes], or "off" (all zero).
func parseAura(args []string) (a database.ItemAura, ok bool) {
	if len(args) == 1 && strings.EqualFold(args[0], "off") {
		return a, true
	}
	if len(args) < 2 || len(args) > 4 {
		return a, false
	}
	k, named := auraKinds[strings.ToLower(args[0])]
	if !named {
		n, err := strconv.Atoi(args[0])
		if err != nil || !database.ValidAuraKind(int64(n)) {
			return a, false
		}
		k = uint8(n)
	}
	a.Kind = k
	if a.Color, ok = parseHex32(args[1]); !ok {
		return a, false
	}
	if len(args) > 2 {
		if a.Source, ok = parseHex32(args[2]); !ok {
			return a, false
		}
	}
	if len(args) > 3 {
		n, err := strconv.Atoi(args[3])
		if err != nil || !database.ValidAuraNodes(int64(n)) {
			return a, false
		}
		a.Nodes = uint8(n)
	}
	return a, true
}

// auraWords says an aura in words.
func auraWords(a database.ItemAura) string {
	if !a.Shows() {
		return "no aura"
	}
	kind := strconv.Itoa(int(a.Kind))
	if int(a.Kind) < len(auraKindNames) {
		kind = auraKindNames[a.Kind]
	}
	src := "the Sword of Virtue's"
	if a.Source != 0 {
		src = fmt.Sprintf("%08X's", a.Source)
	}
	nodes := "the default nodes"
	if a.Nodes != 0 {
		nodes = fmt.Sprintf("%d nodes", a.Nodes)
	}
	return fmt.Sprintf("aura %s in %08X (particles %s, %s)", kind, a.Color, src, nodes)
}

// auraCommand is /aura's: see the file's doc.
func (h *ChatHandler) auraCommand(sess *session.Session, player *entity.Player, args []string) {
	db := h.camps.db
	if db == nil {
		h.gmReply(sess, "/aura: no database: nothing is kept")
		return
	}
	if len(args) > 0 && strings.EqualFold(args[0], "item") {
		if len(args) < 3 {
			h.gmReply(sess, auraUsage)
			return
		}
		id, err := strconv.Atoi(args[1])
		a, ok := parseAura(args[2:])
		if err != nil || id < 0 || !ok {
			h.gmReply(sess, auraUsage)
			return
		}
		a.PatternID = id
		found, err := db.SetItemAuraAudited(a, gmEditOf(sess), fmt.Sprintf("/aura in the game: item %d %s", id, auraWords(a)))
		switch {
		case err != nil:
			h.gmReply(sess, "/aura: not kept: "+err.Error())
			return
		case !found:
			h.gmReply(sess, fmt.Sprintf("/aura: no item %d", id))
			return
		}
		n, err := h.gameWorld.LoadItemAuras(db)
		if err != nil {
			h.gmReply(sess, "/aura: kept, not yet in the world: "+err.Error())
			return
		}
		h.gmReply(sess, fmt.Sprintf("Item %d: %s, on its worn piece at once (%d items with an aura).", id, auraWords(a), n))
		log.Printf("GM: %s aura item %d: %s", player.Name, id, auraWords(a))
		return
	}
	a, ok := parseAura(args)
	if !ok {
		h.gmReply(sess, auraUsage)
		return
	}
	if player.TargetObjectID == 0 || player.TargetObjectID == entity.NoAssist {
		h.gmReply(sess, "/aura: target a creature (or /aura item <id> ...)")
		return
	}
	ent, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	npc, isNPC := ent.(*entity.NPC)
	if !found || !isNPC || database.NPCClassKey(npc.Name) == "" {
		h.gmReply(sess, "/aura: your target is no creature (an item's: /aura item <id> ...)")
		return
	}
	name := database.NPCClassKey(npc.Name)
	if err := db.SetNPCTintAuraAudited(name, a.Color, a.Kind, a.Source, a.Nodes, gmEditOf(sess),
		fmt.Sprintf("/aura in the game: %s %s", name, auraWords(a))); err != nil {
		h.gmReply(sess, "/aura: not kept: "+err.Error())
		return
	}
	_, told, err := h.gameWorld.LoadNPCTints(db)
	if err != nil {
		h.gmReply(sess, "/aura: kept, not yet in the world: "+err.Error())
		return
	}
	h.gmReply(sess, fmt.Sprintf("Every %s: %s, kept; sent to %d client(s) that draw auras.", name, auraWords(a), told))
	log.Printf("GM: %s aura %s: %s", player.Name, name, auraWords(a))
}
