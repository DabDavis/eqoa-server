package entity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// jackRig is the template rig's Gnoll Brute fighting by a swarm (Festering Jack's Swarm Mentality:
// first 10 s into its fight, every 75, rooted 5 s, a quad, then death within 20 of it), a pool of hit
// points and the tank it fights beside it.
func jackRig(t *testing.T) (*templateRig, *NPC, *Player, *session.Session) {
	t.Helper()
	r := newTemplateRig(t)
	r.godTemplate("swarmer", ", class", ", 0",
		`[{"kind":"swarm","every":75,"first":10,"hold":5,"radius":20,"say":"The swarm hungers! Swarm Mentality!"}]`, "gnoll brute")
	boss := r.npc("Gnoll Brute")
	pool(boss, 1_000_000)
	tank, sess := r.rampagePlayer("Tank", boss.X+1.5, boss.Z)
	r.fight(boss, tank.ServerID)
	return r, boss, tank, sess
}

// moments runs boss's moments at d, under the lock.
func (r *templateRig) moments(boss *NPC, d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.bossMomentsLocked(boss, boss.fight(), r.at(d))
}

// chatHas is whether a session was sent a chat line holding s.
func chatHas(sess *session.Session, s string) bool {
	for _, m := range drain(sess) {
		if op := opOfMsg(m); (op == protocol.OpcodeColoredChat || op == protocol.OpcodeClientMessage) && strings.Contains(chatText(m), s) {
			return true
		}
	}
	return false
}

// Packmaster (petition 3122): "it anounces that he is going to use it in say chat, then a few seconds
// later he quads, and immediately after the quad hit he aoe deathtouches anyone within a small range
// from HIS location"; "People can hide out of his line of sight because he's rooted". At its first it
// roots itself and says so; nothing lands until its hold is over; then the tank it fights takes its
// hits and dies with every player within 20 in its sight, while one behind a wall within 20 and one 25
// off live. Its root ends with it.
func TestASwarmRootsQuadsAndKillsWhoItSees(t *testing.T) {
	r, boss, tank, tankSess := jackRig(t)
	open, _ := r.rampagePlayer("Open", boss.X, boss.Z+12)
	hidden, _ := r.rampagePlayer("Hidden", boss.X+12, boss.Z)
	far, _ := r.rampagePlayer("Far", boss.X-25, boss.Z)
	r.w.SetGround(sightGround{{boss.X + 6, 0, boss.Z - 3, boss.X + 7, 40, boss.Z + 3}})

	r.moments(boss, 0)
	r.moments(boss, 9*time.Second)
	if boss.swarmRooted() {
		t.Fatal("rooted before its first")
	}
	r.moments(boss, 10*time.Second)
	if !boss.swarmRooted() || !boss.rooted(r.at(10*time.Second)) {
		t.Fatal("not rooted at its first")
	}
	if !chatHas(tankSess, "Swarm Mentality") {
		t.Fatal("no say naming Swarm Mentality")
	}
	r.moments(boss, 14*time.Second)
	if tank.HP != tank.MaxHP || open.IsDead() {
		t.Fatalf("it landed before its hold: tank %d, open dead %v", tank.HP, open.IsDead())
	}
	r.moments(boss, 15*time.Second)
	if boss.swarmRooted() || boss.rooted(r.at(15*time.Second)) {
		t.Fatal("still rooted after it landed")
	}
	if !tank.IsDead() || !open.IsDead() {
		t.Fatalf("in its sight within 20: tank dead %v, open dead %v", tank.IsDead(), open.IsDead())
	}
	if hidden.IsDead() || hidden.HP != hidden.MaxHP {
		t.Fatalf("the player behind the wall: dead %v, took %d", hidden.IsDead(), hidden.MaxHP-hidden.HP)
	}
	if far.IsDead() || far.HP != far.MaxHP {
		t.Fatalf("the player 25 off: dead %v, took %d", far.IsDead(), far.MaxHP-far.HP)
	}
}

// Its quad: Count of its melee hits on the one it fights before the death (here a tank out of its
// radius of 1, so the hits alone land on it).
func TestASwarmsQuadLandsOnTheOneItFights(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("swarmer", ", class", ", 0", `[{"kind":"swarm","every":75,"radius":1,"count":4}]`, "gnoll brute")
	boss := r.npc("Gnoll Brute")
	pool(boss, 1_000_000)
	tank, _ := r.rampagePlayer("Tank", boss.X+3, boss.Z)
	r.fight(boss, tank.ServerID)
	r.moments(boss, 0)
	r.moments(boss, 5*time.Second)
	if tank.IsDead() || tank.HP == tank.MaxHP {
		t.Fatalf("the tank out of its radius: dead %v, took %d (want its hits alone)", tank.IsDead(), tank.MaxHP-tank.HP)
	}
}

