package entity

import (
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// setHPRig is a dreadnaught at (1010, 1000) in a fight with the fixture's
// player (at 1000, 1000, 5,000 of 5,000), its sethp the ability given.
type setHPRig struct {
	*aggroFix
	boss *NPC
}

func newSetHPRig(t *testing.T, m database.TemplateAbility) *setHPRig {
	t.Helper()
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{m}); err != nil {
		t.Fatal(err)
	}
	f := newAggroFix(t, 55)
	f.p.HP, f.p.MaxHP = 5000, 5000
	boss := f.npc(3200, "dreadnaught", 55, creationBody, 1010, 1000)
	boss.HP, boss.MaxHP = 100000, 100000
	boss.tmpl = &ClassTemplate{moments: []database.TemplateAbility{m}}
	boss.CurrentEnemy = f.p.ServerID
	boss.Threats = map[EntityID]int64{f.p.ServerID: 10}
	return &setHPRig{aggroFix: f, boss: boss}
}

// player puts another player in the world at (x, z) with a session of its own.
func (r *setHPRig) player(id EntityID, name string, hp int, x, z float32) (*Player, *session.Session) {
	s := r.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000 + int(id)}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, CharServerID: uint32(id), Name: name, Level: 55, HP: hp, MaxHP: max(hp, 5000), X: x, Y: 10, Z: z}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, x, z)
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetPlayerObjectID(0x1424A000 + uint32(id))
	return p, s
}

func (r *setHPRig) at(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.bossMomentsLocked(r.boss, r.boss.fight(), r.now.Add(d))
}

// The owner, 2026-11-13: "dreadnaughts are supposed to have an AoE which
// reduces hp to 700 for players and mobs". Within its radius of 30: the
// player it fights (5,000) and another (2,000) to 700, a creature of its own
// side (20,000) to 700 and a player's pet (3,000) to 700; one at 700 and one
// at 300 left as they are; a player and a creature 60 off untouched; the
// dreadnaught itself untouched; a dead creature in reach left dead. Again at
// its every, not before.
func TestASetHPAreaTakesPlayersAndCreaturesDownToItsFigure(t *testing.T) {
	r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 700, Radius: 30, Every: 30, First: 2, Name: "Dread Curse"})
	other, _ := r.player(1002, "Other", 2000, 1020, 1010)
	at700, _ := r.player(1003, "Even", 700, 1000, 1010)
	low, lowSess := r.player(1004, "Low", 300, 1005, 995)
	far, _ := r.player(1005, "Far", 5000, 1070, 1000)
	ally := r.f().npc(3201, "dread wolf", 50, creationBody, 1015, 1005)
	ally.HP, ally.MaxHP = 20000, 20000
	pet := r.f().npc(3202, "a pet", 40, creationBody, 1000, 1005)
	pet.HP, pet.MaxHP, pet.Owner = 3000, 3000, other.ServerID
	lowAlly := r.f().npc(3203, "dread rat", 10, creationBody, 1012, 1002)
	lowAlly.HP, lowAlly.MaxHP = 300, 300
	farAlly := r.f().npc(3204, "far wolf", 50, creationBody, 1075, 1000)
	farAlly.HP, farAlly.MaxHP = 20000, 20000
	corpse := r.f().npc(3205, "dead wolf", 50, creationBody, 1011, 1001)
	corpse.HP, corpse.Dead = 0, true

	r.at(0)
	if r.p.HP != 5000 {
		t.Fatalf("it came before its first: %d", r.p.HP)
	}
	r.at(2 * time.Second)
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"its foe", r.p.HP, 700}, {"another player", other.HP, 700}, {"a player at 700", at700.HP, 700},
		{"a player below", low.HP, 300}, {"a player beyond its radius", far.HP, 5000},
		{"a creature of its side", ally.HP, 700}, {"a pet", pet.HP, 700}, {"a creature below", lowAlly.HP, 300},
		{"a creature beyond its radius", farAlly.HP, 20000}, {"itself", r.boss.HP, 100000}, {"a corpse", corpse.HP, 0},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
	if !corpse.Dead || r.p.IsDead() || other.IsDead() {
		t.Fatal("a death changed")
	}
	if got := figures(lowSess); len(got) != 0 {
		t.Fatalf("one below its figure was told %v", got)
	}
	r.p.HP, ally.HP = 4000, 9000
	r.at(10 * time.Second)
	if r.p.HP != 4000 || ally.HP != 9000 {
		t.Fatalf("it came again before its every: %d, %d", r.p.HP, ally.HP)
	}
	r.at(32 * time.Second)
	if r.p.HP != 700 || ally.HP != 700 {
		t.Fatalf("at its every: %d, %d", r.p.HP, ally.HP)
	}
}

