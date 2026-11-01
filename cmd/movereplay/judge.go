package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/network/udp"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// report is one 0x40 as the log has it.
type report struct {
	sec  time.Time
	seq  uint16
	data []byte
}

// faultLog counts the judge's lines by their fault and keeps them.
type faultLog struct {
	out   *bufio.Writer
	kinds map[string]int
	conn  string
}

func (f *faultLog) Write(b []byte) (int, error) {
	line := string(b)
	if i := strings.Index(line, "MOVECHECK"); i >= 0 {
		line = line[i:]
		kind := "other"
		if j := strings.LastIndex(line, "flags "); j >= 0 {
			rest := line[j:]
			if k := strings.Index(rest, ": "); k >= 0 {
				kind = rest[k+2:]
				for _, cut := range []string{":", " (", " in "} {
					if c := strings.Index(kind, cut); c > 0 {
						kind = kind[:c]
					}
				}
			}
		}
		f.kinds[strings.TrimSpace(kind)]++
		fmt.Fprintf(f.out, "%s %s", f.conn, line)
	}
	return len(b), nil
}

// runJudge replays each connection's reports through a world of its own
// with the ground and the water, the movement check in log mode, the
// reports' times spread evenly through the second the log stamped them in.
// A report far from where the player is counts as a server move (a coach,
// a gate, a respawn: the log does not say), as does a change of world.
func runJudge(r io.Reader, g *ground.Ground, env *ground.Env, speed float32, cheat string, mode entity.MoveCheckMode, out *bufio.Writer) {
	conns := map[string][]report{}
	var order []string
	states := map[string]*session.ClientStateData{}
	addr := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		m := lineRE.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		if rm := recvRE.FindStringSubmatch(m[2]); rm != nil {
			addr = rm[1]
			continue
		}
		hx, ok := strings.CutPrefix(m[2], "UDP raw hex: ")
		if !ok || addr == "" {
			continue
		}
		data, err := hex.DecodeString(hx)
		if err != nil {
			continue
		}
		pkt, err := udp.ParsePacket(data)
		if err != nil || pkt.Body == nil {
			continue
		}
		key := fmt.Sprintf("%s/%04x-%04x", addr, pkt.GetLocalEndpoint(), pkt.GetRemoteEndpoint())
		cs := states[key]
		if cs == nil || pkt.IsNewConnection() {
			cs = session.NewConnectionData(nil, 0, 0).GetClientState()
			states[key] = cs
		}
		sec, _ := time.ParseInLocation("2006/01/02 15:04:05", m[1], time.UTC)
		for _, msg := range pkt.Body.Messages {
			if msg.Type != protocol.MessageTypeClientUpdate || len(msg.Data()) == 0 || !cs.Newer(msg.Sequence) {
				continue
			}
			b, ok := reconstruct(cs, msg)
			if !ok {
				continue
			}
			cs.Took(msg.Sequence)
			if len(b) < 41 {
				continue
			}
			if _, ok := conns[key]; !ok {
				order = append(order, key)
			}
			conns[key] = append(conns[key], report{sec, msg.Sequence, b})
		}
	}
	// the control's stretch: reports 2000-2300 of the longest connection
	longest := ""
	for _, k := range order {
		if len(conns[k]) > len(conns[longest]) {
			longest = k
		}
	}
	var took []time.Duration
	fl := &faultLog{out: out, kinds: map[string]int{}}
	log.SetOutput(fl)
	log.SetFlags(0)
	entity.SetMoveCheck(mode)
	total, moves := 0, 0
	for _, key := range order {
		rs := conns[key]
		fl.conn = key
		w := entity.NewWorld()
		w.SetGround(g)
		w.SetEnv(env)
		var p *entity.Player
		for i := 0; i < len(rs); {
			// the reports of one stamped second, spread through it
			j := i
			for j < len(rs) && rs[j].sec.Equal(rs[i].sec) {
				j++
			}
			for k := i; k < j; k++ {
				b := rs[k].data
				at := rs[k].sec.Add(time.Duration(float64(time.Second) * float64(k-i+1) / float64(j-i+1)))
				world := b[0]
				x := float32(36000.0*float64(u24(b[1:]))/float64(0xFFFFFF) - 4000.0)
				y := float32(2000.0*float64(u24(b[4:]))/float64(0xFFFFFF) - 1000.0)
				z := float32(36000.0*float64(u24(b[7:]))/float64(0xFFFFFF) - 4000.0)
				if key == longest && k >= 2000 && k < 2300 {
					switch cheat {
					case "hover":
						y += 8
					case "under":
						y -= 6
					case "blink":
						if k == 2000 {
							x += 30
						}
					case "fast":
						if k > 2000 {
							pb := rs[k-1].data
							px := float32(36000.0*float64(u24(pb[1:]))/float64(0xFFFFFF) - 4000.0)
							pz := float32(36000.0*float64(u24(pb[7:]))/float64(0xFFFFFF) - 4000.0)
							x, z = x+(x-px)*float32(k-2000), z+(z-pz)*float32(k-2000)
						}
					}
				}
				if p == nil {
					p = w.CreatePlayer(nil, &database.CharacterRecord{CharName: key, Level: 30, X: x, Y: y, Z: z, World: int(world)})
					p.Speed = speed
				}
				if p.World != world || math.Hypot(float64(x-p.X), float64(z-p.Z)) > 40 {
					p.World = world
					w.UpdatePosition(p.ServerID, x, y, z, 0) // a server move, as far as the log shows
					moves++
				}
				u := entity.ClientUpdate{X: x, Y: y, Z: z, World: world, HasWorld: true, At: at}
				u.WalkFlags, u.HasWalkFlags = entity.ClientUpdateWalkFlags(b)
				u.Animation = b[35]
				t0 := time.Now()
				w.ApplyClientUpdate(p.ServerID, u)
				took = append(took, time.Since(t0))
				total++
			}
			i = j
		}
	}
	log.SetOutput(io.Discard)
	fmt.Fprintf(out, "reports %d, server moves assumed %d\n", total, moves)
	if len(took) > 0 {
		sort.Slice(took, func(a, b int) bool { return took[a] < took[b] })
		var sum time.Duration
		for _, d := range took {
			sum += d
		}
		q := func(f float64) time.Duration { return took[int(f*float64(len(took)-1))] }
		fmt.Fprintf(out, "ApplyClientUpdate (mode %s): total %v, mean %v, median %v, p99 %v, p99.9 %v, max %v; ground zones read %d, walls zones read %d\n",
			entity.MoveCheck(), sum, sum/time.Duration(len(took)), q(0.5), q(0.99), q(0.999), took[len(took)-1], g.Loads, g.WallLoads)
	}
	for k, n := range fl.kinds {
		fmt.Fprintf(out, "  %6d  %s\n", n, k)
	}
}
