package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Potions against the cast tests' world (newCaster: a druid of 23 at
// (100, 10, 100), Rotting Flesh at 2 off, 30 hit points). The items' ids and
// icons are made up.

const (
	draughtID = 50023
	quickID   = 50029
	sublimeID = 50031
	thrownID  = 50001
	splashID  = 50018
)

func draught() PotionEffect {
	return NewPotionEffect(draughtID, "Watery Draught", "Self, Decrease Hitpoints by 60, Increase Power per tic by 50, 18sec", "", 0xA2774C47, "", 0, 0)
}

func quickening() PotionEffect {
	return NewPotionEffect(quickID, "Faulty Alacrity Potion", "Self, Sow, 30min", "Crude Quickening", 0x260ECB59, "attack", 0, 0)
}

func sublimation(wear int) PotionEffect {
	return NewPotionEffect(sublimeID, "Faulty Ethereal Potion", "Self, Invis, 4min", "Crude Sublimation", 0x9CD122E0, "attack,damage", wear, 0)
}

func thrown() PotionEffect {
	return NewPotionEffect(thrownID, "Unstable Potion", "Target, Decrease Hitpoints by 20", "", 0, "", 0, 0)
}

func (c *caster) potionTick(d time.Duration) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.effectSystem(c.at(d))
	c.w.potionSystem(c.at(d))
}

func (c *caster) use(t *testing.T, pe PotionEffect, target uint32, d time.Duration) string {
	t.Helper()
	refusal, err := c.w.UsePotion(c.sess, pe, target, c.at(d))
	if err != nil {
		t.Fatal(err)
	}
	return refusal
}

// The words a row may hold: whom it lands on, what ends it, a cure.
func TestPotionEffectRead(t *testing.T) {
	q := quickening()
	if q.Thrown() || !q.BreaksOnAttack || q.BreaksOnDamage || q.Name != "Crude Quickening" {
		t.Errorf("quickening read as %+v", q)
	}
	s := sublimation(1)
	if !s.BreaksOnAttack || !s.BreaksOnDamage || s.WearPct != 1 || s.duration() != 4*time.Minute {
		t.Errorf("sublimation read as %+v", s)
	}
	if th := thrown(); !th.Thrown() || th.Name != "Unstable Potion" {
		t.Errorf("a thrown potion read as %+v (its name the item's)", th)
	}
	cure := NewPotionEffect(1, "Restorative", "Self, Dispel Effect x3", "", 0, "", 0, 0)
	if cure.Cures != 3 || cure.Thrown() {
		t.Errorf("a restorative cures %d, thrown %v; want 3, false", cure.Cures, cure.Thrown())
	}
	if d := draught(); d.duration() != 18*time.Second || d.Thrown() {
		t.Errorf("a draught lasts %v, thrown %v", d.duration(), d.Thrown())
	}
}

