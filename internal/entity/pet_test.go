package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Pets held to Sandstorm's one recorded summon (harvests/sandstorm-spells-
// 20261021/pets/kimura-summon-fire-elemental.json): a level 45 magician's
// Fire Elemental, a level 44 spell, and what came with it.

type petRig struct {
	w     *World
	sess  *session.Session
	p     *Player
	t0    time.Time
	me    uint32
	enemy *NPC
}

// newPetRig is a level 45 magician among this server's own records of the
// Fire Elemental (three rows of 0x8A416241, one of 0x8A41623F) and a beach
// crab to fight.
func newPetRig(t *testing.T) *petRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Kimura", Class: 10, Level: 45, HP: 900, MaxHP: 900, Power: 2000, MaxPower: 2000,
		Speed: 8.833333, X: 8629.6, Y: 76.5, Z: 5421.5, World: 2}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x15519016)
	add := func(n *NPC) *NPC {
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
		return n
	}
	for i, m := range []int64{0x8A416241, 0x8A41623F, 0x8A416241, 0x8A416241} {
		add(&NPC{ServerID: EntityID(3000 + i), Name: "Fire Elemental", Level: 39, HP: 2340, MaxHP: 2340, ModelID: m, Size: 1,
			NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	}
	add(&NPC{ServerID: 3100, Name: "Earth Elementaling ", Level: 15, ModelID: 0x3787521F, Size: 1, X: 20000, Z: 20100})
	add(&NPC{ServerID: 3101, Name: "Animation", Level: 24, ModelID: 0xDC3B344E, Size: 1, X: 20000, Z: 20200})
	crab := add(&NPC{ServerID: 3200, Name: "Desert Crab", Level: 34, HP: 2040, MaxHP: 2040, ModelID: 0x55432169, Size: 1,
		NPCType: 0x0100, X: 8640, Y: 75, Z: 5421.5, World: 2})
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = crab.ServerID
	vs.ChannelObjectID[0] = 0x0133E8A0
	return &petRig{w: w, sess: sess, p: p, t0: time.Unix(1_800_000_000, 0), me: 0x15519016, enemy: crab}
}

func (r *petRig) at(d time.Duration) time.Time { return r.t0.Add(d) }

func (r *petRig) sent() [][]byte {
	var out [][]byte
	for _, m := range r.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// fireElemental is the Fire Elemental's ability as the server casts it: its
// level and line as this server keeps them (spellpattern 717, spelleffect).
func fireElemental() CastSpell {
	return CastSpell{SpellID: 717, DisplayIndex: 34, Level: 44, Scope: 0, Name: "Fire Elemental",
		CastTime: 8 * time.Second, PowerCost: 1038, Def: ParseSpellText("Summons a Fire Elemental to aid you")}
}

// summon casts and lands spell, and returns the pet and what was sent from
// the landing on.
func (r *petRig) summon(t *testing.T, spell CastSpell, at time.Duration) (*NPC, [][]byte) {
	t.Helper()
	if !r.w.StartCast(r.sess, spell, 0xFFFFFFFF, r.at(at)) {
		t.Fatal("the summon did not begin")
	}
	r.sent()
	r.w.mu.Lock()
	r.w.castSystem(r.at(at + spell.CastTime + castLand))
	r.w.mu.Unlock()
	pet := r.w.petOfLocked(r.p)
	if pet == nil {
		t.Fatalf("%q made no pet", spell.Name)
	}
	// but the lockout's end (0x00CD 0), over before the landing
	var msgs [][]byte
	for _, m := range r.sent() {
		if opOfMsg(m) != protocol.OpcodeCooldownTimer {
			msgs = append(msgs, m)
		}
	}
	return pet, msgs
}

// tick runs the systems a pet lives in, as the tick does.
func (r *petRig) tick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.aiSystem(r.at(d))
	r.w.petSystem(r.at(d))
}

// The lines of this server's pet spells (spelleffect) make a pet, the
// creature they name kept; the alchemists' potions and Phase Ring's ring do
// not.
func TestASummoningLineMakesAPet(t *testing.T) {
	for line, want := range map[string]string{
		"Summons a pet":                           "",
		"Summons a Clay Golem to aid you":         "clay golem",
		"Summuns a Stone Golem to aid you":        "stone golem",
		"Summons an Iron Golem to your defense":   "iron golem",
		"Summons a water elementaling to aid you": "water elementaling",
		"Summons an elementalkin to aid you":      "elementalkin",
		"This spell summons pet":                  "",
		"Summon Pet":                              "",
		"Summons a Zombie pet":                    "zombie",
		"Summons Pet":                             "",
	} {
		d := ParseSpellText(line)
		if !d.Summons() || d.Pet != want || d.Target != TargetSelf || d.Harmful {
			t.Errorf("%q: summons %v, pet %q (want %q), target %v, harmful %v", line, d.Summons(), d.Pet, want, d.Target, d.Harmful)
		}
	}
	for _, line := range []string{"SELF Summons Unstable Potion", "SELF Summon Faulty Alacrity Potion (SOW)",
		"Summons a ring to invis, 3 charges", "Pet, Increase STR & DEX by 10, Increase HP per tic by 5, 5min"} {
		if d := ParseSpellText(line); d.Summons() {
			t.Errorf("%q makes a pet", line)
		}
	}
	if d := ParseSpellText("Pet, Increase Hitpoints by 15"); d.Target != TargetPet || d.Harmful || len(d.Effects) != 1 {
		t.Errorf("a pet's heal: %+v", d)
	}
}

// The landing makes Sandstorm's pet: named as the spell, level 44 of a
// caster of 45, the body most of this server's Fire Elementals wear, and
// 0x007E with its id then 0x1003 putting it on the non-collide list, in
// Sandstorm's bytes but for the id. Its channel carries what Sandstorm's
// did: 142 3, 143 1, npc flags 0, the hands' types 1 and 5, level 44.
func TestThePetIsSandstormsFireElemental(t *testing.T) {
	r := newPetRig(t)
	pet, msgs := r.summon(t, fireElemental(), 0)
	if pet.Name != "Fire Elemental" || pet.Level != 44 || pet.ModelID != 0x8A416241 || pet.Owner != r.p.ServerID ||
		pet.World != 2 || pet.NPCID != 0 || pet.NPCType != 0 || !pet.Temporary || pet.HP != NPCMaxHP(44) {
		t.Fatalf("the pet: %+v", *pet)
	}
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d < petFollow-0.01 || d > petFollow+0.01 {
		t.Errorf("the pet is %.2f off its owner, want %v", d, petFollow)
	}
	wantOps(t, msgs, protocol.OpcodeSetPet, protocol.OpcodeNoCollideAdd)
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	if id == 0 || binary.LittleEndian.Uint32(msgs[0][2:]) != id || len(msgs[0]) != 6 {
		t.Errorf("0x007E % X, want the pet's id %#x", msgs[0], id)
	}
	if w := unhex(t, "03100e0000004669726520456c656d656e74616c00000060"); !bytes.Equal(msgs[1], w) {
		t.Errorf("0x1003 % X, want Sandstorm's % X", msgs[1], w)
	}
	buf := SerializeNPCUpdate(id, pet)
	for _, c := range []struct {
		at   int
		want byte
	}{{142, 3}, {143, 1}, {145, 0}, {146, 0}, {91, 1}, {92, 5}, {140, 44}, {58, 0}, {40, 0}} {
		if buf[c.at] != c.want {
			t.Errorf("channel byte %d = %d, want Sandstorm's %d", c.at, buf[c.at], c.want)
		}
	}
	if v := SerializeNPCUpdate(id, r.enemy); v[142] != 0 {
		t.Errorf("a creature's byte 142 is %d", v[142])
	}
}

// Whose body: the spell's name first, then the creature its line names,
// then the name after a possessive, then the class's stand-in.
func TestThePetsBodyIsTheServersOwn(t *testing.T) {
	r := newPetRig(t)
	for _, c := range []struct {
		spell, line string
		class       int
		want        int64
	}{
		{"Fire Elemental", "fire elemental", 10, 0x8A416241},
		{"Earth Elementaling", "earth elementaling", 10, 0x3787521F}, // a trailing space in the row's name
		{"Pendril's Animation", "", 12, 0xDC3B344E},
		{"Shalee's Animation", "", 12, 0xDC3B344E},
		{"Water Elementalkin", "elementalkin", 10, 0x3787521F}, // nothing of either name: the class's stand-in
	} {
		r.w.mu.Lock()
		got := r.w.petTemplateLocked(c.spell, c.line, c.class)
		r.w.mu.Unlock()
		if got == nil || got.ModelID != c.want {
			t.Errorf("%q: %+v, want model %#x", c.spell, got, c.want)
		}
	}
	r.w.mu.Lock()
	if got := r.w.petTemplateLocked("Blade Swarm", "", 0); got != nil {
		t.Errorf("a warrior's pet of nothing known: %q", got.Name)
	}
	r.w.mu.Unlock()
}

// The pet follows: its owner runs off, it runs after at its owner's speed
// (the velocity and locomotion on its channel), and stands 3 off when it
// gets there.
func TestThePetFollowsItsOwner(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.tick(9500 * time.Millisecond)
	if pet.Loco != LocoStand {
		t.Fatalf("the pet moves with its owner standing by it: loco %d", pet.Loco)
	}
	r.w.UpdatePosition(r.p.ServerID, r.p.X+20, r.p.Y, r.p.Z, r.p.Facing)
	r.tick(9600 * time.Millisecond)
	buf := SerializeNPCUpdate(1, pet)
	if pet.Loco != LocoRun || buf[58] != LocoRun || int8(buf[40]) <= 0 || pet.VelX < 8 {
		t.Fatalf("the pet does not run after its owner: loco %d, velocity %.2f (byte %d)", pet.Loco, pet.VelX, int8(buf[40]))
	}
	for ms := 9700; ms < 15000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d < petFollow-0.01 || d > petFollow+0.5 || pet.Loco != LocoStand || pet.VelX != 0 {
		t.Errorf("the pet ends %.2f off its owner, loco %d, velocity %.2f", d, pet.Loco, pet.VelX)
	}
	// a teleport: put beside the owner at once, in its world
	r.p.World = 0
	r.w.UpdatePosition(r.p.ServerID, 25000, 54, 15000, 0)
	r.tick(15100 * time.Millisecond)
	if pet.World != 0 || distXZ(pet.X, pet.Z, r.p.X, r.p.Z) > petFollow+0.01 {
		t.Errorf("the pet stays behind: world %d, %.1f off", pet.World, distXZ(pet.X, pet.Z, r.p.X, r.p.Z))
	}
}

// Told to attack, the pet goes to its target and fights it in rounds: its
// swing to everyone, its hits told to the owner as the owner's, the
// creature's hate on the pet; the kill's XP is the owner's.
func TestThePetFightsWhatItIsToldTo(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(9600*time.Millisecond))
	msgs := r.sent()
	if len(msgs) != 1 || !strings.Contains(chatText(msgs[0]), "attacks Desert Crab") {
		t.Fatalf("the attack's answer: %x", msgs)
	}
	xp := r.p.TotalXP
	var swings, hits int
	for ms := 9700; ms < 120000 && !r.enemy.Dead; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
		for _, m := range r.sent() {
			switch opOfMsg(m) {
			case protocol.OpcodeCastingCombat:
				if binary.LittleEndian.Uint32(m[2:]) == r.w.objectIDForEntityLocked(r.sess, pet.ServerID) {
					swings++
				}
			case protocol.OpcodeDamageIndication:
				if binary.LittleEndian.Uint32(m[2:]) != r.me {
					t.Fatalf("a pet's hit told as %#x's, not the owner's", binary.LittleEndian.Uint32(m[2:]))
				}
				hits++
			}
		}
	}
	if !r.enemy.Dead || swings == 0 || hits == 0 || r.p.TotalXP != xp+34*100 {
		t.Fatalf("the fight: crab dead %v (%d HP), %d swings, %d hits, XP %d -> %d", r.enemy.Dead, r.enemy.HP, swings, hits, xp, r.p.TotalXP)
	}
	if distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z) > petReach {
		t.Errorf("the pet fought from %.1f off", distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z))
	}
	if r.w.pets[r.p.ServerID].target != 0 {
		t.Error("the pet keeps a dead target")
	}
}

