package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// mythicnagafen.go — Mythic Lord Nagafen, an instanced raid on his island (work board 2429, a GM's request
// approved as submitted: "publish Mythic Nagafen's Island through the northwest Heroic Raids NPC").
//
// The island is Lavastorm's cell (2, 2) served as a dungeon of its own world (DungeonDef.World;
// tools/worlds/mythic_nagafen.json), entered only at the Heroic Raids NPC by Permafrost (DungeonDef.Via:
// handlers/heroicraids.go). Each party's instance holds its own copy of the boss, the East and West Valves and the
// way out; the fight below is the instance's alone (dungeonInst.enc): its actors stand in the instance, its damage
// lands on the instance's players and mercenaries, and its completion is the instance's, once.
//
// One set of numbers for every roster (the request's):
//
//   - Lord Nagafen, level 65, 2,000,158 hit points (his template's hp_pct 11927 on a level 65's 16,770); his melee
//     a fixed 570 a hit, four hits a six-second round (a level 65 warrior's Attack4): 380 a second on his tank, 2,280
//     a round, under the 3,000 the request allows (melee_fixed, the data tool's).
//   - Furnace Breath: its first warning 18 s after the pull (or a gate's release), then every 32 s. The warning is
//     seven seconds; the cone (100 degrees, mythicBreathReach deep) is frozen toward his tank of the moment as it
//     begins. His swings pause from 2 s before its impact to 1 s after. Its fire lands as a fire spell's on a player
//     (its fire resist taking its share of mythicBreathFire) and at most 25% of a player's most hit points, 12% of a
//     mercenary's.
//   - Furnace Valves at 70% and 35%: his hit points held at each threshold due (npcTakesLocked); there he shields
//     (no damage lands), stops his swings and drops a breath or brand under way, and one Fire Giant Forgeguard (60,000
//     hit points, its template's 200 a second) comes from the forge. The gate opens when the guard is dead and both
//     valves have turned, in either order, by two different living players, the second within 15 s of the first. A
//     missed window turns the first back (the guard keeps its wounds); a pair done stays done for that gate. His
//     hate list stands through the gate.
//   - Cinder Brand after the second gate: its first warning 10 s after the release, then every 28 s. A living
//     player not his tank is marked and has six seconds to carry it off; where it stands then a pool of fire five
//     across lies 12 s, taking 5% of the most hit points every 2 s of each player in it (the first at once). A pool
//     is put off the landing, the valves and the way out.
//   - Avoidable impacts (the breath's, the brand's) at least 10 s apart, a Rising Heat pulse at least 3 s from
//     either: a warning that would break that waits until it does not, and its next comes from when it began.
//   - Rising Heat every 24 s from the pull, gates too: 360 on each player, 150 on each mercenary; from 17:00 into the
//     fight 180 and 75 more each pulse, to 1,080 and 450.
//   - A wipe (no living player of the instance on the island for mythicWipeGrace, or the boss out of every fight for
//     mythicIdleGrace outside a gate) puts the room back to staging: the boss at his spawn at full health, his gates
//     closed again, the guard and the pools gone. His death completes the instance, once.
//
// A restart that keeps the sessions (keepdungeon.go) keeps an instance's completion; a pull under way is put back to
// staging as a wipe would (its clocks, the guard and the pools are not written down), the boss at full health.

// EncounterMythicNagafen is the dungeon recipe's "encounter" for this fight.
const EncounterMythicNagafen = "mythic_nagafen"

// The fight's actors, by name (the data tool's rows of its world; the guard and the pool are its DungeonDef.Spawned).
const (
	MythicBossName      = "Mythic Lord Nagafen"
	MythicGuardName     = "Fire Giant Forgeguard"
	MythicPoolName      = "cinder pool"
	MythicEastValveName = "East Valve"
	MythicWestValveName = "West Valve"
)

// The fight's numbers (the file's doc).
const (
	MythicBossHP  = 2_000_158
	MythicGuardHP = 60_000

	mythicBreathFirst  = 18 * time.Second
	mythicBreathEvery  = 32 * time.Second
	mythicBreathWarn   = 7 * time.Second
	mythicBreathHalf   = 50 // degrees: a 100-degree cone
	mythicBreathFire   = 5000
	mythicBreathPlayer = 25 // % of most hit points at most
	mythicBreathMerc   = 12
	mythicPauseBefore  = 2 * time.Second
	mythicPauseAfter   = 1 * time.Second

	mythicValveWindow = 15 * time.Second

	mythicBrandFirst = 10 * time.Second
	mythicBrandEvery = 28 * time.Second
	mythicBrandWarn  = 6 * time.Second
	mythicPoolFor    = 12 * time.Second
	mythicPoolEvery  = 2 * time.Second
	mythicPoolPct    = 5

	mythicImpactGap = 10 * time.Second
	mythicHeatGap   = 3 * time.Second

	mythicHeatEvery      = 24 * time.Second
	mythicHeatPlayer     = 360
	mythicHeatMerc       = 150
	mythicHeatRampAt     = 17 * time.Minute
	mythicHeatRampPlayer = 180
	mythicHeatRampMerc   = 75
	mythicHeatMostPlayer = 1080
	mythicHeatMostMerc   = 450

	mythicWipeGrace = 3 * time.Second
	mythicIdleGrace = 6 * time.Second
)

