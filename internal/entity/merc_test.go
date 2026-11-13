package entity

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The mercenaries (merc.go), held to the owner's asks of 2026-11-10: a hire
// makes one merc at its owner's level in the race and looks chosen, a
// player's maxima and defence; a healer heals a hurt team mate (none hurt,
// none), a tank taunts what its owner fights (nothing fought, no taunt), a
// caster casts on its cooldown (not before); it never fights a player; its
// kill gives its owner a solo kill's experience; dismissal, stance, a logout
// and a keep restart; it takes a place in its owner's group.

// The bodies charactermodel holds (the root CLAUDE.md's model ids as the
// database carries them).
const (
	humanMale      uint32 = 1893243078
	humanFemale    uint32 = 223572789
	darkElfMale    uint32 = 2128125354
	darkElfFemale  uint32 = 2358987577
	mercOwnerLevel        = 30
)

type mercRig struct {
	t    *testing.T
	w    *World
	mgr  *session.Manager
	sess *session.Session
	p    *Player
	t0   time.Time
}

// newMercRig is a level 30 human warrior in Tunaria with a session, the
// race bodies charactermodel holds.
func newMercRig(t *testing.T) *mercRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetRaceModels(map[[2]int]uint32{{0, 0}: humanMale, {0, 1}: humanFemale, {2, 0}: darkElfMale, {2, 1}: darkElfFemale})
	r := &mercRig{t: t, w: w, mgr: mgr, t0: time.Unix(1_800_000_000, 0)}
	offbeat := mercOffbeat
	mercOffbeat = false // the rig's casts tick for tick (mercoffbeat.go; its own test turns it on)
	t.Cleanup(func() { mercOffbeat = offbeat })
	r.sess, r.p = r.player("Owner", 0, 0)
	return r
}

func (r *mercRig) at(d time.Duration) time.Time { return r.t0.Add(d) }

// player is a player of the class and race in the world beside the rig's
// first, with a session in the world.
func (r *mercRig) player(name string, class, race int) (*session.Session, *Player) {
	r.w.mu.Lock()
	id := r.w.allocFreeIDLocked() // the World's ids, as CreatePlayer's: a merc's never another's
	r.w.mu.Unlock()
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(id%250)), Port: 7000 + int(id%1000)}, uint32(id), 1, 1, 2, true)
	b := LookupBaseStats(race, class)
	p := &Player{ServerID: id, CharServerID: uint32(id) + 500, Name: name, Class: class, Race: race, Level: mercOwnerLevel,
		Speed: RunSpeed, X: 1000 + float32(len(r.w.Players))*2, Z: 1000, World: 0, ModelID: int64(humanMale)}
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = b.Str, b.Sta, b.Agi, b.Dex, b.Wis, b.Int, b.Cha
	p.RecalcStats(true)
	r.w.Players[p.ServerID] = p
	r.w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x00A00000 + uint32(id))
	return sess, p
}

// creature is a creature beside the owner.
func (r *mercRig) creature(name string, level int, dx float32) *NPC {
	id := EntityID(5000 + len(r.w.NPCs))
	hp := NPCMaxHP(level) * 20
	n := &NPC{ServerID: id, Name: name, Level: level, HP: hp, MaxHP: hp, ModelID: 0x55432169, Size: 1, NPCType: 0x0100,
		X: r.p.X + dx, Y: r.p.Y, Z: r.p.Z, World: r.p.World}
	r.w.mu.Lock()
	r.w.putNPCLocked(n)
	r.w.mu.Unlock()
	r.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	return n
}

// bookSpell is a spell of a merc's book as the cast handler builds one.
func bookSpell(id int, name string, scope int, cast, recast time.Duration, power int, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id, Level: 1, Scope: scope, Name: name, CastTime: cast, Recast: recast,
		PowerCost: power, Range: 30, Def: ParseSpellText(line)}
}

// kit is a merc of the class, race and sex at the owner's level in a plate
// set and a sword, with the spells given.
func (r *mercRig) kit(class, race, sex int, spells ...CastSpell) MercKit {
	k := MercKit{Class: class, Race: race, Sex: sex, Level: r.p.Level, Name: "Ralthas", Base: SimBaseStats(class, race, r.p.Level),
		Gear:   Gear{WeaponCond: 1, AC: 240, Sta: 30, Str: 20, MaxHP: 160, MaxPower: 40, WeaponDamage: 180, WeaponSlot: 14, WeaponType: 1},
		Spells: spells}
	k.Look = MercLook{Face: 1, HairStyle: 2, Chest: 4, Bracer: 4, Gloves: 4, Legs: 4, Boots: 4, Helm: 4, Robe: -1,
		Primary: 0x47D9E5E0, PrimaryType: 1}
	k.Worn[14], k.AttackType = true, 1
	return k
}