// A creature fights back at the pet that hit it, and the pet dies at 0: its
// owner is told (0x007F, the non-collide entry off, a line) and the corpse
// goes after its time.
func TestCreaturesFightBackAtThePet(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.HP = 30
	r.enemy.Level = 60
	r.w.mu.Lock()
	r.w.updateAggro(r.enemy, pet.ServerID, 10)
	r.w.mu.Unlock()
	// the pet stands by the crab
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	r.w.pets[r.p.ServerID].stance = petPassive
	for ms := 10000; ms < 20000 && !pet.Dead; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if !pet.Dead {
		t.Fatalf("the crab never hurt the pet: %d HP", pet.HP)
	}
	r.tick(20100 * time.Millisecond)
	msgs := r.sent()
	var ops []protocol.GameOpcode
	for _, m := range msgs {
		ops = append(ops, opOfMsg(m))
	}
	if len(msgs) < 3 || ops[len(ops)-3] != protocol.OpcodeClearPet || ops[len(ops)-2] != protocol.OpcodeNoCollideRemove ||
		!strings.Contains(chatText(msgs[len(msgs)-1]), "pet has died") {
		t.Fatalf("the pet's death: %v %x", ops, msgs)
	}
	if r.w.petOfLocked(r.p) != nil || r.enemy.CurrentEnemy == pet.ServerID {
		t.Error("the dead pet is still a pet, or still fought")
	}
}

