package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Toymaker Tinsel's toys of 2026-11-17 (the owner: "lets add string of lights
// candy cane staff", "add pegasus plushie too", "add gingerbread illusion
// too"): their auras on the channel and the plushie's winged horse.

// The String of Lights' aura (kind lights) shows on the worn cape alone: on a
// chest piece the same row is none. The Candy Cane Staff's first (stripes)
// goes with its second (the twinkle) on the hand holding it, and on a worn piece never.
func TestTheLightsOnTheCapeAndTheStripesInTheHand(t *testing.T) {
	const cape, staff, chest = 716915, 716916, 716999
	w := NewWorld()
	lights := database.ItemAura{PatternID: cape, Kind: database.AuraLights}
	stripes := database.ItemAura{PatternID: staff, Kind: database.AuraStripes}
	twinkle := database.ItemAura{PatternID: staff, Kind: database.AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	w.SetItemAuras(map[int]database.ItemAura{cape: lights, staff: stripes, chest: lights})
	w.SetItemSecondAuras(map[int]database.ItemAura{staff: twinkle})
	if !lights.Shows() || !stripes.Shows() || !twinkle.SecondShows() || !database.AuraShows(0, database.AuraIcing) {
		t.Fatal("the toys' kinds show with no colour")
	}
	p := ornatePlayer()
	p.CharServerID = 7
	p.SetWornPiece(AuraCape, cape)
	p.SetWornPiece(AuraChest, chest)
	p.SetWornPiece(AuraMainHand, staff)
	a := w.PieceAurasOf(p)
	if a == nil || a[AuraCape] != lights {
		t.Fatalf("the lights on the cape: %+v", a)
	}
	if a[AuraChest] != (database.ItemAura{}) {
		t.Errorf("the lights on a chest piece: %+v", a[AuraChest])
	}
	if a[AuraMainHand] != stripes || a[AuraMainHand2] != twinkle {
		t.Errorf("the staff in hand: first %+v second %+v", a[AuraMainHand], a[AuraMainHand2])
	}
	p.HiddenParts = HideCape
	if a := w.PieceAurasOf(p); a != nil && a[AuraCape] != (database.ItemAura{}) {
		t.Error("a hidden cape shows its lights")
	}
	for _, c := range []struct {
		kind, nodes int64
		ok          bool
	}{{database.AuraLights, 0, true}, {database.AuraIcing, 0, true}, {database.AuraStripes, 9, true}, {database.AuraStripes, 101, false},
		{database.AuraLights, 33, false}, {14, 0, false}} {
		if database.ValidItemAura(c.kind, c.nodes) != c.ok {
			t.Errorf("ValidItemAura(%d, %d) = %v", c.kind, c.nodes, !c.ok)
		}
	}
	if !database.ValidItemAuraSecond(database.AuraStripes, 0) || database.ValidItemAuraSecond(database.AuraLights, 0) {
		t.Error("stripes is a held item's second, lights not")
	}
}

// The Gingerbread Man's icing is the body's aura while its illusion is the
// body shown, over any body aura of the character's own; taken off, gone.
// The control: Santa's Elf's illusion (no icing aura) leaves the body's as it
// was.
func TestTheGingerbreadMansIcing(t *testing.T) {
	c := outfitRig(t)
	const ginger = 9102
	SetItemOutfits(map[int]database.ItemOutfitRow{testElfItem: elfOutfit(), ginger: {PatternID: ginger, Robe: 0xFF}})
	icing := database.ItemAura{PatternID: ginger, Kind: database.AuraIcing}
	c.w.SetItemAuras(map[int]database.ItemAura{ginger: icing})
	if a := c.w.PieceAurasOf(c.p); a != nil && a[AuraBody] != (database.ItemAura{}) {
		t.Fatalf("iced before: %+v", a[AuraBody])
	}
	pe := NewPotionEffect(ginger, "Gingerbread Man", "Self, Unlimited Uses, 30min", "Gingerbread Man", 0x11111111, "", 0, 0)
	pe.SetBody(0xEC6A0604)
	pe.Cooldown = time.Minute
	if r := c.use(t, pe, 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if m, _ := channelBody(c.p); m != 0xEC6A0604 {
		t.Fatalf("body %#x, want the clay golem", m)
	}
	a := c.w.PieceAurasOf(c.p)
	if a == nil || a[AuraBody] != icing {
		t.Fatalf("the illusion's body aura: %+v", a)
	}
	if !c.w.EndItemForm(c.sess, ginger, "Gingerbread Man", c.at(time.Minute)) {
		t.Fatal("used again: not taken off")
	}
	if a := c.w.PieceAurasOf(c.p); a != nil && a[AuraBody] != (database.ItemAura{}) {
		t.Errorf("iced after: %+v", a[AuraBody])
	}
	e := outfitRig(t)
	e.w.SetItemAuras(map[int]database.ItemAura{ginger: icing})
	e.use(t, santasElf(), 0, 0)
	if a := e.w.PieceAurasOf(e.p); a != nil && a[AuraBody] != (database.ItemAura{}) {
		t.Errorf("Santa's Elf iced: %+v", a[AuraBody])
	}
}

// plushieTick runs the plushie's system at d.
func (r *mercRig) plushieTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount++
	r.w.plushieSystem(r.at(d))
}

// A use brings the winged horse out, small, under a cute name, never a target
// and on no hate list, no pet; it follows its owner; the next use sends it
// home. Beside five monkeys it keeps a place of its own. It goes home by
// itself when its owner dies, and with its owner leaving the world.
func TestThePegasusPlushie(t *testing.T) {
	r := newMercRig(t)
	res := r.w.UsePlushie(r.sess, r.at(0))
	if res.Use != PlushieCame || res.Name == "" || r.w.PlushieOut(r.sess) != res.Name {
		t.Fatalf("a use: %+v", res)
	}
	ps := r.w.plushies[r.p.ServerID]
	n := ps.npc
	if uint32(n.ModelID) != PlushieBody || n.Size != PlushieSize || n.NPCType != 0x80 || n.Owner != 0 || !n.Temporary {
		t.Errorf("body %#x size %v type %#x owner %d", uint32(n.ModelID), n.Size, n.NPCType, n.Owner)
	}
	buf := SerializeNPCUpdate(0x1234, n)
	if buf[26] != 0 || buf[142] != 3 || !strings.HasPrefix(string(buf[116:140]), n.Name) {
		t.Errorf("its channel: 26=%d 142=%d name %q", buf[26], buf[142], buf[116:140])
	}
	crab := r.creature("Desert Crab", 28, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, n.ServerID, 5000)
	owner := r.w.npcOwnerLocked(n)
	r.w.mu.Unlock()
	if _, hated := crab.Threats[n.ServerID]; hated || owner != r.p {
		t.Errorf("hated %v, owner %v", hated, owner)
	}
	if r.w.pets[r.p.ServerID] != nil || len(r.w.monkeys) != 0 {
		t.Error("the plushie took a pet's or a monkey's place")
	}
	// it follows a walk of 30 east, and keeps apart from five monkeys
	for i := 0; i < MonkeysMax; i++ {
		r.w.UseMonkeyBarrel(r.sess, r.at(0))
	}
	d := time.Duration(0)
	for step := 0; step < 60; step++ {
		d += 100 * time.Millisecond
		r.w.mu.Lock()
		oldX := r.p.X
		r.p.X += 0.5
		r.p.Facing = headingTo(1, 0)
		r.w.SpatialIndex.Update(r.p.ServerID, oldX, r.p.Z, r.p.X, r.p.Z)
		r.w.mu.Unlock()
		r.plushieTick(d)
		r.monkeyTick(d)
		if dd := distXZ(n.X, n.Z, r.p.X, r.p.Z); dd > 12 {
			t.Fatalf("step %d: %.1f behind", step, dd)
		}
	}
	for i := 0; i < 50; i++ {
		d += 100 * time.Millisecond
		r.plushieTick(d)
		r.monkeyTick(d)
	}
	if dd := distXZ(n.X, n.Z, r.p.X, r.p.Z); dd > plushieRadius+followSlack+0.5 {
		t.Errorf("standing: %.1f off its owner", dd)
	}
	for _, ms := range r.w.monkeys[r.p.ServerID] {
		if distXZ(n.X, n.Z, ms.npc.X, ms.npc.Z) < 0.5 {
			t.Errorf("stacked on %q", ms.npc.Name)
		}
	}
	if res := r.w.UsePlushie(r.sess, r.at(d)); res.Use != PlushieHome || res.Name != n.Name || r.w.NPCs[n.ServerID] != nil || len(r.w.plushies) != 0 {
		t.Fatalf("the second use: %+v", res)
	}
	if len(r.w.monkeys[r.p.ServerID]) != MonkeysMax {
		t.Error("the plushie's going home sent the monkeys home")
	}
	// out again; its owner dies: home
	r.w.UsePlushie(r.sess, r.at(d))
	r.p.HP = 0
	r.plushieTick(d + time.Second)
	if len(r.w.plushies) != 0 {
		t.Error("its owner died: still out")
	}
	if res := r.w.UsePlushie(r.sess, r.at(d+2*time.Second)); res.Use != PlushieDead {
		t.Errorf("a use while dead: %+v", res)
	}
	seen := map[string]bool{}
	for _, s := range plushieNames {
		if seen[s] || s == "" || len(s) > 24 {
			t.Errorf("the list's %q", s)
		}
		seen[s] = true
	}
}

// Its owner leaving the world takes it.
func TestThePlushieLeavesWithItsOwner(t *testing.T) {
	r := newMercRig(t)
	r.w.UsePlushie(r.sess, r.at(0))
	n := r.w.plushies[r.p.ServerID].npc
	r.w.mu.Lock()
	r.w.plushieLeavingLocked(r.p)
	r.w.mu.Unlock()
	if len(r.w.plushies) != 0 || r.w.NPCs[n.ServerID] != nil {
		t.Error("left the world: the plushie stayed")
	}
}

// The String of Lights on the worn cape shows over the character's own cape
// aura (charaura 8: Dust's red shell hid his lights, the owner 2026-11-18),
// and the own aura comes back with any other cape. The control: the own cape
// aura with a cape of no aura of its own is the own.
func TestTheLightsShowOverTheOwnCapeAura(t *testing.T) {
	const lightsCape, plainCape = 716915, 716998
	w := NewWorld()
	lights := database.ItemAura{PatternID: lightsCape, Kind: database.AuraLights}
	w.SetItemAuras(map[int]database.ItemAura{lightsCape: lights})
	var own [database.CharAuraSlots]database.ItemAura
	red := database.ItemAura{Color: 0xFF2121FF, Kind: 5}
	own[AuraCape] = red
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own})
	p := ornatePlayer()
	p.CharServerID = 197
	p.SetWornPiece(AuraCape, lightsCape)
	if a := w.PieceAurasOf(p); a == nil || a[AuraCape] != lights {
		t.Fatalf("the lights over the own cape aura: %+v", a)
	}
	p.SetWornPiece(AuraCape, plainCape)
	if a := w.PieceAurasOf(p); a == nil || a[AuraCape] != red {
		t.Fatalf("the control: the own cape aura on a plain cape: %+v", a)
	}
}