// hire hires the kit's merc for the owner, which must come, and answers yes
// to its box (a merc fights only in its owner's group: mercgroup.go).
func (r *mercRig) hire(k MercKit, stance MercStance) *mercState {
	r.t.Helper()
	ms := r.hireAsked(k, stance)
	r.w.mu.Lock()
	_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
	r.w.mu.Unlock()
	if up {
		r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	}
	return ms
}

// hireAsked hires the kit's merc for the owner, which must come, its box left
// unanswered.
func (r *mercRig) hireAsked(k MercKit, stance MercStance) *mercState {
	r.t.Helper()
	if _, why := r.w.HireMerc(r.sess, k, stance, r.at(0)); why != "" {
		r.t.Fatalf("the hire was refused: %s", why)
	}
	ms := r.w.merc1(r.p.ServerID)
	if ms == nil {
		r.t.Fatal("no merc after the hire")
	}
	return ms
}

// tick runs the systems a merc lives in, as the tick does, at d.
func (r *mercRig) tick(d time.Duration) {
	w := r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TickCount++
	now := r.at(d)
	w.aiSystem(now)
	w.petSystem(now)
	w.mercSystem(now)
	w.familiarSystem(now)
	w.castSystem(now)
	w.effectSystem(now)
}

// run ticks from a to b every 100 ms.
func (r *mercRig) run(a, b time.Duration) {
	for d := a; d <= b; d += 100 * time.Millisecond {
		r.tick(d)
	}
}

// sent is what the owner's session was sent since the last look, by opcode.
func (r *mercRig) sent() map[protocol.GameOpcode]int {
	out := map[protocol.GameOpcode]int{}
	for _, m := range r.sess.Queue().GetPending() {
		out[opOfMsg(m.Data())]++
	}
	return out
}

// group makes the owner lead a group of the players given, as their yeses do.
func (r *mercRig) group(members ...*Player) *Group {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	g := r.w.groups[r.p.GroupID]
	if r.p.GroupID == 0 || g == nil {
		g = r.w.formGroupLocked(r.p)
	}
	for _, m := range members {
		g.Members = append(g.Members, m.ServerID)
		m.GroupID = g.ID
	}
	return g
}

// A hire makes one merc at its owner's level, owned, in the race and sex
// chosen and its kit's looks, a player's maxima and swing of its class and
// level; a second hire takes its place (one a player). The control: the
// owner's own race when no other is chosen is its own body.
func TestMercHireIsAPlayerOfItsLevel(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(9, 2, 1) // a dark elf woman cleric
	ms := r.hire(k, MercAssist)
	m := ms.npc
	if m.Owner != r.p.ServerID || m.Level != mercOwnerLevel || m.Class != 9 || !m.Temporary || m.NPCType != 0 {
		t.Fatalf("merc: owner %d level %d class %d temporary %v type %#x", m.Owner, m.Level, m.Class, m.Temporary, m.NPCType)
	}
	if uint32(m.ModelID) != darkElfFemale || m.Race != "2" {
		t.Errorf("body %#x race %q, want the dark elf woman's %#x and race 2", uint32(m.ModelID), m.Race, darkElfFemale)
	}
	if m.Torso != 4 || m.Forearm != 4 || m.NPCGloves != 4 || m.NPCLegs != 4 || m.Feet != 4 || m.Head != 4 ||
		uint32(m.PrimaryWeapon) != 0x47D9E5E0 || m.PrimaryType != 1 || m.Face != 1 || m.HairStyle != 2 || m.RobeStyle != "" {
		t.Errorf("looks: torso %d forearm %d gloves %d legs %d feet %d head %d primary %#x type %d face %d hair %d robe %q",
			m.Torso, m.Forearm, m.NPCGloves, m.NPCLegs, m.Feet, m.Head, uint32(m.PrimaryWeapon), m.PrimaryType, m.Face, m.HairStyle, m.RobeStyle)
	}
	// a player of its class, race and level in the same gear
	want := &Player{Class: 9, Race: 2, Level: mercOwnerLevel}
	b := k.Base
	want.BaseStr, want.BaseSta, want.BaseAgi, want.BaseDex, want.BaseWis, want.BaseInt, want.BaseCha = b[0], b[1], b[2], b[3], b[4], b[5], b[6]
	want.Gear = k.Gear
	want.RecalcStats(true)
	if m.MaxHP != want.MaxHPNow() || m.MaxPower != want.MaxPowerNow() || ms.body.StatNow(StatAC) != 240 {
		t.Errorf("maxima: HP %d power %d AC %d, a player's %d, %d, 240", m.MaxHP, m.MaxPower, ms.body.StatNow(StatAC),
			want.MaxHPNow(), want.MaxPowerNow())
	}
	if m.HP != m.MaxHP || m.Power != m.MaxPower {
		t.Errorf("a new merc at %d/%d HP and %d/%d power, not whole", m.HP, m.MaxHP, m.Power, m.MaxPower)
	}
	if !m.SwingFixed || m.SwingKind != GetSwing(9, mercOwnerLevel, k.Worn) {
		t.Errorf("swing %d (fixed %v), a cleric's of 30 %d", m.SwingKind, m.SwingFixed, GetSwing(9, mercOwnerLevel, k.Worn))
	}
	buf := SerializeNPCUpdate(1, m)
	if buf[142] != 3 || buf[143] != 1 || buf[144] != 2 || buf[146] != 0 || buf[93] != 4 {
		t.Errorf("channel: name colour %d plate %d race %d npc bit %#x chest %d", buf[142], buf[143], buf[144], buf[146], buf[93])
	}
	// a second hire takes its place
	k2 := r.kit(0, 0, 0)
	k2.Name = "Belthas"
	r.hire(k2, MercAssist)
	n := 0
	for _, npc := range r.w.NPCs {
		if npc.isMerc() {
			n++
			if npc.Name != "Belthas" || uint32(npc.ModelID) != humanMale {
				t.Errorf("the merc out is %q in %#x, want Belthas in the human man's body", npc.Name, uint32(npc.ModelID))
			}
		}
	}
	if n != 1 {
		t.Errorf("%d mercs in the world after a second hire, want one", n)
	}
}