func (r *setHPRig) f() *aggroFix { return r.aggroFix }

// A player sees it as a hit: a 0x00DB of the difference onto itself, its stat
// channel dirty, a line naming it.
func TestASetHPShowsTheDifferenceToThePlayerAsDamage(t *testing.T) {
	r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 700, Radius: 30, Every: 30, Name: "Dread Curse"})
	r.p.StatsDirty = false
	r.at(0)
	ls := lines(r.sess)
	if len(ls) != 1 || !strings.Contains(ls[0], "Dread Curse takes 4300 from you, leaving you 700 hit points") {
		t.Fatalf("lines %q", ls)
	}
	var hits [][]byte
	for _, m := range r.sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && opOfMsg(d) == 0x00DB {
			hits = append(hits, append([]byte(nil), d...))
		}
	}
	if len(hits) != 1 {
		t.Fatalf("0x00DB sent %d times", len(hits))
	}
	if got := figures(r.sess); len(got) != 1 || got[0] != -4300 {
		t.Fatalf("figures %v, want [-4300]", got)
	}
	if to := uint32(hits[0][len(hits[0])-4]) | uint32(hits[0][len(hits[0])-3])<<8 | uint32(hits[0][len(hits[0])-2])<<16 |
		uint32(hits[0][len(hits[0])-1])<<24; to != r.sess.PlayerObjectID() {
		t.Fatalf("the figure is on %08X, want the player's %08X", to, r.sess.PlayerObjectID())
	}
	if r.p.HP != 700 || !r.p.StatsDirty {
		t.Fatalf("HP %d, stats dirty %v", r.p.HP, r.p.StatsDirty)
	}
}

// Never a death: Cazic Thule's "sets to 10hp" on a player at 11 leaves 10, at
// 10 or 1 leaves it; and a sethp of 0 is refused (it would kill).
func TestASetHPNeverKills(t *testing.T) {
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilitySetHP, SetHP: 0, Every: 30}}); err == nil {
		t.Fatal("a sethp to 0 passed")
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilitySelfDoT, Damage: 5, Every: 30, SetHP: 10}}); err == nil {
		t.Fatal("set_hp on a self dot passed")
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilityNuke, Every: 30, Radius: 10}}); err == nil {
		t.Fatal("radius on a nuke passed")
	}
	for _, hp := range []int{11, 10, 1} {
		r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 10, Every: 30})
		r.p.HP = hp
		r.at(0)
		if want := min(hp, 10); r.p.HP != want || r.p.IsDead() {
			t.Fatalf("from %d: HP %d (want %d), dead %v", hp, r.p.HP, want, r.p.IsDead())
		}
	}
}

