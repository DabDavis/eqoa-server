package handlers

import (
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// teleportFacing is the last 0x00B2 queued for the session: its world, place
// and heading (Msg00B2: u8 world, pos, then the heading CLIENT's Respawn turns
// the player to).
func teleportFacing(s *session.Session) (world byte, x, y, z, facing float32, ok bool) {
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 19 && protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeTeleport {
			world, x, y, z, facing, ok = d[2], f32(d[3:]), f32(d[7:]), f32(d[11:]), f32(d[15:]), true
		}
	}
	return
}

// A teleport orb (entity/orbs.go) through the hooks the server sets: a
// player's reports walk it into the orb, the tick sends it by travelPlayer,
// and its client is told to stand at the row's destination turned to the
// row's facing, which the server's player takes too. The control: a gate's
// traveller (no facing) lands as it faced. The places are made up.
func TestAnOrbsTripLandsTurnedToItsFacing(t *testing.T) {
	c := newChatWorld(t)
	ch := &CharacterHandler{sessionManager: c.sm, gameWorld: c.gw}
	c.gw.OnTravel(ch.travelPlayer)
	c.gw.SetLoadableWorlds(LoadableWorlds)
	s, p := c.join(1, "Stepper", 5000, 5000, 0)
	p.HP, p.MaxHP = 100, 100
	c.gw.UpdatePosition(p.ServerID, 5000, 50, 5000, 2.5)
	c.gw.SetOrbs([]database.Orb{{ID: 7, Name: "a made orb", X: 5000, Y: 50, Z: 5006,
		ToX: 5300, ToY: 51, ToZ: 5296.5, ToFacing: 1.25}})
	now := time.Unix(1_800_000_000, 0)
	for z := float32(5001); z <= 5006; z++ {
		now = now.Add(100 * time.Millisecond)
		c.gw.ApplyClientUpdate(p.ServerID, entity.ClientUpdate{X: 5000, Y: 50, Z: z, Facing: 0, HasFacing: true, At: now})
	}
	s.Queue().GetPending()
	c.gw.Tick(now)
	world, x, y, z, facing, ok := teleportFacing(s)
	if !ok {
		t.Fatal("no 0x00B2 after the step into the orb")
	}
	if world != 0 || x != 5300 || y != 51 || z != 5296.5 || facing != 1.25 {
		t.Errorf("0x00B2 to world %d (%.1f, %.1f, %.1f) facing %.2f, want world 0 (5300, 51, 5296.5) facing 1.25", world, x, y, z, facing)
	}
	if p.X != 5300 || p.Z != 5296.5 || p.Facing != 1.25 {
		t.Errorf("the server's player at (%.1f, %.1f) facing %.2f, want the destination facing 1.25", p.X, p.Z, p.Facing)
	}

	// the control: a gate's traveller lands as it faced
	s.Queue().GetPending()
	c.gw.UpdatePosition(p.ServerID, 5300, 51, 5290, 2.75)
	ch.travelPlayer(entity.GateTravel{Sess: s, P: p, To: entity.GateSpot{World: 0, X: 5100, Y: 50, Z: 5100, Place: "a made gate"}, Spell: "a made gate"})
	if _, _, _, _, facing, ok := teleportFacing(s); !ok || math.Abs(float64(facing-2.75)) > 1e-6 || p.Facing != 2.75 {
		t.Errorf("the control: a gate's traveller sent facing %.2f (%v), the player facing %.2f; want its own 2.75", facing, ok, p.Facing)
	}
}