// A merc of a racedefs race wears that race's body and passes as its wire
// race on the channel; one of the game's races its race's body; the control,
// a race with no body known, its owner's.
func TestMercRaceBodies(t *testing.T) {
	SetRaceDefs([]database.RaceDefRow{{ID: 13, Name: "Goblin", WireRace: 2, BodyMale: 0x6C0E0D20, BodyFemale: 0x6C0E0D20}})
	t.Cleanup(func() { SetRaceDefs(nil) })
	r := newMercRig(t)
	m := r.hire(r.kit(6, 13, 0), MercAssist).npc
	if uint32(m.ModelID) != 0x6C0E0D20 || m.Race != strconv.Itoa(2) {
		t.Errorf("a goblin merc: body %#x race %q, want %#x and its wire race 2", uint32(m.ModelID), m.Race, 0x6C0E0D20)
	}
	m = r.hire(r.kit(6, 0, 1), MercAssist).npc
	if uint32(m.ModelID) != humanFemale || m.Race != "0" {
		t.Errorf("a human woman merc: body %#x race %q", uint32(m.ModelID), m.Race)
	}
	r.p.ModelID = 0x12345678
	m = r.hire(r.kit(6, 7, 0), MercAssist).npc // a halfling: no body in this rig's charactermodel
	if uint32(m.ModelID) != 0x12345678 {
		t.Errorf("a race with no body known wears %#x, want its owner's %#x", uint32(m.ModelID), 0x12345678)
	}
}

// A merc of a race whose own body takes no armour (the hill giant's) wears
// the body of the armour on its chest, as its players do (the owner,
// 2026-11-11: "mercenaries arent always wearing armor visually, like uwu's
// merc ithiel", a hill giant necromancer): a robe's cloth the sand giant's, a
// chain the fire giant's. The control: a bare chest, the race's own body.
func TestMercWearsItsRacesArmourBody(t *testing.T) {
	const own, sand, fire = 0xFFB1EA2C, 0xE6170C3B, 0x72A6B03B
	SetRaceDefs([]database.RaceDefRow{{ID: 11, Name: "Hill Giant", WireRace: 9, BodyMale: own, BodyFemale: own}})
	t.Cleanup(func() { SetRaceDefs(nil) })
	r := newMercRig(t)
	r.w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "cloth", Body: sand}, {Race: 11, Armor: "chain", Body: fire}})
	for _, c := range []struct {
		armor ArmorKind
		want  uint32
	}{{ArmorCloth, sand}, {ArmorChain, fire}, {ArmorNone, own}} {
		k := r.kit(11, 11, 0)
		k.Gear.Armor = c.armor
		if m := r.hire(k, MercAssist).npc; uint32(m.ModelID) != c.want {
			t.Errorf("a hill giant merc in %v: body %#x, want %#x", c.armor, uint32(m.ModelID), c.want)
		}
	}
}

