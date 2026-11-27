package entity

import (
	"strings"
	"testing"
	"time"
)

func TestTestScrollOfReturningKeepAndExpiry(t *testing.T) {
	r := newDungeons(t, 1)
	r.w.SetDungeons(false, 14, nil, DungeonSettings{})
	p, sess := r.ps[0], r.ss[0]
	marked := GateSpot{World: 1, X: 6310, Y: 57, Z: 1210, Place: "the marked spot"}
	home := GateSpot{World: 0, X: 5000, Y: 50, Z: 5004}
	r.w.PlaceFacing(p.ServerID, marked.X, marked.Y, marked.Z, marked.World, 0.75)
	fail := func(GateSpot) bool { return false }
	if res, line := r.w.TestReturnRead(sess, r.now, &home, fail); res != ReturnOut {
		t.Fatalf("read out: %v %q", res, line)
	}
	r.land()
	kept := r.w.KeepDungeons(r.now)
	if kept == nil || len(kept.TestReturns) != 1 {
		t.Fatalf("keep lost the mark with dungeons off: %+v", kept)
	}
	r.w.testReturns = nil
	r.w.RestoreDungeons(kept)
	if res, line := r.w.TestReturnRead(sess, r.now.Add(2*time.Second), &home, fail); res != ReturnRefused || !strings.Contains(line, "3 sec") {
		t.Fatalf("kept cooldown: %v %q", res, line)
	}
	if res, _ := r.w.TestReturnRead(sess, r.now.Add(5*time.Second), &home, fail); res != ReturnRefused || len(r.land()) != 0 {
		t.Fatal("failed spend queued a return")
	}
	if res, line := r.w.TestReturnRead(sess, r.now.Add(5*time.Second), &home, func(to GateSpot) bool {
		return to == marked
	}); res != ReturnBack {
		t.Fatalf("failed spend lost the mark or started a cooldown: %v %q", res, line)
	}
	due := r.land()
	if len(due) != 1 || due[0].To != marked || !due[0].HasFacing || due[0].Facing != 0.75 {
		t.Fatalf("marked location and facing: %+v", due)
	}
	if res, _ := r.w.TestReturnRead(sess, r.now.Add(10*time.Second), &home, fail); res != ReturnOut {
		t.Fatal("could not mark again")
	}
	r.land()
	late := r.now.Add(DungeonReturnWindow + time.Minute)
	if kept := r.w.KeepDungeons(late); kept != nil {
		t.Fatalf("kept an expired mark: %+v", kept)
	}
	if res, _ := r.w.TestReturnRead(sess, late, &home, fail); res != ReturnOut {
		t.Fatal("an expired trip prevented the next mark")
	}
}

func TestTestScrollOfReturningKeepsDungeonMarksSeparate(t *testing.T) {
	r := newDungeons(t, 1)
	p, sess := r.ps[0], r.ss[0]
	if why, ok := r.w.DungeonEnter(sess, "guk", r.now); !ok {
		t.Fatal(why)
	}
	r.land()
	inst := p.Instance
	home := GateSpot{World: 0, X: 5000, Y: 50, Z: 5004}
	spend := func(GateSpot) bool { return true }
	if res, line := r.w.TestReturnRead(sess, r.now, &home, spend); res != ReturnOut {
		t.Fatalf("heroic departure: %v %q", res, line)
	}
	r.land()
	if res, _ := r.w.DungeonReturnRead(sess, r.now.Add(5*time.Second)); res != ReturnRefused {
		t.Fatal("the original scroll redeemed the test scroll's mark")
	}
	if res, line := r.w.TestReturnRead(sess, r.now.Add(5*time.Second), &home, spend); res != ReturnBack || p.Instance != inst {
		t.Fatalf("heroic return: %v %q, instance %d", res, line, p.Instance)
	}
}

func TestTestScrollOfReturningRefusesHiddenDungeons(t *testing.T) {
	r := newMythic(t, 1)
	r.enter(t, 0)
	home := GateSpot{World: 0, X: 5000, Y: 50, Z: 5004}
	res, line := r.w.TestReturnRead(r.ss[0], r.now, &home, func(GateSpot) bool {
		t.Error("refused scroll was spent")
		return true
	})
	if res != ReturnRefused || line != returnLineNotHere || len(r.w.testReturns) != 0 || len(r.w.gates.due) != 0 {
		t.Fatalf("hidden dungeon scroll result %v %q, marks %d, trips %d", res, line, len(r.w.testReturns), len(r.w.gates.due))
	}
}