// The commands: each stance answered, a back off lets go, a dismiss takes
// the pet away; 0 and 6 are nothing.
func TestThePetCommands(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	if ps.stance != petDefensive {
		t.Errorf("a new pet is %d, want defensive", ps.stance)
	}
	for cmd := byte(1); cmd <= 4; cmd++ {
		r.w.PetCommand(r.sess, cmd, 0, r.at(10*time.Second))
		if msgs := r.sent(); ps.stance != petStance(cmd) || len(msgs) != 1 || chatText(msgs[0]) != petStanceWords[petStance(cmd)] {
			t.Errorf("command %d: stance %d, %x", cmd, ps.stance, msgs)
		}
	}
	for _, cmd := range []byte{0, 6, 9} {
		r.w.PetCommand(r.sess, cmd, 0, r.at(10*time.Second))
		if msgs := r.sent(); len(msgs) != 0 {
			t.Errorf("command %d answered %x", cmd, msgs)
		}
	}
	ps.target = r.enemy.ServerID
	r.w.PetCommand(r.sess, petCmdBackOff, 0, r.at(11*time.Second))
	if ps.target != 0 || !strings.Contains(chatText(r.sent()[0]), "backs off") {
		t.Error("back off kept the fight")
	}
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(12*time.Second))
	msgs := r.sent()
	wantOps(t, msgs, protocol.OpcodeClearPet, protocol.OpcodeNoCollideRemove, protocol.OpcodeClientMessage)
	if _, ok := r.w.NPCs[pet.ServerID]; ok || r.w.petOfLocked(r.p) != nil {
		t.Error("the dismissed pet is still in the world")
	}
	r.w.PetCommand(r.sess, 1, 0, r.at(13*time.Second))
	if msgs := r.sent(); len(msgs) != 1 || chatText(msgs[0]) != castNoPet {
		t.Errorf("a command with no pet: %x", msgs)
	}
}