// A hill giantess merc (racedefs' female body her own, the GM Jibrish's petition
// 1647) wears her own body in any armour: the race's armour bodies are the male's
// (a male giant's each). The control: a hill giant man in the same robe takes the
// sand giant's.
func TestMercGiantessKeepsHerBodyInArmour(t *testing.T) {
	const own, giantess, sand = 0xFFB1EA2C, 0xF34501CB, 0xE6170C3B
	SetRaceDefs([]database.RaceDefRow{{ID: 11, Name: "Hill Giant", WireRace: 9, BodyMale: signed(own), BodyFemale: signed(giantess)}})
	t.Cleanup(func() { SetRaceDefs(nil) })
	r := newMercRig(t)
	r.w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "cloth", Body: sand}})
	for _, c := range []struct {
		sex  int
		want uint32
	}{{1, giantess}, {0, sand}} {
		k := r.kit(11, 11, c.sex)
		k.Gear.Armor = ArmorCloth
		if m := r.hire(k, MercAssist).npc; uint32(m.ModelID) != c.want {
			t.Errorf("a hill giant merc of sex %d in cloth: body %#x, want %#x", c.sex, uint32(m.ModelID), c.want)
		}
	}
}

// A creature's hit on a merc is its roll on a player of the merc's class with
// the merc's AC and AGI (defence.go), not a pet's tank of its level.
func TestMercIsHitAsAPlayer(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0), MercAssist)
	crab := r.creature("Desert Crab", 32, 3)
	mid := func(n int) int { return n / 2 }
	got := npcHitOnPet(crab, ms.npc, mid)
	want := npcMeleeRollFor(crab, ms.body).hitOn(ms.body, mid)
	if got != want {
		t.Errorf("a crab's hit on the merc %d, a player's of its class and armour %d", got, want)
	}
	pet := &NPC{Level: mercOwnerLevel, Owner: r.p.ServerID}
	if p := npcHitOnPet(crab, pet, mid); p == got {
		t.Errorf("a pet of its level is hit %d, as the merc: the merc's armour counted for nothing", p)
	}
}

// A healer heals its owner: with no one hurt it casts nothing (the control);
// its owner at half its hit points is healed, and so is the healer itself; a
// member of its owner's group is not (merccast.go: its owner's own alone).
func TestMercHealerHealsItsTeam(t *testing.T) {
	r := newMercRig(t)
	_, mate := r.player("Mate", 0, 0)
	r.group(mate)
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	hot := bookSpell(464, "Healing", 1, 3*time.Second, time.Second, 200, "Target, Increase HP per tic by 125 ; Increase 25% based on CHA, 24sec")
	ms := r.hire(r.kit(9, 0, 0, heal, hot), MercAssist)
	r.run(0, 3*time.Second)
	if len(r.w.mercCasts) != 0 || len(ms.ready) != 0 {
		t.Fatalf("no one hurt: %d casts on their way, %d spells cast", len(r.w.mercCasts), len(ms.ready))
	}
	top := mate.MaxHPNow()
	mate.HP = top / 2
	r.run(3*time.Second+100*time.Millisecond, 6*time.Second)
	if len(r.w.mercCasts) != 0 || len(ms.ready) != 0 || mate.HP != top/2 {
		t.Fatalf("a group mate hurt, not its owner's: %d casts, %d cast, the mate %d/%d", len(r.w.mercCasts), len(ms.ready), mate.HP, top)
	}
	mate.HP = top
	otop := r.p.MaxHPNow()
	r.p.HP = otop / 2
	r.run(6*time.Second+100*time.Millisecond, 12*time.Second)
	if r.p.HP <= otop/2 {
		t.Errorf("the owner at half its hit points was not healed: %d/%d", r.p.HP, otop)
	}
	if len(ms.ready) == 0 {
		t.Error("no heal was cast")
	}
	// itself hurt, its owner whole: it heals itself
	r.p.HP = otop
	m := ms.npc
	m.HP = m.MaxHP / 2
	r.run(30*time.Second, 40*time.Second)
	if m.HP <= m.MaxHP/2 {
		t.Errorf("the healer at half its hit points did not heal itself: %d/%d", m.HP, m.MaxHP)
	}
}

// A tank taunts what its owner fights: with nothing fought it casts nothing
// (the control); its owner on a creature, its taunt lands and the creature
// turns on it.
func TestMercTankTauntsWhatItsOwnerFights(t *testing.T) {
	r := newMercRig(t)
	taunt := bookSpell(2295, "Taunt", 1, 0, 5*time.Second, 19, "Target, Increase Hate by 100 ; Increase 10% based on STR")
	ms := r.hire(r.kit(0, 0, 0, taunt), MercAssist)
	crab := r.creature("Desert Crab", 28, 4)
	r.run(0, 3*time.Second)
	if len(ms.ready) != 0 || ms.target != 0 {
		t.Fatalf("nothing fought: target %d, %d spells cast", ms.target, len(ms.ready))
	}
	// its owner swings at the crab, which fights its owner
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 400)
	r.w.mu.Unlock()
	r.run(3*time.Second+100*time.Millisecond, 8*time.Second)
	if ms.target != crab.ServerID {
		t.Fatalf("the tank fights %d, not its owner's crab", ms.target)
	}
	if _, cast := ms.ready[taunt.SpellID]; !cast {
		t.Fatal("the tank did not taunt")
	}
	if crab.CurrentEnemy != ms.npc.ServerID {
		t.Errorf("the crab fights %d after the taunt, not the tank %d", crab.CurrentEnemy, ms.npc.ServerID)
	}
}

