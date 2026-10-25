package handlers

import (
	"log"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// respawnPlayer is the World's respawn hook (entity/respawn.go): a player who
// died PlayerRespawnDelay ago, HP and Power already restored, is put at its
// home (homeOf: its bind point, else where every login starts) and sent a
// MemoryDump.
//
// The dump, not a 0x00B2 teleport, because it is the only thing that ends a
// death on the client: CLIENT clears its dead flag (m_pC1B0) in Msg000D and
// nowhere a teleport reaches, so a PS2 that was only warped would stay black
// and deaf to its pad. Sandstorm respawns the same way (pcaps/breath_capture.pcap:
// the stat channel with HP back at its maximum, then the dump at the bind
// point). The dump goes alone, without the Camera1 and Time a login sends
// before it, and the client answers it with 0x0014 as it answers a login's.
func (h *CharacterHandler) respawnPlayer(sess *session.Session, p *entity.Player) {
	gw := h.gameWorld
	if gw == nil || sess == nil || p == nil {
		return
	}
	snap, ok := gw.PlayerSnapshot(p.ServerID)
	if !ok {
		return
	}
	x, y, z, world := snap.X, snap.Y, snap.Z, snap.World
	where := "where it died"
	if hx, hy, hz, hw, via, found := homeOf(gw, sess, p.ServerID); !found {
		log.Printf("Respawn: no home for '%s'; it respawns where it died", snap.Name)
	} else {
		x, y, z, world, where = hx, hy, hz, hw, via
	}

	placed, worldChanged, ok := gw.PlaceRespawn(p.ServerID, x, y, z, world)
	if !ok {
		return
	}
	// Another world is loaded as a warp's is: the old world's entities go and
	// the state channels wait for the load (released by the client's 0x0014
	// here, HandleCharacterInWorld, or its first 0x40 or 0x0049).
	if worldChanged {
		beginWorldLoad(sess, true)
	}
	gw.SyncSelfChannels(sess)
	h.queueMemoryDump(sess, &placed)

	log.Printf("Respawn: '%s' at %s (%.1f, %.1f, %.1f) world=%d (world changed: %v), %d/%d HP",
		placed.Name, where, placed.X, placed.Y, placed.Z, placed.World, worldChanged, placed.HP, placed.MaxHP)
}
