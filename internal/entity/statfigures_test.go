package entity

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The stat window's offence and defence figures (statfigures.go, petition 2925).

// figSets is the bodies of every OpcodeStatFiguresData queued for the rig's
// session, read as id -> value; the queue drained.
func (r *keepRig) figSets(t *testing.T) []map[uint8]int32 {
	t.Helper()
	var out []map[uint8]int32
	for _, m := range r.sess.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 || opOfMsg(d) != protocol.OpcodeStatFiguresData {
			continue
		}
		b := d[2:]
		if len(b) < 1 || len(b) != 1+5*int(b[0]) {
			t.Fatalf("a set of %d bytes: %x", len(b), b)
		}
		set := map[uint8]int32{}
		for i := range int(b[0]) {
			e := b[1+5*i:]
			set[e[0]] = int32(binary.LittleEndian.Uint32(e[1:5]))
		}
		out = append(out, set)
	}
	return out
}

// pass runs the figures' system at its tick.
func (r *keepRig) figPass() {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount = statFiguresTicks
	r.w.statFiguresSystem()
}

// reportRanges is what #stats says of the player: its hit's low and high and
// a creature's hit's on it.
func (r *keepRig) reportRanges(t *testing.T) (lo, hi, dlo, dhi int32) {
	t.Helper()
	lines := strings.Join(r.w.StatsReport(r.sess, 0, time.Now()), "\n")
	n := func(re string) (int32, int32) {
		m := regexp.MustCompile(re).FindStringSubmatch(lines)
		if m == nil {
			t.Fatalf("no %q in\n%s", re, lines)
		}
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		return int32(a), int32(b)
	}
	lo, hi = n(`creature (-?\d+)-(-?\d+) (?:a hit|an arrow)`)
	dlo, dhi = n(`hits for (-?\d+)-(-?\d+)`)
	return
}

// Nothing goes to a client that never asked (a PS2); at the ask the set goes
// at once, the figures #stats states (melee ids 1 and 2, defence 5 and 6);
// nothing while they stay; again when the weapon changes, and with a ranged
// weapon the arrow's (ids 3 and 4); nothing once it stops.
func TestStatFiguresGoToAClientThatAsked(t *testing.T) {
	r := newKeepRig(t, "a gnoll pup", false)
	r.p.Gear.WeaponDamage, r.p.Gear.WeaponCond = 20, 1
	r.figPass()
	if sets := r.figSets(t); len(sets) != 0 {
		t.Fatalf("sent %v to a client that never asked", sets)
	}
	if !r.w.SetStatFigures(r.sess, true) {
		t.Fatal("no player")
	}
	r.figPass()
	sets := r.figSets(t)
	lo, hi, dlo, dhi := r.reportRanges(t)
	want := map[uint8]int32{StatFigMeleeLow: lo, StatFigMeleeHigh: hi, StatFigDefenceLow: dlo, StatFigDefenceHigh: dhi}
	if len(sets) != 1 || len(sets[0]) != 4 || sets[0][1] != lo || sets[0][2] != hi || sets[0][5] != dlo || sets[0][6] != dhi {
		t.Fatalf("at the ask %v, want one %v (#stats')", sets, want)
	}
	if lo > hi || dlo > dhi {
		t.Fatalf("ranges upside down: %v", sets[0])
	}
	r.figPass()
	if sets := r.figSets(t); len(sets) != 0 {
		t.Fatalf("unchanged, sent again: %v", sets)
	}
	r.p.Gear.WeaponDamage = 40
	r.figPass()
	sets = r.figSets(t)
	if lo2, hi2, _, _ := r.reportRanges(t); len(sets) != 1 || sets[0][1] != lo2 || sets[0][2] != hi2 || hi2 <= hi {
		t.Fatalf("a bigger weapon: %v, #stats %d-%d (was %d-%d)", sets, lo2, hi2, lo, hi)
	}
	r.p.Gear.WeaponSlot = 18
	r.figPass()
	sets = r.figSets(t)
	alo, ahi, _, _ := r.reportRanges(t)
	if len(sets) != 1 || len(sets[0]) != 4 || sets[0][3] != alo || sets[0][4] != ahi {
		t.Fatalf("a ranged weapon: %v, want 3: %d 4: %d", sets, alo, ahi)
	}
	if _, ok := sets[0][StatFigMeleeLow]; ok {
		t.Fatal("melee and ranged both sent")
	}
	r.w.SetStatFigures(r.sess, false)
	r.p.Gear.WeaponDamage = 60
	r.figPass()
	if sets := r.figSets(t); len(sets) != 0 {
		t.Fatalf("stopped, sent %v", sets)
	}
}

// A keep restart keeps the ask, and the set goes again at the first pass; a
// takeover's new client asks again.
func TestStatFiguresKeptOverARestart(t *testing.T) {
	now := time.Now()
	old := newKeepRig(t, "a gnoll pup", false)
	old.w.SetStatFigures(old.sess, true)
	kp, ok := old.w.KeepPlayer(old.sess, now)
	if !ok || !kp.StatFigures {
		t.Fatalf("kept %+v", kp)
	}
	r, sess := through(t, old, "a gnoll pup")
	if !r.w.RestoreKeptPlayer(sess, kp, now) || !r.p.StatFigures {
		t.Fatal("not restored")
	}
	r.figPass()
	if sets := r.figSets(t); len(sets) != 1 {
		t.Fatalf("after the restart %v, want the set", sets)
	}
	r.w.mu.Lock()
	r.w.handedOverLocked(r.p)
	r.w.mu.Unlock()
	if r.p.StatFigures {
		t.Fatal("a takeover kept the old client's ask")
	}
}