// A caster casts its nuke again only once its lockout (cast time, recast,
// castLockoutExtra) is over: a second cast not before, then one.
func TestMercCasterCastsOnCooldown(t *testing.T) {
	r := newMercRig(t)
	nuke := bookSpell(950, "Ice Bolt", 1, 2*time.Second, 10*time.Second, 40, "Target, Decrease Hitpoints by 300")
	ms := r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	crab := r.creature("Desert Crab", 28, 10)
	crab.HP, crab.MaxHP = 900000, 900000
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	var begun []time.Duration
	last := time.Time{}
	for d := time.Duration(0); d <= 26*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		if until := ms.ready[nuke.SpellID]; !until.Equal(last) {
			last = until
			begun = append(begun, until.Sub(r.t0)-(nuke.CastTime+nuke.Recast+castLockoutExtra))
		}
	}
	lock := nuke.CastTime + nuke.Recast + castLockoutExtra
	next := mercCombatWait + lock
	if len(begun) < 2 || begun[0] != mercCombatWait || begun[1] < next || begun[1] > next+200*time.Millisecond {
		t.Fatalf("the nuke began at %v, want at %v and again at %v", begun, mercCombatWait, next)
	}
	if crab.HP >= 900000 {
		t.Error("the nuke never landed")
	}
	if !ms.npc.RoundStart.IsZero() {
		t.Error("a caster swung at its foe")
	}
}

// A merc never takes on a player (its owner's duel opponent), nor a creature
// players cannot attack, nor another's pet; passive, nothing fighting its
// owner. In assist, a creature that aggroed on its owner is not taken on until
// its owner hits it (GM ticket 1959, merccall.go); then it is (the control).
func TestMercNeverFightsAPlayer(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	_, foe := r.player("Foe", 0, 0)
	r.w.duels.add(&Duel{ID: 1, A: r.p.ServerID, B: foe.ServerID, who: [2]*Player{r.p, foe}, Phase: duelOn})
	r.p.AttackTarget, r.p.AutoAttack = foe.ServerID, true
	r.run(0, time.Second)
	if ms.target != 0 {
		t.Fatalf("the merc took on %d in its owner's duel", ms.target)
	}
	r.w.mu.Lock()
	if r.w.duelFoeLocked(foe, ms.npc.ServerID) {
		t.Error("the merc is its owner's duel opponent's foe")
	}
	r.w.mu.Unlock()
	banker := r.creature("Banker", 30, 3)
	banker.NPCType = 0x80
	r.p.AttackTarget = banker.ServerID
	r.run(time.Second, 2*time.Second)
	if ms.target != 0 {
		t.Fatalf("the merc took on %q, whom players cannot attack", banker.Name)
	}
	r.p.AttackTarget, r.p.AutoAttack = 0, false
	r.w.duels.remove(r.w.duels.of[r.p.ServerID]) // the duel over: a merc stands down for one (ownfight.go)
	crab := r.creature("Desert Crab", 28, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 50)
	r.w.mu.Unlock()
	r.w.SetMercStance(r.sess, 0, MercPassive)
	r.run(2*time.Second, 3*time.Second)
	if ms.target != 0 {
		t.Fatalf("a passive merc took on %d", ms.target)
	}
	r.w.SetMercStance(r.sess, 0, MercAssist)
	r.run(3*time.Second, 4*time.Second)
	if ms.target != 0 {
		t.Fatalf("an assisting merc took on a crab its owner never touched (target %d)", ms.target)
	}
	r.w.mu.Lock()
	r.w.updateAggro(crab, r.p.ServerID, 10) // its owner's hit
	r.w.mu.Unlock()
	r.run(4*time.Second, 5*time.Second)
	if ms.target != crab.ServerID {
		t.Errorf("an assisting merc left the crab its owner hit (target %d)", ms.target)
	}
}

// A merc's kill is its owner's and gives it what a solo kill gives (the merc
// takes no share and cuts none: a group of its owner and it is a group of one
// to the split); the control, a second player in the group: the two-player
// split.
func TestMercTakesNoExperience(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	r.group()
	if g := r.w.groups[r.p.GroupID]; g == nil || len(g.Members) != 2 {
		t.Fatalf("the owner and its merc are not a group of two: %+v", g)
	}
	kill := func() int {
		crab := r.creature("Desert Crab", 30, 3)
		before := r.p.TotalXP
		r.w.mu.Lock()
		if !r.w.mercDamageLocked(ms, crab, crab.HP, numberMelee, r.at(0)) {
			r.t.Fatal("no kill")
		}
		r.w.mu.Unlock()
		return r.p.TotalXP - before
	}
	if got, want := kill(), groupExp(30, mercOwnerLevel, []int{mercOwnerLevel}); got != want {
		t.Errorf("the merc's kill gave its owner %d, a solo kill's %d", got, want)
	}
	_, mate := r.player("Mate", 0, 0)
	r.group(mate)
	if got, want := kill(), groupExp(30, mercOwnerLevel, []int{mercOwnerLevel, mercOwnerLevel}); got != want {
		t.Errorf("with a second player its owner got %d, the two players' share %d", got, want)
	}
}

