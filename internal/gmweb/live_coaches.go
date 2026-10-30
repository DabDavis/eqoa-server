package gmweb

import (
	"math"
	"strings"

	"github.com/eqoa/server/internal/entity"
)

// The stables' ledgers in the panel (table charcoaches, handlers/
// coachledger.go): a row is a town a character has signed, so a GM sees a
// character's and grants or takes one as any row is changed, audited and
// revertible. The game reads the table at each talk to a coachman, so a
// change needs nothing done to the running world.

// coachTownHelp is charcoaches.town's help: the names a ledger may name.
func coachTownHelp() string {
	return "the town as the coach menus list it, one of: " + strings.Join(entity.CoachTownNames(), ", ")
}

// checkCoachRow holds a charcoaches row to a character and a town a coach
// menu lists (entity.CoachTownNames, spelled as they are), signed in the game
// or granted.
func checkCoachRow(row map[string]any) error {
	id, ok := asFloat(row["serverid"])
	if !ok || id != math.Trunc(id) || id <= 0 {
		return badRequest("serverid is the character's (characters.serverid)")
	}
	town, _ := row["town"].(string)
	names := entity.CoachTownNames()
	known := false
	for _, n := range names {
		if n == town {
			known = true
			break
		}
	}
	if !known {
		for _, n := range names {
			if strings.EqualFold(strings.TrimSpace(town), n) {
				return badRequest("town %q is spelled %q", town, n)
			}
		}
		return badRequest("town %q is no coach town: one of %s", town, strings.Join(names, ", "))
	}
	if v, ok := row["source"]; ok && v != nil {
		if src, _ := v.(string); src != "talk" && src != "gm" {
			return badRequest("source is talk (signed in the game) or gm (granted here)")
		}
	}
	return nil
}