// The fight's places' sizes (world units; the layout's choices, claude-notes/mythic-nagafen-20261127.md).
const (
	mythicBreathReach float32 = 35 // the cone's depth from his middle
	mythicPoolRadius  float32 = 5
	mythicPoolHeight  float32 = 4  // how far over or under a pool's floor a player stands in it
	mythicValveReach  float32 = 10 // how near a valve a player turns it
	mythicLandingPad  float32 = 6
	mythicExitPad     float32 = 5
	mythicValvePad    float32 = 5
)

// mythicGates are the gates' thresholds, in percent of his most hit points.
var mythicGates = [2]int{70, 35}

// mythicIntn is the brand's pick (a test's own).
var mythicIntn = rand.Intn

// The fight's FX: his breath is his own spell's (Nagafen's Sweep, E78024DB), the brand Flame Arrow's, a pool's
// Fire Storm's.
const (
	mythicBreathFX uint32 = 0xE78024DB
	mythicBrandFX  uint32 = 0x2425AAFC
	mythicPoolFX   uint32 = 0x1AF4AF0D
)

// The fight's lines (ours).
const (
	mythicLinePull      = "Lord Nagafen wakes in his fury, and the island's heat begins to rise!"
	mythicLineBreath    = "Lord Nagafen draws a furnace breath toward %s! Get out of its path!"
	mythicLineBreathHit = "Furnace Breath burns you for %d."
	mythicLineDropped   = "The forge's pull smothers the fire he was gathering."
	mythicLineGate      = "Lord Nagafen draws on the forge and its fire shields him! A Fire Giant Forgeguard comes: slay it, and turn the East and West Valves, two of you, within 15 seconds of each other."
	mythicLineTurned    = "%s turns the %s! The %s must turn within 15 seconds, by another hand."
	mythicLinePair      = "%s turns the %s: both valves stand open."
	mythicLinePairWait  = "Both valves stand open; the shield fails when the Forgeguard falls."
	mythicLineMissed    = "The %s grinds shut: both valves must turn within 15 seconds of each other. Try again."
	mythicLineRelease   = "The forge's shield breaks, and Lord Nagafen fights on!"
	mythicLineBrandYou  = "You bear a Cinder Brand! Carry it to a dry lane away from the others: it bursts in 6 seconds."
	mythicLineBrand     = "%s bears a Cinder Brand!"
	mythicLinePool      = "The Cinder Brand bursts into a pool of fire."
	mythicLinePoolHit   = "The cinder pool burns you for %d."
	mythicLineHeat      = "The island's Rising Heat burns you for %d."
	mythicLineWipe      = "The island falls quiet, and Lord Nagafen returns to his rest."
	mythicLineDone      = "Mythic Lord Nagafen falls! This instance is complete."
	mythicLineValveCold = "The valve is fixed fast; it turns only while Lord Nagafen draws on the forge."
	mythicLineValveSame = "The %s is turned already; the %s must turn."
	mythicLineValveHand = "One pair of hands cannot turn both valves: another must turn the %s."
	mythicLineValveFar  = "You are too far from the valve to turn it."
	mythicLineValveDead = "The dead turn no valves."
)

var (
	mythicInfoRGB = [3]byte{230, 190, 120}
	mythicWarnRGB = [3]byte{255, 140, 40}
	mythicHurtRGB = [3]byte{255, 63, 63}
)

type mythicPhase int

const (
	mythicStaging mythicPhase = iota
	mythicActive
	mythicGate
	mythicDone
)

func (p mythicPhase) String() string {
	return [...]string{"staging", "active", "gate", "done"}[p]
}

// mythicPool is a cinder pool lying.
type mythicPool struct {
	npc       EntityID // its body (0: none, no template row)
	x, y, z   float32
	until     time.Time
	nextBurn  time.Time
	landedAtT time.Time
}

// mythicFight is one instance's fight.
type mythicFight struct {
	inst  uint32
	world uint8
	boss  EntityID
	// valves are the East and West Valves' copies, exit the way out (its warden), landing the arrival
	valves  [2]EntityID
	exitX   float32
	exitZ   float32
	hasExit bool
	landing HomeSpot

	phase  mythicPhase
	pulled time.Time
	gates  int // gates released

	guard EntityID
	// the valves' pair this gate: the first turned (-1 none), by whom and when; latched once both turned
	first   int
	firstBy uint32
	firstAt time.Time
	latched bool

	nextBreath   time.Time
	breathImpact time.Time // zero: no breath under way
	breathCone   areaCone
	pauseFrom    time.Time // the swings' pause round an impact
	pauseUntil   time.Time

	nextBrand   time.Time // zero: none due (before the second gate)
	brandImpact time.Time
	brandOn     EntityID // the marked player
	brandX      float32
	brandY      float32
	brandZ      float32

	nextHeat time.Time
	ramp     int // pulses past mythicHeatRampAt
	// lastImpact is the last avoidable impact landed (a breath's or a brand's burst)
	lastImpact time.Time

	pools []mythicPool

	noneSince time.Time // since when no living player of the instance stands on the island
	idleSince time.Time // since when the boss is out of every fight
	completed bool
}

// MythicState is a fight's state for tests and the log.
type MythicState struct {
	Phase                 string
	Gates                 int
	Guard                 EntityID
	Latched               bool
	FirstValve            int
	BreathImpact          time.Time
	NextBreath, NextBrand time.Time
	BrandImpact           time.Time
	BrandOn               EntityID
	NextHeat              time.Time
	Pools                 int
	PoolSpots             [][3]float32
	Completed             bool
	Boss                  EntityID
	Valves                [2]EntityID
}

