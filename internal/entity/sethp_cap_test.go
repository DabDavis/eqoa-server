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

// dreadCurse is the dreadnaught's curse as the owner gave it, 2026-11-26: a
// maximum hit point ceiling of 750 for 24 s on a 30 s clock, and its cold
// damage over time of 150 a tick (15 at the cap).
var dreadCurse = database.TemplateAbility{Kind: database.AbilitySetHP, Name: "Dread Curse", SetHP: 750, Radius: 30, Every: 30, First: 2,
	Tick: 150, TickAtCap: 15, Resist: "CR", CapMaxHP: true, CapFor: 24}

func capsOn(p *Player) []*ActiveEffect {
	var es []*ActiveEffect
	for _, e := range p.Effects {
		if e.SpellID == setHPCapSpellID {
			es = append(es, e)
		}
	}
	return es
}

func (r *setHPRig) effects(at time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.effectSystem(r.now.Add(at))
}

// At its landing every player it reaches bears "Dread Curse": its maximum
// hit points at 750, its hit points clamped there (the fall), then its first
// cold tick of 150 (no CR); one already below 750 keeps its hit points, takes
// the ceiling and the tick. Unresisted: a player at the cap of CR bears the
// ceiling too. A creature of its side falls as before and bears no ceiling.
func TestASetHPCapHoldsMaximumHitPoints(t *testing.T) {
	resistRand = func() float64 { return 0.5 }
	defer func() { resistRand = rand.Float64 }()
	r := newSetHPRig(t, dreadCurse)
	low, _ := r.player(1003, "Low", 300, 1005, 995)
	capped, _ := r.player(1004, "Capped", 5000, 1005, 1005)
	capped.ColdR = 315
	ally := r.f().npc(3201, "dread wolf", 50, creationBody, 1015, 1005)
	ally.HP, ally.MaxHP = 20000, 20000
	r.at(0)
	r.at(2 * time.Second)

	for _, c := range []struct {
		name          string
		p             *Player
		hp, wantHP    int
		wantMax, caps int
	}{
		{"its foe", r.p, r.p.HP, 600, 750, len(capsOn(r.p))},
		{"one below", low, low.HP, 150, 750, len(capsOn(low))},
		{"one at the CR cap", capped, capped.HP, 735, 750, len(capsOn(capped))},
	} {
		if c.hp != c.wantHP || c.p.MaxHPNow() != c.wantMax || c.caps != 1 {
			t.Errorf("%s: HP %d/%d with %d ceilings, want %d/%d with 1", c.name, c.hp, c.p.MaxHPNow(), c.caps, c.wantHP, c.wantMax)
		}
	}
	e := capsOn(r.p)[0]
	if e.Name != "Dread Curse" || !e.Rules.HPCapped || e.Rules.HPCap != 750 || !e.Until.Equal(r.now.Add(26*time.Second)) {
		t.Fatalf("the ceiling: %+v", e)
	}
	named := 0
	for _, b := range r.p.Buffs {
		if b.Name == "Dread Curse" {
			named++
		}
	}
	if named != 2 { // the ceiling and its damage over time
		t.Fatalf("the effects list: %+v", r.p.Buffs)
	}
	if ls := strings.Join(lines(r.sess), "|"); !strings.Contains(ls, "Dread Curse holds your maximum hit points at 750 for 24 seconds") {
		t.Fatalf("lines %q", ls)
	}
	if n := countDoT(r.p.Effects); n != 1 {
		t.Fatalf("%d of its damage over time", n)
	}
	if ally.HP != 600 || ally.MaxHP != 20000 || len(ally.Effects) != 1 {
		t.Fatalf("the creature: %d/%d, %d effects", ally.HP, ally.MaxHP, len(ally.Effects))
	}
}

// Its damage over time ticks on under the ceiling and never refreshes it; at
// its time the maximum is the player's own again and its hit points stay
// where they were, to regenerate.
func TestASetHPCapEndsAtItsTimeAndTheTicksNeverRefreshIt(t *testing.T) {
	resistRand = func() float64 { return 0.5 }
	defer func() { resistRand = rand.Float64 }()
	r := newSetHPRig(t, dreadCurse)
	r.at(0)
	r.at(2 * time.Second)
	until := capsOn(r.p)[0].Until
	for i, want := range []int{450, 300, 150} {
		r.effects(time.Duration(8+6*i) * time.Second)
		if r.p.HP != want || r.p.MaxHPNow() != 750 {
			t.Fatalf("tick %d: HP %d/%d, want %d/750", i+2, r.p.HP, r.p.MaxHPNow(), want)
		}
		if cs := capsOn(r.p); len(cs) != 1 || !cs[0].Until.Equal(until) {
			t.Fatalf("tick %d: the ceiling %d, refreshed?", i+2, len(cs))
		}
	}
	r.p.HP = 400
	r.effects(26*time.Second - time.Millisecond)
	if len(capsOn(r.p)) != 1 || r.p.MaxHPNow() != 750 {
		t.Fatal("the control: gone before its time")
	}
	hp := r.p.HP
	r.effects(26 * time.Second)
	if len(capsOn(r.p)) != 0 || r.p.MaxHPNow() != 5000 || r.p.HP != hp {
		t.Fatalf("at its end: %d ceilings, HP %d/%d (was %d)", len(capsOn(r.p)), r.p.HP, r.p.MaxHPNow(), hp)
	}
	for _, b := range r.p.Buffs {
		if b.Name == "Dread Curse" {
			t.Fatalf("a slot left: %+v", r.p.Buffs)
		}
	}
}

