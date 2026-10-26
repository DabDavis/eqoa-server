package handlers

import (
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Every camp is a one-word name #tele takes (it reads one word), listed by its
// levels, in a world every game loads, and no camp or alias hides a place.
func TestCampDestsAreTeleNames(t *testing.T) {
	if len(campDests) < 40 {
		t.Fatalf("%d camps; tools/gen_camp_teles.py wrote 49", len(campDests))
	}
	for n, d := range campDests {
		if n != strings.ToLower(n) || strings.ContainsAny(n, " \t") {
			t.Errorf("camp %q is not one lower-case word", n)
		}
		if _, clash := teleDests[n]; clash {
			t.Errorf("camp %q hides the place of that name", n)
		}
		if !d.Near || d.NPC == "" || d.Levels == "" || d.Label == "" {
			t.Errorf("camp %q: %+v", n, d)
		}
		if !DiscWorld(d.World) {
			t.Errorf("camp %q is in world %d, which not every game loads", n, d.World)
		}
		if key, _, ok := findTeleDest(strings.ToUpper(n)); !ok || key != n {
			t.Errorf("findTeleDest(%q) = %q, %v", strings.ToUpper(n), key, ok)
		}
	}
	for a, n := range campAliases {
		if _, ok := campDests[n]; !ok {
			t.Errorf("alias %q names no camp (%q)", a, n)
		}
		if _, clash := teleDests[a]; clash {
			t.Errorf("alias %q hides the place of that name", a)
		}
		if _, clash := campDests[a]; clash {
			t.Errorf("alias %q is a camp's own name too", a)
		}
		if key, _, ok := findTeleDest(a); !ok || key != n {
			t.Errorf("findTeleDest(%q) = %q, %v; want %q", a, key, ok, n)
		}
	}
	if _, _, ok := findTeleDest("nosuchcamp"); ok {
		t.Error("an unknown name found a shortcut")
	}
}

// The list fits its width and names every camp once, lowest levels first.
func TestCampListLines(t *testing.T) {
	lines := campListLines(70)
	seen := map[string]bool{}
	for _, l := range lines {
		if len(l) > 70 {
			t.Errorf("line of %d: %q", len(l), l)
		}
		for _, item := range strings.Split(l, ", ") {
			seen[strings.Fields(item)[0]] = true
		}
	}
	if len(seen) != len(campDests) {
		t.Errorf("%d camps listed of %d", len(seen), len(campDests))
	}
	low := func(n string) int {
		v, _ := strconv.Atoi(strings.SplitN(campDests[n].Levels, "-", 2)[0])
		return v
	}
	names := campNames()
	for i := 1; i < len(names); i++ {
		if low(names[i-1]) > low(names[i]) {
			t.Errorf("not by level: %q (%s) before %q (%s)", names[i-1], campDests[names[i-1]].Levels, names[i], campDests[names[i]].Levels)
		}
	}
}

// A camp resolves to ITS creature of the anchor's name, the one nearest the
// written spawn, never a namesake across the world (which SpawnOfNPC, by name
// alone, may pick); with none near, the written spawn itself.
func TestCampResolvesToItsOwnCreature(t *testing.T) {
	d := campDests["dfc"]
	gw := entity.NewWorld()
	far := &entity.NPC{Name: d.NPC, World: d.World, SpawnX: d.X + 4000, SpawnY: 10, SpawnZ: d.Z}
	near := &entity.NPC{Name: d.NPC, World: d.World, SpawnX: d.X + 12, SpawnY: d.Y + 1, SpawnZ: d.Z - 5}
	other := &entity.NPC{Name: d.NPC, World: 2, SpawnX: d.X, SpawnY: 99, SpawnZ: d.Z}
	copyOf := &entity.NPC{Name: d.NPC, World: d.World, SpawnX: d.X, SpawnY: 77, SpawnZ: d.Z, Temporary: true}
	gw.NPCs[1], gw.NPCs[2], gw.NPCs[3], gw.NPCs[4] = far, near, other, copyOf

	x, y, z, world, via, ok := ResolveTeleDest(gw, "dfc")
	if !ok || world != d.World || x != near.SpawnX || y != near.SpawnY || z != near.SpawnZ {
		t.Fatalf("dfc resolved to (%.0f, %.1f, %.0f) world %d (%s), want the camp's own %s at (%.0f, %.1f, %.0f)",
			x, y, z, world, via, d.NPC, near.SpawnX, near.SpawnY, near.SpawnZ)
	}
	if _, _, _, _, _, ok := ResolveTeleDest(gw, "deathfist"); !ok {
		t.Error("the alias did not resolve")
	}

	// the negative control: the camp's creature gone, the far namesake is not
	// taken; the written spawn is
	delete(gw.NPCs, 2)
	x, y, z, _, via, _ = ResolveTeleDest(gw, "dfc")
	if x != d.X || y != d.Y || z != d.Z || !strings.Contains(via, "fallback") {
		t.Errorf("with the camp's creature gone: (%.0f, %.1f, %.0f) via %q, want the written spawn, said", x, y, z, via)
	}
}

// The GM panel's list has the places first, then every camp.
func TestTeleDestinationsListCamps(t *testing.T) {
	all := TeleDestinations()
	if len(all) != len(teleDests)+len(campDests) {
		t.Fatalf("%d destinations, want %d places and %d camps", len(all), len(teleDests), len(campDests))
	}
	if all[len(teleDests)].Name != campNames()[0] || !strings.HasPrefix(all[len(teleDests)].Label, "XP camp, levels ") {
		t.Errorf("first camp entry %+v", all[len(teleDests)])
	}
}