// "The secret well kept, is that a rogue could use harrass on him to stop the deathtouch." A Harass
// landing on it while it is rooted breaks it: no hits, no death, the root gone, a line to the rogue and
// its hate list. Another spell does not.
func TestAHarassBreaksTheSwarm(t *testing.T) {
	r, boss, tank, tankSess := jackRig(t)
	rogue, rogueSess := r.rampagePlayer("Rogue", boss.X-2, boss.Z)
	boss.Threats[rogue.ServerID] = 1
	r.moments(boss, 0)
	r.moments(boss, 10*time.Second)
	if !boss.swarmRooted() {
		t.Fatal("not rooted")
	}
	drain(tankSess)
	drain(rogueSess)
	r.w.mu.Lock()
	broke := r.w.swarmBreakLocked(boss, rogue, "Fireball", r.at(11*time.Second))
	r.w.mu.Unlock()
	if broke || !boss.swarmRooted() {
		t.Fatal("another spell broke it")
	}
	r.w.mu.Lock()
	r.w.landOnNPCLocked(&castState{spell: CastSpell{SpellID: 901290, Name: "Harass", Scope: 1}}, rogue, rogueSess, boss, r.at(12*time.Second))
	r.w.mu.Unlock()
	if boss.swarmRooted() || boss.rooted(r.at(12*time.Second)) {
		t.Fatal("a Harass that landed left it rooted")
	}
	if !chatHas(rogueSess, "breaks Gnoll Brute's Swarm Mentality") || !chatHas(tankSess, "breaks Gnoll Brute's Swarm Mentality") {
		t.Fatal("no line to the rogue and the tank")
	}
	r.moments(boss, 20*time.Second)
	if tank.IsDead() || rogue.IsDead() || tank.HP != tank.MaxHP {
		t.Fatalf("it landed after its break: tank dead %v (took %d), rogue dead %v", tank.IsDead(), tank.MaxHP-tank.HP, rogue.IsDead())
	}
	// and at its every it comes again
	r.moments(boss, 85*time.Second)
	if !boss.swarmRooted() {
		t.Fatal("not again at its every")
	}
}

// Rooted, it stands where it is while what it fights stands off; its fight's end takes the root.
func TestASwarmsRootHoldsItWhereItStands(t *testing.T) {
	r, boss, tank, _ := jackRig(t)
	r.moments(boss, 0)
	r.moments(boss, 10*time.Second)
	x, z := boss.X, boss.Z
	r.w.mu.Lock()
	r.w.SpatialIndex.Update(tank.ServerID, tank.X, tank.Z, boss.X+15, boss.Z)
	tank.X = boss.X + 15
	r.w.mu.Unlock()
	for d := 10 * time.Second; d < 14*time.Second; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		r.w.aiSystem(r.at(d))
		r.w.mu.Unlock()
	}
	if boss.X != x || boss.Z != z {
		t.Fatalf("it moved while rooted: (%.1f, %.1f) from (%.1f, %.1f)", boss.X, boss.Z, x, z)
	}
	boss.endTemplateFight()
	if boss.swarmRooted() || boss.rooted(r.at(14*time.Second)) {
		t.Fatal("rooted after its fight ended")
	}
}

// Its fields: an every no longer than its hold, a radius past the most, a break on another kind and a
// damage are refused; Festering Jack's row is taken.
func TestASwarmsFieldsAreChecked(t *testing.T) {
	ok := database.TemplateAbility{Kind: database.AbilitySwarm, Every: 75, First: 15, Hold: 5, Radius: 20,
		Say: "Swarm Mentality!", Breaks: []string{"Harass"}}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{ok}); err != nil {
		t.Fatal(err)
	}
	// the row petfix_3122_jack_20261125.py writes, beside his AoE
	var jack []database.TemplateAbility
	if err := json.Unmarshal([]byte(`[{"kind":"aoe","spell_id":905002,"damage_pct":67,"every":20,"first":8},`+
		`{"kind":"swarm","name":"Swarm Mentality","every":75,"first":20,"hold":5,"count":4,"radius":20,`+
		`"say":"The swarm hungers as one! Swarm Mentality!","breaks":["Harass"]}]`), &jack); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckTemplateAbilities(jack); err != nil {
		t.Fatalf("Festering Jack's row: %v", err)
	}
	for _, bad := range []database.TemplateAbility{
		{Kind: database.AbilitySwarm, Every: 5, Hold: 5},
		{Kind: database.AbilitySwarm, Every: 75, Radius: 61},
		{Kind: database.AbilitySwarm, Every: 75, Damage: 100},
		{Kind: database.AbilitySetHP, SetHP: 700, Every: 30, Breaks: []string{"Harass"}},
	} {
		if err := database.CheckTemplateAbilities([]database.TemplateAbility{bad}); err == nil {
			t.Errorf("%+v taken", bad)
		}
	}
}
