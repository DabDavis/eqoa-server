package entity

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The Testing Heroes (heroes.go; petition 1471): who is credited for which
// kill and by how much, the floor and the ceiling, the switch, and what a
// character may spend. The people and creatures are made up.

// heroesFactions is a faction table with the Testing Heroes' row as
// tools/seed_testing_heroes.py writes it, beside one other.
var heroesFactions = []database.FactionDefault{
	{Name: "Citizens of Freeport", Standing: 1},
	{Name: HeroesFaction, Standing: FactionMin},
}

// heroesOn turns the Testing Heroes on in w with their faction in the table.
func heroesOn(w *World) {
	w.SetFactionDefaults(heroesFactions)
	w.SetTestingHeroes(true)
}

// heroLines are the Testing Heroes' faction lines among what sess was sent.
func heroLines(sess *session.Session) []string {
	var out []string
	for _, l := range lines(sess) {
		if strings.Contains(l, " faction with "+HeroesFaction) {
			out = append(out, l)
		}
	}
	return out
}

func (k *killWorld) kill(npc *NPC) {
	k.w.mu.Lock()
	defer k.w.mu.Unlock()
	k.w.awardKillXPLocked(k.sess[0], k.players[0], npc)
}

func (k *killWorld) standing(i int) int {
	k.w.mu.Lock()
	defer k.w.mu.Unlock()
	return k.w.standingLocked(k.players[i], HeroesFaction)
}

// A kill that gives its killer experience gives it 1 with the Testing Heroes,
// from the floor, told as a faction's change is, after the experience's line;
// a creature too low to be worth experience (a level 44 to a level 60: grey)
// gives none and says nothing. The control for the kill's worth: the same
// world's level 60 creature.
func TestAKillThatGivesExperienceCreditsTheTestingHeroes(t *testing.T) {
	k := newKillWorld(t, 60)
	heroesOn(k.w)
	if got := k.standing(0); got != FactionMin {
		t.Fatalf("a character starts at %d with the Testing Heroes, want the floor %d", got, FactionMin)
	}
	grey := &NPC{ServerID: 5001, Name: "a rat", Level: 44, X: 1000, Z: 1000}
	if groupExp(grey.Level, 60, []int{60}) != 0 {
		t.Fatal("no test: the grey creature is worth experience")
	}
	k.kill(grey)
	if got := k.standing(0); got != FactionMin || len(heroLines(k.sess[0])) != 0 || len(k.players[0].Factions) != 0 {
		t.Fatalf("a kill worth no experience: standing %d, lines %q, own standings %v", got, heroLines(k.sess[0]), k.players[0].Factions)
	}

	worth := &NPC{ServerID: 5002, Name: "a froglok", Level: 60, X: 1000, Z: 1000}
	k.kill(worth)
	if got := k.standing(0); got != FactionMin+1 {
		t.Fatalf("after a kill worth experience: %d, want %d", got, FactionMin+1)
	}
	all := lines(k.sess[0])
	want := "-1999 faction with Testing Heroes (+1)"
	if got := heroLines(k.sess[0]); len(got) != 1 || got[0] != want {
		t.Fatalf("the lines %q, want %q", got, want)
	}
	// the experience's line first, then the faction's
	xpAt, heroAt := -1, -1
	for i, l := range all {
		if strings.Contains(l, "class mastery XP") || strings.Contains(l, "You receive") {
			xpAt = i
		}
		if l == want {
			heroAt = i
		}
	}
	if xpAt < 0 || heroAt < xpAt {
		t.Errorf("the experience's line %d, the faction's %d: %q", xpAt, heroAt, all)
	}
	// a second and a third kill: one each
	k.kill(worth)
	k.kill(worth)
	if got := k.standing(0); got != FactionMin+3 {
		t.Errorf("after three kills: %d, want %d", got, FactionMin+3)
	}
}

