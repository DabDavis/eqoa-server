package entity

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Aggro (aggro.go): a creature hostile by nature attacks a player within its
// level range who comes within its radius, through a hit's own entry, and the
// target box shows exactly those players the red face.

const (
	humanBody uint32 = 0x70D898C6 // a players' race's body (townsfolk, guards, bandits)
	bearBody  uint32 = 0x1B2B3B4B // a creature's
)

type aggroFix struct {
	w    *World
	sess *session.Session
	p    *Player
	now  time.Time
}

// newAggroFix is a world with one player of the level, in the world, at
// (1000, 10, 1000).
func newAggroFix(t *testing.T, level int) *aggroFix {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, CharServerID: 77, Name: "Walker", Level: level, HP: 1000, MaxHP: 1000, X: 1000, Y: 10, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1424A1D0)
	w.playerModels = map[uint32]bool{humanBody: true}
	return &aggroFix{w: w, sess: sess, p: p, now: time.Unix(1_800_000_000, 0)}
}

// npc puts an NPC of a body down at (x, 10, z), as the loader makes one: no
// NPC bit, neutral, its radius the loader's.
func (f *aggroFix) npc(id EntityID, name string, level int, body uint32, x, z float32) *NPC {
	n := &NPC{ServerID: id, Name: name, Level: level, ModelID: int64(body), HP: 500, MaxHP: 500,
		X: x, Y: 10, Z: z, SpawnX: x, SpawnY: 10, SpawnZ: z, Faction: FactionNeutral,
		AggroRadius: 30, LeashRadius: DefaultLeash}
	f.w.NPCs[id] = n
	f.w.SpatialIndex.Insert(id, x, z)
	return n
}

// town puts ten townsfolk in a row from (x, z), 20 apart: a town (towns.go).
func (f *aggroFix) town(x, z float32) {
	for i := 0; i < 10; i++ {
		f.npc(EntityID(5000+i), fmt.Sprintf("Citizen %d", i), 20, humanBody, x+float32(20*i), z)
	}
}

// look is one of the aggro scan's looks (it runs every aggroScanTicks).
func (f *aggroFix) look() {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.w.TickCount = aggroScanTicks * (f.w.TickCount/aggroScanTicks + 1)
	f.w.aggroSystem(f.now)
}

func (f *aggroFix) face(n *NPC) byte { return f.w.TargetFace(f.p, n) }

func (f *aggroFix) attacks(n *NPC) bool { return n.CurrentEnemy == f.p.ServerID }

// A bear 20 off a player half its level again attacks it, as a hit would
// (its threat on the player), and shows it the red face; one 35 off shows the
// red face too but has not noticed it.
func TestAHostileCreatureAttacksAPlayerWhoComesNear(t *testing.T) {
	f := newAggroFix(t, 40)
	near := f.npc(2001, "Brown Bear", 20, bearBody, 1020, 1000)
	far := f.npc(2002, "Brown Bear", 20, bearBody, 1035, 1000)
	f.look()
	if !f.attacks(near) || near.Threats[f.p.ServerID] <= 0 {
		t.Errorf("the bear 20 off did not attack: enemy %d, threats %v", near.CurrentEnemy, near.Threats)
	}
	if f.attacks(far) {
		t.Error("the bear 35 off attacked: its radius is 30")
	}
	for _, n := range []*NPC{near, far} {
		if got := f.face(n); got != FaceHostile {
			t.Errorf("%d's face %d, want the red %d", n.ServerID, got, FaceHostile)
		}
	}
}

