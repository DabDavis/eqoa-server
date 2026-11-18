package entity

import (
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// What a monster carries of its row's weapons and shield (heldgear.go): rolled
// at each spawn, kept for that life, shown on its channel; the named, a loot
// profile's, an NPC, a copy kept whole; a name's own chances; a keep restart
// carrying what each held; and, the control, everything whole with it off.

const (
	heldSword       = 0x11111111
	heldAxe         = 0x22222222
	heldShieldModel = 0x33333333
)

type heldRow struct {
	id      EntityID
	name    string
	npcType int
	p, s, h int // primary, secondary, shield models
}

// heldWorld is a world of the rows (monsters on the ground at 2000, 2000 and
// apart), with the rules, the corpse times short, the spawn roll seeded, and
// ArmNPCs run.
func heldWorld(t *testing.T, rules HeldGearRules, rows []heldRow, seed int64) *World {
	t.Helper()
	w := NewWorld()
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	w.held.rnd = rand.New(rand.NewSource(seed))
	w.mu.Lock()
	for i, r := range rows {
		rec := &database.NPCRecord{ID: int(r.id), NPCName: r.name, NPCType: r.npcType, NPCLevel: 30,
			XCoord: 2000 + float32(i)*50, ZCoord: 2000, ModelID: 0x12345678,
			PrimaryWeapon: r.p, SecondaryWeapon: r.s, Shield: r.h}
		npc := NPCFromRecord(rec, 0, nil)
		w.applyNPCSettingsLocked(npc)
		w.putNPCLocked(npc)
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	}
	w.mu.Unlock()
	w.SetHeldGear(rules)
	w.ArmNPCs()
	return w
}

// carried is what npc carries: primary, secondary, shield.
func carried(n *NPC) [3]int { return [3]int{n.PrimaryWeapon, n.SecondaryWeapon, n.Shield} }

func TestHeldGearRollsMonstersAtEachSpawn(t *testing.T) {
	rules := DefaultHeldGear
	rules.Profiles = map[string]bool{"gnoll boss": true}
	rules.Names = map[string]HeldChance{"gnoll sentry": {Weapon: 100, Shield: 0}}
	rows := []heldRow{
		{501, "gnoll scout", 0, heldSword, 0, heldShieldModel},
		{502, "gnoll scout", 0, heldSword, heldAxe, 0},
		{503, "Ice goblin warrior", 0, heldAxe, 0, heldShieldModel}, // a capital on many rows: a monster's kind
		{504, "Ice goblin warrior", 0, heldAxe, 0, heldShieldModel},
		{505, "Ice goblin warrior", 0, heldAxe, 0, heldShieldModel},
		{506, "Ice goblin warrior", 0, heldAxe, 0, heldShieldModel},
		{507, "Gorvak One-Eye", 0, heldSword, 0, heldShieldModel}, // named: one row
		{508, "gnoll boss", 0, heldSword, 0, heldShieldModel},     // a loot profile's
		{509, "Merchant Bob", 0x81, heldSword, 0, 0},              // an NPC
		{510, "a guard post", 0x80, heldSword, 0, heldShieldModel},
		{511, "gnoll sentry", 0, heldSword, 0, heldShieldModel}, // its own chances: always the sword, never the shield
		{512, "rat", 0, 0, 0, 0},                                // nothing to carry
	}
	w := heldWorld(t, rules, rows, 7)
	kit := map[EntityID][3]int{}
	for _, r := range rows {
		kit[r.id] = [3]int{r.p, r.s, r.h}
	}
	whole := map[EntityID]bool{507: true, 508: true, 509: true, 510: true, 512: true}
	seen := map[EntityID]map[[3]int]bool{}
	for life := 0; life < 40; life++ {
		for _, r := range rows {
			n := w.NPCs[r.id]
			c := carried(n)
			if seen[r.id] == nil {
				seen[r.id] = map[[3]int]bool{}
			}
			seen[r.id][c] = true
			k := kit[r.id]
			for i := range c {
				if c[i] != 0 && c[i] != k[i] {
					t.Fatalf("%s carries %v of its kit %v", r.name, c, k)
				}
			}
			// the hands' attack types go with what it carries
			if (c[0] == 0) != (n.PrimaryType == 0) || (c[1] == 0) != (n.SecondaryType == 0) {
				t.Errorf("%s carries %v with types %d, %d", r.name, c, n.PrimaryType, n.SecondaryType)
			}
			// the channel shows what it carries
			buf := SerializeNPCUpdate(100001, n)
			if got := [3]int{int(binary.LittleEndian.Uint32(buf[67:71])), int(binary.LittleEndian.Uint32(buf[71:75])),
				int(binary.LittleEndian.Uint32(buf[75:79]))}; got != c {
				t.Errorf("%s's channel shows %v, it carries %v", r.name, got, c)
			}
			if whole[r.id] && c != k {
				t.Errorf("%s (kept whole) carries %v of %v", r.name, c, k)
			}
			if r.id == 511 && c != [3]int{heldSword, 0, 0} {
				t.Errorf("gnoll sentry at 100/0 carries %v", c)
			}
		}
		// what it carries stays for its life
		before := map[EntityID][3]int{}
		for _, r := range rows {
			before[r.id] = carried(w.NPCs[r.id])
		}
		for i := 0; i < 3; i++ {
			w.Tick(time.Now())
		}
		for _, r := range rows {
			if c := carried(w.NPCs[r.id]); c != before[r.id] {
				t.Fatalf("%s carried %v, then %v in one life", r.name, before[r.id], c)
			}
		}
		for _, r := range rows {
			killAndBack(t, w, r.id)
		}
	}
	for _, id := range []EntityID{501, 502, 503, 504, 505, 506} {
		if len(seen[id]) < 3 {
			t.Errorf("%d: %d ways armed over 40 lives, want variety: %v", id, len(seen[id]), seen[id])
		}
		if !seen[id][kit[id]] || !seen[id][[3]int{}] {
			t.Errorf("%d: never whole or never bare over 40 lives: %v", id, seen[id])
		}
	}
}

// The control: held gear off, every monster carries its whole kit at every spawn.
func TestHeldGearOffCarriesTheWholeKit(t *testing.T) {
	rules := DefaultHeldGear
	rules.On = false
	rows := []heldRow{{521, "gnoll scout", 0, heldSword, heldAxe, heldShieldModel}, {522, "gnoll scout", 0, heldSword, 0, heldShieldModel}}
	w := heldWorld(t, rules, rows, 1)
	for life := 0; life < 20; life++ {
		for _, r := range rows {
			if c := carried(w.NPCs[r.id]); c != [3]int{r.p, r.s, r.h} {
				t.Fatalf("off: %d carries %v", r.id, c)
			}
			killAndBack(t, w, r.id)
		}
	}
}

// A keep restart carries what each carried, alive or dead; one the file does
// not name carried its whole kit; a copy carries its row's whole kit; a GM's
// reload of an unchanged row leaves it as it is.
func TestHeldGearKeptOverARestart(t *testing.T) {
	rows := []heldRow{
		{531, "gnoll scout", 0, heldSword, heldAxe, heldShieldModel},
		{532, "gnoll scout", 0, heldSword, 0, heldShieldModel},
		{533, "gnoll scout", 0, heldSword, 0, heldShieldModel},
	}
	w := heldWorld(t, HeldGearRules{On: true, Default: HeldChance{Weapon: 0, Shield: 0}, NamedRows: 3}, rows, 1)
	w.mu.Lock()
	w.NPCs[533].carryKit(0) // as if it rolled whole
	w.mu.Unlock()
	if !w.KillNPCNow(532) {
		t.Fatal("not killed")
	}
	kept := w.KeepNPCs()
	byID := map[uint32]KeptNPC{}
	for _, k := range kept {
		byID[k.ID] = k
	}
	if byID[531].Unarmed != heldPrimary|heldSecondary|heldShield || byID[532].Unarmed != heldPrimary|heldShield {
		t.Fatalf("kept: %+v", byID)
	}
	if _, ok := byID[533]; ok {
		t.Errorf("533 carries its whole kit on its spawn: nothing to keep, kept %+v", byID[533])
	}

	// the new process rolls every one bare; the file says otherwise
	w2 := heldWorld(t, HeldGearRules{On: true, Default: HeldChance{Weapon: 0, Shield: 0}, NamedRows: 3}, rows, 2)
	r := w2.RestoreNPCs(kept)
	if r.Unarmed != 2 {
		t.Errorf("restored %d short of a piece, want 2: %v", r.Unarmed, r)
	}
	if c := carried(w2.NPCs[531]); c != [3]int{} {
		t.Errorf("531 carries %v, kept bare", c)
	}
	if n := w2.NPCs[532]; !n.Dead || carried(n) != [3]int{0, 0, 0} {
		t.Errorf("532 dead %v carrying %v, kept dead and bare", n.Dead, carried(n))
	}
	if c := carried(w2.NPCs[533]); c != [3]int{heldSword, 0, heldShieldModel} {
		t.Errorf("533 (not in the file) carries %v, want its whole kit", c)
	}

	// a copy of 531 (bare this life) carries the row's whole kit
	w2.mu.Lock()
	c := w2.spawnCopyLocked(w2.NPCs[531], 2100, 0, 2100, 0, 0)
	w2.mu.Unlock()
	if got := carried(c); got != [3]int{heldSword, heldAxe, heldShieldModel} {
		t.Errorf("a copy carries %v, want its row's whole kit", got)
	}

	// a reload of the same row is the same row, whatever its spawn rolled
	fresh := NPCFromRecord(&database.NPCRecord{ID: 531, NPCName: "gnoll scout", NPCLevel: 30, XCoord: 2000, ZCoord: 2000,
		ModelID: 0x12345678, PrimaryWeapon: heldSword, SecondaryWeapon: heldAxe, Shield: heldShieldModel}, 0, nil)
	w2.mu.Lock()
	w2.applyNPCSettingsLocked(fresh)
	same := npcSameRecord(w2.NPCs[531], fresh)
	w2.mu.Unlock()
	if !same {
		t.Error("a reload takes a monster short of a piece for a changed row")
	}
	fresh.kit.Shield = heldSword
	if npcSameRecord(w2.NPCs[531], fresh) {
		t.Error("a row's shield changed reads as the same row")
	}
}
