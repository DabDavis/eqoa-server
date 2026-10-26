package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Teleporting a character from somewhere that is not the game.
//
// The account pages on the web port can move any character on the logged-in
// account. That is the same act #tele performs, so it resolves through the same
// table and the same rules rather than growing a second idea of where "freeport"
// is — a destination that drifts between the two would be found by a player
// standing in the wrong place, not by a test.

// TeleDestInfo is one named destination, as something outside this package sees
// it. The coordinates are deliberately absent: they depend on where the defining
// NPC actually spawned, which is a question only ResolveTeleDest can answer.
type TeleDestInfo struct {
	Name  string
	Label string
	World byte
}

// TeleDestinations lists the named shortcuts in the same stable order the in-game
// help does.
func TeleDestinations() []TeleDestInfo {
	names := teleNames()
	out := make([]TeleDestInfo, 0, len(names)+len(campDests))
	for _, n := range names {
		d := teleDests[n]
		out = append(out, TeleDestInfo{Name: n, Label: d.Label, World: d.World})
	}
	// then the XP camps, by level (camps.go)
	for _, n := range campNames() {
		d := campDests[n]
		out = append(out, TeleDestInfo{Name: n, Label: fmt.Sprintf("XP camp, levels %s: %s", d.Levels, d.Label), World: d.World})
	}
	return out
}

// WarpDestinations lists what a Harbormaster offers — every cross-world travel
// NPC, at its own spawn point — keyed by NPC name so a route survives its label
// being rewritten. Only the disc's six worlds: the list is for a page, which
// cannot know what the console behind a character loads (LoadableWorlds).
func WarpDestinations(gw *entity.World) []TeleDestInfo {
	if gw == nil {
		return nil
	}
	points := gw.WarpPoints(DiscWorld)
	out := make([]TeleDestInfo, 0, len(points))
	for _, wp := range points {
		out = append(out, TeleDestInfo{Name: wp.NPC, Label: wp.Label, World: wp.World})
	}
	return out
}

// ResolveTeleDest turns a shortcut name into a place.
//
// It prefers the spawn point of the NPC that DEFINES the destination and falls
// back to the literal coordinates only when that NPC is not loaded — saying so
// in `via`, because a silent fallback is how a destination goes on working after
// the thing that defines it is gone. Extracted from the #tele handler so the web
// route cannot answer this question differently.
func ResolveTeleDest(gw *entity.World, name string) (x, y, z float32, world byte, via string, ok bool) {
	_, dest, found := findTeleDest(name)
	if !found {
		// Not a #tele shortcut, so try the travel NPCs — the account pages offer
		// exactly what a Harbormaster offers, and name a route by the NPC that
		// defines it rather than by its label, which is display text and gets
		// renamed the moment a world gets a real name.
		//
		// WarpPoints is the whole allowed set, which is what keeps this from
		// being "teleport to any NPC by name": a route has to be one a player
		// could have walked up to and asked for.
		// The whole set: whether the client can load the world is the
		// mover's to check (CanLoadWorld), with the client in hand.
		if gw != nil {
			for _, wp := range gw.WarpPoints(nil) {
				if strings.EqualFold(wp.NPC, strings.TrimSpace(name)) {
					return wp.X, wp.Y, wp.Z, wp.World, wp.Label, true
				}
			}
		}
		return 0, 0, 0, 0, "", false
	}
	x, y, z, world = dest.X, dest.Y, dest.Z, dest.World
	via = dest.Label
	if dest.NPC != "" && gw != nil {
		spawnOf := gw.SpawnOfNPC
		if dest.Near {
			// a camp's: beside its own creatures of the name, not on one, nor
			// at a namesake across the world (entity/camplanding.go)
			spawnOf = func(name string) (float32, float32, float32, float32, uint8, bool) {
				x, y, z, ok := gw.CampLanding(name, dest.World, dest.X, dest.Z, campAnchorReach)
				return x, y, z, 0, dest.World, ok
			}
		}
		if nx, ny, nz, _, nw, spawned := spawnOf(dest.NPC); spawned {
			x, y, z, world = nx, ny, nz, nw
		} else {
			via = dest.Label + " [fallback: " + dest.NPC + " not loaded]"
			log.Printf("tele %s: %q is not loaded, using literal "+
				"(%.1f, %.1f, %.1f) world=%d", name, dest.NPC, x, y, z, world)
		}
	}
	return x, y, z, world, via, true
}