// Sandstorm's level-51 player (pcaps/ps2_sandstorm_1min10secs.pcap): level
// 18 and 19 creatures showed the red face and level 17 ones the white, all of
// them green to it; the white Black Bear Cub came within 7.1 and did not
// attack. The control is the same creatures under a planted rule of "any
// hostile creature" (every one attacks).
func TestTheLevelCutSandstormShowed(t *testing.T) {
	rows := []struct {
		name  string
		level int
		red   bool
	}{
		{"Black Bear Cub", 18, true},
		{"Lavacrawler ", 19, true},
		{"Young Lioness", 19, true},
		{"Young Bear", 17, false},
		{"Black Bear Cub", 17, false},
	}
	f := newAggroFix(t, 51)
	for i, r := range rows {
		n := f.npc(EntityID(2100+i), r.name, r.level, bearBody, 1000+7.1, 1000)
		f.look()
		want := FaceNeutral
		if r.red {
			want = FaceHostile
		}
		if got := f.face(n); got != want {
			t.Errorf("%s level %d: face %d, want %d", r.name, r.level, got, want)
		}
		if f.attacks(n) != r.red {
			t.Errorf("%s level %d, 7.1 off a level 51: attacked %v, want %v", r.name, r.level, f.attacks(n), r.red)
		}
		n.Threats, n.CurrentEnemy = nil, 0
	}
	// the control: a level-17 creature attacks anyone at all within reach
	if !inAggroLevels(17, 50) || inAggroLevels(17, 51) || !inAggroLevels(18, 51) {
		t.Error("the cut is not at three times the level")
	}
}

// An invisible player walks past a bear that shows it the red face; seen
// again, it is attacked.
func TestAnInvisiblePlayerWalksPast(t *testing.T) {
	f := newAggroFix(t, 30)
	bear := f.npc(2001, "Brown Bear", 25, bearBody, 1010, 1000)
	f.p.Effects = []*ActiveEffect{{SpellID: 99, Name: "Invisibility", Invis: true}}
	f.look()
	if f.attacks(bear) {
		t.Fatal("attacked an invisible player")
	}
	if f.face(bear) != FaceHostile {
		t.Error("an invisible player's bear shows no red face: the face is its nature, not its sight")
	}
	f.p.Effects = nil
	f.look()
	if !f.attacks(bear) {
		t.Error("not attacked once visible again")
	}
}

// An NPC with the NPC bit (a merchant, a coachman) attacks no one, whatever a
// GM says: no one may attack it back. Its face stays its kind's.
func TestTheNPCBitNeverAttacks(t *testing.T) {
	f := newAggroFix(t, 10)
	merchant := f.npc(2001, "Merchant Tull", 20, bearBody, 1005, 1000)
	merchant.NPCType, merchant.Faction = 0x81, FactionFriendly
	coach := f.npc(2002, "Coachman Tull", 20, bearBody, 1006, 1000)
	coach.NPCType = 0x80
	f.w.SetAggroRules(map[string]AggroRule{"Merchant Tull": {Aggressive: true}, "Coachman Tull": {Aggressive: true}})
	f.look()
	if f.attacks(merchant) || f.attacks(coach) {
		t.Fatal("an NPC-bit NPC attacked")
	}
	if f.face(merchant) != FaceAmiable || f.face(coach) != FaceNeutral {
		t.Errorf("faces %d and %d, want %d and %d", f.face(merchant), f.face(coach), FaceAmiable, FaceNeutral)
	}
}

