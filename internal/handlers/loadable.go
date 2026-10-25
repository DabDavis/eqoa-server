package handlers

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Which worlds a client can load, and places named by the NPCs standing there.
//
// A Frontiers disc names six worlds in CLIENT's gWorldInfo (discWorlds), and a
// retail PS2, eqoa-rust-client-vanilla and the web page all load from that
// table; only a PS2 running a patch has more: Sandstorm's S2 (eight, the eighth
// a second Secrets) and, chained after it, our hard-disk loader (WorldCount). A
// teleport into a world the client's table lacks moves nothing on the client
// while the server files the player under that world: invisible to everyone,
// alone with no NPC, until a GM or a relog rescues it. A GM's teleport to "Guk"
// from a page did exactly that (F3 20261025-000749: the label of an August test
// gatekeeper in world 8). So the travel menus, the GM's place names and the GM
// panel offer what the client can load, and a teleport anywhere else is refused.

// DiscWorld reports a world every client has: one of the disc's six.
func DiscWorld(world uint8) bool { return int(world) < discWorlds }

// s2Worlds is how many worlds Sandstorm's S2 names (the disc's six, world6 and
// a second Secrets).
const s2Worlds = 8

// LoadableWorlds is the test of what sess's client can load: the disc's six,
// the whole table for a client that said it runs the July S2 (DiscVersion
// 0x13, S2's marker on its world session, which our hard-disk loader rides
// on), or S2's eight for Sandstorm's live S2 (0x15, which has no hard-disk
// loader after it). No session (an offline character, a list for no one in
// particular) is the disc's six.
func LoadableWorlds(sess *session.Session) func(world uint8) bool {
	if sess != nil {
		switch sess.DiscVersion() {
		case protocol.GameVersionSandstormS2:
			return func(world uint8) bool { return int(world) < WorldCount }
		case protocol.GameVersionSandstormS2Live:
			return func(world uint8) bool { return int(world) < s2Worlds }
		}
	}
	return DiscWorld
}

// CanLoadWorld reports whether sess's client can load world.
func CanLoadWorld(sess *session.Session, world uint8) bool { return LoadableWorlds(sess)(world) }

// CannotLoad is the refusal for a teleport of who into a world their game
// lacks.
func CannotLoad(who string, world uint8) string {
	return fmt.Sprintf("%s's game cannot load %s (world %d): it has the disc's %d worlds",
		who, worldName(world), world, discWorlds)
}

// namesakeSpan is how far from their middle the NPCs a name finds may stand
// and still be one place: half a zone of the world's grid (zones are 2,000
// across), a camp or a dungeon's rooms, not a road's length of guards.
const namesakeSpan = 1000

// NamesakePlace is where the NPCs whose names hold q (as searchNames finds
// them: the name begins with it, or a later word does) stand, of those in a
// world loadable takes (nil: any), when they stand together in one world: the
// spawn spot of the one nearest their middle, and how many there are. "guk"
// is the "froglok guk ..." of Guk. ok is false when none is found, or they
// stand apart (Guard Dahvel and the Captain of the Guard are not one place),
// or in more than one world.
//
// A place is named by its people because the server holds nothing else that
// names it: the zones' names are the game's own (CLIENT's ZoneName), which
// this repository does not carry.
func NamesakePlace(gw *entity.World, q string, loadable func(world uint8) bool) (entity.Place, int, bool) {
	q = strings.ToLower(strings.TrimSpace(q))
	if gw == nil || q == "" {
		return entity.Place{}, 0, false
	}
	var spots []entity.Place
	for _, s := range gw.NPCsNamed(func(name string) bool {
		l := strings.ToLower(name)
		return strings.HasPrefix(l, q) || strings.Contains(l, " "+q)
	}) {
		if loadable == nil || loadable(s.World) {
			spots = append(spots, s)
		}
	}
	if len(spots) == 0 {
		return entity.Place{}, 0, false
	}
	var mx, mz float64
	for _, s := range spots {
		if s.World != spots[0].World {
			return entity.Place{}, len(spots), false
		}
		mx += float64(s.X)
		mz += float64(s.Z)
	}
	mx /= float64(len(spots))
	mz /= float64(len(spots))
	best, bestD := 0, math.Inf(1)
	for i, s := range spots {
		d := math.Hypot(float64(s.X)-mx, float64(s.Z)-mz)
		if d > namesakeSpan {
			return entity.Place{}, len(spots), false
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	return spots[best], len(spots), true
}
