package entity

import (
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Mythic Lord Nagafen (work board 2429; mythicnagafen.go): the request's "Before publishing" checks the server
// can run alone.

// mythicRig is a world with the dungeons on (world 14, none of its own) and Mythic Lord Nagafen's island served in
// world 18, entered only at the Heroic Raids board: the boss, the two valves, the guard's and the pool's rows and
// the way out (a warden standing in every instance) as the data tool writes them, and n players by the board.
type mythicRig struct {
	w   *World
	ps  []*Player
	ss  []*session.Session
	now time.Time
}

const (
	mythicTestBoss  EntityID = 930001
	mythicTestEast  EntityID = 930002
	mythicTestWest  EntityID = 930003
	mythicTestGuard EntityID = 930004
	mythicTestPool  EntityID = 930005
	mythicTestExit  EntityID = 930006
)

func mythicTestDef() DungeonDef {
	return DungeonDef{Key: "mythic_nagafen", Name: "Mythic Lord Nagafen", World: 18, Via: "Heroic Raids",
		Encounter: EncounterMythicNagafen, Spawned: []string{MythicGuardName, MythicPoolName},
		Boxes:    []DungeonBox{{X0: 4770, Z0: 4310, X1: 4910, Z1: 4445}},
		Entrance: HomeSpot{X: 4867.64, Y: -28.79, Z: 4423.88, Facing: 3.58}}
}

func newMythic(t *testing.T, n int) *mythicRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	r := &mythicRig{w: w, now: time.Unix(1_800_000_000, 0)}
	w.mu.Lock()
	for _, b := range []*NPC{
		{ServerID: mythicTestBoss, Name: MythicBossName, X: 4841.72, Y: -28.25, Z: 4369.27, World: 18, Level: 65, HP: MythicBossHP, MaxHP: MythicBossHP},
		{ServerID: mythicTestEast, Name: MythicEastValveName, X: 4884, Y: -28.72, Z: 4398, World: 18, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: mythicTestWest, Name: MythicWestValveName, X: 4798, Y: -29.09, Z: 4398, World: 18, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: mythicTestGuard, Name: MythicGuardName, X: 4841, Y: -28.25, Z: 4395, World: 18, Level: 65, HP: 16770, MaxHP: 16770},
		{ServerID: mythicTestPool, Name: MythicPoolName, X: 4790, Y: -28.78, Z: 4425, World: 18, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: mythicTestExit, Name: DungeonWardenName, X: 4852, Y: -29.08, Z: 4430, World: 18, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1},
	} {
		b.SpawnX, b.SpawnY, b.SpawnZ = b.X, b.Y, b.Z
		w.putNPCLocked(b)
		w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
	}
	w.mu.Unlock()
	w.SetDungeons(true, 14, []DungeonDef{mythicTestDef()}, DungeonSettings{CloseAfter: 10 * time.Minute})
	t.Cleanup(func() {
		for _, wd := range []uint8{14, 18} {
			SetInstancedWorld(wd, false)
			dungeonWorlds[wd].Store(false)
		}
	})
	old := mythicIntn
	mythicIntn = func(n int) int { return 0 }
	t.Cleanup(func() { mythicIntn = old })
	for i := 0; i < n; i++ {
		p := &Player{ServerID: EntityID(640000001 + i), Name: fmt.Sprintf("Mythic%d", i), Level: 60, HP: 8000, MaxHP: 8000,
			X: 5520 + float32(i), Y: 132.2, Z: 4016, World: 0, CharServerID: uint32(640000001 + i)}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 4, 0, byte(i+1)), Port: 7400 + i}, uint32(400+i), uint32(400+i), 1, 2, true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(uint32(0x18000000 + i))
		s.SetServerWorlds([]uint8{18})
		r.ps = append(r.ps, p)
		r.ss = append(r.ss, s)
	}
	return r
}

// land moves every due traveller to its spot.
func (r *mythicRig) land() {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	due := r.w.gates.due
	r.w.gates.due = nil
	for _, t := range due {
		r.w.SpatialIndex.Remove(t.P.ServerID, t.P.X, t.P.Z)
		t.P.World, t.P.X, t.P.Y, t.P.Z = t.To.World, t.To.X, t.To.Y, t.To.Z
		r.w.SpatialIndex.Insert(t.P.ServerID, t.P.X, t.P.Z)
	}
}

func (r *mythicRig) group(ps ...*Player) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	g := r.w.formGroupLocked(ps[0])
	for _, p := range ps[1:] {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
}

// enter sends player i in at the board (its party's instance), landed; it reports the instance.
func (r *mythicRig) enter(t *testing.T, i int) uint32 {
	t.Helper()
	if why, ok := r.w.DungeonEnterVia(r.ss[i], "mythic_nagafen", "Heroic Raids", r.now); !ok {
		t.Fatalf("%s not let in: %s", r.ps[i].Name, why)
	}
	r.land()
	if r.ps[i].World != 18 || r.ps[i].Instance == 0 {
		t.Fatalf("%s in world %d instance %d", r.ps[i].Name, r.ps[i].World, r.ps[i].Instance)
	}
	return r.ps[i].Instance
}

// boss is the instance's copy of the boss.
func (r *mythicRig) boss(t *testing.T, inst uint32) *NPC {
	t.Helper()
	st, ok := r.w.MythicStateOf(inst)
	if !ok || st.Boss == 0 {
		t.Fatalf("instance %d has no fight: %+v", inst, st)
	}
	return r.w.NPCs[st.Boss]
}

// step runs the fight's ticks over d, 100 ms at a time.
func (r *mythicRig) step(d time.Duration) {
	for end := r.now.Add(d); r.now.Before(end); {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.mu.Lock()
		r.w.encounterSystem(r.now)
		r.w.mu.Unlock()
	}
}