// Stances: defensive takes on what attacks its owner, neutral does not;
// aggressive takes on what its owner fights; passive takes on nothing.
func TestThePetsStance(t *testing.T) {
	for _, c := range []struct {
		stance    petStance
		attacks   bool // the crab fights the owner
		fights    bool // the owner fights the crab
		wantFight bool
	}{
		{petDefensive, true, false, true},
		{petNeutral, true, false, false},
		{petPassive, true, true, false},
		{petAggressive, false, true, true},
		{petDefensive, false, true, false},
	} {
		r := newPetRig(t)
		r.summon(t, fireElemental(), 0)
		ps := r.w.pets[r.p.ServerID]
		ps.stance = c.stance
		r.enemy.X = r.p.X + 5 // within a creature's reach of the owner
		if c.attacks {
			r.enemy.CurrentEnemy = r.p.ServerID
		}
		if c.fights {
			r.p.AttackTarget = r.enemy.ServerID
		}
		r.tick(10 * time.Second)
		if got := ps.target == r.enemy.ServerID; got != c.wantFight {
			t.Errorf("stance %d, crab attacking %v, owner fighting %v: pet fights %v", c.stance, c.attacks, c.fights, got)
		}
	}
}

// A pet is on the players' side: no auto-attack and no harm cast at it; a
// pet spell lands on it whatever the target; with no pet a pet spell is
// refused with CLIENT's own words.
func TestAPetIsOnThePlayersSide(t *testing.T) {
	r := newPetRig(t)
	buff := CastSpell{SpellID: 638, Scope: 3, Name: "Infuse", Level: 4, CastTime: time.Second,
		Def: ParseSpellText("Pet, Increase Hitpoints by 15")}
	if r.w.StartCast(r.sess, buff, 0xFFFFFFFF, r.at(0)) {
		t.Fatal("a pet spell began with no pet")
	}
	if msgs := r.sent(); len(msgs) != 1 || !bytes.Contains(msgs[0], utf16le(castNoPet)) {
		t.Fatalf("no pet: %x", msgs)
	}
	pet, _ := r.summon(t, fireElemental(), time.Second)
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	if r.w.EngageAttack(r.sess, id, Swing{Kind: 0, Delay: 3 * time.Second, Reach: 5}, r.at(20*time.Second)) {
		t.Error("the owner engaged its pet")
	}
	if r.w.StartCast(r.sess, CastSpell{SpellID: 650, Scope: 1, Name: "Burst", Range: 30,
		Def: ParseSpellText("Target, Decrease Hitpoints by 26")}, id, r.at(21*time.Second)) {
		t.Error("harm cast at the pet")
	}
	r.sent()
	pet.HP = pet.MaxHP - 100
	if !r.w.StartCast(r.sess, buff, 0x0133E8A0, r.at(30*time.Second)) {
		t.Fatal("the pet spell did not begin")
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(30*time.Second + buff.CastTime + castLand))
	r.w.mu.Unlock()
	if pet.HP != pet.MaxHP-85 {
		t.Errorf("Infuse on the pet (the crab targeted): %d HP, want %d", pet.HP, pet.MaxHP-85)
	}
	if r.enemy.HP != r.enemy.MaxHP {
		t.Error("the pet spell landed on the target")
	}
}