// encounterOpenLocked gives a new instance its dungeon's fight. Must be called with the lock held.
func (w *World) encounterOpenLocked(in *dungeonInst, now time.Time) {
	d := &w.dungeons.defs[in.def]
	if !strings.EqualFold(d.Encounter, EncounterMythicNagafen) {
		return
	}
	f := &mythicFight{inst: in.id, world: d.World, landing: d.Entrance, first: -1}
	for _, id := range in.npcs {
		c := w.NPCs[id]
		if c == nil {
			continue
		}
		switch {
		case strings.EqualFold(c.Name, MythicBossName):
			f.boss = c.ServerID
			c.mythic = f
		case strings.EqualFold(c.Name, MythicEastValveName):
			f.valves[0] = c.ServerID
		case strings.EqualFold(c.Name, MythicWestValveName):
			f.valves[1] = c.ServerID
		}
	}
	// the way out: the warden standing in every instance, the nearest the landing
	best := float32(-1)
	for _, n := range w.NPCs {
		if n.Temporary || n.World != d.World || !IsDungeonWarden(n.Name) {
			continue
		}
		if dd := distXZ(n.X, n.Z, d.Entrance.X, d.Entrance.Z); best < 0 || dd < best {
			best, f.exitX, f.exitZ, f.hasExit = dd, n.X, n.Z, true
		}
	}
	in.enc = f
	if f.boss == 0 {
		log.Printf("Mythic: instance %d of %s has no %q among its creatures: no fight", in.id, d.Name, MythicBossName)
	}
	log.Printf("Mythic: instance %d's fight set (boss %d, valves %v, exit %v)", in.id, f.boss, f.valves, f.hasExit)
}

// encounterCloseLocked takes an instance's fight's own spawns out of the world as it closes; it reports how many.
func (w *World) encounterCloseLocked(in *dungeonInst) int {
	if in.enc == nil {
		return 0
	}
	return w.mythicClearSpawnsLocked(in.enc)
}

// mythicClearSpawnsLocked takes the guard and the pools out of the world. Must be called with the lock held.
func (w *World) mythicClearSpawnsLocked(f *mythicFight) int {
	n := 0
	if g := w.NPCs[f.guard]; g != nil && f.guard != 0 && g.Temporary && g.Instance == f.inst {
		w.removeNPCLocked(g)
		n++
	}
	f.guard = 0
	for _, p := range f.pools {
		if c := w.NPCs[p.npc]; c != nil && p.npc != 0 && c.Temporary && c.Instance == f.inst {
			w.removeNPCLocked(c)
			n++
		}
	}
	f.pools = nil
	return n
}

// encounterSystem runs each open instance's fight. Every tick; must be called with the lock held.
func (w *World) encounterSystem(now time.Time) {
	if !w.dungeons.on || len(w.dungeons.insts) == 0 {
		return
	}
	for _, in := range w.dungeons.insts {
		if in.enc != nil {
			w.mythicStepLocked(in, in.enc, now)
		}
	}
}

// mythicPlayersLocked is the living players of the fight's instance on its island, by id.
func (w *World) mythicPlayersLocked(f *mythicFight) []*Player {
	var ps []*Player
	for _, p := range w.Players {
		if p.World == f.world && p.Instance == f.inst && !p.IsDead() && !p.respawnPending() {
			ps = append(ps, p)
		}
	}
	sortPlayersByID(ps)
	return ps
}

// mythicMercsLocked is the living mercenaries in the fight's instance on its island, by id.
func (w *World) mythicMercsLocked(f *mythicFight) []*NPC {
	var ns []*NPC
	for _, ms := range w.allMercsLocked() {
		if n := ms.npc; n != nil && !n.Dead && n.HP > 0 && n.World == f.world && n.Instance == f.inst {
			ns = append(ns, n)
		}
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].ServerID < ns[j].ServerID })
	return ns
}

// mythicTellLocked tells every player of the instance on its island a line.
func (w *World) mythicTellLocked(f *mythicFight, line string, rgb [3]byte) {
	for _, p := range w.Players {
		if p.World != f.world || p.Instance != f.inst {
			continue
		}
		if s := w.findSessionByEntityIDLocked(p.ServerID); s != nil {
			w.sendColoredLocked(s, line, rgb)
		}
	}
}

