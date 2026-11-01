// Command movereplay reads a game server's log (the receive loop's "UDP raw
// hex" lines) and writes every player's own state channel, 0x40, as the
// server reconstructed it, one line a message, with what the ground says of
// the place: the surfaces under and over it and the environment there. It is
// how the server's view of a player's moves (entity/movecheck.go) was
// measured against real players before it judged anyone.
//
//	movereplay -log gameserver.log[.gz] -ground worlds/ground > moves.tsv
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/network/udp"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

var (
	lineRE = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) (.*)$`)
	recvRE = regexp.MustCompile(`^UDP received \d+ bytes from (\S+)$`)
)

type conn struct {
	state *session.ClientStateData
}

func main() {
	logPath := flag.String("log", "", "the server's log (gzip read when it ends .gz)")
	dir := flag.String("ground", "", "the ground files (<world>.ground, .walls, .env)")
	judge := flag.Bool("judge", false, "run every report through the server's movement check (log mode) and print what it would not take")
	speed := flag.Float64("speed", 8.833333*1.3, "the run speed every player is taken to have (default: a Spirit of the Wolf's)")
	modeWord := flag.String("mode", "log", "with -judge, the movement check's mode: off (to time the update without it), log or enforce")
	cheat := flag.String("cheat", "", "with -judge, a control: hover (8 up), blink (30 aside once), under (6 down), fast (each step twice as long) in a stretch of the longest connection")
	flag.Parse()
	if *logPath == "" || *dir == "" {
		flag.Usage()
		os.Exit(2)
	}
	g, err := ground.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	env, err := ground.OpenEnv(*dir)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Open(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	var r io.Reader = f
	if strings.HasSuffix(*logPath, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			log.Fatal(err)
		}
		r = zr
	}
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	if *judge {
		mode, err := entity.ParseMoveCheckMode(*modeWord)
		if err != nil {
			log.Fatal(err)
		}
		runJudge(r, g, env, float32(*speed), *cheat, mode, out)
		return
	}
	fmt.Fprintln(out, "ts\tconn\tseq\tworld\tx\ty\tz\tvx\tvy\tvz\tfacing\tflags\tloco\taa\tbelowUp\tbelowAny\tabove\tnsurf\tenvC\tenvU\tenvD")
	conns := map[string]*conn{}
	var nPkt, nBad, nMsg, nOld, nNoBase, nShort int
	defer func() {
		log.Printf("packets %d (unparsed %d), 0x40s %d: old %d, no base %d, short %d", nPkt, nBad, nMsg, nOld, nNoBase, nShort)
	}()
	addr := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		m := lineRE.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		ts, rest := m[1], m[2]
		if rm := recvRE.FindStringSubmatch(rest); rm != nil {
			addr = rm[1]
			continue
		}
		hx, ok := strings.CutPrefix(rest, "UDP raw hex: ")
		if !ok || addr == "" {
			continue
		}
		data, err := hex.DecodeString(hx)
		if err != nil {
			continue
		}
		nPkt++
		pkt, err := udp.ParsePacket(data)
		if err != nil || pkt.Body == nil {
			nBad++
			continue
		}
		// a connection by its address and its two endpoints: a packet names
		// its instance only now and then
		key := fmt.Sprintf("%s/%04x-%04x", addr, pkt.GetLocalEndpoint(), pkt.GetRemoteEndpoint())
		c := conns[key]
		if c == nil || pkt.IsNewConnection() {
			c = &conn{state: session.NewConnectionData(nil, 0, 0).GetClientState()}
			conns[key] = c
		}
		for _, msg := range pkt.Body.Messages {
			if msg.Type != protocol.MessageTypeClientUpdate || len(msg.Data()) == 0 {
				continue
			}
			nMsg++
			if !c.state.Newer(msg.Sequence) {
				nOld++
				continue
			}
			b, ok := reconstruct(c.state, msg)
			if !ok {
				nNoBase++
				continue
			}
			c.state.Took(msg.Sequence)
			if len(b) < 41 {
				nShort++
				continue
			}
			row(out, g, env, ts, key, msg.Sequence, b)
		}
	}
	if err := sc.Err(); err != nil {
		log.Fatal(err)
	}
}

// reconstruct is processClientUpdate's XOR against the base the message
// names, the result kept as the next base.
func reconstruct(cs *session.ClientStateData, msg *protocol.Message) ([]byte, bool) {
	d := msg.Data()
	if msg.XORByte != 0 {
		base := cs.GetBase(msg.Sequence - uint16(msg.XORByte))
		if base == nil {
			return nil, false
		}
		n := max(len(d), len(base))
		out := make([]byte, n)
		for i := range out {
			var x, y byte
			if i < len(d) {
				x = d[i]
			}
			if i < len(base) {
				y = base[i]
			}
			out[i] = x ^ y
		}
		d = out
	} else {
		d = append([]byte(nil), d...)
	}
	cs.AddBase(msg.Sequence, d)
	return d, true
}

func u24(b []byte) uint32 { return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]) }

func row(out *bufio.Writer, g *ground.Ground, env *ground.Env, ts, key string, seq uint16, b []byte) {
	world := b[0]
	x := float32(36000.0*float64(u24(b[1:]))/float64(0xFFFFFF) - 4000.0)
	y := float32(2000.0*float64(u24(b[4:]))/float64(0xFFFFFF) - 1000.0)
	z := float32(36000.0*float64(u24(b[7:]))/float64(0xFFFFFF) - 4000.0)
	v, _ := entity.ClientUpdateVelocity(b)
	facing, _ := entity.ClientUpdateFacing(b)
	flags := binary.LittleEndian.Uint32(b[31:])
	// the surfaces from 40 under the feet to 40 over them
	surf := g.Surfaces(world, x, z, y-40, y+40)
	belowUp, belowAny, above := float32(math.NaN()), float32(math.NaN()), float32(math.NaN())
	for _, s := range surf {
		d := y - s.Y
		if d >= -0.25 {
			if s.Up && (math.IsNaN(float64(belowUp)) || d < belowUp) {
				belowUp = d
			}
			if math.IsNaN(float64(belowAny)) || d < belowAny {
				belowAny = d
			}
		} else if math.IsNaN(float64(above)) || -d < above {
			above = -d
		}
	}
	e := func(dy float32) int64 {
		f, ok := env.Flags(world, x, y+dy, z)
		if !ok {
			return -1
		}
		return int64(f)
	}
	fmt.Fprintf(out, "%s\t%s\t%d\t%d\t%.3f\t%.3f\t%.3f\t%.2f\t%.2f\t%.2f\t%.3f\t%#x\t%d\t%d\t%.3f\t%.3f\t%.3f\t%d\t%d\t%d\t%d\n",
		ts, key, seq, world, x, y, z, v[0], v[1], v[2], facing, flags, b[35], b[36],
		belowUp, belowAny, above, len(surf), e(1.2), e(1.3), e(1.1))
}