// Landing again replaces it, never two: its time from the new landing.
func TestASetHPCapNeverStacks(t *testing.T) {
	r := newSetHPRig(t, dreadCurse)
	r.at(0)
	r.at(2 * time.Second)
	r.w.mu.Lock()
	r.w.setHPMomentLocked(r.boss, &r.boss.tmpl.moments[0], r.now.Add(12*time.Second))
	r.w.mu.Unlock()
	cs := capsOn(r.p)
	if len(cs) != 1 || !cs[0].Until.Equal(r.now.Add(36*time.Second)) || r.p.MaxHPNow() != 750 {
		t.Fatalf("again within its time: %d ceilings, max %d", len(cs), r.p.MaxHPNow())
	}
	named := 0
	for _, b := range r.p.Buffs {
		if b.Name == "Dread Curse" {
			named++
		}
	}
	if named != 2 {
		t.Fatalf("the effects list: %+v", r.p.Buffs)
	}
	r.at(32 * time.Second) // its clock's next
	if len(capsOn(r.p)) != 1 {
		t.Fatalf("at its every: %d ceilings", len(capsOn(r.p)))
	}
}

// It is kept over a logout (its damage over time, a creature's, is not) and
// holds the maximum again at the login.
func TestASetHPCapIsKeptOverALogout(t *testing.T) {
	r := newSetHPRig(t, dreadCurse)
	r.at(0)
	r.at(2 * time.Second)
	rows, ok := r.w.SavedEffects(r.p.ServerID, r.now.Add(12*time.Second))
	if !ok || len(rows) != 1 || rows[0].SpellID != setHPCapSpellID || rows[0].Name != "Dread Curse" ||
		rows[0].LeftMS != (14*time.Second).Milliseconds() {
		t.Fatalf("kept: %+v", rows)
	}
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, 1), Port: 7300}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 2001, Name: "Back", Level: 55, HP: 5000, MaxHP: 5000}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	login := r.now.Add(time.Hour)
	if n := w.RestoreEffects(s, rows, login); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if p.MaxHPNow() != 750 || p.HP != 750 || len(p.Buffs) != 1 || p.Buffs[0].Name != "Dread Curse" {
		t.Fatalf("after the login: HP %d/%d, slots %+v", p.HP, p.MaxHPNow(), p.Buffs)
	}
	w.mu.Lock()
	w.effectSystem(login.Add(14 * time.Second))
	w.mu.Unlock()
	if p.MaxHPNow() != 5000 || p.HP != 750 {
		t.Fatalf("at its end after the login: HP %d/%d", p.HP, p.MaxHPNow())
	}
}

// cap_max_hp and cap_for are a sethp's, cap_for a cap_max_hp's, and the rows
// the owner's design writes (tools: ~/.cache/dreadmax1126/apply_after.py) read.
func TestASetHPCapsFields(t *testing.T) {
	for _, bad := range []database.TemplateAbility{
		{Kind: database.AbilityNuke, Damage: 350, Name: "x", Every: 30, CapMaxHP: true},
		{Kind: database.AbilitySetHP, SetHP: 750, Every: 30, CapFor: 24},
		{Kind: database.AbilitySetHP, SetHP: 750, Every: 30, CapMaxHP: true, CapFor: -1},
		{Kind: database.AbilitySetHP, SetHP: 750, Every: 30, CapMaxHP: true, CapFor: 4000},
	} {
		if database.CheckTemplateAbilities([]database.TemplateAbility{bad}) == nil {
			t.Errorf("taken: %+v", bad)
		}
	}
	for _, row := range []string{
		`[{"kind":"sethp","name":"Dread Curse","set_hp":750,"radius":30,"every":30,"first":2,"tick":150,"tick_at_cap":15,"resist":"CR","cap_max_hp":true,"cap_for":24},` +
			`{"kind":"nuke","name":"Dread Frost","damage":350,"resist":"CR","fx":1813322335,"every":30,"first":10}]`,
		`[{"kind":"nuke","name":"Siliskor's Blight","damage":350,"resist":"PR","fx":1658985755,"every":30,"first":10},` +
			`{"kind":"sethp","name":"Siliskor's Venom","set_hp":750,"radius":40,"players_only":true,"every":30,"first":12,"tick":150,"tick_at_cap":15,"resist":"PR","cap_max_hp":true,"cap_for":24}]`,
	} {
		if _, err := database.ParseTemplateAbilities(row); err != nil {
			t.Errorf("%s: %v", row, err)
		}
	}
}
