package handlers

// /roll: a random whole number, said to the roller and every player within
// sayRadius in its world (the user, 2026-10-30: "roll doesnt work"). No
// retail client has one: the Frontiers disc's CLIENT knows /alarm /command
// /create /follow /g(roup) /gu(ild) /mastery /r(eply) /s(ay) /sa /shout
// /t(ell) /time /tradelvl, and SOE's final live client adds none for it, so
// the command and its line are this server's.
//
//	/roll           1 to 100
//	/roll 20        1 to 20
//	/roll 5 50      5 to 50 (either order)
//
// /random is the same. A PS2 keeps an unknown /command to itself, so its
// player types #roll or #random, which go out as chat; vanilla forwards the
// slash.

import (
	"fmt"
	"log"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// rollMost is the largest number a roll takes.
const rollMost = 1_000_000_000

// rollRGB is a roll's line colour (0x0A7B): gold, apart from says (white),
// shouts (green) and the server's red.
var rollRGB = [3]uint8{0xFF, 0xD7, 0x40}

// rollDice is a whole number in [0, n): math/rand/v2's IntN (a test sets it).
var rollDice = rand.IntN

// isRollWord is whether a chat line's first word is the roll.
func isRollWord(w string) bool {
	switch strings.ToLower(w) {
	case "/roll", "/random", "#roll", "#random":
		return true
	}
	return false
}

// rollRange is a roll's numbers: none 1 to 100, one n 1 to n, two in either
// order; whole numbers from 0 (1 for a lone most) to rollMost.
func rollRange(args []string) (lo, hi int, ok bool) {
	num := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil && n >= 0 && n <= rollMost
	}
	switch len(args) {
	case 0:
		return 1, 100, true
	case 1:
		n, ok := num(args[0])
		if !ok || n < 1 {
			return 0, 0, false
		}
		return 1, n, true
	case 2:
		a, okA := num(args[0])
		b, okB := num(args[1])
		if !okA || !okB {
			return 0, 0, false
		}
		return min(a, b), max(a, b), true
	}
	return 0, 0, false
}

// roll rolls for player and tells everyone in say range, the roller too.
func (h *ChatHandler) roll(sess *session.Session, player *entity.Player, args []string) {
	lo, hi, ok := rollRange(args)
	if !ok {
		SendErrorMessage(sess, fmt.Sprintf("Roll: /roll, /roll <most> or /roll <least> <most>, whole numbers up to %d.", rollMost))
		return
	}
	n := lo + rollDice(hi-lo+1)
	line := fmt.Sprintf("%s rolls %d (%d to %d).", player.Name, n, lo, hi)
	hear := h.inSayRange(sess, player)
	for _, l := range hear {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeColoredChat)
		msg.SetData(coloredChatPayload(lineFor(l.p, line), rollRGB[0], rollRGB[1], rollRGB[2]))
		l.s.Queue().AddWithSequence(msg)
	}
	log.Printf("Roll: %q to %d players", line, len(hear))
}
