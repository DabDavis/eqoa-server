package entity

import (
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A were's name says its line, whichever way round it is spelled and whatever
// its rank; no other creature's does.
func TestWereLine(t *testing.T) {
	for name, want := range map[string]string{
		"wolfwere": "wolfwere", "Elder Werewolf": "wolfwere", "bearwere": "bearwere", "werebear": "bearwere",
		"wererat": "ratwere", "Ratwere": "ratwere", "lionwere": "lionwere", "Werelion": "lionwere",
		"gatorwere": "gatorwere", "Ancient Weregator": "gatorwere",
		"grass snake": "", "wolf": "", "alligator": "", "Levandius": "", "The Bone Crusher": "", "": "",
	} {
		if got := WereLine(name); got != want {
			t.Errorf("WereLine(%q) = %q, want %q", name, got, want)
		}
	}
}

// wereRig is a world of weres of each line on a fresh database (its seeded
// roamer template given to most of them, as tools/seed_roamers.py gives it:
// 30 s crits from 10-20 s in, 7.4 a level), a control roamer that is no were,
// and the bites its hearer heard; the dice count each roll and answer roll.
type wereRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	w     *World
	mgr   *session.Manager
	t0    time.Time
	ids   map[string]EntityID
	heard []InfectionHit
	rolls int
	roll  int
}

var wereRigNPCs = []struct {
	name     string
	level    int
	template string
}{
	{"wolfwere", 30, "roamer"},
	{"Elder Werebear", 45, "roamer"},
	{"wererat", 60, "roamer"},
	{"lionwere", 40, "roamer"},
	{"Ancient Weregator", 55, ""},       // one of Heirloom's 34 of no template
	{"gnashing ratwere", 50, "wrecker"}, // a template whose crit has its own damage
	{"kicking wolfwere", 40, "melee"},   // a template with a kick and no timed crit
	{"grass snake", 34, "roamer"},       // the control: no were
}

