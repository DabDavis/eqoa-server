package handlers

import (
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// tools/sky_events2_20261114.py's triggers (its --dump) over sky-events': Desolation 1's
// patroller in the quarter of its placeholder, the sleeping Arbiter hidden while she fights
// and her first two lives corpses of nothing, Sun 2's wall opened for everyone by the talisman.
func skySet2(w *trigWorld) {
	w.t.Helper()
	skySet(w)
	py, _ := exec.LookPath("python3")
	out, err := exec.Command(py, "-I", filepath.Join("..", "..", "tools", "sky_events2_20261114.py"), "--dump").Output()
	if err != nil {
		w.t.Fatal(err)
	}
	var f struct {
		Triggers []struct {
			Slot   int             `json:"slot"`
			Name   string          `json:"name"`
			Scope  string          `json:"scope"`
			Radius float32         `json:"radius"`
			World  int             `json:"world"`
			X      float32         `json:"x"`
			Y      float32         `json:"y"`
			Z      float32         `json:"z"`
			Steps  json.RawMessage `json:"steps"`
		} `json:"triggers"`
		SQL [][]json.RawMessage `json:"sql"`
	}
	if err := json.Unmarshal(out, &f); err != nil {
		w.t.Fatal(err)
	}
	for _, t := range f.Triggers {
		w.setAt(t.Slot, t.Name, t.Scope, t.Radius, t.World, t.X, t.Y, t.Z, string(t.Steps), true)
	}
	// the set's own rewrites of sky-events' 212 and 213
	for _, q := range f.SQL {
		var stmt string
		json.Unmarshal(q[0], &stmt)
		if stmt != "UPDATE gmtriggers SET steps = ?, notes = ? WHERE slot = ?" {
			continue
		}
		var p []any
		json.Unmarshal(q[1], &p)
		if _, err := w.db.Exec(stmt, p...); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := w.e.Reload(); err != nil {
		w.t.Fatal(err)
	}
}

func TestSkyDeso1QuartersSleepAndLastLife(t *testing.T) {
	w := newTrigWorld(t)
	skySet2(w)
	scout := &entity.NPC{ServerID: 600002, Name: "erych scout", HP: 100, MaxHP: 100, Level: 63, X: 4800, Y: 578, Z: 6250, World: 4}
	w.gw.NPCs[scout.ServerID] = scout
	w.gp.World, w.gp.X, w.gp.Y, w.gp.Z = 4, 4760, 569, 6260
	w.e.StartNear() // the sleeping Arbiter
	sleeping := w.skyAlive("Erych Arbiter")
	if len(sleeping) != 1 || sleeping[0].NPCType&0x80 == 0 {
		t.Fatalf("the sleeping Arbiter: %+v", sleeping)
	}
	w.e.rand = func() float64 { return 0.01 }
	w.e.Killed(entity.KillCredit{NPCName: "erych scout", NPCID: scout.ServerID})
	p := w.skyAlive("erych patroller")
	if len(p) != 1 || math.Hypot(float64(p[0].X-4816.53), float64(p[0].Z-6252.7)) > 2 {
		t.Fatalf("the patroller of the east quarter: %+v", p)
	}
	for i := 0; i < 4; i++ {
		w.e.Killed(entity.KillCredit{NPCName: "erych scout", NPCID: scout.ServerID})
	}
	w.killAll("erych patroller")
	w.at(time.Second)
	w.killAll("erych guardian")
	w.at(2 * time.Second)
	clock := 2 * time.Second
	if !sleeping[0].HiddenUntil.After(w.now) {
		t.Fatal("the sleeping Arbiter is in view while she fights")
	}
	for life := 1; life <= 3; life++ {
		var fighting *entity.NPC
		for _, n := range w.skyAlive("Erych Arbiter") {
			if n.NPCType&0x80 == 0 {
				fighting = n
			}
		}
		if fighting == nil || fighting.NoLoot != (life < 3) {
			t.Fatalf("life %d: %+v", life, fighting)
		}
		w.kill(fighting)
		w.e.Killed(entity.KillCredit{NPCName: fighting.Name, NPCID: fighting.ServerID})
		clock += time.Second
		w.at(clock)
		clock += 11 * time.Second
		w.at(clock)
	}
	if sleeping[0].HiddenUntil.After(w.now) {
		t.Fatal("the sleeping Arbiter still hidden after the ring")
	}
}