// mythicStepLocked is a tick of the fight. Must be called with the lock held.
func (w *World) mythicStepLocked(in *dungeonInst, f *mythicFight, now time.Time) {
	boss := w.NPCs[f.boss]
	if boss == nil || f.completed {
		return
	}
	if boss.Dead {
		w.mythicCompleteLocked(in, f, now)
		return
	}
	engaged := boss.CurrentEnemy != 0 || len(boss.Threats) > 0
	if f.phase == mythicStaging {
		if engaged {
			w.mythicPullLocked(f, now)
		}
		return
	}
	// a wipe: no living player of the instance on the island, or the boss out of every fight
	if len(w.mythicPlayersLocked(f)) == 0 {
		if f.noneSince.IsZero() {
			f.noneSince = now
		}
		if now.Sub(f.noneSince) >= mythicWipeGrace {
			w.mythicResetLocked(f, "no one standing", now)
			return
		}
	} else {
		f.noneSince = time.Time{}
	}
	if !engaged && f.phase != mythicGate {
		if f.idleSince.IsZero() {
			f.idleSince = now
		}
		if now.Sub(f.idleSince) >= mythicIdleGrace {
			w.mythicResetLocked(f, "out of every fight", now)
			return
		}
	} else {
		f.idleSince = time.Time{}
	}
	if f.phase == mythicActive && f.gates < len(mythicGates) && boss.HP <= mythicThreshold(boss, f.gates) {
		w.mythicGateLocked(in, f, boss, now)
	}
	switch f.phase {
	case mythicGate:
		w.mythicGateStepLocked(f, now)
	case mythicActive:
		w.mythicBreathStepLocked(f, boss, now)
		w.mythicBrandStepLocked(in, f, boss, now)
	}
	w.mythicHeatStepLocked(f, now)
	w.mythicPoolsStepLocked(f, now)
}

// mythicThreshold is gate k's hit points: k's percent of his most, rounded down.
func mythicThreshold(boss *NPC, k int) int {
	return int(int64(boss.MaxHP) * int64(mythicGates[k]) / 100)
}

// mythicPullLocked begins the fight. Must be called with the lock held.
func (w *World) mythicPullLocked(f *mythicFight, now time.Time) {
	f.phase = mythicActive
	f.pulled = now
	f.nextBreath = now.Add(mythicBreathFirst)
	f.nextHeat = now.Add(mythicHeatEvery)
	f.nextBrand = time.Time{}
	f.noneSince, f.idleSince = time.Time{}, time.Time{}
	w.mythicTellLocked(f, mythicLinePull, mythicWarnRGB)
	log.Printf("Mythic: instance %d pulled", f.inst)
}

// mythicTakesLocked is what of a hit of n lands on the boss: nothing while he shields, and never past his next
// gate's hit points. Called by npcTakesLocked for an NPC with a fight; must be called with the lock held.
func (w *World) mythicTakesLocked(npc *NPC, n int, now time.Time) int {
	f := npc.mythic
	if f == nil || npc.ServerID != f.boss || f.completed || npc.Dead {
		return n
	}
	if f.phase == mythicGate {
		return 0 // shielded
	}
	if f.gates < len(mythicGates) {
		th := mythicThreshold(npc, f.gates)
		if npc.HP-n < th {
			n = max(0, npc.HP-th)
		}
	}
	return n
}

// mythicHoldsLocked reports the boss holding his swings now: shielded at a gate, or round a breath's impact.
func (w *World) mythicHoldsLocked(npc *NPC, now time.Time) bool {
	f := npc.mythic
	if f == nil || npc.ServerID != f.boss || f.completed {
		return false
	}
	if f.phase == mythicGate {
		return true
	}
	return !f.pauseFrom.IsZero() && !now.Before(f.pauseFrom) && now.Before(f.pauseUntil)
}

// mythicGateLocked begins gate f.gates: the shield, the guard, a breath or brand under way dropped.
func (w *World) mythicGateLocked(in *dungeonInst, f *mythicFight, boss *NPC, now time.Time) {
	f.phase = mythicGate
	if !f.breathImpact.IsZero() || f.brandOn != 0 {
		w.mythicTellLocked(f, mythicLineDropped, mythicInfoRGB)
	}
	f.breathImpact, f.pauseFrom, f.pauseUntil = time.Time{}, time.Time{}, time.Time{}
	f.brandImpact, f.brandOn = time.Time{}, 0
	f.first, f.firstBy, f.firstAt, f.latched = -1, 0, time.Time{}, false
	boss.HP = min(boss.HP, mythicThreshold(boss, f.gates))
	boss.stopMoving()
	boss.endRound()
	f.guard = 0
	if g := w.mythicSpawnLocked(in, f, MythicGuardName, 0, 0, 0, false); g != nil {
		g.tbase.maxHP = MythicGuardHP // a template's hp_pct scales from it (classtemplate.go)
		g.MaxHP, g.HP = MythicGuardHP, MythicGuardHP
		ids := make([]EntityID, 0, len(boss.Threats))
		for id := range boss.Threats {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if boss.Threats[id] > 0 {
				w.addThreatLocked(g, id, 1)
			}
		}
		w.retargetLocked(g)
		f.guard = g.ServerID
	}
	w.mythicTellLocked(f, mythicLineGate, mythicWarnRGB)
	log.Printf("Mythic: instance %d gate %d at %d/%d hit points (guard %d)", f.inst, f.gates+1, boss.HP, boss.MaxHP, f.guard)
}

// mythicSpawnLocked is a copy of the fight's template row of the name standing in the instance: at its row's spawn,
// or at (x, y, z) when at. Nil when the world holds no such row.
func (w *World) mythicSpawnLocked(in *dungeonInst, f *mythicFight, name string, x, y, z float32, at bool) *NPC {
	var t *NPC
	for _, n := range w.NPCs {
		if w.dungeonTemplateLocked(n) && n.World == f.world && strings.EqualFold(n.Name, name) && (t == nil || n.ServerID < t.ServerID) {
			t = n
		}
	}
	if t == nil {
		log.Printf("Mythic: no %q row in world %d", name, f.world)
		return nil
	}
	if !at {
		x, y, z = t.SpawnX, t.SpawnY, t.SpawnZ
	}
	c := w.spawnCopyLocked(t, x, y, z, t.SpawnFacing, t.World)
	c.Instance = in.id
	return c
}