// A draught takes its health at once and gives its power each tick of its
// 18 s, shown by the item's name; one whose health would kill is refused and
// does nothing.
func TestDraughtTurnsHealthIntoPower(t *testing.T) {
	c := newCaster(t)
	c.p.Power = 100
	if r := c.use(t, draught(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.p.HP != 440 || c.p.Power != 150 {
		t.Fatalf("drunk: %d HP, %d power; want 440, 150 (the first tick at once)", c.p.HP, c.p.Power)
	}
	if len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Watery Draught" || c.p.Buffs[0].Icon != 0xA2774C47 {
		t.Errorf("effects channel %+v", c.p.Buffs)
	}
	c.potionTick(6 * time.Second)
	c.potionTick(12 * time.Second)
	c.potionTick(18 * time.Second)
	if c.p.Power != 250 || len(c.p.Effects) != 0 {
		t.Errorf("after 18 s: %d power, %d effects; want 250, 0", c.p.Power, len(c.p.Effects))
	}

	c = newCaster(t)
	c.p.HP = 60
	if r := c.use(t, draught(), 0, 0); r != PotionTooWeak {
		t.Errorf("60 HP drinking a 60-HP draught: %q", r)
	}
	if c.p.HP != 60 || len(c.p.Effects) != 0 {
		t.Errorf("refused, yet %d HP, %d effects", c.p.HP, len(c.p.Effects))
	}
}

// A quickening is Spirit of the Wolf by its own name, ended by its drinker's
// attack (auto-attack engaged) and not by a hit taken; a second one replaces
// the first.
func TestQuickeningEndsOnAnAttack(t *testing.T) {
	c := newCaster(t)
	if r := c.use(t, quickening(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.p.MoveRate() != moveRateMax || len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Crude Quickening" {
		t.Fatalf("move rate %d, effects %+v", c.p.MoveRate(), c.p.Buffs)
	}
	c.use(t, quickening(), 0, time.Second)
	if len(c.p.Effects) != 1 {
		t.Errorf("two quickenings: %d effects, want 1", len(c.p.Effects))
	}
	c.p.LastCombatTime = c.at(2 * time.Second) // hit
	c.potionTick(3 * time.Second)
	if c.p.MoveRate() != moveRateMax {
		t.Error("the control: a hit taken ended the quickening")
	}
	if !c.w.EngageAttack(c.sess, c.npcO, Swing{Kind: 0, Delay: 3 * time.Second, Reach: 5}, c.at(4*time.Second)) {
		t.Fatal("no fight")
	}
	c.potionTick(4 * time.Second)
	if c.p.MoveRate() == moveRateMax || len(c.p.Effects) != 0 {
		t.Errorf("an attack left the quickening: rate %d, %d effects", c.p.MoveRate(), len(c.p.Effects))
	}
}

// A sublimation is invisibility by its own name, gone with a hit taken, and
// by its chance at a tick (wear 100 here, so the first tick ends it; wear 0
// never does).
func TestSublimationWearsOffOrBreaksOnAHit(t *testing.T) {
	c := newCaster(t)
	c.use(t, sublimation(0), 0, 0)
	if !c.p.Invisible() || c.p.Buffs[0].Name != "Crude Sublimation" {
		t.Fatalf("invisible %v, effects %+v", c.p.Invisible(), c.p.Buffs)
	}
	c.potionTick(time.Minute)
	if !c.p.Invisible() {
		t.Fatal("the control: wear 0 wore off")
	}
	c.p.LastCombatTime = c.at(61 * time.Second)
	c.potionTick(62 * time.Second)
	if c.p.Invisible() {
		t.Error("a hit left the sublimation on")
	}

	c = newCaster(t)
	c.use(t, sublimation(100), 0, 0)
	c.potionTick(5 * time.Second)
	if !c.p.Invisible() {
		t.Fatal("worn off before its first tick")
	}
	c.potionTick(6 * time.Second)
	if c.p.Invisible() {
		t.Error("wear 100 left it on at the first tick")
	}
}

// Thrown at the creature targeted, a potion lands as a spell's hit (the cast
// played to those who see it, the figure told); refused, as a cast is, with no
// target, at a friendly creature and out of its range.
func TestThrownPotionHitsTheTarget(t *testing.T) {
	c := newCaster(t)
	if r := c.use(t, thrown(), c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.npc.HP != 10 {
		t.Errorf("Rotting Flesh at %d HP, want 10", c.npc.HP)
	}
	if c.npc.Threats[c.p.ServerID] == 0 {
		t.Error("the creature holds no grudge")
	}
	var ops []protocol.GameOpcode
	for _, m := range c.sent() {
		ops = append(ops, opOfMsg(m))
	}
	has := func(op protocol.GameOpcode) bool {
		for _, o := range ops {
			if o == op {
				return true
			}
		}
		return false
	}
	if !has(0x00B1) || !has(0x00DB) {
		t.Errorf("sent %x; want a 0x00B1 and a 0x00DB among them", ops)
	}

	c = newCaster(t)
	if r := c.use(t, thrown(), 0xFFFFFFFF, 0); r != castNoTarget {
		t.Errorf("no target: %q", r)
	}
	c.npc.NPCType = 0x80
	if r := c.use(t, thrown(), c.npcO, 0); r != castFriendly {
		t.Errorf("a friendly creature: %q", r)
	}
	c.npc.NPCType = 0
	c.npc.X = 100 + PotionRange + 1
	if r := c.use(t, thrown(), c.npcO, 0); r != castOutOfRange {
		t.Errorf("out of range: %q", r)
	}
	if c.npc.HP != 30 {
		t.Errorf("refused, yet the creature at %d HP", c.npc.HP)
	}
}

// A splash lands on every creature that may be fought within its area of the
// target, and on nothing past it, nothing friendly, nothing in another world.
func TestThrownPotionSplash(t *testing.T) {
	c := newCaster(t)
	add := func(id EntityID, x float32, world uint8, flags int) *NPC {
		n := &NPC{ServerID: id, Name: "Rat", Level: 1, HP: 50, MaxHP: 50, X: x, Y: 10, Z: 100, World: world, NPCType: flags}
		c.w.NPCs[id] = n
		c.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	c.w.SpatialIndex.Insert(c.npc.ServerID, c.npc.X, c.npc.Z)
	near := add(3001, 108, 0, 0)
	far := add(3002, 115, 0, 0)
	friend := add(3003, 104, 0, 0x80)
	elsewhere := add(3004, 103, 1, 0)
	splash := NewPotionEffect(splashID, "Klik'Anon Cocktail", "Target, Decrease Hitpoints by 20", "", 0, "", 0, 10)
	if r := c.use(t, splash, c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.npc.HP != 10 || near.HP != 30 {
		t.Errorf("the target %d HP, the one 6 off %d; want 10, 30", c.npc.HP, near.HP)
	}
	if far.HP != 50 || friend.HP != 50 || elsewhere.HP != 50 {
		t.Errorf("past the splash %d, friendly %d, in another world %d: all want 50", far.HP, friend.HP, elsewhere.HP)
	}
}

// Worn as the thrown weapon, a potion is each auto-attack's throw: its hit
// lands, the stack is told one less, and the last one ends the fight.
func TestThrownPotionWornIsTheAutoAttack(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100, 100
	c.w.SetPotionEffects([]PotionEffect{thrown()})
	took := make(chan int, 4)
	c.w.OnPotionThrown(func(_ *session.Session, sid uint32, pattern int) { took <- pattern })
	swing := Swing{Kind: 8, Delay: 3500 * time.Millisecond, Reach: 20, Potion: thrownID, PotionLeft: 2}
	if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
		t.Fatal("no fight")
	}
	c.p.AutoAttack = true
	round := func(d time.Duration) {
		c.w.mu.Lock()
		c.w.attackSystem(c.at(d))
		c.w.mu.Unlock()
	}
	round(250 * time.Millisecond) // the round begins
	round(800 * time.Millisecond) // its one hit, 0.5 s in
	if c.npc.HP != 80 {
		t.Fatalf("after one throw the creature at %d HP, want 80", c.npc.HP)
	}
	if got := <-took; got != thrownID {
		t.Errorf("took %d off a stack, want %d", got, thrownID)
	}
	round(3750 * time.Millisecond)
	round(4300 * time.Millisecond)
	if c.npc.HP != 60 {
		t.Errorf("after two throws the creature at %d HP, want 60", c.npc.HP)
	}
	<-took
	if c.p.AttackTarget != 0 {
		t.Error("the last potion thrown, the fight goes on")
	}
}

// A spell's words that summon an item are handed to the hook at its landing,
// with the count its words give (0: the item's own), Allakhazam's "Summon
// Item: X" too; a pet's words are not, nor a line that names no item.
func TestSummonedItemsGoToTheHook(t *testing.T) {
	type got struct {
		item  string
		count int
		spell string
	}
	ch := make(chan got, 4)
	c := newCaster(t)
	c.w.OnSummonItem(func(_ *session.Session, _ uint32, item string, count int, spell string) {
		ch <- got{item, count, spell}
	})
	land := func(name, line string) {
		spell := CastSpell{SpellID: 9, Scope: 0, Name: name, Def: ParseSpellNamed(name, line, "")}
		cs := &castState{spell: spell, caster: c.p.ServerID, targets: []EntityID{c.p.ServerID}}
		c.w.mu.Lock()
		c.w.landLocked(cs, c.p, c.at(0))
		c.w.mu.Unlock()
	}
	land("Crude Quickening", "SELF Summon Faulty Alacrity Potion (SOW)")
	if g := <-ch; g != (got{"faulty alacrity potion", 0, "Crude Quickening"}) {
		t.Errorf("Crude Quickening summoned %+v", g)
	}
	land("Unstable Mixtures", "Summons Unstable Potion X3")
	if g := <-ch; g != (got{"unstable potion", 3, "Unstable Mixtures"}) {
		t.Errorf("Unstable Mixtures summoned %+v", g)
	}
	land("Forage Berries", "Self, Summon Item: foraged berries")
	if g := <-ch; g != (got{"foraged berries", 0, "Forage Berries"}) {
		t.Errorf("Forage Berries summoned %+v", g)
	}
	land("Forage Grub", "Creates no trade/no rent food.") // names no item: nothing
	land("Summon Golem", "Summons an Iron Golem to your defense")
	land("Wilting Flesh", "Summons a zombie to your aid") // a pet the pet words miss
	select {
	case g := <-ch:
		t.Errorf("a pet's summoning summoned an item: %+v", g)
	case <-time.After(50 * time.Millisecond):
	}
}

// A potion's effect comes back after a logout under its own id, so what ends
// it still does.
func TestPotionEffectKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.w.SetPotionEffects([]PotionEffect{quickening()})
	c.use(t, quickening(), 0, 0)
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(time.Minute))
	if !ok || len(rows) != 1 || rows[0].SpellID != PotionEffectID(quickID) {
		t.Fatalf("saved %+v", rows)
	}
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Druid", Level: 23, HP: 500, MaxHP: 536, Speed: 8.833333}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	w.SetPotionEffects([]PotionEffect{quickening()})
	if n := w.RestoreEffects(sess, rows, c.at(2*time.Minute)); n != 1 || p.MoveRate() != moveRateMax {
		t.Fatalf("restored %d, move rate %d", n, p.MoveRate())
	}
	p.AttackTarget = 2002
	w.mu.Lock()
	w.potionSystem(c.at(2*time.Minute + time.Second))
	w.mu.Unlock()
	if p.MoveRate() == moveRateMax {
		t.Error("restored, the quickening no longer ends on an attack")
	}
}
