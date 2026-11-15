package entity

import (
	"fmt"
	"testing"
	"time"
)

// critRig is the template rig's Gnoll Brute at level 40 (Attack4: a round every
// 6 s, hits at 0.5, 1, 1.7 and 2.4 s) on a template whose Critical Hit is ready
// every 10 s, its first 0 s in and no jitter (the Plane of Sky twins' clock: work
// 1339), 7.4 a level (296 at 40).
type critRig struct {
	*templateRig
	brute *NPC
}

func newCritRig(t *testing.T) *critRig {
	t.Helper()
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, special, special_damage, special_every, special_first, special_jitter)
		VALUES ('twin', 0, 'Critical Hit', 7.4, 10, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll brute", "twin")
	b := r.npc("Gnoll Brute")
	b.Level, b.HP, b.MaxHP = 40, 100000, 100000
	b.OffMod, b.WeaponDamage = NPCOffMod(40), NPCWeaponDamage(40)
	return &critRig{templateRig: r, brute: b}
}

// fightLog is what a session saw of the brute: its round swings' times (an
// Attack4's 0x00B1), its crits' (the special's 0x00B1), and the times of the
// brute's 0x00DBs on the player, with the crit's figure apart.
type fightLog struct {
	swings, crits, hits, critHits []time.Duration
}

func (l *fightLog) take(r *critRig, d time.Duration, msgs [][]byte) {
	src := r.objectID(r.brute.ServerID)
	for _, c := range castsIn(msgs) {
		if c.caster != src {
			continue
		}
		switch c.fx {
		case templateSpecialFX:
			l.crits = append(l.crits, d)
		case HashResourceID("Attack4"):
			l.swings = append(l.swings, d)
		}
	}
	for _, f := range damageIndications(msgs) {
		if f[0] != int64(src) {
			continue
		}
		if f[1] == -296 {
			l.critHits = append(l.critHits, d)
		} else {
			l.hits = append(l.hits, d)
		}
	}
}

func ds(ms ...int) string {
	out := make([]time.Duration, len(ms))
	for i, m := range ms {
		out[i] = time.Duration(m) * time.Millisecond
	}
	return fmt.Sprint(out)
}