// MythicTurnValve is a player's turn of a valve (handlers/heroicraids.go's talk to it); it reports the line for the
// player ("" for none: the instance was told) and whether the NPC is a fight's valve at all.
func (w *World) MythicTurnValve(p *Player, valve EntityID, now time.Time) (line string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v := w.NPCs[valve]
	if p == nil || v == nil || v.Instance == 0 {
		return "", false
	}
	in := w.dungeons.insts[v.Instance]
	if in == nil || in.enc == nil {
		return "", false
	}
	f := in.enc
	which := -1
	for i, id := range f.valves {
		if id == valve {
			which = i
		}
	}
	if which < 0 {
		return "", false
	}
	names := [2]string{MythicEastValveName, MythicWestValveName}
	switch {
	case p.IsDead() || p.respawnPending():
		return mythicLineValveDead, true
	case p.World != v.World || p.Instance != v.Instance || distXZ(p.X, p.Z, v.X, v.Z) > mythicValveReach:
		return mythicLineValveFar, true
	case f.phase != mythicGate:
		return mythicLineValveCold, true
	case f.latched:
		return mythicLinePairWait, true
	}
	if f.first >= 0 && now.Sub(f.firstAt) > mythicValveWindow {
		w.mythicMissLocked(f)
	}
	switch {
	case f.first < 0:
		f.first, f.firstBy, f.firstAt = which, p.CharServerID, now
		w.mythicTellLocked(f, fmt.Sprintf(mythicLineTurned, p.Name, names[which], names[1-which]), mythicWarnRGB)
		log.Printf("Mythic: instance %d: %s turns the %s (gate %d)", f.inst, p.Name, names[which], f.gates+1)
	case which == f.first:
		return fmt.Sprintf(mythicLineValveSame, names[which], names[1-which]), true
	case p.CharServerID == f.firstBy:
		return fmt.Sprintf(mythicLineValveHand, names[which]), true
	default:
		f.latched = true
		w.mythicTellLocked(f, fmt.Sprintf(mythicLinePair, p.Name, names[which]), mythicWarnRGB)
		log.Printf("Mythic: instance %d: %s turns the %s %.1fs after the first: the pair holds (gate %d)", f.inst, p.Name,
			names[which], now.Sub(f.firstAt).Seconds(), f.gates+1)
		w.mythicGateStepLocked(f, now)
	}
	return "", true
}

// mythicMissLocked turns a pair's first valve back: its window passed.
func (w *World) mythicMissLocked(f *mythicFight) {
	names := [2]string{MythicEastValveName, MythicWestValveName}
	w.mythicTellLocked(f, fmt.Sprintf(mythicLineMissed, names[f.first]), mythicWarnRGB)
	log.Printf("Mythic: instance %d: the %s's window passed", f.inst, names[f.first])
	f.first, f.firstBy, f.firstAt = -1, 0, time.Time{}
}

// mythicGateStepLocked is a gate's tick: a missed window, and the release once the guard is dead and the pair holds.
func (w *World) mythicGateStepLocked(f *mythicFight, now time.Time) {
	if f.phase != mythicGate {
		return
	}
	if f.first >= 0 && !f.latched && now.Sub(f.firstAt) > mythicValveWindow {
		w.mythicMissLocked(f)
	}
	g := w.NPCs[f.guard]
	guardDown := f.guard == 0 || g == nil || g.Dead
	if !guardDown || !f.latched {
		return
	}
	f.gates++
	f.phase = mythicActive
	f.first, f.latched = -1, false
	f.nextBreath = now.Add(mythicBreathFirst)
	if f.gates >= 2 {
		f.nextBrand = now.Add(mythicBrandFirst)
	}
	w.mythicTellLocked(f, mythicLineRelease, mythicWarnRGB)
	log.Printf("Mythic: instance %d gate %d released", f.inst, f.gates)
}

// mythicImpactClear is impact at, as a warning of kind self ("breath" or "brand") would land it, checked against
// the other impact under way and the last one landed (mythicImpactGap) and the next heat pulse (mythicHeatGap):
// false and the earliest impact that might be clear when it is not.
func (f *mythicFight) mythicImpactClear(at time.Time, self string) (time.Time, bool) {
	other := f.brandImpact
	if self == "brand" {
		other = f.breathImpact
	}
	for _, o := range []time.Time{other, f.lastImpact} {
		if !o.IsZero() {
			if d := at.Sub(o); d < mythicImpactGap && d > -mythicImpactGap {
				return o.Add(mythicImpactGap), false
			}
		}
	}
	if !f.nextHeat.IsZero() {
		if d := at.Sub(f.nextHeat); d < mythicHeatGap && d > -mythicHeatGap {
			return f.nextHeat.Add(mythicHeatGap), false
		}
	}
	return at, true
}

// mythicTankSpotLocked is where the boss's foe of the moment stands.
func (w *World) mythicTankSpotLocked(boss *NPC) (x, z float32, name string, ok bool) {
	if p := w.Players[boss.CurrentEnemy]; p != nil {
		return p.X, p.Z, p.Name, true
	}
	if n := w.NPCs[boss.CurrentEnemy]; n != nil {
		return n.X, n.Z, n.Name, true
	}
	return 0, 0, "", false
}