// A raid boss's kill is worth five: a creature whose class template gives it
// 1,000% or more of its level's hit points, or one the loot's tables make a
// boss (by its name, any case, a trailing $ or not). The controls: a template
// of 999%, and a name that is no boss's, are worth one.
func TestARaidBossIsWorthFive(t *testing.T) {
	for _, tc := range []struct {
		name string
		npc  NPC
		want int
	}{
		{"a template of 1,000%", NPC{Name: "Lord Nagafen", tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 1000}}}, HeroesBossKill},
		{"a template of 10,417%", NPC{Name: "Somebody", tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 10417}}}, HeroesBossKill},
		{"a template of 999%", NPC{Name: "Somebody", tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 999}}}, HeroesKill},
		{"a boss by the loot's tables", NPC{Name: "Skahyir"}, HeroesBossKill},
		{"the same in another case", NPC{Name: " SKAHYIR$ "}, HeroesBossKill},
		{"no boss", NPC{Name: "a froglok"}, HeroesKill},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newKillWorld(t, 60)
			heroesOn(k.w)
			if n := k.w.SetRaidBossNames([]string{"skahyir", "Lady Vox"}); n != 2 {
				t.Fatalf("%d boss names kept, want 2", n)
			}
			npc := tc.npc
			npc.ServerID, npc.Level, npc.X, npc.Z = 5003, 60, 1000, 1000
			k.kill(&npc)
			if got := k.standing(0) - FactionMin; got != tc.want {
				t.Errorf("worth %d, want %d", got, tc.want)
			}
			wantLine := "faction with Testing Heroes (+1)"
			if tc.want == HeroesBossKill {
				wantLine = "-1995 faction with Testing Heroes (+5)"
			}
			if got := heroLines(k.sess[0]); len(got) != 1 || !strings.Contains(got[0], wantLine) {
				t.Errorf("the line %q, want %q", got, wantLine)
			}
		})
	}
	if HeroesKill != 1 || HeroesBossKill != 5 || RaidBossHPPct != 1000 {
		t.Errorf("the petition's figures: %d a kill, %d a raid boss; the raid loot's %d%%", HeroesKill, HeroesBossKill, RaidBossHPPct)
	}
}

// Every member of a group the kill gives experience to is credited, once each;
// a member too far to share it, and one too low for the group's kills to be
// worth anything to it, get none.
func TestEachGroupMemberIsCreditedOnce(t *testing.T) {
	k := newKillWorld(t, 60, 60, 60, 30)
	heroesOn(k.w)
	far := k.players[2]
	far.X += KillShareRange + 50
	npc := &NPC{ServerID: 5004, Name: "a froglok", Level: 60, X: 1000, Z: 1000}
	if groupExp(60, 30, []int{60, 60, 30}) != 0 || groupExp(60, 60, []int{60, 60, 30}) == 0 {
		t.Fatal("no test: the low member shares the kill, or the others do not")
	}
	k.kill(npc)
	for i, want := range []int{1, 1, 0, 0} {
		if got := k.standing(i) - FactionMin; got != want {
			t.Errorf("member %d (level %d): credited %d, want %d", i, k.players[i].Level, got, want)
		}
		if got := len(heroLines(k.sess[i])); got != want {
			t.Errorf("member %d: %d lines, want %d", i, got, want)
		}
	}
}

// A raid group's kill, a fifth of the experience each, still credits every
// member near it one, and a raid boss five.
func TestARaidGroupIsCreditedEach(t *testing.T) {
	r := newRaidRig(t, 6, 60)
	heroesOn(r.w)
	r.form(6)
	r.drainAll()
	npc := r.killable(60)
	r.kill(0, npc)
	boss := r.killable(60)
	boss.ServerID, boss.tmpl = 9001, &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 3623}}
	r.kill(3, boss)
	for i, p := range r.p {
		r.w.mu.Lock()
		got := r.w.standingLocked(p, HeroesFaction) - FactionMin
		r.w.mu.Unlock()
		if got != HeroesKill+HeroesBossKill {
			t.Errorf("%s: credited %d, want %d", p.Name, got, HeroesKill+HeroesBossKill)
		}
		if l := heroLines(r.sess[i]); len(l) != 2 {
			t.Errorf("%s: lines %q, want two", p.Name, l)
		}
	}
}