// The pet goes with its owner: a logout takes it out of the world; the
// owner's death too, the owner told.
func TestThePetGoesWithItsOwner(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.p.HP = 0
	r.p.DiedAt = r.at(10 * time.Second)
	r.tick(10 * time.Second)
	if _, ok := r.w.NPCs[pet.ServerID]; ok {
		t.Error("the pet outlived its owner")
	}
	wantOps(t, r.sent(), protocol.OpcodeClearPet, protocol.OpcodeNoCollideRemove)

	r = newPetRig(t)
	pet, _ = r.summon(t, fireElemental(), 0)
	delete(r.w.Players, r.p.ServerID)
	r.tick(10 * time.Second)
	if _, ok := r.w.NPCs[pet.ServerID]; ok || len(r.w.pets) != 0 {
		t.Error("the pet stays when its owner leaves")
	}
}

// A MemoryDump clears CLIENT's pet: the next tick names it again. A second
// summon replaces the first: 0x007F before the new 0x007E, so the bar
// takes the new name.
func TestThePetIsToldAgain(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.tick(10 * time.Second)
	if msgs := r.sent(); len(msgs) != 0 {
		t.Fatalf("told again with nothing changed: %x", msgs)
	}
	r.w.PetDumped(r.sess)
	r.tick(10100 * time.Millisecond)
	msgs := r.sent()
	wantOps(t, msgs, protocol.OpcodeSetPet)
	if binary.LittleEndian.Uint32(msgs[0][2:]) != id {
		t.Errorf("0x007E names %#x, want %#x", binary.LittleEndian.Uint32(msgs[0][2:]), id)
	}
	r.p.Power = r.p.MaxPower
	_, msgs = r.summon(t, fireElemental(), 20*time.Second)
	wantOps(t, msgs, protocol.OpcodeClearPet, protocol.OpcodeNoCollideRemove, protocol.OpcodeSetPet, protocol.OpcodeNoCollideAdd)
	if _, ok := r.w.NPCs[pet.ServerID]; ok {
		t.Error("the first pet stays")
	}
}