// mythicBreathStepLocked is the breath's tick: a warning due begun, an impact due landed.
func (w *World) mythicBreathStepLocked(f *mythicFight, boss *NPC, now time.Time) {
	if !f.breathImpact.IsZero() {
		if !now.Before(f.breathImpact) {
			w.mythicBreathLandLocked(f, boss, now)
		}
		return
	}
	if f.nextBreath.IsZero() || now.Before(f.nextBreath) {
		return
	}
	at, clear := f.mythicImpactClear(now.Add(mythicBreathWarn), "breath")
	if !clear {
		f.nextBreath = at.Add(-mythicBreathWarn) // waits; its next comes from when it begins
		return
	}
	tx, tz, tank, ok := w.mythicTankSpotLocked(boss)
	a := SpellArea{Centre: AreaCone, Cone: mythicBreathHalf}
	c := a.coneOf(boss.X, boss.Y, boss.Z, boss.Facing, ok, tx, tz)
	c.r = mythicBreathReach
	f.breathCone = *c
	f.breathImpact = at
	f.pauseFrom, f.pauseUntil = at.Add(-mythicPauseBefore), at.Add(mythicPauseAfter)
	f.nextBreath = now.Add(mythicBreathEvery)
	if !ok {
		tank = "the island"
	}
	w.broadcastNPCCastLocked(boss, boss.CurrentEnemy, uint32(mythicBreathWarn/time.Millisecond), mythicBreathFX)
	w.mythicTellLocked(f, fmt.Sprintf(mythicLineBreath, tank), mythicWarnRGB)
	log.Printf("Mythic: instance %d: Furnace Breath toward %s, its impact in %v (cone (%.2f, %.2f))", f.inst, tank,
		mythicBreathWarn, c.dx, c.dz)
}

// mythicBreathLandLocked is the breath's impact on everyone in its cone.
func (w *World) mythicBreathLandLocked(f *mythicFight, boss *NPC, now time.Time) {
	c := f.breathCone
	f.breathImpact = time.Time{}
	f.lastImpact = now
	hit := 0
	for _, p := range w.mythicPlayersLocked(f) {
		if in, _ := c.holds(p.X, p.Y, p.Z, 0); !in {
			continue
		}
		n := resolvePlayerElemental(playerElem{d0: mythicBreathFire, resist: p.StatNow(StatFR)}).Taken
		n = min(n, p.MaxHPNow()*mythicBreathPlayer/100)
		w.mythicHurtPlayerLocked(boss, p, n, mythicLineBreathHit, now)
		hit++
	}
	for _, m := range w.mythicMercsLocked(f) {
		if in, _ := c.holds(m.X, m.Y, m.Z, 0); !in {
			continue
		}
		w.mythicHurtMercLocked(boss, m, min(mythicBreathFire, m.MaxHP*mythicBreathMerc/100), "Furnace Breath")
		hit++
	}
	log.Printf("Mythic: instance %d: Furnace Breath lands on %d", f.inst, hit)
}

// mythicHurtPlayerLocked takes n off p, as a creature's fire: its number on the player, its line, its death.
func (w *World) mythicHurtPlayerLocked(boss *NPC, p *Player, n int, line string, now time.Time) {
	n = min(n, p.HP)
	if n <= 0 {
		return
	}
	p.HP -= n
	p.StatsDirty = true
	p.LastCombatTime = now
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess != nil {
		me := sess.PlayerObjectID()
		src := me
		if boss != nil {
			if o := w.objectIDForEntityLocked(sess, boss.ServerID); o != 0 {
				src = o
			}
		}
		w.sendDamageIndication(sess, src, -n, me, 0)
		w.sendColoredLocked(sess, fmt.Sprintf(line, n), mythicHurtRGB)
	}
	if p.HP <= 0 {
		p.setDeathCause(CauseCreature, MythicBossName) // hardcore.go
		w.playerDiedLocked(sess, p, now)
	}
}

// mythicHurtMercLocked takes n off a mercenary, its death at nothing (merc.go sees to it).
func (w *World) mythicHurtMercLocked(boss *NPC, m *NPC, n int, what string) {
	n = w.npcTakesLocked(m, n, time.Now())
	m.HP = w.dpsTakeHPLocked(m, n)
	log.Printf("Mythic: %s on the mercenary %q for %d (HP %d/%d)", what, m.Name, n, m.HP, m.MaxHP)
	if m.HP == 0 {
		w.killNPC(m)
		w.dropAggroLocked(m.ServerID)
	}
}