// The owner's example (work 1340): a 6 s round and a 10 s crit, in continuous
// melee, pick crit, quad, crit, quad, crit at round starts 0, 6, 12, 18 and 24 s:
// each crit lands alone at the round's impact (0.5 s in) for its fixed 296, the
// crit round sends no swing and lands none of the quad's hits, and the quads are
// whole (four hits at 0.5, 1, 1.7 and 2.4 s). The crit ready at 10 s and at 22 s
// waits for the next round start. The fight opens with the crit.
func TestACritReplacesARoundAndOpensTheFight(t *testing.T) {
	r := newCritRig(t)
	p, sess := r.player("Hero", 2001.5, 100)
	p.AC = 1763
	r.fight(r.brute, p.ServerID)
	var l fightLog
	for d := time.Duration(0); d < 30*time.Second; d += 100 * time.Millisecond {
		p.HP = p.MaxHP
		r.tick(d)
		l.take(r, d, drain(sess))
	}
	if got, want := fmt.Sprint(l.crits), ds(500, 12500, 24500); got != want {
		t.Errorf("crits at %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(l.critHits), ds(500, 12500, 24500); got != want {
		t.Errorf("the crit's 296 at %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(l.swings), ds(6000, 18000); got != want {
		t.Errorf("quad swings at %s, want %s (none at a crit round's start)", got, want)
	}
	if got, want := fmt.Sprint(l.hits), ds(6500, 7000, 7700, 8400, 18500, 19000, 19700, 20400); got != want {
		t.Errorf("normal hits at %s, want %s (none in a crit round)", got, want)
	}
}

// The same rounds on a player's pet, through its own combat entry (npcFightPetLocked):
// the fight opens with the crit on the pet (296 off it), and the crits and quads go as
// on a player.
func TestACritRoundOnAPet(t *testing.T) {
	r := newCritRig(t)
	owner, sess := r.player("Owner", 2030, 100)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 40, HP: 1000000, MaxHP: 1000000, X: 2001.5, Y: 10, Z: 100, Owner: owner.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	r.fight(r.brute, pet.ServerID)
	var l fightLog
	hp := pet.HP
	var critTaken []int
	for d := time.Duration(0); d < 30*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		before := len(l.crits)
		l.take(r, d, drain(sess))
		if len(l.crits) > before {
			critTaken = append(critTaken, hp-pet.HP)
		}
		hp = pet.HP
	}
	if got, want := fmt.Sprint(l.crits), ds(500, 12500, 24500); got != want {
		t.Errorf("crits on the pet at %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(l.swings), ds(6000, 18000); got != want {
		t.Errorf("quad swings at the pet at %s, want %s", got, want)
	}
	for i, n := range critTaken {
		if n != 296 {
			t.Errorf("crit %d took %d off the pet, want 296", i, n)
		}
	}
}

// Every new engagement opens with the crit: a fight ended (its enemy gone, the
// fight forgotten) and begun again at once, mid-way through the crit's 10 s,
// opens with the crit again. A target changed inside the fight is no new
// engagement: the crit's clock runs on, and its new round is a quad.
func TestEveryEngagementOpensWithTheCrit(t *testing.T) {
	r := newCritRig(t)
	p, sess := r.player("Hero", 2001.5, 100)
	q, qsess := r.player("Other", 2001.5, 101)
	r.fight(r.brute, p.ServerID)
	var l, lq fightLog
	for d := time.Duration(0); d < 4*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		l.take(r, d, drain(sess))
	}
	// the fight ends: no enemy, the next tick puts it away; a new one begins
	r.brute.CurrentEnemy, r.brute.Threats = 0, nil
	r.tick(4 * time.Second)
	drain(sess)
	r.fight(r.brute, p.ServerID)
	for d := 4100 * time.Millisecond; d < 7*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		l.take(r, d, drain(sess))
	}
	if got, want := fmt.Sprint(l.crits), ds(500, 4600); got != want {
		t.Errorf("crits at %s, want %s (the second fight's opening at 4.6 s)", got, want)
	}
	// the target changes at 7 s, 3 s into the crit's 10: the new round is a quad
	drain(qsess) // what it saw of the fight with the other
	r.fight(r.brute, q.ServerID)
	for d := 7 * time.Second; d < 12*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		lq.take(r, d, drain(qsess))
	}
	if len(lq.crits) != 0 || fmt.Sprint(lq.swings) != ds(7000) {
		t.Errorf("after the target change: crits %v, swings %v; want none and the quad at 7 s", lq.crits, lq.swings)
	}
	// its clock ran on: ready at 14.1 s (10 s after 4.1 s), the round at 13 s a quad, 19 s the crit
	for d := 12 * time.Second; d < 20*time.Second; d += 100 * time.Millisecond {
		q.HP = q.MaxHP
		r.tick(d)
		lq.take(r, d, drain(qsess))
	}
	if got, want := fmt.Sprint(lq.crits), ds(19500); got != want {
		t.Errorf("crits on the new target at %s, want %s", got, want)
	}
}

// A crit round whose impact goes by out of reach loses the crit unlanded and the
// crit is ready again: the next round in reach is the crit.
func TestALostCritIsReadyAgain(t *testing.T) {
	r := newCritRig(t)
	p, sess := r.player("Hero", 2001.5, 100)
	r.fight(r.brute, p.ServerID)
	var l fightLog
	for d := time.Duration(0); d < 3*time.Second; d += 100 * time.Millisecond {
		if d == 300*time.Millisecond {
			r.brute.Effects = append(r.brute.Effects, &ActiveEffect{Name: "Root", Root: true, Until: r.at(5 * time.Second)})
			p.X = r.brute.X + 15 // out of its reach before the crit's impact
		}
		r.tick(d)
		l.take(r, d, drain(sess))
	}
	if len(l.crits) != 0 || len(l.critHits) != 0 {
		t.Fatalf("a crit landed from out of reach: %v %v", l.crits, l.critHits)
	}
	p.X = r.brute.X + 1.5
	for d := 5 * time.Second; d < 8*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		l.take(r, d, drain(sess))
	}
	if got, want := fmt.Sprint(l.crits), ds(6500); got != want {
		t.Errorf("crits at %s, want %s: the round back in reach (6 s) is the crit", got, want)
	}
}