// With no radius it is the one its creature fights alone (Cazic Thule's
// debuff: "the tank only"): the player beside it untouched, the creature
// beside it untouched; on a pet it fights, the pet. Its chance: the due times
// that fail the roll pass it by until the next.
func TestASetHPOfNoRadiusIsItsFoesAlone(t *testing.T) {
	r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 10, Every: 6, Chance: 30, Name: "Touch of Cazic"})
	other, _ := r.player(1002, "Other", 5000, 1005, 1000)
	ally := r.f().npc(3201, "ally", 50, creationBody, 1012, 1000)
	ally.HP, ally.MaxHP = 20000, 20000
	rolls := []int{50, 10}
	bossMomentIntn = func(int) int { v := rolls[0]; rolls = rolls[1:]; return v }
	defer func() { bossMomentIntn = rand.Intn }()
	r.at(0)
	if r.p.HP != 5000 {
		t.Fatalf("a roll of 50 against 30%% went off: %d", r.p.HP)
	}
	r.at(6 * time.Second)
	if r.p.HP != 10 || other.HP != 5000 || ally.HP != 20000 {
		t.Fatalf("its foe %d (want 10), the player beside %d, the creature beside %d", r.p.HP, other.HP, ally.HP)
	}
	pet := r.f().npc(3202, "a pet", 40, creationBody, 1005, 1000)
	pet.HP, pet.MaxHP, pet.Owner = 3000, 3000, r.p.ServerID
	r.boss.CurrentEnemy = pet.ServerID
	bossMomentIntn = func(int) int { return 0 }
	r.at(12 * time.Second)
	if pet.HP != 10 || pet.Dead {
		t.Fatalf("the pet it fights: %d", pet.HP)
	}
}

// The owner, 2026-11-13: the dreadnaught's "aoe that debuffs to 700hp also
// has a damage over time ticks 650 with no CR, with max CR is ticking around
// 80". At its landing, after the fall to 700: a player at no CR takes 650
// (50 left), one at the cap (315) 80 (620 left), one already at 300 at the
// cap is not lowered but takes its 80 (220); a creature of its side is
// lowered and ticked for 650, no hate for the dreadnaught given. Each is told
// the tick (a figure and a line). A second landing replaces the first.
func TestASetHPDamageOverTimeScalesWithColdResist(t *testing.T) {
	resistRand = func() float64 { return 0.5 } // a resist at the cap holds
	defer func() { resistRand = rand.Float64 }()
	m := database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 700, Radius: 30, Every: 30, Name: "Dread Curse",
		Tick: 650, TickAtCap: 80, Resist: "CR"}
	r := newSetHPRig(t, m)
	capped, cappedSess := r.player(1002, "Capped", 5000, 1005, 1005)
	capped.ColdR = 315
	low, _ := r.player(1003, "Low", 300, 1005, 995)
	low.ColdR = 400
	ally := r.f().npc(3201, "dread wolf", 50, creationBody, 1015, 1005)
	ally.HP, ally.MaxHP = 20000, 20000
	vs := GetOrCreateVisibility(cappedSess) // the dreadnaught in its view
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = r.boss.ServerID, 0x01312EA0
	r.at(0)
	if r.p.HP != 50 || capped.HP != 620 || low.HP != 220 || ally.HP != 50 {
		t.Fatalf("no CR %d (want 50), at the cap %d (want 620), below and capped %d (want 220), a creature %d (want 50)",
			r.p.HP, capped.HP, low.HP, ally.HP)
	}
	if len(ally.Threats) != 0 {
		t.Fatalf("its own side was given hate: %v", ally.Threats)
	}
	if got := figures(cappedSess); len(got) != 2 || got[0] != -4300 || got[1] != -80 {
		t.Fatalf("the capped player's figures %v, want [-4300 -80]", got)
	}
	if n := countDoT(r.p.Effects); n != 1 {
		t.Fatalf("%d of its damage over time on the player", n)
	}
	r.p.HP, ally.HP = 5000, 20000
	r.at(30 * time.Second)
	if n := countDoT(r.p.Effects); n != 1 || r.p.HP != 50 {
		t.Fatalf("again: %d on the player, HP %d", n, r.p.HP)
	}
	if n := countDoT(ally.Effects); n != 1 {
		t.Fatalf("%d of its damage over time on the creature", n)
	}
	// the calibration: no resist 650, the cap 80, half the cap between
	half := database.TemplateAbility{Tick: 650, TickAtCap: 80}
	share := ResistShare(157) * bossDoTAtCap(&half) / ResistCapShare
	if got := int(650*(1-share) + 0.5); got != 366 {
		t.Fatalf("at 157 CR a tick of %d, want 366", got)
	}
}

func countDoT(es []*ActiveEffect) int {
	n := 0
	for _, e := range es {
		if e.SpellID == setHPDoTSpellID {
			n++
		}
	}
	return n
}
