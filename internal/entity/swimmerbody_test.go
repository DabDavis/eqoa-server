package entity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/esf"
)

// The Megaladon (the owner, 2026-11-11: "im not seeing megaladons attack
// animations") on the channel: treading water standing, swimming moving, so
// CLIENT's SetAnimation turns its swing into its own attack (44); a corpse
// still 14, a land creature's walk as it was.
func TestASeaCreatureTreadsWaterOnTheChannel(t *testing.T) {
	const megaladon = 0x69842C02
	w := roamWorld(t)
	shark := addNPC(w, 100, "megaladon", 1000, 1000, megaladon, 0)
	if b := SerializeNPCUpdate(1, shark); b[58] != LocoTread {
		t.Fatalf("standing: animation %d, want %d (tread water)", b[58], LocoTread)
	}
	w.stepNPCToward(shark, 1010, 1000, NPCWalkSpeed, 0.1)
	if b := SerializeNPCUpdate(1, shark); shark.Loco != LocoWalk || b[58] != LocoSwim {
		t.Fatalf("walking (loco %d): animation %d, want %d (swim)", shark.Loco, b[58], LocoSwim)
	}
	shark.Loco = LocoRun
	if b := SerializeNPCUpdate(1, shark); b[58] != LocoSwim {
		t.Fatalf("running: animation %d, want %d", b[58], LocoSwim)
	}
	shark.Loco = 14
	if b := SerializeNPCUpdate(1, shark); b[58] != 14 {
		t.Fatalf("dead: animation %d, want 14", b[58])
	}

	beetle := addNPC(w, 101, "beetle", 1000, 1000, creatureModel, 0)
	if b := SerializeNPCUpdate(1, beetle); b[58] != LocoStand {
		t.Fatalf("a land body standing: animation %d", b[58])
	}
	w.stepNPCToward(beetle, 1010, 1000, NPCWalkSpeed, 0.1)
	if b := SerializeNPCUpdate(1, beetle); b[58] != LocoWalk {
		t.Fatalf("a land body walking: animation %d", b[58])
	}

	// a land creature in a sea creature's body (an illusion) swims too: the
	// body is what plays it
	beetle.Effects = append(beetle.Effects, &ActiveEffect{Form: megaladon, Until: time.Now().Add(time.Hour)})
	if b := SerializeNPCUpdate(1, beetle); b[58] != LocoSwim {
		t.Fatalf("a beetle in the Megaladon's body walking: animation %d", b[58])
	}
}

// swimmerBodies is every CHAR.ESF body whose play list is the sea
// creature's six slots: re-read off the disc's CHAR.ESF (EQOA_CHAR_ESF, or
// the extracted one at ~/claude-eqoa/extracted-assets), skipped without it.
func TestSwimmerBodiesAreCharESFs(t *testing.T) {
	path := os.Getenv("EQOA_CHAR_ESF")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "claude-eqoa/extracted-assets/CHAR.ESF")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no CHAR.ESF at %s", path)
	}
	f, err := esf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.BuildDictionary(); err != nil {
		t.Fatal(err)
	}
	want := []int32{0, 12, 13, 14, 34, 44}
	found, bodies := map[uint32]bool{}, 0
	for _, id := range f.DictKeys() {
		o, err := f.FindObject(id)
		if err != nil || o == nil {
			continue
		}
		cs, ok := o.(*esf.CSprite)
		if !ok {
			continue
		}
		bodies++
		slots := map[int32]bool{}
		for _, p := range cs.PlayList {
			slots[p.Index] = true
		}
		same := len(slots) == len(want)
		for _, s := range want {
			same = same && slots[s]
		}
		// none of the land's attacks (22, 23: SetDefaultEntries' sources)
		if same && !slots[22] && !slots[23] {
			found[uint32(id)] = true
		}
		f.ReleaseCache()
	}
	if bodies < 500 {
		t.Fatalf("read %d bodies off %s", bodies, path)
	}
	for m := range found {
		if !swimmerBodies[m] {
			t.Errorf("0x%08X has the sea creature's play list, not in swimmerBodies", m)
		}
	}
	for m := range swimmerBodies {
		if !found[m] {
			t.Errorf("0x%08X in swimmerBodies is not a sea creature's body", m)
		}
	}
}

// Every body a player may wear (a race's own, or one its body rows offer)
// swings with an attack of its own on every client: its play list has 23 or
// 22, from which CLIENT's SetDefaultEntries fills every weapon's attack slot
// the body lacks (24-33, 35, 36, 40-44, 50-53: vicsprite.cpp), or it is a
// sea body, whose swing becomes its 44 while byte 58 says it swims or treads
// water (bodyLoco). A body with neither would swing its idle, once, to every
// viewer (the owner, 2026-11-11, of the Jal'Raeth, whose three bodies have
// only 22 and 23: they swing). Read off the disc's CHAR.ESF where it is at
// hand, as TestSwimmerBodiesAreCharESFs.
func TestPlayerBodiesHaveAnAttack(t *testing.T) {
	path := os.Getenv("EQOA_CHAR_ESF")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "claude-eqoa/extracted-assets/CHAR.ESF")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no CHAR.ESF at %s", path)
	}
	f, err := esf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.BuildDictionary(); err != nil {
		t.Fatal(err)
	}
	// the races' bodies with no body row (the pixie's, the fairy's, the
	// Wisp's humans) beside every row's
	bodies := append(OptionBodies(), bodyPixie, bodyFairy, bodyHumanMale, bodyHumanFemale)
	if len(bodies) < 100 {
		t.Fatalf("%d option bodies", len(bodies))
	}
	for _, b := range bodies {
		o, err := f.FindObject(int32(b))
		cs, ok := o.(*esf.CSprite)
		if err != nil || !ok {
			t.Errorf("0x%08X is not a CSprite of CHAR.ESF", b)
			continue
		}
		own := false
		for _, p := range cs.PlayList {
			own = own || p.Index == 22 || p.Index == 23
		}
		if !own && !swimmerBodies[b] {
			t.Errorf("0x%08X has no attack of its own (22, 23) and is no sea body: every swing shows its idle", b)
		}
		f.ReleaseCache()
	}
}