// mythicBrandStepLocked is the brand's tick (after the second gate): a warning due begun on a player not the tank,
// its burst due a pool where its player stands.
func (w *World) mythicBrandStepLocked(in *dungeonInst, f *mythicFight, boss *NPC, now time.Time) {
	if f.brandOn != 0 {
		if p := w.Players[f.brandOn]; p != nil && p.World == f.world && p.Instance == f.inst && !p.IsDead() {
			f.brandX, f.brandY, f.brandZ = p.X, p.Y, p.Z // where it bursts: where its player stands
		}
		if !now.Before(f.brandImpact) {
			w.mythicBrandBurstLocked(in, f, now)
		}
		return
	}
	if f.nextBrand.IsZero() || now.Before(f.nextBrand) {
		return
	}
	at, clear := f.mythicImpactClear(now.Add(mythicBrandWarn), "brand")
	if !clear {
		f.nextBrand = at.Add(-mythicBrandWarn)
		return
	}
	f.nextBrand = now.Add(mythicBrandEvery)
	var can []*Player
	for _, p := range w.mythicPlayersLocked(f) {
		if p.ServerID != boss.CurrentEnemy {
			can = append(can, p)
		}
	}
	if len(can) == 0 {
		log.Printf("Mythic: instance %d: no living player but the tank for a Cinder Brand", f.inst)
		return
	}
	p := can[mythicIntn(len(can))]
	f.brandOn, f.brandImpact = p.ServerID, at
	f.brandX, f.brandY, f.brandZ = p.X, p.Y, p.Z
	w.broadcastNPCCastLocked(boss, p.ServerID, uint32(mythicBrandWarn/time.Millisecond), mythicBrandFX)
	for _, q := range w.Players {
		if q.World != f.world || q.Instance != f.inst {
			continue
		}
		if s := w.findSessionByEntityIDLocked(q.ServerID); s != nil {
			if q == p {
				w.sendColoredLocked(s, mythicLineBrandYou, mythicWarnRGB)
			} else {
				w.sendColoredLocked(s, fmt.Sprintf(mythicLineBrand, p.Name), mythicWarnRGB)
			}
		}
	}
	log.Printf("Mythic: instance %d: Cinder Brand on %s, its burst in %v", f.inst, p.Name, mythicBrandWarn)
}

// mythicPadsLocked are the places a pool keeps off: the landing, the way out and the valves (x, z, radius).
func (w *World) mythicPadsLocked(f *mythicFight) [][3]float32 {
	pads := [][3]float32{{f.landing.X, f.landing.Z, mythicLandingPad}}
	if f.hasExit {
		pads = append(pads, [3]float32{f.exitX, f.exitZ, mythicExitPad})
	}
	for _, id := range f.valves {
		if v := w.NPCs[id]; v != nil && id != 0 {
			pads = append(pads, [3]float32{v.X, v.Z, mythicValvePad})
		}
	}
	return pads
}

// mythicOffPads is (x, z) moved off every pad: a pool whose edge would reach one is put at the nearest place (on
// rings round where it burst, a unit apart, every 15 degrees, the nearest first) whose edge reaches none.
func mythicOffPads(x, z float32, pads [][3]float32) (float32, float32) {
	clear := func(px, pz float32) bool {
		for _, p := range pads {
			if distXZ(px, pz, p[0], p[1]) < p[2]+mythicPoolRadius+0.5 {
				return false
			}
		}
		return true
	}
	if clear(x, z) {
		return x, z
	}
	for r := float32(1); r <= 60; r++ {
		for a := 0; a < 360; a += 15 {
			s, c := math.Sincos(float64(a) * math.Pi / 180)
			px, pz := x+r*float32(s), z+r*float32(c)
			if clear(px, pz) {
				return px, pz
			}
		}
	}
	return x, z
}

// mythicBrandBurstLocked is the brand's burst: a pool where its player stood, off the pads.
func (w *World) mythicBrandBurstLocked(in *dungeonInst, f *mythicFight, now time.Time) {
	x, z := mythicOffPads(f.brandX, f.brandZ, w.mythicPadsLocked(f))
	y := f.brandY
	f.brandOn, f.brandImpact = 0, time.Time{}
	f.lastImpact = now
	pool := mythicPool{x: x, y: y, z: z, until: now.Add(mythicPoolFor), nextBurn: now, landedAtT: now}
	if c := w.mythicSpawnLocked(in, f, MythicPoolName, x, y, z, true); c != nil {
		pool.npc = c.ServerID
		w.broadcastNPCCastLocked(c, c.ServerID, 0, mythicPoolFX)
	}
	f.pools = append(f.pools, pool)
	w.mythicTellLocked(f, mythicLinePool, mythicWarnRGB)
	log.Printf("Mythic: instance %d: a cinder pool at (%.1f, %.1f, %.1f) for %v", f.inst, x, y, z, mythicPoolFor)
}

// mythicPoolsStepLocked burns those in the pools on their clocks, and takes the spent ones away.
func (w *World) mythicPoolsStepLocked(f *mythicFight, now time.Time) {
	if len(f.pools) == 0 {
		return
	}
	boss := w.NPCs[f.boss]
	kept := f.pools[:0]
	for _, pl := range f.pools {
		for !pl.nextBurn.After(now) && pl.nextBurn.Before(pl.until) {
			for _, p := range w.mythicPlayersLocked(f) {
				if distXZ(p.X, p.Z, pl.x, pl.z) <= mythicPoolRadius && math.Abs(float64(p.Y-pl.y)) <= float64(mythicPoolHeight) {
					w.mythicHurtPlayerLocked(boss, p, max(1, p.MaxHPNow()*mythicPoolPct/100), mythicLinePoolHit, now)
				}
			}
			if c := w.NPCs[pl.npc]; c != nil && pl.npc != 0 && pl.nextBurn.After(pl.landedAtT) {
				w.broadcastNPCCastLocked(c, c.ServerID, 0, mythicPoolFX)
			}
			pl.nextBurn = pl.nextBurn.Add(mythicPoolEvery)
		}
		if now.Before(pl.until) {
			kept = append(kept, pl)
			continue
		}
		if c := w.NPCs[pl.npc]; c != nil && pl.npc != 0 && c.Temporary {
			w.removeNPCLocked(c)
		}
	}
	f.pools = kept
}