// Dismissed, the merc is out of the world and its group (one left alone is
// no group); passive, it lets its target go; a logout takes it out with its
// owner, its hire kept (MercNoteGone).
func TestMercDismissStanceLogout(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	r.group()
	ms.target = r.creature("Desert Crab", 28, 3).ServerID
	r.w.SetMercStance(r.sess, 0, MercPassive)
	if ms.target != 0 || ms.stance != MercPassive {
		t.Errorf("passive: target %d stance %d", ms.target, ms.stance)
	}
	if name, ok := r.w.DismissMerc(r.sess, 1, r.at(0)); !ok || name != "Ralthas" {
		t.Fatalf("dismiss: %q %v", name, ok)
	}
	if r.w.NPCs[ms.npc.ServerID] != nil || r.w.merc1(r.p.ServerID) != nil || r.p.GroupID != 0 {
		t.Errorf("after the dismissal: npc %v, merc %v, group %d", r.w.NPCs[ms.npc.ServerID] != nil, r.w.merc1(r.p.ServerID) != nil, r.p.GroupID)
	}
	ms = r.hire(r.kit(0, 0, 0), MercAssist)
	r.w.RemovePlayer(r.p.ServerID)
	if r.w.NPCs[ms.npc.ServerID] != nil || len(r.w.mercs) != 0 {
		t.Error("the merc stayed after its owner logged out")
	}
	if len(r.w.mercNotes) != 1 || r.w.mercNotes[0].Kind != MercNoteGone {
		t.Errorf("notes after the logout: %+v", r.w.mercNotes)
	}
}

// A merc takes a place in its owner's group: a player in no group is asked
// into one with it in its own box, and its yes makes the two a group, its
// owner leading, the merc on the roster; a group of four takes none (the hire
// refused), a group of three does, and the dismissal frees the place; a raid
// flag's group takes one past four.
func TestMercTakesAGroupPlace(t *testing.T) {
	r := newMercRig(t)
	r.sent()
	ms := r.hireAsked(r.kit(9, 0, 0), MercAssist)
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("a player in no group was not asked into one with its merc: %v", got)
	}
	if r.p.GroupID != 0 {
		t.Fatal("the owner is in a group before its yes")
	}
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	g := r.w.groups[r.p.GroupID]
	if g == nil || len(g.Members) != 2 || g.Members[0] != r.p.ServerID || g.Members[1] != ms.npc.ServerID {
		t.Fatalf("after the yes: group %+v", g)
	}
	if got := r.sent(); got[protocol.OpcodeCreateGroup] == 0 {
		t.Error("no roster after the yes")
	}
	r.w.mu.Lock()
	views := r.w.shownViewsLocked(g, r.p)
	r.w.mu.Unlock()
	if len(views) != 2 || views[1].name != "Ralthas" || views[1].top != ms.npc.MaxHP {
		t.Errorf("the group window's members: %+v", views)
	}
	// three more players: the group of four (with the merc) takes no fifth
	var others []*Player
	for i := range 2 {
		_, o := r.player("Member"+strconv.Itoa(i), 0, 0)
		others = append(others, o)
	}
	r.group(others...)
	if n := len(g.Members); n != 4 {
		t.Fatalf("%d members, want 4", n)
	}
	k := r.kit(0, 0, 0)
	k.Name = "Belthas"
	if _, why := r.w.HireMerc(r.sess, k, MercAssist, r.at(0)); why != "" {
		t.Errorf("a new merc in the old one's place was refused: %s", why)
	}
	_, fourth := r.player("Fourth", 0, 0)
	if r.w.DismissMerc(r.sess, 1, r.at(0)); len(g.Members) != 3 {
		t.Fatalf("the dismissal left %d members", len(g.Members))
	}
	r.group(fourth)
	// a group of four players: the merc goes on standby (mercbench.go), its
	// hire kept, out of the group and the world
	if _, why := r.w.HireMerc(r.sess, r.kit(0, 0, 0), MercAssist, r.at(0)); why != "" || len(g.Members) != 4 ||
		r.w.benchedAtLocked(r.p, 1) == nil || r.w.merc1(r.p.ServerID) != nil {
		t.Errorf("a group of four players took a merc (%q, %d members) or it is not on standby", why, len(g.Members))
	}
	g.flagged = true
	if _, why := r.w.HireMerc(r.sess, r.kit(0, 0, 0), MercAssist, r.at(0)); why != "" || len(g.Members) != 5 {
		t.Errorf("a raid flag's group refused a merc (%q) or did not take it (%d members)", why, len(g.Members))
	}
	// its owner leaves: its merc goes from the group with it
	r.w.GroupLeave(r.sess)
	for _, id := range g.Members {
		if n := r.w.NPCs[id]; n.isMerc() {
			t.Errorf("the merc stayed in the group its owner left")
		}
	}
}