// A bandit with a players' race's body and a town's cat leave the player
// alone and show the white face; a GM's rule sets the bandits on it.
func TestTownsfolkAndATownsCreaturesLeaveYouAlone(t *testing.T) {
	f := newAggroFix(t, 10)
	f.town(1000, 1200) // ten townsfolk from (1000, 1200) to (1180, 1200)
	// a lone bandit is no town (towns.go wants ten): 200 off the town's people
	bandit := f.npc(2001, "Bandit", 12, humanBody, 1010, 1000)
	wild := f.npc(2003, "Brown Bear", 12, bearBody, 990, 1000)
	cat := f.npc(2002, "City Cat", 12, bearBody, 1020, 1170) // 30 from a townsperson
	f.look()
	if f.attacks(bandit) || f.face(bandit) != FaceNeutral {
		t.Errorf("the bandit attacked (%v) or showed face %d", f.attacks(bandit), f.face(bandit))
	}
	if !f.attacks(wild) {
		t.Error("the control, a bear out of town, did not attack")
	}
	// beside the town's cat
	f.w.SpatialIndex.Update(f.p.ServerID, f.p.X, f.p.Z, 1020, 1160)
	f.p.X, f.p.Z = 1020, 1160
	f.look()
	if f.attacks(cat) || f.face(cat) != FaceNeutral {
		t.Errorf("the town's cat attacked (%v) or showed face %d", f.attacks(cat), f.face(cat))
	}
	f.w.SpatialIndex.Update(f.p.ServerID, f.p.X, f.p.Z, 1000, 1000)
	f.p.X, f.p.Z = 1000, 1000
	f.w.SetAggroRules(map[string]AggroRule{"Bandit": {Aggressive: true}})
	f.look()
	if !f.attacks(bandit) || f.face(bandit) != FaceHostile {
		t.Errorf("a GM's rule did not set the bandit on the player: attacked %v, face %d", f.attacks(bandit), f.face(bandit))
	}
}

// A GM's rule keeps a creature peaceful, or lets it notice from farther.
func TestAGMRuleNamesWhoAttacksAndFromHowFar(t *testing.T) {
	f := newAggroFix(t, 20)
	calm := f.npc(2001, "Moss Snake", 15, bearBody, 1010, 1000)
	keen := f.npc(2002, "Hill Giant", 30, bearBody, 1045, 1000)
	f.look()
	if !f.attacks(calm) || f.attacks(keen) {
		t.Fatalf("the controls: snake %v (want true), giant 45 off %v (want false)", f.attacks(calm), f.attacks(keen))
	}
	calm.Threats, calm.CurrentEnemy = nil, 0
	f.w.SetAggroRules(map[string]AggroRule{"Moss Snake": {Aggressive: false}, "Hill Giant": {Aggressive: true, Radius: 50}})
	f.look()
	if f.attacks(calm) || f.face(calm) != FaceNeutral {
		t.Errorf("a peaceful rule's snake attacked (%v) or showed face %d", f.attacks(calm), f.face(calm))
	}
	if !f.attacks(keen) {
		t.Error("the giant with a radius of 50 did not notice the player 45 off")
	}
}

// A GM's #aggro off: creatures leave the character alone until #aggro on.
func TestAGMWhoTurnedAggroOffIsLeftAlone(t *testing.T) {
	f := newAggroFix(t, 20)
	bear := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	if !f.w.SetAggroIgnored(f.sess, true) {
		t.Fatal("not in the world")
	}
	if ign, _, _ := f.w.AggroIgnored(f.sess); !ign {
		t.Fatal("not kept")
	}
	f.look()
	if f.attacks(bear) {
		t.Fatal("attacked a GM who turned aggro off")
	}
	f.w.SetAggroIgnored(f.sess, false)
	f.look()
	if !f.attacks(bear) {
		t.Error("not attacked after #aggro on")
	}
}

