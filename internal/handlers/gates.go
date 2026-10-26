package handlers

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/entity"
)

// gateLeftBehind is told a group member a gate would take to a world its
// game cannot load (entity/gates.go); ours, as the casters' refusal is.
const gateLeftBehind = "Your game cannot load %s: you stay behind."

// travelPlayer is the World's travel hook (entity/gates.go; jib's review, F3
// pass 19): a player a gate spell sends, landed as Return Home lands one
// (gatePlayer): the teleport (0x07F6) there, the world loaded when it
// changed, the player alive, so no dump. An evacuation's traveller goes to
// its caster's home, homeOf as Return Home takes the caster (its bind point,
// else the login spawn); a group lands round its caster's spot
// (entity.GateRingSpot). A player whose game cannot load the place stays,
// told in red; its pet comes along with it, or stays, as through any
// teleport (entity/pet.go).
func (h *CharacterHandler) travelPlayer(t entity.GateTravel) {
	gw := h.gameWorld
	if gw == nil || t.Sess == nil || t.P == nil {
		return
	}
	to := t.To
	if t.Home != 0 {
		x, y, z, world, via, ok := homeOf(gw, gw.FindSessionByEntityID(t.Home), t.Home)
		if !ok {
			log.Printf("Gate: no home for %q's caster; %s stays", t.Spell, t.P.Name)
			return
		}
		to = entity.GateSpot{World: world, X: x, Y: y, Z: z, Place: via}
	}
	to = gw.GateRingSpot(to, t.Ring)
	if !CanLoadWorld(t.Sess, to.World) {
		SendColoredChat(t.Sess, fmt.Sprintf(gateLeftBehind, to.Place), 255, 63, 63)
		log.Printf("Gate: %s's game cannot load world %d; %q leaves it behind", t.P.Name, to.World, t.Spell)
		return
	}
	if _, err := ValidateTeleport(to.X, to.Y, to.Z, to.World); err != nil {
		log.Printf("Gate: %q's landing for %s is no place to stand (%v); it stays", t.Spell, t.P.Name, err)
		return
	}
	// the world and the place set under the lock, standing still, as a
	// respawn's are (the same move)
	placed, worldChanged, ok := gw.PlaceRespawn(t.P.ServerID, to.X, to.Y, to.Z, to.World)
	if !ok {
		return
	}
	sendTeleportGM(t.Sess, to.X, to.Y, to.Z, placed.Facing, to.World)
	beginWorldLoad(t.Sess, worldChanged)
	log.Printf("Gate: %q lands %s at %s (%.1f, %.1f, %.1f) world=%d", t.Spell, t.P.Name, to.Place, to.X, to.Y, to.Z, to.World)
}