// A group of a player and its merc alone gives way to another player's
// invitation: its client told it is out (0x069A's 8, no line) before the box,
// and the yes takes both into the inviter's group. A no asks the owner into a
// group with its merc again, and not while the box is up. Out of a group of
// others the owner is asked again; out of the group of it and its merc alone it
// is not. The control: a group of the owner, its merc and another player is a
// group to an invitation (the inviter told so, the group kept).
func TestMercGroupGivesWay(t *testing.T) {
	r := newMercRig(t)
	ms := r.hireAsked(r.kit(9, 0, 0), MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	own := r.p.GroupID
	isess, inviter := r.player("Inviter", 0, 0)
	r.sent()
	r.w.GroupInvite(isess, r.w.IDSeenBy(isess, r.p))
	if r.p.GroupID != 0 || r.w.groups[own] != nil {
		t.Fatalf("the group of the owner and its merc did not give way: group %d", r.p.GroupID)
	}
	var ops []protocol.GameOpcode
	for _, m := range r.sess.Queue().GetPending() {
		ops = append(ops, opOfMsg(m.Data()))
		if opOfMsg(m.Data()) == protocol.OpcodeRemoveGroupMember && m.Data()[2] != 8 {
			t.Errorf("the owner's client was told %d, want 8 (no line)", m.Data()[2])
		}
	}
	if len(ops) != 2 || ops[0] != protocol.OpcodeRemoveGroupMember || ops[1] != protocol.OpcodeGroupInviteReceived {
		t.Fatalf("the owner's client was sent %04X, want 069A then 0773", ops)
	}
	r.run(0, time.Second) // the upkeep: no box of its own while the invitation's is up
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 0 {
		t.Error("the owner was asked into a group with its merc while another box was up")
	}
	r.w.GroupDecline(r.sess, r.w.IDSeenBy(r.sess, inviter), 1)
	r.run(time.Second, 2*time.Second)
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("after its no the owner was not asked into a group with its merc again: %v", got)
	}
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	// a second invitation, its yes: the owner and its merc in the inviter's group
	r.w.GroupInvite(isess, r.w.IDSeenBy(isess, r.p))
	r.w.GroupAccept(r.sess, r.w.IDSeenBy(r.sess, inviter))
	g := r.w.groups[inviter.GroupID]
	if g == nil || r.p.GroupID != g.ID || len(g.Members) != 3 || g.Members[0] != inviter.ServerID || !groupHas(g, ms.npc.ServerID) {
		t.Fatalf("after the yes: group %+v, the owner's %d", g, r.p.GroupID)
	}
	// the control: a group of others is a group to an invitation
	jsess, _ := r.player("Another", 0, 0)
	r.w.GroupInvite(jsess, r.w.IDSeenBy(jsess, r.p))
	if r.p.GroupID != g.ID || len(g.Members) != 3 {
		t.Fatalf("an invitation took the owner out of a group of others: group %d, %d members", r.p.GroupID, len(g.Members))
	}
	// out of a group of others: asked again
	r.sent()
	r.w.GroupLeave(r.sess)
	r.run(2*time.Second, 3*time.Second)
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("out of a group of others the owner was not asked again: %v", got)
	}
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	if g := r.w.groups[r.p.GroupID]; g == nil || len(g.Members) != 2 {
		t.Fatalf("the owner and its merc are not a group again: %+v", g)
	}
	// out of the group of it and its merc alone: not asked again, dismissed
	// (petition 1504: a merc fights only in its owner's group)
	r.sent()
	r.w.GroupLeave(r.sess)
	r.run(3*time.Second, 4*time.Second)
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 0 || r.p.GroupID != 0 {
		t.Errorf("leaving the group of it and its merc asked again (%v) or kept a group (%d)", got, r.p.GroupID)
	}
	if r.w.merc1(r.p.ServerID) != nil || r.w.NPCs[ms.npc.ServerID] != nil {
		t.Fatal("the merc stayed after its owner left the group of the two of them")
	}
	// a raid flag's box too: the group of the owner and its merc gives way
	ms = r.hire(r.kit(9, 0, 0), MercAssist)
	if g := r.w.groups[r.p.GroupID]; g == nil || len(g.Members) != 2 {
		t.Fatalf("/merc group made no group of the owner and its merc: %+v", g)
	}
	lsess, leader := r.player("Leader", 0, 0)
	r.sent()
	r.w.mu.Lock()
	r.w.raidFlagLocked(lsess, leader, r.at(4*time.Second))
	r.w.mu.Unlock()
	if got := r.sent(); r.p.GroupID != 0 || got[protocol.OpcodeRemoveGroupMember] != 1 || got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Errorf("a raid flag's box: group %d, sent %v", r.p.GroupID, got)
	}
}