// The standing stays within the factions' range: at the ceiling a kill holds
// it and the line says the standing held with the change asked, as every
// faction's does.
func TestTheTestingHeroesStandingHoldsAtTheCeiling(t *testing.T) {
	k := newKillWorld(t, 60)
	heroesOn(k.w)
	k.players[0].Factions = map[string]int{HeroesFaction: FactionMax - 1}
	npc := &NPC{ServerID: 5005, Name: "Somebody", Level: 60, X: 1000, Z: 1000, tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 2000}}}
	k.kill(npc)
	if got := k.standing(0); got != FactionMax {
		t.Fatalf("standing %d, want the ceiling %d", got, FactionMax)
	}
	if got := heroLines(k.sess[0]); len(got) != 1 || got[0] != "2000 faction with Testing Heroes (+5)" {
		t.Errorf("the line %q", got)
	}
}

// With the switch off nothing changes: no kill is credited, nothing is said,
// the faction window leaves the faction out though its row is in the table, a
// balance is no one's and a purchase takes nothing. Turned on, the same kill
// credits and the window lists the faction, at the floor for a newcomer.
func TestTheTestingHeroesAreOffUnlessSet(t *testing.T) {
	k := newKillWorld(t, 60)
	k.w.SetFactionDefaults(heroesFactions)
	npc := &NPC{ServerID: 5006, Name: "a froglok", Level: 60, X: 1000, Z: 1000}
	k.kill(npc)
	if len(k.players[0].Factions) != 0 || len(heroLines(k.sess[0])) != 0 {
		t.Fatalf("off: standings %v, lines %q", k.players[0].Factions, heroLines(k.sess[0]))
	}
	listed := func() (int, bool) {
		for _, f := range k.w.PlayerFactions(k.sess[0]) {
			if f.Name == HeroesFaction {
				return f.Standing, true
			}
		}
		return 0, false
	}
	if _, ok := listed(); ok {
		t.Error("off: the faction window lists the Testing Heroes")
	}
	if n := len(k.w.PlayerFactions(k.sess[0])); n != len(heroesFactions)-1 {
		t.Errorf("off: the window lists %d factions, want %d", n, len(heroesFactions)-1)
	}
	if _, _, ok := k.w.HeroesBalance(k.sess[0]); ok {
		t.Error("off: a balance")
	}
	if _, _, ok := k.w.HeroesSpend(k.sess[0], 1); ok {
		t.Error("off: a purchase took a standing")
	}
	if k.w.TestingHeroes() || k.w.HeroesOpen() {
		t.Error("off: reported on")
	}

	k.w.SetTestingHeroes(true)
	if v, ok := listed(); !ok || v != FactionMin {
		t.Errorf("on: the window lists it %v at %d, want at the floor", ok, v)
	}
	k.kill(npc)
	if v, ok := listed(); !ok || v != FactionMin+1 || k.standing(0) != FactionMin+1 {
		t.Errorf("on: after a kill the window says %d (%v), the standing %d", v, ok, k.standing(0))
	}
	// the window's order: standing high to low, so the floor's newcomer last
	list := k.w.PlayerFactions(k.sess[0])
	if list[len(list)-1].Name != HeroesFaction {
		t.Errorf("the window's last is %q", list[len(list)-1].Name)
	}
}

// On with no row of the faction in the table (the binary in before the data
// tool): nothing is credited, where a faction the table lacks would read as
// standing 0, 2000 to spend, and no balance is anyone's.
func TestTheTestingHeroesNeedTheirFaction(t *testing.T) {
	k := newKillWorld(t, 60)
	k.w.SetFactionDefaults(heroesFactions[:1])
	k.w.SetTestingHeroes(true)
	k.kill(&NPC{ServerID: 5007, Name: "a froglok", Level: 60, X: 1000, Z: 1000})
	if len(k.players[0].Factions) != 0 || len(heroLines(k.sess[0])) != 0 {
		t.Fatalf("no row: standings %v, lines %q", k.players[0].Factions, heroLines(k.sess[0]))
	}
	if b, s, ok := k.w.HeroesBalance(k.sess[0]); ok {
		t.Errorf("no row: a balance of %d at standing %d", b, s)
	}
	if k.w.HeroesOpen() {
		t.Error("no row: open")
	}
}