// mythicHeatStepLocked is the Rising Heat's pulse when due, away from an impact.
func (w *World) mythicHeatStepLocked(f *mythicFight, now time.Time) {
	if f.nextHeat.IsZero() || now.Before(f.nextHeat) {
		return
	}
	for _, at := range []time.Time{f.breathImpact, f.brandImpact, f.lastImpact} {
		if !at.IsZero() {
			if d := now.Sub(at); d < mythicHeatGap && d > -mythicHeatGap {
				f.nextHeat = at.Add(mythicHeatGap)
				return
			}
		}
	}
	if now.Sub(f.pulled) >= mythicHeatRampAt {
		f.ramp++
	}
	pn, mn := MythicHeat(f.ramp)
	boss := w.NPCs[f.boss]
	for _, p := range w.mythicPlayersLocked(f) {
		w.mythicHurtPlayerLocked(boss, p, pn, mythicLineHeat, now)
	}
	for _, m := range w.mythicMercsLocked(f) {
		w.mythicHurtMercLocked(boss, m, mn, "Rising Heat")
	}
	f.nextHeat = now.Add(mythicHeatEvery)
	log.Printf("Mythic: instance %d: Rising Heat %d / %d (%v in)", f.inst, pn, mn, now.Sub(f.pulled).Round(time.Second))
}

// MythicHeat is a heat pulse's damage on a player and on a mercenary, ramp pulses after 17:00.
func MythicHeat(ramp int) (player, merc int) {
	return min(mythicHeatPlayer+mythicHeatRampPlayer*ramp, mythicHeatMostPlayer),
		min(mythicHeatMerc+mythicHeatRampMerc*ramp, mythicHeatMostMerc)
}

// mythicResetLocked puts the room back to staging (a wipe): the boss home at full health out of his fight, the guard
// and the pools gone, the gates closed again. Must be called with the lock held.
func (w *World) mythicResetLocked(f *mythicFight, why string, now time.Time) {
	w.mythicClearSpawnsLocked(f)
	if boss := w.NPCs[f.boss]; boss != nil && !boss.Dead {
		w.respawnNPCLocked(boss)
		boss.endRound()
		boss.endTemplateFight()
		w.wakeNPCLocked(boss)
	}
	w.mythicTellLocked(f, mythicLineWipe, mythicInfoRGB)
	log.Printf("Mythic: instance %d back to staging (%s) after %v, %d gates released", f.inst, why,
		now.Sub(f.pulled).Round(time.Second), f.gates)
	*f = mythicFight{inst: f.inst, world: f.world, boss: f.boss, valves: f.valves, exitX: f.exitX, exitZ: f.exitZ,
		hasExit: f.hasExit, landing: f.landing, first: -1}
}

// mythicCompleteLocked is the boss's death: the instance complete, once.
func (w *World) mythicCompleteLocked(in *dungeonInst, f *mythicFight, now time.Time) {
	if f.completed {
		return
	}
	f.completed = true
	f.phase = mythicDone
	w.mythicClearSpawnsLocked(f)
	w.mythicTellLocked(f, mythicLineDone, mythicInfoRGB)
	var who []string
	for _, p := range w.mythicPlayersLocked(f) {
		who = append(who, p.Name)
	}
	log.Printf("Mythic: instance %d (%s's) COMPLETE after %v, standing: %s", f.inst, in.ownerName,
		now.Sub(f.pulled).Round(time.Second), strings.Join(who, ", "))
}

// MythicStateOf is the fight of the instance (tests, the log); false for none.
func (w *World) MythicStateOf(inst uint32) (MythicState, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	in := w.dungeons.insts[inst]
	if in == nil || in.enc == nil {
		return MythicState{}, false
	}
	f := in.enc
	st := MythicState{Phase: f.phase.String(), Gates: f.gates, Guard: f.guard, Latched: f.latched, FirstValve: f.first,
		BreathImpact: f.breathImpact, NextBreath: f.nextBreath, NextBrand: f.nextBrand, BrandImpact: f.brandImpact,
		BrandOn: f.brandOn, NextHeat: f.nextHeat, Pools: len(f.pools), Completed: f.completed, Boss: f.boss, Valves: f.valves}
	for _, p := range f.pools {
		st.PoolSpots = append(st.PoolSpots, [3]float32{p.x, p.y, p.z})
	}
	return st, true
}

// encounterRestoreLocked gives an instance made again after a restart its fight (keepdungeon.go): completed as it
// was, else at staging with the boss at full health (a pull under way is not carried). Must be called with the lock
// held.
func (w *World) encounterRestoreLocked(in *dungeonInst, done bool, now time.Time) {
	w.encounterOpenLocked(in, now)
	f := in.enc
	if f == nil {
		return
	}
	boss := w.NPCs[f.boss]
	if done || boss != nil && boss.Dead {
		f.completed, f.phase = true, mythicDone
		return
	}
	if boss == nil {
		return // no boss made again (its row gone): no fight
	}
	if boss.HP < boss.MaxHP || boss.CurrentEnemy != 0 {
		w.respawnNPCLocked(boss)
		log.Printf("Mythic: instance %d made again at staging: the boss back at full health", in.id)
	}
}

// mythicDone reports an instance's fight completed (keepdungeon.go).
func (in *dungeonInst) mythicDone() bool { return in.enc != nil && in.enc.completed }
