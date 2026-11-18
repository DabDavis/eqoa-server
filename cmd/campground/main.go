// Command campground answers tools/xp_camps_20261118.py's questions of the ground, with the ground package's own
// floors and walls as the server's creatures and the movement check use them (internal/ground: Stand, Floor,
// Embedded, Free, Wall; internal/entity/groundnpc.go's inGroundLocked and liftOutLocked; movecheck.go's body).
//
// It reads JSON lines on stdin and writes one JSON line for each:
//
//	{"op":"check","k":<key>,"w":<world>,"x":..,"y":..,"z":..}
//	    a spawn as the server sets it down: stand (Stand from its height: the floor within the step over it, the
//	    terrain's or anything placed), floor (the terrain's alone), inground (no floor under it and the terrain's rock
//	    over it: inGroundLocked), lift (liftOutLocked's floor over it, the lowest), embedded (its body in something
//	    placed: the movement check's ball), free (Free's nearest place it stands clear, when embedded) and env (what
//	    a body's middle is in: water 1, slime 2, lava 4, mud 8)
//	{"op":"spot","k":<key>,"w":<world>,"x":..,"z":..,"hint":..,"fx":..,"fy":..,"fz":..}
//	    a place for a new spawn near hint (the height of the camp's creature it copies, at fx,fy,fz): its floor
//	    within reach of hint, whether it is roofed, embedded, wet (env at the body's middle) and whether a wall (the
//	    terrain's or placed) stands between the copied creature and it at a body's middle
//
//	campground -ground worlds/ground < questions.jsonl > answers.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/eqoa/server/internal/ground"
)

// as internal/entity: groundnpc.go's floorStepUp and groundOver, movecheck.go's mcMiddle, mcBall and mcClear
const (
	floorStepUp float32 = 2
	groundOver  float32 = 400
	mcMiddle    float32 = 1.2
	mcBall      float32 = 0.45
	mcClear     float32 = 0.75
	spotReach   float32 = 12 // a new spawn's floor within this of the copied creature's height
)

type question struct {
	Op   string  `json:"op"`
	K    any     `json:"k"`
	W    uint8   `json:"w"`
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Z    float32 `json:"z"`
	Hint float32 `json:"hint"`
	FX   float32 `json:"fx"`
	FY   float32 `json:"fy"`
	FZ   float32 `json:"fz"`
}

type answer struct {
	K        any         `json:"k"`
	Knows    bool        `json:"knows"`
	Stand    *float32    `json:"stand,omitempty"`
	Placed   bool        `json:"placed,omitempty"`
	Roofed   bool        `json:"roofed,omitempty"`
	Floor    *float32    `json:"floor,omitempty"`
	InGround bool        `json:"inground,omitempty"`
	Lift     *float32    `json:"lift,omitempty"`
	Embedded bool        `json:"embedded,omitempty"`
	Free     *[3]float32 `json:"free,omitempty"`
	Env      uint32      `json:"env"`
	Walled   bool        `json:"walled,omitempty"`
}

func ptr(v float32) *float32 { return &v }

func main() {
	dir := flag.String("ground", "worlds/ground", "the ground files' directory")
	flag.Parse()
	g, err := ground.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(*dir)
	if err != nil {
		log.Fatal(err)
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)
	for in.Scan() {
		var q question
		if err := json.Unmarshal(in.Bytes(), &q); err != nil {
			log.Fatalf("%s: %v", in.Bytes(), err)
		}
		a := answer{K: q.K, Knows: g.Knows(q.W)}
		switch q.Op {
		case "check":
			if h, placed, roofed, ok := g.Stand(q.W, q.X, q.Y, q.Z); ok {
				a.Stand, a.Placed, a.Roofed = ptr(h), placed, roofed
			}
			if f, ok := g.Floor(q.W, q.X, q.Y, q.Z); ok {
				a.Floor = ptr(f)
			}
			if a.Stand == nil {
				if _, over := g.Floor(q.W, q.X, q.Y+groundOver, q.Z); over {
					a.InGround = true
					for up := floorStepUp; up <= groundOver; up += floorStepUp {
						if f, ok := g.Floor(q.W, q.X, q.Y+up, q.Z); ok && f > q.Y {
							a.Lift = ptr(f)
							break
						}
					}
				}
			}
			if g.Embedded(q.W, q.X, q.Y, q.Z, mcMiddle, mcBall) {
				a.Embedded = true
				if fx, fy, fz, _, ok := g.Free(q.W, q.X, q.Y, q.Z, mcMiddle, mcClear); ok {
					a.Free = &[3]float32{fx, fy, fz}
				}
			}
			a.Env, _ = env.Flags(q.W, q.X, q.Y+mcMiddle, q.Z)
		case "spot":
			h, placed, roofed, ok := g.Stand(q.W, q.X, q.Hint+spotReach-ground.StepUp, q.Z)
			if ok && h >= q.Hint-spotReach {
				a.Stand, a.Placed, a.Roofed = ptr(h), placed, roofed
				a.Embedded = g.Embedded(q.W, q.X, h, q.Z, mcMiddle, mcClear)
				a.Env, _ = env.Flags(q.W, q.X, h+mcMiddle, q.Z)
				a.Walled = g.Wall(q.W, q.FX, q.FY+mcMiddle, q.FZ, q.X, h+mcMiddle, q.Z)
			}
		default:
			log.Fatalf("op %q", q.Op)
		}
		if err := enc.Encode(a); err != nil {
			log.Fatal(err)
		}
	}
	if err := in.Err(); err != nil {
		log.Fatal(err)
	}
}