// A character may spend its standing above the floor and no more: a cost that
// would take it below the floor takes nothing, one that takes it to the floor
// exactly is paid, each payment told as a faction's change is, and a refund
// gives it back.
func TestSpendingStopsAtTheFloor(t *testing.T) {
	k := newKillWorld(t, 60)
	heroesOn(k.w)
	sess, p := k.sess[0], k.players[0]
	if b, s, ok := k.w.HeroesBalance(sess); !ok || b != 0 || s != FactionMin {
		t.Fatalf("a newcomer's balance %d at %d (%v), want 0 at the floor", b, s, ok)
	}
	if _, _, ok := k.w.HeroesSpend(sess, 1); ok {
		t.Fatal("a newcomer bought something")
	}
	p.Factions = map[string]int{HeroesFaction: FactionMin + 100}
	if b, s, ok := k.w.HeroesBalance(sess); !ok || b != 100 || s != -1900 {
		t.Fatalf("balance %d at %d (%v), want 100 at -1900", b, s, ok)
	}
	if before, after, ok := k.w.HeroesSpend(sess, 101); ok || before != -1900 || after != -1900 || p.Factions[HeroesFaction] != -1900 {
		t.Fatalf("101 of 100: paid %v, %d -> %d, standing %d", ok, before, after, p.Factions[HeroesFaction])
	}
	if got := heroLines(sess); len(got) != 0 {
		t.Fatalf("a refused payment said %q", got)
	}
	if before, after, ok := k.w.HeroesSpend(sess, 25); !ok || before != -1900 || after != -1925 {
		t.Fatalf("25 of 100: paid %v, %d -> %d", ok, before, after)
	}
	if before, after, ok := k.w.HeroesSpend(sess, 75); !ok || before != -1925 || after != FactionMin {
		t.Fatalf("75 of 75: paid %v, %d -> %d, want to the floor", ok, before, after)
	}
	if _, _, ok := k.w.HeroesSpend(sess, 0); ok {
		t.Error("a cost of nothing was paid")
	}
	if after, ok := k.w.HeroesRefund(sess, 75); !ok || after != -1925 {
		t.Errorf("a refund of 75: %d (%v), want -1925", after, ok)
	}
	want := []string{"-1925 faction with Testing Heroes (-25)", "-2000 faction with Testing Heroes (-75)", "-1925 faction with Testing Heroes (+75)"}
	if got := heroLines(sess); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines %q, want %q", got, want)
	}
}

// A client with Faction Messages off is credited and told nothing.
func TestTheTestingHeroesLinesFollowFactionMessages(t *testing.T) {
	k := newKillWorld(t, 60)
	heroesOn(k.w)
	k.sess[0].SetFactionMessagesOff(true)
	k.kill(&NPC{ServerID: 5008, Name: "a froglok", Level: 60, X: 1000, Z: 1000})
	if k.standing(0) != FactionMin+1 || len(heroLines(k.sess[0])) != 0 {
		t.Errorf("standing %d, lines %q", k.standing(0), heroLines(k.sess[0]))
	}
}

// The kills' own paths: a melee kill credits its killer, and a GM's #kill,
// which gives no experience, credits no one.
func TestAMeleeKillCreditsAndAGMsKillDoesNot(t *testing.T) {
	f := newFight(t)
	heroesOn(f.w)
	f.npc.HP = 1
	if groupExp(f.npc.Level, f.p.Level, []int{f.p.Level}) == 0 {
		t.Fatal("no test: the crab is worth nothing")
	}
	f.w.mu.Lock()
	killed := f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	if !killed || f.p.Factions[HeroesFaction] != FactionMin+1 {
		t.Fatalf("a melee kill (%v): standing %d, want %d", killed, f.p.Factions[HeroesFaction], FactionMin+1)
	}

	g := newFight(t)
	heroesOn(g.w)
	if name, ok := g.w.KillNPCByObjectID(g.sess, g.npcID); !ok || name != "Crab" {
		t.Fatalf("#kill: %q %v", name, ok)
	}
	if len(g.p.Factions) != 0 || len(heroLines(g.sess)) != 0 {
		t.Errorf("a GM's #kill: standings %v, lines %q", g.p.Factions, heroLines(g.sess))
	}
}