// ValidateTeleport refuses the two placements this codebase has already been
// stranded by, and returns the world it would actually save.
//
// Both are guards that exist because the bad value was written once and then had
// to be detected forever afterwards:
//
//   - a height at or below the world floor is a FALL, not a position. Saving one
//     makes the next login spawn at the floor and fall again, permanently.
//   - a world and a position that do not agree strand a character where
//     visibility filters to a world they are not in and no NPC ever appears.
//
// A web form can produce both by typing, which is exactly why it must not be the
// one place that skips the check.
func ValidateTeleport(x, y, z float32, world byte) (byte, error) {
	if int(world) >= WorldCount {
		return world, fmt.Errorf("world %d does not exist (0-%d)", world, WorldCount-1)
	}
	if y <= entity.WorldFloorY {
		return world, fmt.Errorf("y=%.1f is at or under the world floor (%.0f); "+
			"that is a fall, not a place to stand", y, entity.WorldFloorY)
	}
	if w, ok := entity.CoherentWorld(world, x, z); !ok {
		return w, fmt.Errorf("world %d does not contain (%.0f, %.0f)", world, x, z)
	}
	return world, nil
}

// TeleportOnlineCharacter moves a character who is currently in the world, and
// reports whether it found one.
//
// FALSE IS NOT A FAILURE — it means the character is not logged in, and the
// caller should write the database row instead. Which of the two is right is not
// a preference: a logged-in character's position is held in memory and written
// back on disconnect, so a database write would be silently overwritten by the
// player's own logout; and a logged-out character has no session to send a
// teleport packet to.
func TeleportOnlineCharacter(gw *entity.World, sm *session.Manager,
	serverID entity.EntityID, x, y, z float32, world byte) bool {

	if gw == nil || sm == nil {
		return false
	}

	var (
		target *session.Session
		player *entity.Player
	)
	sm.ForEach(func(sess *session.Session) bool {
		p := gw.SessionPlayer(sess)
		if p == nil || p.ServerID != serverID {
			return true
		}
		target, player = sess, p
		return false
	})
	if target == nil {
		return false
	}

	// A HARBORMASTER'S WARP, not a GM #goto — same primitive, same order.
	//
	// The travel NPCs increment a per-player teleport counter and send it with
	// the packet; #goto always sends 0. That counter is the only difference
	// between the two sends, and the NPC route is the one proven to move a
	// player who is standing in the world, so this performs the warp exactly as
	// they do rather than approximately.
	//
	// The state change goes through BeginWarp because this runs on an HTTP
	// goroutine: the handlers write player.World and the counter directly, which
	// is safe only on the goroutine the UDP listener serialises.
	facing, counter, worldChanged, ok := gw.BeginWarp(player.ServerID, world)
	if !ok {
		return false
	}

	// Order matters and is the NPCs': tell the client, hold the channels until
	// it reports back with 0x0049, and only then move it in the world.
	sendTeleportWarp(target, world, x, y, z, facing, counter)
	beginWorldLoad(target, worldChanged)
	gw.UpdatePosition(player.ServerID, x, y, z, facing)

	log.Printf("Web warp: %s to (%.1f, %.1f, %.1f) world=%d (counter=%d, worldChanged=%v)",
		player.Name, x, y, z, world, counter, worldChanged)
	return true
}

// WorldNames lists the client's world table by index, so a page outside this
// package can say "Tunaria" rather than "world 0". Slots 8 upward are filled by
// init() from the SD's naming convention, so this is generated rather than a
// second copy that would go stale the moment a world is added.
func WorldNames() []string {
	out := make([]string, WorldCount)
	for i := range out {
		out[i] = worldName(byte(i))
	}
	return out
}

// IsSendOnlyWorld reports a world that may be sent to but never stored.
//
// THERE IS NO SUCH WORLD TODAY, and the function stays because the CLASS is
// real rather than because slot 6 was in it. World 6 held the HDD setup screen:
// a world change to it made the SD payload ExecPS2 EQSCENE.BIN instead of
// opening anything, so it was somewhere a player GOES and never somewhere a
// player IS -- a character whose row carried it relaunched the screen on every
// login. The screen is gone (its disk verbs are on the command channel), and
// slot 6 is an ordinary unused world again.
//
// Keeping the guard means the next send-only destination is one line here
// instead of a rediscovery of why storing it breaks a character's row.
func IsSendOnlyWorld(world byte) bool {
	return false
}