// A creature in a fight, or on its way home after one, looks at no one else;
// with aggro off for the world none does; a player loading a world is not
// noticed; a pet, a dummy and a corpse attack no one.
func TestWhoLooksAndWhoIsSeen(t *testing.T) {
	f := newAggroFix(t, 20)
	busy := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	busy.CurrentEnemy, busy.Threats = 4242, map[EntityID]int64{4242: 50}
	home := f.npc(2002, "Brown Bear", 20, bearBody, 1012, 1000)
	home.chase.returning = true
	pet := f.npc(2003, "Brown Bear", 20, bearBody, 1014, 1000)
	pet.Owner = 4242
	dummy := f.npc(2004, "Training Dummy", 20, bearBody, 1016, 1000)
	dummy.Dummy = TrainingDummy
	corpse := f.npc(2005, "Brown Bear", 20, bearBody, 1018, 1000)
	corpse.Dead = true
	f.look()
	if busy.CurrentEnemy != 4242 || f.attacks(home) || f.attacks(pet) || f.attacks(dummy) || f.attacks(corpse) {
		t.Errorf("busy %d, home %v, pet %v, dummy %v, corpse %v", busy.CurrentEnemy, f.attacks(home), f.attacks(pet), f.attacks(dummy), f.attacks(corpse))
	}
	if f.face(pet) != FaceAlly {
		t.Errorf("a pet's face %d, want the players' %d", f.face(pet), FaceAlly)
	}

	g := newAggroFix(t, 20)
	bear := g.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	g.w.SetAggro(false)
	g.look()
	if g.attacks(bear) {
		t.Fatal("attacked with aggro off for the world")
	}
	g.w.SetAggro(true)
	g.sess.HoldStateChannels(15 * time.Second)
	g.look()
	if g.attacks(bear) {
		t.Fatal("attacked a player still loading its world")
	}
	g.sess.HoldStateChannels(0)
	g.look()
	if !g.attacks(bear) {
		t.Error("the control: not attacked once the world is loaded")
	}
}

// The fight aggro starts is a hit's: the creature runs at its enemy (the
// chase) and swings once in reach.
func TestTheFightAggroStartsIsTheChases(t *testing.T) {
	f := newAggroFix(t, 20)
	bear := f.npc(2001, "Brown Bear", 20, bearBody, 1025, 1000)
	bear.OffMod, bear.WeaponDamage = NPCOffMod(20), NPCWeaponDamage(20)
	f.look()
	if !f.attacks(bear) {
		t.Fatal("no aggro")
	}
	for i := 1; i <= 40; i++ {
		f.w.mu.Lock()
		f.w.aiSystem(f.now.Add(time.Duration(i) * 100 * time.Millisecond))
		f.w.mu.Unlock()
	}
	if d := distXZ(bear.X, bear.Z, f.p.X, f.p.Z); d > npcReach(bear) {
		t.Errorf("after 4 s the bear stands %.1f off, outside its reach %.1f: no chase", d, npcReach(bear))
	}
	if f.p.HP >= f.p.MaxHP {
		t.Error("no hit landed in 4 s in reach")
	}
}

// What the player sees is what attacks: over levels, bodies, the town and
// distance, a noticeable player within the radius is attacked exactly when its
// face is red.
func TestTheFaceAndTheAttackAgree(t *testing.T) {
	for _, pl := range []int{1, 5, 10, 30, 51, 60} {
		f := newAggroFix(t, pl)
		f.town(3000, 3000)
		id := EntityID(2000)
		type place struct {
			body   uint32
			x, z   float32
			inTown bool
		}
		for _, at := range []place{{bearBody, 1010, 1000, false}, {humanBody, 1010, 1000, false}, {bearBody, 3010, 3020, true}} {
			for lvl := 1; lvl <= 65; lvl += 4 {
				id++
				n := f.npc(id, fmt.Sprintf("Thing %d", id), lvl, at.body, at.x, at.z)
				// the player stands beside it, 10 off
				f.w.SpatialIndex.Update(f.p.ServerID, f.p.X, f.p.Z, at.x-10, at.z)
				f.p.X, f.p.Z = at.x-10, at.z
				f.look()
				red := f.face(n) == FaceHostile
				if red != f.attacks(n) {
					t.Fatalf("player %d, level %d, body %#x, in town %v: red %v, attacked %v", pl, lvl, at.body, at.inTown, red, f.attacks(n))
				}
				wantRed := at.body == bearBody && !at.inTown && lvl*3 > pl
				if red != wantRed {
					t.Errorf("player %d, level %d, body %#x, in town %v: red %v, want %v", pl, lvl, at.body, at.inTown, red, wantRed)
				}
				// out of the way of the next one
				f.w.SpatialIndex.Remove(n.ServerID, n.X, n.Z)
				delete(f.w.NPCs, n.ServerID)
			}
		}
	}
}
