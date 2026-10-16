package handlers

import (
	"strconv"
	"testing"
)

// TestEveryReachableWorldHasAPath pins worldPaths against worldNames.
//
// These are two hand-maintained tables describing the same set of worlds, and
// they drifted: worldNames gained "Test" (index 9) so `#goto x,y,z,9` would
// accept it, while worldPaths stopped at 8. A world that
// worldNames accepts but worldPaths lacks is reachable by teleport — which
// indexes the CLIENT's world table — and unreachable by login, which uses this
// string. So the world change works and the relog silently lands the character
// in Tunaria at the coordinates they left the other world at.
//
// That is not a cosmetic difference and it does not announce itself. World 9 is
// Tunaria zone 127 lifted, so its coordinates are legal in world 0 too: the
// position/world pair stays self-consistent, CoherentWorld sees nothing wrong,
// and the server keeps filtering visibility to world 9 while the client renders
// world 0. Harbormaster Machado, a world 9 NPC, appeared in world 0 that way.
func TestEveryReachableWorldHasAPath(t *testing.T) {
	for i := range worldNames {
		w := uint8(i)
		if _, ok := worldPaths[w]; !ok {
			t.Errorf("world %d (%q) is reachable by #goto but has no esfPath; "+
				"a login into it would be sent to %q instead",
				i, worldNames[i], worldPaths[0])
		}
	}
	if len(worldPaths) != len(worldNames) {
		t.Errorf("worldPaths has %d entries, worldNames %d — a path for a world "+
			"nothing can name is as wrong as a name with no path",
			len(worldPaths), len(worldNames))
	}
}

// TestWorldPathForDoesNotSilentlyRelocate checks that the worlds we actually
// build resolve to their own file rather than the Tunaria fallback. The fallback
// is deliberate for a genuinely unknown index, but it must never be how a world
// we ship gets loaded.
func TestWorldPathForDoesNotSilentlyRelocate(t *testing.T) {
	// Slots 0-7 are S2's; 8 upward follow the SD's naming convention, and these
	// strings are the client's baked table entries, not a choice made here —
	// make_hdd_sd.py writes "hdd:worldN.ESF" into slot N. Guk is deliberately
	// absent: it was the MEMORY CARD world, and the SD no longer hooks the
	// "mc:" prefix that reached it.
	cases := map[uint8]string{
		0:              "data\\tunaria.esf",
		8:              "hdd:world8.ESF",
		9:              "hdd:world9.ESF",
		WorldCount - 1: "hdd:world29.ESF",
		// 6 held "hdd:HDDSETUP.ESF", the setup SCREEN. With the screen gone it
		// follows the same convention as every slot above 7.
		6: "hdd:world6.ESF",
	}
	for world, want := range cases {
		if got := worldPathFor(world); got != want {
			t.Errorf("worldPathFor(%d) = %q, want %q", world, got, want)
		}
	}
}

// TestWorldNamesMatchTheConvention pins the names the SD bakes into the client's
// table against the ones this server prints. They are two copies of the same
// decision — the client shows its string, GM output shows ours — and a mismatch
// is invisible until someone reads a world name that is not the world they are
// standing in. That drift is what put "Odus(spare)" on a slot for a while.
func TestWorldNamesMatchTheConvention(t *testing.T) {
	for i := 8; i < WorldCount; i++ {
		want := "world" + strconv.Itoa(i)
		if worldNames[i] != want {
			t.Errorf("worldNames[%d] = %q, want %q (make_hdd_sd.py bakes %q "+
				"into the client's own table)", i, worldNames[i], want, want)
		}
	}
}

// TestWorldSixIsAnOrdinaryUnusedWorld is the other half of removing the setup
// screen, and it is the half that can rot silently.
//
// Slot 6 was never a world: a change to it made the SD payload ExecPS2
// EQSCENE.BIN instead of opening a file. The screen is gone, so the slot must
// carry NO path -- a leftover "hdd:HDDSETUP.ESF" would be a path no payload
// watches for any more, and a player sent there would sit on a load screen
// forever while the server believed they had arrived.
func TestWorldSixIsAnOrdinaryUnusedWorld(t *testing.T) {
	if p := worldPaths[6]; p != "hdd:world6.ESF" {
		t.Errorf("world 6 names %q; the setup screen is gone and the slot "+
			"follows the hdd:worldN convention now", p)
	}
	// And it is a PLACE again, so the guards that were skipped for it apply.
	if IsSendOnlyWorld(6) {
		t.Error("world 6 is still send-only; the setup screen it was reserved " +
			"for no longer exists")
	}
}