// pull puts the boss in a fight with p.
func (r *mythicRig) pull(b *NPC, p *Player) {
	r.w.mu.Lock()
	b.Threats = map[EntityID]int64{p.ServerID: 100}
	b.CurrentEnemy = p.ServerID
	r.w.mu.Unlock()
	r.step(100 * time.Millisecond)
}

// hit lands n on the boss as every hit does (npcTakesLocked first), reporting what landed.
func (r *mythicRig) hit(b *NPC, n int) int {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	n = r.w.npcTakesLocked(b, n, r.now)
	b.HP = max(b.HP-n, 0)
	if b.HP == 0 {
		r.w.killNPC(b)
	}
	return n
}

func (r *mythicRig) state(t *testing.T, inst uint32) MythicState {
	t.Helper()
	st, ok := r.w.MythicStateOf(inst)
	if !ok {
		t.Fatalf("no fight in instance %d", inst)
	}
	return st
}

// gateDone kills the guard and turns both valves with players a and b.
func (r *mythicRig) gateDone(t *testing.T, inst uint32, a, b *Player) {
	t.Helper()
	st := r.state(t, inst)
	if st.Gates == 1 {
		r.step(time.Minute) // these fixture clears reuse the first gate's valve operators
	}
	r.killGuard(t, inst)
	r.turn(t, a, st.Valves[0])
	r.turn(t, b, st.Valves[1])
	r.step(100 * time.Millisecond)
	if got := r.state(t, inst); got.Phase != "active" {
		t.Fatalf("the gate held after the guard and the pair: %+v", got)
	}
}

func (r *mythicRig) killGuard(t *testing.T, inst uint32) {
	t.Helper()
	st := r.state(t, inst)
	r.w.mu.Lock()
	g := r.w.NPCs[st.Guard]
	if g == nil {
		r.w.mu.Unlock()
		t.Fatalf("no guard: %+v", st)
	}
	g.HP = 0
	r.w.killNPC(g)
	r.w.mu.Unlock()
}

// turn walks p beside the valve and turns it.
func (r *mythicRig) turn(t *testing.T, p *Player, valve EntityID) string {
	t.Helper()
	r.w.mu.Lock()
	v := r.w.NPCs[valve]
	p.X, p.Y, p.Z = v.X+2, v.Y, v.Z+2
	r.w.mu.Unlock()
	line, ok := r.w.MythicTurnValve(p, valve, r.now)
	if !ok {
		t.Fatalf("valve %d is no fight's", valve)
	}
	return line
}

// toGate takes the boss down to gate k's threshold with one hit far past it.
func (r *mythicRig) toGate(t *testing.T, inst uint32, b *NPC, k int) {
	t.Helper()
	want := int(int64(b.MaxHP) * int64(mythicGates[k]) / 100)
	r.hit(b, b.HP-want+500_000)
	if b.HP != want {
		t.Fatalf("gate %d: HP %d, want it held at %d", k+1, b.HP, want)
	}
	r.step(100 * time.Millisecond)
	if st := r.state(t, inst); st.Phase != "gate" || st.Guard == 0 {
		t.Fatalf("gate %d not begun: %+v", k+1, st)
	}
}