// A keep restart carries the merc: its shares of hit points and power, its
// place, its stance and its group place, made again from its kit in the new
// process; its owner and it alone a group again with no box.
func TestMercKeepRestart(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(0, 0, 0)
	ms := r.hireAsked(k, MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	ms.stance = MercAggressive
	ms.npc.HP = ms.npc.MaxHP / 2
	ms.npc.X += 1 // within its slot's slack (followslot.go)
	kp, ok := r.w.KeepPlayer(r.sess, r.at(0))
	if len(kp.Mercs) == 1 {
		kp.Merc = &kp.Mercs[0] // one merc: the test of one
	}
	if !ok || kp.Merc == nil || !kp.Merc.Grouped || kp.Merc.Stance != byte(MercAggressive) {
		t.Fatalf("kept: %+v", kp.Merc)
	}
	place := kp.Merc.Place
	// the new process: the player built again, no merc, no group
	r2 := newMercRig(t)
	r2.p.X, r2.p.Z = r.p.X, r.p.Z
	r2.sent()
	if !r2.w.RestoreMerc(r2.sess, k, *kp.Merc, r2.at(0)) {
		t.Fatal("the merc did not come back")
	}
	m := r2.w.merc1(r2.p.ServerID)
	r2.run(0, 600*time.Millisecond) // the group upkeep, once the kept groups are formed again
	if m.npc.X != place[0] || m.npc.Z != place[2] || m.stance != MercAggressive || m.npc.HP != m.npc.MaxHP/2 {
		t.Errorf("back at (%.1f, %.1f) stance %d HP %d/%d, want (%.1f, %.1f), aggressive, half", m.npc.X, m.npc.Z, m.stance,
			m.npc.HP, m.npc.MaxHP, place[0], place[2])
	}
	g := r2.w.groups[r2.p.GroupID]
	if g == nil || len(g.Members) != 2 {
		t.Errorf("the owner and its merc are not a group again: %+v", g)
	}
	if got := r2.sent(); got[protocol.OpcodeGroupInviteReceived] != 0 {
		t.Error("a carried merc's owner was asked again")
	}
}

// A player's group spell reaches the group's merc (castTargetsLocked's group).
func TestGroupSpellsReachTheMerc(t *testing.T) {
	r := newMercRig(t)
	ms := r.hireAsked(r.kit(0, 0, 0), MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	spell := bookSpell(700189, "Group Daring", 2, 0, 0, 0, "Group, Increase AC by 200, Increase Maximum HP by 200, Radius 30, 30min")
	r.w.mu.Lock()
	ids, why := r.w.castTargetsLocked(r.sess, r.p, &spell, 0)
	r.w.mu.Unlock()
	found := false
	for _, id := range ids {
		found = found || id == ms.npc.ServerID
	}
	if why != "" || !found {
		t.Errorf("a group buff's targets %v (%q): no merc", ids, why)
	}
}

// A merc and a summoned pet are out together: the pet keeps the pet's slot
// (its 0x007E, w.pets), the merc is no pet of its owner's, and both are
// pinned in its owner's view.
func TestMercBesideTheClassPet(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.sent()
	kit := MercKit{Class: 9, Race: 0, Level: r.p.Level, Name: "Ralthas", Base: SimBaseStats(9, 0, r.p.Level), Look: MercLook{Robe: -1}}
	if _, why := r.w.HireMerc(r.sess, kit, MercAssist, r.at(10*time.Second)); why != "" {
		t.Fatalf("the hire was refused with a pet out: %s", why)
	}
	ms := r.w.merc1(r.p.ServerID)
	if ms == nil || r.w.petOfLocked(r.p) != pet || r.w.pets[r.p.ServerID].npc == ms.npc {
		t.Fatalf("pet %v, merc %v: not both out, apart", r.w.petOfLocked(r.p) != nil, ms != nil)
	}
	for _, m := range r.sent() {
		if opOfMsg(m) == protocol.OpcodeSetPet {
			t.Error("the hire told the client a new pet")
		}
	}
	r.w.mu.Lock()
	cands := r.w.pinPetLocked(r.p, nil)
	r.w.mu.Unlock()
	if len(cands) != 2 {
		t.Errorf("pinned in its owner's view: %v, want the pet and the merc", cands)
	}
}