func newWereRig(t *testing.T) *wereRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/weres.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &wereRig{t: t, db: db, t0: time.Unix(1_800_000_000, 0), ids: map[string]EntityID{}}
	r.exec(`INSERT INTO classtemplates (name, class, special, special_damage, special_every, special_first, special_jitter)
		VALUES ('wrecker', 0, 'Rending Claw', 20, 12, 3, 2)`)
	for i, n := range wereRigNPCs {
		id := 600 + i
		r.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, 100, 0, 1893243078, 1.0, ?, 0, '0')`, id, n.name, float64(1000*(i+1)), n.level)
		if n.template != "" {
			r.exec(`INSERT INTO npctemplate (name, template) VALUES (?, ?)`, database.NPCClassKey(n.name), n.template)
		}
		r.ids[n.name] = EntityID(id)
	}
	r.w = NewWorld()
	r.mgr = session.NewManager()
	r.w.SessionManager = r.mgr
	r.w.LoadNPCs(database.NewNPCStore(db))
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.w.OnInfection(func(h InfectionHit) { r.heard = append(r.heard, h) })
	old := infectIntn
	infectIntn = func(n int) int {
		if n != 10000 {
			t.Fatalf("an infection rolled in %d, not in ten thousand", n)
		}
		r.rolls++
		return r.roll
	}
	t.Cleanup(func() { infectIntn = old })
	r.roll = 9999 // no infection unless a test says
	return r
}

func (r *wereRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.db.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *wereRig) at(d time.Duration) time.Time { return r.t0.Add(d) }
func (r *wereRig) npc(name string) *NPC         { return r.w.NPCs[r.ids[name]] }

// player is a player of level beside the NPC, seeing it.
func (r *wereRig) player(name string, level int, by *NPC) (*Player, *session.Session) {
	r.t.Helper()
	id := EntityID(2001 + len(r.w.Players))
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, byte(id)), Port: 7000}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, CharServerID: 90000 + uint32(id), Name: name, Class: 0, Level: level, HP: 1000000, MaxHP: 1000000,
		AC: 100000, Speed: 8.833333, X: by.X + 1.5, Y: by.Y, Z: by.Z}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14000000 + uint32(id))
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = by.ServerID
	vs.ChannelObjectID[0] = 0x00300000 + uint32(by.ServerID)
	return p, sess
}

// fightFor sets n on foe and runs the fight from from to to (exclusive), every
// 100 ms; the 0x00B1s of n that sess saw, by visual, at their times.
func (r *wereRig) fightFor(n *NPC, foe EntityID, sess *session.Session, from, to time.Duration) map[uint32][]time.Duration {
	n.Threats = map[EntityID]int64{foe: 1}
	n.CurrentEnemy = foe
	seen := map[uint32][]time.Duration{}
	for d := from; d < to; d += 100 * time.Millisecond {
		if p := r.w.Players[foe]; p != nil {
			p.HP = p.MaxHP
		}
		r.w.mu.Lock()
		r.w.aiSystem(r.at(d))
		r.w.templateSystem(r.at(d))
		r.w.mu.Unlock()
		if sess != nil {
			for _, c := range castsIn(drain(sess)) {
				if c.caster == 0x00300000+uint32(n.ServerID) {
					seen[c.fx] = append(seen[c.fx], d)
				}
			}
		}
	}
	return seen
}

// stop ends n's fight, as its enemy gone does, at d.
func (r *wereRig) stop(n *NPC, d time.Duration) {
	n.CurrentEnemy, n.Threats = 0, nil
	r.w.mu.Lock()
	r.w.aiSystem(r.at(d))
	r.w.mu.Unlock()
}

// Every were fights by a copy of its template of its own: a Critical Hit every
// 30 s, no opening delay, no jitter, from any level, the rest as its template
// has it (class, hit points, melee, kick); one of no template by one of the crit
// alone; one whose template's crit has damage of its own keeps it. The roamer's
// row, which the snake shares, is not changed.
func TestEveryWereHasItsOwnCrit(t *testing.T) {
	r := newWereRig(t)
	for _, n := range wereRigNPCs {
		npc := r.npc(n.name)
		tm := npc.tmpl
		if tm == nil {
			t.Fatalf("%s fights by no template", n.name)
		}
		if n.name == "grass snake" {
			if tm.were || tm.SpecialEvery != 30 || tm.SpecialFirst != 15 || tm.SpecialJitter != 5 {
				t.Errorf("the snake's roamer: were %v, every %g first %g jitter %g; want the roamer's own 30/15/5", tm.were, tm.SpecialEvery,
					tm.SpecialFirst, tm.SpecialJitter)
			}
			continue
		}
		if !tm.were || tm.SpecialEvery != 30 || tm.SpecialFirst != 0 || tm.SpecialJitter != 0 || tm.SpecialFrom != 0 || !npc.critRounds() {
			t.Errorf("%s: were %v, every %g first %g jitter %g from %d", n.name, tm.were, tm.SpecialEvery, tm.SpecialFirst, tm.SpecialJitter, tm.SpecialFrom)
		}
		wantDamage, wantName, wantClass := 7.4, "Critical Hit", 0
		switch n.template {
		case "":
			wantName, wantClass = "Critical Hit", -1
		case "wrecker":
			wantDamage, wantName = 20, "Rending Claw"
		case "melee":
			wantName, wantClass = "Kick", 4
		}
		if tm.SpecialDamage != wantDamage || tm.Special != wantName || tm.Class != wantClass {
			t.Errorf("%s: %q for %g a level, class %d; want %q, %g, %d", n.name, tm.Special, tm.SpecialDamage, tm.Class, wantName, wantDamage, wantClass)
		}
		if n.template == "melee" && (tm.SpecialChance != 20 || tm.TakenPct != 100 || tm.MeleePct != 115) {
			t.Errorf("the kicking wolfwere lost its melee template's profile: %+v", tm.ClassTemplate)
		}
		if n.template == "" && (tm.HPPct != 100 || tm.TakenPct != 100 || tm.MeleePct != 100) {
			t.Errorf("the untemplated weregator's own figures changed: %+v", tm.ClassTemplate)
		}
	}
	if r.w.templates.byName["roamer"].SpecialFirst != 15 {
		t.Error("the shared roamer row changed")
	}
}

// The opening crit carries the infection: at the fight's first impact (0.5 s)
// with Disease Cloud's visual, for the crit's damage (7.4 x 30 = 222), one roll;
// the crits after it every 30 s at a round's start are plain (the special's
// visual) and roll nothing, nor do the quads. A roll under the chance infects
// and is heard with the character, its line, the were and a key.
func TestTheOpeningCritCarriesTheInfection(t *testing.T) {
	r := newWereRig(t)
	were := r.npc("wolfwere")
	p, sess := r.player("Hero", 30, were) // white
	r.roll = 499
	hp := p.MaxHP
	seen := r.fightFor(were, p.ServerID, sess, 0, 70*time.Second)
	if got := fmt.Sprint(seen[WereInfectFX]); got != ds(500) {
		t.Errorf("the infection's visual at %s, want at 0.5 s alone", got)
	}
	// Attack2 (level 30): 4 s rounds; ready again at 30 s, the round at 32 s; then 62 s -> 64 s
	if got := fmt.Sprint(seen[templateSpecialFX]); got != ds(32500, 64500) {
		t.Errorf("plain crits at %s, want 32.5 and 64.5 s", got)
	}
	if r.rolls != 1 || len(r.heard) != 1 {
		t.Fatalf("%d rolls, %d heard; want 1 and 1", r.rolls, len(r.heard))
	}
	h := r.heard[0]
	if h.ServerID != p.CharServerID || h.Line != "wolfwere" || h.NPCName != "wolfwere" || h.Session != sess || h.Key == "" ||
		h.NPCID != uint32(were.ServerID) || !h.At.Equal(r.at(500*time.Millisecond)) {
		t.Errorf("heard %+v", h)
	}
	_ = hp
	if !r.at(500 * time.Millisecond).Add(WereInfectCooldown).Equal(were.wereReady) {
		t.Errorf("ready again at %v, want a day after the opening crit", were.wereReady)
	}
}

// The chance by con, at its exact thresholds: 1% (100 in 10,000) from a were
// that cons green to the player, 5% (500) from every other con; a roll at the
// chance does not infect, one under it does. Each case is a fresh spawn of each
// line's were (the five families) at levels 30, 45 and 60.
func TestTheInfectionsChanceByCon(t *testing.T) {
	seen := map[byte]bool{}
	for _, name := range []string{"wolfwere", "Elder Werebear", "wererat", "lionwere", "Ancient Weregator"} {
		for _, level := range []int{30, 45, 60} {
			r := newWereRig(t)
			were := r.npc(name)
			were.Level = level
			for _, pl := range []int{level - 4, level - 1, level, level + 1, level + 4, level + 20} {
				con := ConColor(pl, level)
				seen[con] = true
				chance := 500
				if con == ConGreen {
					chance = 100
				}
				p, _ := r.player(fmt.Sprintf("Hero%d", pl), pl, were)
				for _, roll := range []int{chance, chance - 1} {
					r.rolls, r.heard, r.roll = 0, nil, roll
					were.wereReady = time.Time{} // a fresh spawn's
					r.w.mu.Lock()
					r.w.critHitLocked(were, p.ServerID, r.at(0))
					r.w.mu.Unlock()
					want := 0
					if roll < chance {
						want = 1
					}
					if r.rolls != 1 || len(r.heard) != want {
						t.Errorf("%s at %d on a level %d (con %s), roll %d: %d rolls, %d infections; want 1 and %d", name, level, pl,
							conWords(con), roll, r.rolls, len(r.heard), want)
					}
					if want == 1 && r.heard[0].Line != WereLine(name) {
						t.Errorf("%s: line %q", name, r.heard[0].Line)
					}
				}
			}
		}
	}
	for _, con := range []byte{ConRed, ConYellow, ConWhite, ConDarkBlue, ConLightBlue, ConGreen} {
		if !seen[con] {
			t.Errorf("con %s untested", conWords(con))
		}
	}
}

// The ability waits a day per spawn, shared across its targets: a re-engagement
// opens with the plain crit and no roll, another player's fight too; once the day
// is over the next crit rolls again; a death and a new spawn are ready at once.
// What a pet takes first spends it with no roll, and a lethal opening hit spends
// it with none either.
func TestTheInfectionWaitsADayPerSpawn(t *testing.T) {
	r := newWereRig(t)
	were := r.npc("lionwere")
	p, sess := r.player("Hero", 40, were)
	q, qsess := r.player("Other", 40, were)
	r.fightFor(were, p.ServerID, sess, 0, 2*time.Second)
	if r.rolls != 1 {
		t.Fatalf("the opening: %d rolls", r.rolls)
	}
	r.stop(were, 2*time.Second)
	seen := r.fightFor(were, p.ServerID, sess, 2100*time.Millisecond, 4*time.Second) // a re-pull
	if len(seen[WereInfectFX]) != 0 || len(seen[templateSpecialFX]) != 1 || r.rolls != 1 {
		t.Errorf("a re-pull: infection visuals %v, plain crits %v, %d rolls; want none, one and 1", seen[WereInfectFX], seen[templateSpecialFX], r.rolls)
	}
	r.stop(were, 4*time.Second)
	drain(qsess)
	seen = r.fightFor(were, q.ServerID, qsess, 4100*time.Millisecond, 6*time.Second) // another player
	if len(seen[WereInfectFX]) != 0 || len(seen[templateSpecialFX]) != 1 || r.rolls != 1 {
		t.Errorf("another player: infection visuals %v, plain crits %v, %d rolls", seen[WereInfectFX], seen[templateSpecialFX], r.rolls)
	}
	r.stop(were, 6*time.Second)
	// a day after the opening crit (0.5 s): ready again
	day := WereInfectCooldown + time.Second
	seen = r.fightFor(were, p.ServerID, sess, day, day+2*time.Second)
	if len(seen[WereInfectFX]) != 1 || r.rolls != 2 {
		t.Errorf("a day on: infection visuals %v, %d rolls; want one and 2", seen[WereInfectFX], r.rolls)
	}
	r.stop(were, day+2*time.Second)
	// its death and new spawn: ready at once
	r.w.mu.Lock()
	were.Dead = true
	r.w.respawnNPCLocked(were)
	r.w.mu.Unlock()
	if !were.wereAbilityReady(r.at(day+3*time.Second)) || were.spawnGen != 1 {
		t.Errorf("a new spawn: ready %v, generation %d", were.wereAbilityReady(r.at(day+3*time.Second)), were.spawnGen)
	}

	// a pet first: spent, no roll, nothing for its owner
	r2 := newWereRig(t)
	were2 := r2.npc("wererat")
	owner, osess := r2.player("Owner", 58, were2)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 58, HP: 1000000, MaxHP: 1000000, X: were2.X + 1.5, Y: 10, Z: were2.Z, Owner: owner.ServerID}
	r2.w.NPCs[pet.ServerID] = pet
	r2.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	r2.roll = 0
	seen = r2.fightFor(were2, pet.ServerID, osess, 0, 2*time.Second)
	if len(seen[WereInfectFX]) != 1 || r2.rolls != 0 || len(r2.heard) != 0 || were2.wereAbilityReady(r2.at(time.Second)) {
		t.Errorf("a pet first: infection visuals %v, %d rolls, %d infections, still ready %v", seen[WereInfectFX], r2.rolls, len(r2.heard),
			were2.wereAbilityReady(r2.at(time.Second)))
	}
	r2.stop(were2, 2*time.Second)
	r2.fightFor(were2, owner.ServerID, osess, 2100*time.Millisecond, 4*time.Second)
	if r2.rolls != 0 {
		t.Errorf("the owner after its pet: %d rolls", r2.rolls)
	}

	// a lethal opening: spent, no roll
	r3 := newWereRig(t)
	were3 := r3.npc("wolfwere")
	v, _ := r3.player("Victim", 30, were3)
	v.HP, v.MaxHP = 10, 10
	r3.roll = 0
	r3.w.mu.Lock()
	r3.w.critHitLocked(were3, v.ServerID, r3.at(0))
	r3.w.mu.Unlock()
	if !v.IsDead() || r3.rolls != 0 || were3.wereAbilityReady(r3.at(time.Second)) {
		t.Errorf("a lethal opening: dead %v, %d rolls, still ready %v", v.IsDead(), r3.rolls, were3.wereAbilityReady(r3.at(time.Second)))
	}
}

// Nothing but the crit rolls: a were's quads and its template's kick (the
// melee template's, in a fifth of its rounds) land without a roll, the
// infection spent on the opening crit.
func TestKicksAndHitsNeverRoll(t *testing.T) {
	r := newWereRig(t)
	were := r.npc("kicking wolfwere")
	p, sess := r.player("Hero", 40, were)
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 } // every chance special comes
	seen := r.fightFor(were, p.ServerID, sess, 0, 40*time.Second)
	if len(seen[WereInfectFX]) != 1 || len(seen[templateSpecialFX]) < 3 || r.rolls != 1 {
		t.Errorf("infection visuals %v, kicks and plain crits %v, %d rolls; want one, several, 1", seen[WereInfectFX], seen[templateSpecialFX], r.rolls)
	}
}

// The crit's damage is the template's: 7.4 a level on the roamer's (round(7.4 x
// level) at 30, 45, 60), the wrecker's own 20 a level, 7.4 on the untemplated.
func TestAWeresCritDamage(t *testing.T) {
	for _, c := range []struct {
		name  string
		level int
		per   float64
	}{{"wolfwere", 30, 7.4}, {"Elder Werebear", 45, 7.4}, {"wererat", 60, 7.4}, {"gnashing ratwere", 50, 20}, {"Ancient Weregator", 55, 7.4}} {
		r := newWereRig(t)
		were := r.npc(c.name)
		p, _ := r.player("Hero", c.level, were)
		hp := p.HP
		r.w.mu.Lock()
		r.w.critHitLocked(were, p.ServerID, r.at(0))           // the infection's
		r.w.critHitLocked(were, p.ServerID, r.at(time.Second)) // the plain
		r.w.mu.Unlock()
		want := 2 * int(math.Round(float64(c.level)*c.per))
		if got := hp - p.HP; got != want {
			t.Errorf("%s at %d: two crits took %d, want %d", c.name, c.level, got, want)
		}
	}
}

// A restart that keeps the sessions carries a living were's spent infection and
// its spawn's generation; a dead one's is not carried (its new spawn is ready).
func TestARestartKeepsAWeresInfectionSpent(t *testing.T) {
	r := newWereRig(t)
	were := r.npc("wolfwere")
	now := time.Now()
	were.wereReady, were.spawnGen = now.Add(10*time.Hour), 3
	r.w.mu.Lock()
	k, ok := r.w.keptNPCLocked(were, now)
	r.w.mu.Unlock()
	if !ok || !k.WereReady.Equal(were.wereReady) || k.SpawnGen != 3 {
		t.Fatalf("kept %v: %+v", ok, k)
	}
	r2 := newWereRig(t)
	got := r2.w.RestoreNPCs([]KeptNPC{k})
	w2 := r2.npc("wolfwere")
	if got.Were != 1 || !w2.wereReady.Equal(were.wereReady) || w2.spawnGen != 3 || w2.wereAbilityReady(now) {
		t.Errorf("restored %v: ready %v, generation %d", got, w2.wereReady, w2.spawnGen)
	}
	were.Dead, were.DeathTime = true, now
	r.w.mu.Lock()
	k, _ = r.w.keptNPCLocked(were, now)
	r.w.mu.Unlock()
	if !k.WereReady.IsZero() {
		t.Errorf("a dead were's infection was kept: %+v", k)
	}
}