func mythicHasLine(ls []string, sub string) bool {
	for _, l := range ls {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// The island is hidden: no warden's or board's choice names it, /dungeon's status leaves it out, and an entry by its
// key from anywhere but the Heroic Raids board answers as though there were none; the board's entry lets an
// ordinary player in, onto the landing in world 18. Opening leaves the guard and the pool out.
func TestMythicHiddenAndEnteredAtItsBoard(t *testing.T) {
	r := newMythic(t, 1)
	if got := r.w.DungeonChoices(r.ss[0]); len(got) != 0 {
		t.Fatalf("a warden offers %+v", got)
	}
	for _, via := range []string{"", DungeonWardenName, DungeonRaidBoardName, "heroic raid"} {
		if why, ok := r.w.DungeonEnterVia(r.ss[0], "mythic_nagafen", via, r.now); ok || !strings.Contains(why, "There is no dungeon named") {
			t.Fatalf("entered by key from %q: %v %q", via, ok, why)
		}
	}
	if why, ok := r.w.DungeonEnter(r.ss[0], "Mythic Lord Nagafen", r.now); ok {
		t.Fatalf("entered by its name: %q", why)
	}
	if got := r.w.DungeonViaChoices(r.ss[0], "heroic raids"); len(got) != 1 || got[0].Key != "mythic_nagafen" || got[0].World != 18 || !got[0].MayOpen {
		t.Fatalf("the board's choices: %+v", got)
	}
	inst := r.enter(t, 0)
	p := r.ps[0]
	if p.X != 4867.64 || p.Z != 4423.88 {
		t.Fatalf("landed at (%.2f, %.2f)", p.X, p.Z)
	}
	if line := r.w.DungeonStatus(r.ss[0]); strings.Contains(line, "Nagafen") {
		t.Fatalf("/dungeon names it: %q", line)
	}
	if line := r.w.DungeonReset(r.ss[0], "mythic_nagafen"); !strings.Contains(line, "There is no dungeon named") {
		t.Fatalf("/dungeon reset by key: %q", line)
	}
	names := map[string]int{}
	r.w.mu.RLock()
	for _, n := range r.w.NPCs {
		if n.Temporary && n.Instance == inst {
			names[n.Name]++
		}
	}
	r.w.mu.RUnlock()
	if names[MythicBossName] != 1 || names[MythicEastValveName] != 1 || names[MythicWestValveName] != 1 ||
		names[MythicGuardName] != 0 || names[MythicPoolName] != 0 {
		t.Fatalf("the instance holds %v", names)
	}
	if b := r.boss(t, inst); b.HP != MythicBossHP || b.World != 18 {
		t.Fatalf("the boss: %d/%d world %d", b.HP, b.MaxHP, b.World)
	}
	// a Scroll of Returning kept inside is refused, and a summon into it
	if res, line := r.w.DungeonReturnRead(r.ss[0], r.now); res != ReturnRefused || line != returnLineNotHere {
		t.Fatalf("a scroll read inside: %v %q", res, line)
	}
}

// Two parties' instances are their own: the second's boss, valves and fight stand apart; a gate in one is not in the
// other, and a player of one cannot turn the other's valves.
func TestMythicTwoPartiesApart(t *testing.T) {
	r := newMythic(t, 4)
	a1, a2, b1, b2 := r.ps[0], r.ps[1], r.ps[2], r.ps[3]
	r.group(a1, a2)
	r.group(b1, b2)
	ia := r.enter(t, 0)
	r.enter(t, 1)
	ib := r.enter(t, 2)
	r.enter(t, 3)
	if ia == ib || a2.Instance != ia || b2.Instance != ib {
		t.Fatalf("instances %d/%d, members in %d/%d", ia, ib, a2.Instance, b2.Instance)
	}
	ba, bb := r.boss(t, ia), r.boss(t, ib)
	if ba == bb || ba.Instance != ia || bb.Instance != ib {
		t.Fatalf("bosses %d (inst %d) and %d (inst %d)", ba.ServerID, ba.Instance, bb.ServerID, bb.Instance)
	}
	r.pull(ba, a1)
	r.toGate(t, ia, ba, 0)
	if st := r.state(t, ib); st.Phase != "staging" || bb.HP != MythicBossHP {
		t.Fatalf("the other instance moved: %+v, HP %d", st, bb.HP)
	}
	g := r.w.NPCs[r.state(t, ia).Guard]
	if g.Instance != ia || g.World != 18 || g.MaxHP != MythicGuardHP || g.HP != MythicGuardHP {
		t.Fatalf("the guard: instance %d world %d HP %d/%d", g.Instance, g.World, g.HP, g.MaxHP)
	}
	// b1 beside A's valve, in its own instance: not A's valve to turn
	stA := r.state(t, ia)
	r.w.mu.Lock()
	v := r.w.NPCs[stA.Valves[0]]
	b1.X, b1.Z = v.X+1, v.Z+1
	r.w.mu.Unlock()
	if line, _ := r.w.MythicTurnValve(b1, stA.Valves[0], r.now); line != mythicLineValveFar {
		t.Fatalf("another party's player turned it: %q", line)
	}
	if st := r.state(t, ia); st.FirstValve != -1 {
		t.Fatalf("the valve turned: %+v", st)
	}
	// B's valves are cold: B is at no gate
	stB := r.state(t, ib)
	if line := r.turn(t, b1, stB.Valves[0]); line != mythicLineValveCold {
		t.Fatalf("B's valve at staging: %q", line)
	}
}

// The thresholds hold: no hit takes him past 70% while that gate is due, none lands while he shields, and the second
// gate holds at 35%; past both, a hit lands whole.
func TestMythicThresholdsClamp(t *testing.T) {
	r := newMythic(t, 2)
	r.group(r.ps[0], r.ps[1])
	inst := r.enter(t, 0)
	r.enter(t, 1)
	b := r.boss(t, inst)
	r.pull(b, r.ps[0])
	if st := r.state(t, inst); st.Phase != "active" {
		t.Fatalf("not pulled: %+v", st)
	}
	r.toGate(t, inst, b, 0)
	if b.HP != 1_400_110 {
		t.Fatalf("gate 1 at %d, want 1,400,110 (70%% of 2,000,158)", b.HP)
	}
	if n := r.hit(b, 50_000); n != 0 || b.HP != 1_400_110 {
		t.Fatalf("a hit landed %d through the shield (HP %d)", n, b.HP)
	}
	r.w.mu.Lock()
	holds := r.w.mythicHoldsLocked(b, r.now)
	r.w.mu.Unlock()
	if !holds {
		t.Fatal("he swings while shielded")
	}
	if b.Threats[r.ps[0].ServerID] != 100 || b.CurrentEnemy != r.ps[0].ServerID {
		t.Fatalf("his hate dropped at the gate: %v %d", b.Threats, b.CurrentEnemy)
	}
	r.gateDone(t, inst, r.ps[0], r.ps[1])
	if b.Threats[r.ps[0].ServerID] != 100 || b.CurrentEnemy != r.ps[0].ServerID {
		t.Fatalf("his hate dropped at the release: %v %d", b.Threats, b.CurrentEnemy)
	}
	r.toGate(t, inst, b, 1)
	if b.HP != 700_055 {
		t.Fatalf("gate 2 at %d, want 700,055 (35%%)", b.HP)
	}
	r.gateDone(t, inst, r.ps[0], r.ps[1])
	if st := r.state(t, inst); st.Gates != 2 {
		t.Fatalf("gates: %+v", st)
	}
	if n := r.hit(b, 600_000); n != 600_000 || b.HP != 100_055 {
		t.Fatalf("past both gates a hit landed %d (HP %d)", n, b.HP)
	}
}

// A gate opens on the guard's death and a pair of turns: by two different living players, the second within 15 s of
// the first. One player turning both is refused; the same valve twice is refused; a pair missed turns back and the
// guard keeps its wounds; a pair done holds until the guard falls.
func TestMythicGuardAndValvePair(t *testing.T) {
	r := newMythic(t, 3)
	r.group(r.ps[0], r.ps[1], r.ps[2])
	inst := r.enter(t, 0)
	r.enter(t, 1)
	r.enter(t, 2)
	a, b, c := r.ps[0], r.ps[1], r.ps[2]
	boss := r.boss(t, inst)
	r.pull(boss, a)
	r.toGate(t, inst, boss, 0)
	st := r.state(t, inst)
	east, west := st.Valves[0], st.Valves[1]
	if line := r.turn(t, a, east); line != "" {
		t.Fatalf("the first turn: %q", line)
	}
	if line := r.turn(t, b, east); !strings.Contains(line, "turned already") {
		t.Fatalf("the same valve again: %q", line)
	}
	if b.hasEffect(mythicBurnedHandsID) {
		t.Fatal("an unsuccessful turn burned the second player's hands")
	}
	if line := r.turn(t, a, west); !strings.Contains(line, "hands are still burned") {
		t.Fatalf("one player turned both: %q", line)
	}
	// the guard wounded, then the window passes
	g := r.w.NPCs[st.Guard]
	g.HP = 20_000
	r.step(15*time.Second + 200*time.Millisecond)
	if st := r.state(t, inst); st.FirstValve != -1 || st.Latched || st.Phase != "gate" {
		t.Fatalf("a missed window held: %+v", st)
	}
	if g.HP != 20_000 || g.Dead {
		t.Fatalf("the guard's wounds: %d dead=%v", g.HP, g.Dead)
	}
	if !mythicHasLine(redLines(r.ss[2]), "grinds shut") {
		t.Fatalf("the miss not told: %q", lines(r.ss[2]))
	}
	if line := r.turn(t, a, east); !strings.Contains(line, "hands are still burned") {
		t.Fatalf("the missed pair cleared its operator's lockout: %q", line)
	}
	// a dead player turns none
	c.HP = 0
	if line := r.turn(t, c, west); line != mythicLineValveDead {
		t.Fatalf("a dead player's turn: %q", line)
	}
	c.HP = 8000
	// the retry: west by b, east by c 14 s later, both alive: it holds; the gate waits for the guard
	r.turn(t, b, west)
	r.step(14 * time.Second)
	r.turn(t, c, east)
	if st := r.state(t, inst); !st.Latched || st.Phase != "gate" {
		t.Fatalf("the pair: %+v", st)
	}
	r.step(30 * time.Second)
	if st := r.state(t, inst); !st.Latched || st.Phase != "gate" {
		t.Fatalf("the pair did not hold, or the gate opened with the guard alive: %+v", st)
	}
	if line := r.turn(t, a, west); line != mythicLinePairWait {
		t.Fatalf("a turn after the pair: %q", line)
	}
	r.killGuard(t, inst)
	r.step(100 * time.Millisecond)
	if st := r.state(t, inst); st.Phase != "active" || st.Gates != 1 {
		t.Fatalf("not released: %+v", st)
	}
	// its breath comes 18 s after the release
	if st := r.state(t, inst); !st.NextBreath.Equal(r.now.Add(mythicBreathFirst)) {
		t.Fatalf("next breath %v, now %v", st.NextBreath, r.now)
	}
}

func TestMythicBurnedHands(t *testing.T) {
	r := newMythic(t, 2)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	a, b := r.ps[0], r.ps[1]
	boss := r.boss(t, inst)
	r.pull(boss, a)
	r.toGate(t, inst, boss, 0)
	valves := r.state(t, inst).Valves
	firstTurn := r.now

	// A raider can already have both client effects displays full.
	for i := 0; i < BuffWindowPlayerMost; i++ {
		r.w.addEffectLocked(a, &ActiveEffect{SpellID: 10000 + i, Name: fmt.Sprintf("Buff %d", i),
			Icon: uint32(i + 1), Until: r.now.Add(time.Hour)}, r.now)
	}
	before := SerializeBuffUpdate(a)
	if line := r.turn(t, a, valves[0]); line != "" {
		t.Fatal(line)
	}
	window := r.w.buffWindowSetLocked(a, r.now)
	last := window[len(window)-1]
	if last.Name != "Burned hands" || last.Icon != 0x31782B37 || last.Removable || last.LeftMS != 60000 {
		t.Fatalf("timed debuff in the full effects window: %+v", last)
	}
	icons, names, _ := channel43(SerializeBuffUpdate(a), EffectRecordSize)
	if len(names) != MaxEffects || names[MaxEffects-1] != "Burned hands" || icons[MaxEffects-1] != last.Icon {
		t.Fatalf("legacy effects display: icons %v, names %v", icons, names)
	}

	r.step(5 * time.Second)
	r.killGuard(t, inst)
	if line := r.turn(t, b, valves[1]); line != "" {
		t.Fatal(line)
	}
	r.toGate(t, inst, boss, 1)
	r.w.clearSpellsLocked(b) // death and resurrection must not give a second turn
	if !b.hasEffect(mythicBurnedHandsID) {
		t.Fatal("the player's second valve turn gave no debuff surviving death")
	}
	rows := savedEffects(b, r.now)
	r.w.removeEffectLocked(b, mythicBurnedHandsID)

	r.step(firstTurn.Add(time.Minute - 100*time.Millisecond).Sub(r.now))
	if n := r.w.RestoreEffects(r.ss[1], rows, r.now); n != 1 {
		t.Fatalf("restored %d effects before the second operator's deadline", n)
	}
	for _, valve := range valves {
		if line := r.turn(t, a, valve); !strings.Contains(line, "still burned") {
			t.Fatalf("turn at 59.9 seconds: %q", line)
		}
	}
	r.step(100 * time.Millisecond)
	r.w.effectSystem(r.now)
	if !slices.Equal(SerializeBuffUpdate(a), before) {
		t.Fatal("the original effects display did not return on expiry")
	}
	if line := r.turn(t, a, valves[0]); line != "" {
		t.Fatalf("turn at exactly 60 seconds: %q", line)
	}
	if line := r.turn(t, b, valves[1]); !strings.Contains(line, "5s") {
		t.Fatalf("second operator's remaining cooldown after reconnect: %q", line)
	}
	r.w.removeEffectLocked(b, mythicBurnedHandsID)
	r.step(5 * time.Second)
	if n := r.w.RestoreEffects(r.ss[1], rows, r.now); n != 0 {
		t.Fatal("reconnecting restored an expired Burned hands")
	}
	if line := r.turn(t, b, valves[1]); line != "" || !r.state(t, inst).Latched {
		t.Fatalf("second operator could not complete the pair at their own 60-second deadline: %q", line)
	}
}

// An unattended party (four players, each with mercenaries in every slot, the mercs killing the guard) stalls at the
// first gate: mercenaries turn no valve, so he shields on, however long.
func TestMythicMercsAloneStallAtTheFirstGate(t *testing.T) {
	r := newMythic(t, 4)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	for i := 1; i < 4; i++ {
		r.enter(t, i)
	}
	boss := r.boss(t, inst)
	r.w.mu.Lock()
	if r.w.mercs == nil {
		r.w.mercs = map[EntityID][]*mercState{}
	}
	id := EntityID(931000)
	for _, p := range r.ps {
		for slot := 1; slot <= MercSlots; slot++ {
			n := &NPC{ServerID: id, Name: fmt.Sprintf("merc%d", id), Level: 60, HP: 9000, MaxHP: 9000, X: p.X, Y: p.Y, Z: p.Z,
				World: 18, Instance: inst, Owner: p.ServerID}
			n.merc = &mercState{owner: p, npc: n, slot: slot, body: &Player{ServerID: id, Name: n.Name, Level: 60, HP: 9000, MaxHP: 9000}}
			r.w.putNPCLocked(n)
			r.w.mercs[p.ServerID] = append(r.w.mercs[p.ServerID], n.merc)
			id++
		}
	}
	r.w.mu.Unlock()
	r.pull(boss, r.ps[0])
	r.toGate(t, inst, boss, 0)
	r.killGuard(t, inst) // the mercs' work
	for i := 0; i < 20; i++ {
		r.step(30 * time.Second)
		r.hit(boss, 100_000)
		// the players stand alive, healed between pulses, and touch nothing
		for _, p := range r.ps {
			p.HP = p.MaxHP
		}
	}
	if st := r.state(t, inst); st.Phase != "gate" || st.Gates != 0 || boss.HP != 1_400_110 {
		t.Fatalf("an unattended party passed the first gate: %+v, HP %d", st, boss.HP)
	}
}

// Furnace Breath: its warning 18 s after the pull at the soonest; here the first heat pulse (24 s) would fall a
// second before its impact (25 s), so it waits until its impact is 3 s past the pulse (its warning at 20 s, its
// impact at 27 s). Its cone is frozen toward the tank as it begins (the tank's step aside after does not move it),
// its impact 7 s on, the next 32 s after it began; it lands at most 25% of a player's most hit points and 12% of a
// mercenary's, on those in the cone alone; his swings pause round the impact.
func TestMythicBreath(t *testing.T) {
	r := newMythic(t, 3)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	r.enter(t, 2)
	tank, out, in := r.ps[0], r.ps[1], r.ps[2]
	boss := r.boss(t, inst)
	// the tank south of him, one beside the tank (in the cone), one north (behind him)
	tank.X, tank.Y, tank.Z = boss.X, boss.Y, boss.Z+6
	in.X, in.Y, in.Z = boss.X+3, boss.Y, boss.Z+15
	out.X, out.Y, out.Z = boss.X, boss.Y, boss.Z-12
	r.w.mu.Lock()
	m := &NPC{ServerID: 932001, Name: "merc", Level: 60, HP: 9000, MaxHP: 9000, X: boss.X - 2, Y: boss.Y, Z: boss.Z + 10,
		World: 18, Instance: inst, Owner: tank.ServerID}
	m.merc = &mercState{owner: tank, npc: m, slot: 1, body: &Player{ServerID: m.ServerID, Name: "merc", HP: 9000, MaxHP: 9000}}
	r.w.putNPCLocked(m)
	if r.w.mercs == nil {
		r.w.mercs = map[EntityID][]*mercState{}
	}
	r.w.mercs[tank.ServerID] = append(r.w.mercs[tank.ServerID], m.merc)
	r.w.mu.Unlock()
	r.pull(boss, tank)
	pulled := r.now
	r.step(20*time.Second - 200*time.Millisecond)
	if st := r.state(t, inst); !st.BreathImpact.IsZero() {
		t.Fatalf("a breath before 20 s: %+v", st)
	}
	r.step(200 * time.Millisecond)
	st := r.state(t, inst)
	began := r.now
	if want := pulled.Add(27 * time.Second); st.BreathImpact.Sub(want).Abs() > 150*time.Millisecond {
		t.Fatalf("its impact at %v, want 27s", st.BreathImpact.Sub(pulled))
	}
	if !mythicHasLine(redLines(r.ss[1]), "furnace breath toward Mythic0") {
		t.Fatalf("the warning not told: %q", lines(r.ss[1]))
	}
	// the tank steps aside after the warning began: the cone stays where it was aimed
	tank.X = boss.X + 30
	r.step(mythicBreathWarn - mythicPauseBefore)
	for _, p := range []*Player{tank, in, out} { // the heat pulse at 24 s healed off
		p.HP = p.MaxHP
	}
	m.HP = m.MaxHP
	r.w.mu.Lock()
	holds := r.w.mythicHoldsLocked(boss, r.now)
	r.w.mu.Unlock()
	if !holds {
		t.Fatal("his swings go on 2 s before the impact")
	}
	r.step(mythicPauseBefore + 100*time.Millisecond)
	if in.HP == in.MaxHP || out.HP != out.MaxHP || tank.HP != tank.MaxHP {
		t.Fatalf("the breath took: in-cone %d, behind %d, the tank stepped aside %d", in.HP, out.HP, tank.HP)
	}
	if lost := in.MaxHP - in.HP; lost > in.MaxHP*25/100 {
		t.Fatalf("it took %d of %d (more than 25%%)", lost, in.MaxHP)
	}
	if lost := m.MaxHP - m.HP; lost == 0 || lost > m.MaxHP*12/100 {
		t.Fatalf("it took %d of the merc's %d (12%% at most)", lost, m.MaxHP)
	}
	r.step(mythicPauseAfter)
	r.w.mu.Lock()
	holds = r.w.mythicHoldsLocked(boss, r.now)
	r.w.mu.Unlock()
	if holds {
		t.Fatal("his swings still pause 1 s after the impact")
	}
	if st := r.state(t, inst); !st.NextBreath.Equal(began.Add(mythicBreathEvery)) {
		t.Fatalf("the next breath at %v, want 32 s after this one began", st.NextBreath.Sub(began))
	}
}

// Cinder Brand after the second gate: its warning 10 s after the release on a living player not the tank, its burst
// 6 s on where that player stands; a pool lies 12 s, burning 22% of each player's maximum HP every 2 s
// (six burns), and is put off the pads.
func TestMythicBrandAndPool(t *testing.T) {
	r := newMythic(t, 3)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	r.enter(t, 2)
	tank, carrier, other := r.ps[0], r.ps[1], r.ps[2]
	boss := r.boss(t, inst)
	r.pull(boss, tank)
	r.toGate(t, inst, boss, 0)
	if st := r.state(t, inst); !st.NextBrand.IsZero() {
		t.Fatalf("a brand before the second gate: %+v", st)
	}
	r.gateDone(t, inst, carrier, other)
	r.toGate(t, inst, boss, 1)
	r.gateDone(t, inst, carrier, other)
	released := r.now.Add(-100 * time.Millisecond)
	if st := r.state(t, inst); st.NextBrand.Sub(released.Add(mythicBrandFirst)).Abs() > 150*time.Millisecond {
		t.Fatalf("the first brand due at %v after the release", st.NextBrand.Sub(released))
	}
	// Both players are on the west lane when the pool forms.
	carrier.X, carrier.Y, carrier.Z = 4790, -28.78, 4425
	other.X, other.Y, other.Z = carrier.X, carrier.Y, carrier.Z
	r.step(mythicBrandFirst - 100*time.Millisecond)
	st := r.state(t, inst)
	for st.BrandOn == 0 && r.now.Sub(released) < mythicBrandFirst+mythicBrandWarn {
		r.step(100 * time.Millisecond) // a heat pulse too near its burst puts it off a little
		st = r.state(t, inst)
	}
	if st.BrandOn != carrier.ServerID {
		t.Fatalf("the brand on %d, want the carrier (not the tank %d)", st.BrandOn, tank.ServerID)
	}
	marked := r.now
	const personalWarning = "Nagafen has branded you, move away from others"
	for i, s := range r.ss {
		lines := redLines(s)
		if slices.Contains(lines, personalWarning) != (r.ps[i] == carrier) {
			t.Errorf("wrong personal brand warning for %s: %q", r.ps[i].Name, lines)
		}
		if slices.Contains(lines, "Mythic1 bears a Cinder Brand!") != (r.ps[i] != carrier) {
			t.Errorf("wrong raid brand warning for %s: %q", r.ps[i].Name, lines)
		}
	}
	r.step(mythicBrandWarn - 200*time.Millisecond)
	if st := r.state(t, inst); st.Pools != 0 {
		t.Fatalf("the pool before 6 s: %+v", st)
	}
	carrier.Mods.MaxHP = 2000 // 10,000 effective maximum HP for the carrier, 8,000 for the other player
	const startingHP = 6000
	carrier.HP, other.HP = startingHP, startingHP
	r.step(200 * time.Millisecond)
	st = r.state(t, inst)
	if st.Pools != 1 || st.BrandOn != 0 || r.now.Sub(marked) != mythicBrandWarn {
		t.Fatalf("no pool at the burst (%v after the mark): %+v", r.now.Sub(marked), st)
	}
	r.w.dungeons.insts[inst].enc.nextHeat = r.now.Add(time.Hour) // no heat pulse in the count below
	if p := st.PoolSpots[0]; distXZ(p[0], p[2], 4790, 4425) > 0.01 {
		t.Fatalf("the pool at %v, not where its carrier stood", p)
	}
	if lost := startingHP - carrier.HP; lost != 2200 {
		t.Errorf("carrier's first burn took %d, want 2200", lost)
	}
	if lost := startingHP - other.HP; lost != 1760 {
		t.Errorf("other player's first burn took %d, want 1760", lost)
	}
	other.X -= 10 // leave the pool after the first burn
	for tick := 2; tick <= 6; tick++ {
		carrier.HP = startingHP // heal between burns to observe the whole lifetime
		r.step(1900 * time.Millisecond)
		if carrier.HP != startingHP {
			t.Fatalf("burn %d landed early", tick)
		}
		r.step(100 * time.Millisecond)
		if lost := startingHP - carrier.HP; lost != 2200 {
			t.Fatalf("burn %d took %d, want 2200", tick, lost)
		}
	}
	r.step(2 * time.Second)
	if st := r.state(t, inst); st.Pools != 0 {
		t.Fatalf("the pool outlived 12 s: %+v", st)
	}
	if carrier.HP != startingHP-2200 || other.HP != startingHP-1760 {
		t.Fatalf("unexpected extra burn: carrier HP %d, other HP %d", carrier.HP, other.HP)
	}
}

// A pool is put off the landing, the way out and the valves.
func TestMythicPoolOffThePads(t *testing.T) {
	pads := [][3]float32{{4867.64, 4423.88, mythicLandingPad}, {4852, 4430, mythicExitPad}, {4884, 4398, mythicValvePad},
		{4798, 4398, mythicValvePad}}
	for _, at := range [][2]float32{{4867.64, 4423.88}, {4852, 4430}, {4884, 4399}, {4798, 4398}, {4860, 4427}, {4790, 4425}} {
		x, z := mythicOffPads(at[0], at[1], pads)
		for _, p := range pads {
			if d := distXZ(x, z, p[0], p[1]); d < p[2]+mythicPoolRadius {
				t.Fatalf("a pool burst at %v lies at (%.1f, %.1f), %.1f from the pad %v", at, x, z, d, p)
			}
		}
	}
	if x, z := mythicOffPads(4790, 4425, pads); x != 4790 || z != 4425 {
		t.Fatalf("a pool on the west lane was moved to (%.1f, %.1f)", x, z)
	}
}

// Impacts at least 10 s apart, heat pulses at least 3 s from them: after the second gate the brand's first impact
// (release + 16 s) and the breath's (+25) would be 9 s apart, so the breath waits a second (its impact at +26), and
// every heat pulse in a long fight keeps 3 s from every impact.
func TestMythicImpactsApart(t *testing.T) {
	r := newMythic(t, 3)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	r.enter(t, 2)
	boss := r.boss(t, inst)
	r.pull(boss, r.ps[0])
	r.toGate(t, inst, boss, 0)
	r.gateDone(t, inst, r.ps[1], r.ps[2])
	r.toGate(t, inst, boss, 1)
	r.gateDone(t, inst, r.ps[1], r.ps[2])
	var impacts, heats []time.Time
	lastBreath, lastBrand := time.Time{}, time.Time{}
	lastHeat := r.state(t, inst).NextHeat
	for i := 0; i < 3000; i++ { // five minutes
		for _, p := range r.ps {
			p.HP = p.MaxHP
		}
		r.step(100 * time.Millisecond)
		st := r.state(t, inst)
		if !st.BreathImpact.IsZero() && !st.BreathImpact.Equal(lastBreath) {
			lastBreath = st.BreathImpact
			impacts = append(impacts, lastBreath)
		}
		if !st.BrandImpact.IsZero() && !st.BrandImpact.Equal(lastBrand) {
			lastBrand = st.BrandImpact
			impacts = append(impacts, lastBrand)
		}
		if !st.NextHeat.Equal(lastHeat) && st.NextHeat.Sub(r.now) == mythicHeatEvery {
			heats = append(heats, r.now)
		}
		lastHeat = st.NextHeat
	}
	if len(impacts) < 15 || len(heats) < 10 {
		t.Fatalf("%d impacts and %d heat pulses in five minutes", len(impacts), len(heats))
	}
	for i, a := range impacts {
		for _, b := range impacts[i+1:] {
			if d := a.Sub(b).Abs(); d < mythicImpactGap {
				t.Fatalf("two impacts %v apart", d)
			}
		}
		for _, h := range heats {
			if d := a.Sub(h).Abs(); d < mythicHeatGap {
				t.Fatalf("a heat pulse %v from an impact", d)
			}
		}
	}
}

// Drive the encounter clock, guard deaths and actual valve interactions. Both target categories keep the same
// 24-second pulse schedule while the two gates freeze and restore different ordinary amounts.
func TestMythicRisingHeat(t *testing.T) {
	r := newMythic(t, 3)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	r.enter(t, 2)
	boss := r.boss(t, inst)
	tank, player := r.ps[0], r.ps[1]
	merc := &NPC{ServerID: 931000, Name: "healer", Owner: player.ServerID, World: player.World, Instance: inst,
		X: player.X, Y: player.Y, Z: player.Z, Level: 60, HP: 20000, MaxHP: 20000}
	merc.merc = &mercState{owner: player, npc: merc, slot: 1, body: &Player{HP: merc.HP, MaxHP: merc.MaxHP}}
	r.w.putNPCLocked(merc)
	r.w.mercs = map[EntityID][]*mercState{player.ServerID: {merc.merc}}
	r.pull(boss, tank)

	due := r.now.Add(mythicHeatEvery)
	pulse := func(wantPlayer, wantMerc int) {
		t.Helper()
		if got := r.state(t, inst).NextHeat; !got.Equal(due) {
			t.Fatalf("next heat at %v, want %v", got, due)
		}
		r.step(due.Sub(r.now) - 100*time.Millisecond)
		playerHP, mercHP := player.HP, merc.HP
		r.step(100 * time.Millisecond)
		if gotPlayer, gotMerc := playerHP-player.HP, mercHP-merc.HP; gotPlayer != wantPlayer || gotMerc != wantMerc {
			t.Fatalf("heat at %v took %d / %d, want %d / %d", due, gotPlayer, gotMerc, wantPlayer, wantMerc)
		}
		for _, p := range r.ps {
			p.HP = p.MaxHP
		}
		merc.HP = merc.MaxHP
		due = due.Add(mythicHeatEvery)
	}

	pulse(360, 150)
	pulse(510, 300)
	r.toGate(t, inst, boss, 0)
	valves := r.state(t, inst).Valves
	pulse(1020, 600)
	r.killGuard(t, inst)
	r.turn(t, tank, valves[0])
	pulse(1020, 600)              // killing the guard and letting one valve expire leaves overdrive active
	r.turn(t, r.ps[2], valves[0]) // the tank's hands are still burned from the missed pair
	r.turn(t, player, valves[1])
	pulse(510, 300) // restore the amount before the giant appeared
	pulse(660, 450)

	r.toGate(t, inst, boss, 1)
	pulse(1320, 900)
	r.turn(t, tank, valves[1])
	r.turn(t, player, valves[0])
	if s := r.state(t, inst); s.Phase != "gate" || !s.Latched {
		t.Fatalf("the valves should be open while the living guard holds the shield: %+v", s)
	}
	pulse(660, 450)
	pulse(810, 600)
	r.killGuard(t, inst)
	pulse(960, 750) // shield release does not cool the same gate a second time
	pulse(1110, 900)
}

// A wipe puts the room back to staging: no living player on the island for 3 s, and the boss is home at full
// health, his gates closed again, the guard gone; the next pull starts over.
func TestMythicWipeResets(t *testing.T) {
	r := newMythic(t, 2)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	boss := r.boss(t, inst)
	r.pull(boss, r.ps[0])
	r.step(2 * mythicHeatEvery) // the first pull has already increased its heat
	r.toGate(t, inst, boss, 0)
	guard := r.state(t, inst).Guard
	boss.X += 10
	for _, p := range r.ps {
		p.HP = 0
	}
	r.step(mythicWipeGrace + 200*time.Millisecond)
	st := r.state(t, inst)
	if st.Phase != "staging" || st.Gates != 0 || st.Guard != 0 || boss.HP != boss.MaxHP || boss.X != boss.SpawnX ||
		boss.CurrentEnemy != 0 || len(boss.Threats) != 0 {
		t.Fatalf("not staged again: %+v, boss HP %d at %.1f (spawn %.1f), enemy %d", st, boss.HP, boss.X, boss.SpawnX, boss.CurrentEnemy)
	}
	if r.w.NPCs[guard] != nil {
		t.Fatal("the guard stayed")
	}
	for _, p := range r.ps {
		p.HP = p.MaxHP
	}
	r.pull(boss, r.ps[0])
	if st := r.state(t, inst); st.Phase != "active" {
		t.Fatalf("the next pull: %+v", st)
	}
	hp := r.ps[1].HP
	r.step(mythicHeatEvery)
	if got := hp - r.ps[1].HP; got != 360 {
		t.Fatalf("first heat after wipe took %d, want 360", got)
	}
}

// His death completes the instance once: the line once, the fight done, and nothing more of it.
func TestMythicCompletesOnce(t *testing.T) {
	r := newMythic(t, 2)
	r.group(r.ps...)
	inst := r.enter(t, 0)
	r.enter(t, 1)
	boss := r.boss(t, inst)
	r.pull(boss, r.ps[0])
	r.toGate(t, inst, boss, 0)
	r.gateDone(t, inst, r.ps[0], r.ps[1])
	r.toGate(t, inst, boss, 1)
	r.gateDone(t, inst, r.ps[0], r.ps[1])
	r.hit(boss, boss.HP)
	if !boss.Dead {
		t.Fatalf("the last hit did not kill him: %d", boss.HP)
	}
	r.step(time.Minute)
	st := r.state(t, inst)
	if !st.Completed || st.Phase != "done" {
		t.Fatalf("not complete: %+v", st)
	}
	n := 0
	for _, l := range redLines(r.ss[1]) {
		if strings.Contains(l, "This instance is complete") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the completion told %d times", n)
	}
	// and the other party's instance knows nothing of it
	r2 := r.state(t, inst)
	if r2.Gates != 2 {
		t.Fatalf("%+v", r2)
	}
}

// A restart that keeps the sessions keeps the instance: a completed fight stays completed, and a pull under way comes
// back at staging, the boss at full health, the guard not carried.
func TestMythicKeepRestart(t *testing.T) {
	for _, done := range []bool{false, true} {
		r := newMythic(t, 2)
		r.group(r.ps...)
		inst := r.enter(t, 0)
		r.enter(t, 1)
		boss := r.boss(t, inst)
		r.pull(boss, r.ps[0])
		r.toGate(t, inst, boss, 0)
		if done {
			r.gateDone(t, inst, r.ps[0], r.ps[1])
			r.toGate(t, inst, boss, 1)
			r.gateDone(t, inst, r.ps[0], r.ps[1])
			r.hit(boss, boss.HP)
			r.step(time.Second)
		}
		k := r.w.KeepDungeons(r.now)
		if k == nil || len(k.Insts) != 1 || k.Insts[0].Done != done {
			t.Fatalf("kept: %+v", k)
		}
		for _, n := range k.Insts[0].NPCs {
			if n.Name == MythicGuardName || n.Name == MythicPoolName {
				t.Fatalf("the encounter's spawn %q was kept", n.Name)
			}
		}
		// the new process: the same rows, the dungeons, then the instances
		r2 := newMythic(t, 0)
		r2.w.RestoreDungeons(k)
		st, ok := r2.w.MythicStateOf(inst)
		if !ok {
			t.Fatal("no fight made again")
		}
		if done {
			if !st.Completed {
				t.Fatalf("the completion was lost: %+v", st)
			}
			continue
		}
		b2 := r2.w.NPCs[st.Boss]
		if st.Phase != "staging" || st.Completed || b2 == nil || b2.HP != b2.MaxHP || st.Guard != 0 {
			t.Fatalf("made again: %+v boss %+v", st, b2)
		}
		for _, n := range r2.w.NPCs {
			if n.Name == MythicGuardName && n.Temporary {
				t.Fatal("a guard came back")
			}
		}
	}
}
