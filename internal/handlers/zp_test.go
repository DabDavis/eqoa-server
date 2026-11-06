package handlers

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Every trip of Zaoran Plateau's lands on an NPC that has a trip back to
// where it began, so no portal strands a raid (tools/seed_zp.py places them).
func TestZaoranTripsGoBothWays(t *testing.T) {
	zp := []string{"Portal of Rallos", "Portal to the Akessan Bridges", "Razeal's Summoning Post", "Portal to the Plateau"}
	for _, from := range zp {
		if !IsWaysender(from) {
			t.Errorf("%s is not a waysender", from)
		}
		routes := wayRoutes[from]
		if len(routes) != 1 {
			t.Fatalf("%s has %d trips, want 1", from, len(routes))
		}
		back := wayRoutes[routes[0].Arrive]
		if len(back) != 1 || back[0].Arrive != from {
			t.Errorf("%s lands on %s, whose trips %v do not lead back", from, routes[0].Arrive, back)
		}
	}
}

// The Portal of Rallos, in the Akessan town's north-west corner, sends a level
// 60 to Rallos Zek's arena, in front of the Portal to the Akessan Bridges, and
// that portal sends anyone back in front of it; Razeal's Summoning Post by the
// cemetery does the same for Razeal's isle. The control: a level 59 is too
// weak for either and is sent nowhere.
func TestZaoranPortalsSendTheRaidOn(t *testing.T) {
	for _, c := range []struct {
		from, to, place string
		fx, fy, fz      float32
		ffacing         float64
		tx, ty, tz      float32
		tfacing         float64
	}{
		// facing bytes as tools/seed_zp.py writes them, in radians as the loader has them
		{"Portal of Rallos", "Portal to the Akessan Bridges", "Rallos Zek's arena",
			5525, 152.4, 6410, 0, 5670, 218.2, 6295, 96.0 / 256 * 2 * math.Pi},
		{"Razeal's Summoning Post", "Portal to the Plateau", "Razeal's isle",
			5025, 147.6, 6800, math.Pi, 5295, 336.2, 7570, math.Pi / 2},
	} {
		tw := newTalkWorld(t, c.fx+2, c.fz+2)
		tw.p.World, tw.p.Y, tw.p.Level = 5, c.fy, 59
		from := tw.add(1, &entity.NPC{ServerID: 1, Name: c.from, World: 5, SpawnX: c.fx, SpawnY: c.fy, SpawnZ: c.fz,
			SpawnFacing: float32(c.ffacing)})
		to := &entity.NPC{ServerID: 2, Name: c.to, World: 5, SpawnX: c.tx, SpawnY: c.ty, SpawnZ: c.tz,
			SpawnFacing: float32(c.tfacing)}
		tw.w.NPCs[2] = to

		tw.talk(from)
		if box := boxSaid(tw.s); !strings.Contains(box, "too weak") || !strings.Contains(box, c.place) {
			t.Fatalf("a level 59 at the %s was told %q, want too weak for %s", c.from, box, c.place)
		}
		tw.s.Queue().GetPending()
		tw.pick(from, 0)
		if _, _, _, moved := teleported(tw.s); moved {
			t.Fatalf("the control: a level 59 was sent to %s", c.place)
		}

		tw.p.Level = 60
		tw.talk(from)
		if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
			t.Fatalf("a level 60 at the %s was offered %q, want Yes and No", c.from, got)
		}
		tw.pick(from, 0)
		ax, az, _ := inFrontOf(to.SpawnX, to.SpawnZ, to.SpawnFacing)
		if x, z, world, moved := teleported(tw.s); !moved || world != 5 || math.Abs(float64(x-ax)) > 0.01 || math.Abs(float64(z-az)) > 0.01 {
			t.Errorf("the %s sent a level 60 to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f), in front of the %s",
				c.from, x, z, world, moved, ax, az, c.to)
		}

		// the way back, at any level
		tw.s.Queue().GetPending()
		tw.p.X, tw.p.Y, tw.p.Z, tw.p.Level = ax, c.ty, az, 1
		way := tw.add(3, to)
		tw.talk(way)
		tw.s.Queue().GetPending()
		tw.pick(way, 0)
		bx, bz, _ := inFrontOf(c.fx, c.fz, float32(c.ffacing))
		if x, z, world, moved := teleported(tw.s); !moved || world != 5 || math.Abs(float64(x-bx)) > 0.01 || math.Abs(float64(z-bz)) > 0.01 {
			t.Errorf("the %s sent a level 1 to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f), in front of the %s",
				c.to, x, z, world, moved, bx, bz, c.from)
		}
	}
}