// The pet keeps its slot among nearer creatures: it is first in its
// owner's view, wherever it is in the owner's world.
func TestThePetKeepsItsSlot(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	for i := 0; i < 40; i++ {
		n := &NPC{ServerID: EntityID(5000 + i), Name: "rat", Level: 1, World: 2, ModelID: 1,
			X: r.p.X + 0.5, Z: r.p.Z + float32(i)*0.01}
		r.w.NPCs[n.ServerID] = n
		r.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	pet.X += 200 // left behind, beyond the view
	r.w.SpatialIndex.Update(pet.ServerID, pet.X-200, pet.Z, pet.X, pet.Z)
	r.w.mu.Lock()
	r.w.TickCount = VisibilityTickInterval
	r.w.visibilitySystem()
	got := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.w.mu.Unlock()
	if got != id {
		t.Errorf("the pet's id %#x after the view filled, want %#x", got, id)
	}
}

// chatText is a 0x0A7A-style line's text (u32 length, UTF-16).
func chatText(m []byte) string {
	if len(m) < 6 {
		return ""
	}
	n := int(binary.LittleEndian.Uint32(m[2:]))
	var b strings.Builder
	for i := 0; i < n && 6+2*i+1 < len(m); i++ {
		b.WriteRune(rune(binary.LittleEndian.Uint16(m[6+2*i:])))
	}
	return b.String()
}

func utf16le(s string) []byte {
	var out []byte
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

// A dead pet's corpse lies its own ten seconds, not a creature's three
// minutes, then leaves the world, and a pet never wanders, whatever its
// radius (pet.go with corpse.go and roam.go). The controls: a creature's
// corpse still lies at the same time, and the crab beside it wanders.
func TestAPetsCorpseGoesInItsOwnTimeAndAPetDoesNotWander(t *testing.T) {
	r := newPetRig(t)
	r.w.SetCorpseTimes(3*time.Minute, time.Minute)
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.RoamRadius, r.enemy.RoamRadius = 15, 15
	r.w.mu.Lock()
	if r.w.wanders(pet) || !r.w.wanders(r.enemy) {
		t.Errorf("wanders: the pet %v, the crab %v", r.w.wanders(pet), r.w.wanders(r.enemy))
	}
	r.w.killNPC(pet)
	r.w.killNPC(r.enemy)
	r.w.corpseSystem(pet.DeathTime.Add(petCorpseTime + time.Second))
	r.w.mu.Unlock()
	if _, ok := r.w.NPCs[pet.ServerID]; ok {
		t.Error("the pet's corpse lies past its ten seconds")
	}
	if _, lies := r.w.CorpseLies(r.enemy.ServerID); !lies {
		t.Error("the crab's corpse went with the pet's")
	}
}
