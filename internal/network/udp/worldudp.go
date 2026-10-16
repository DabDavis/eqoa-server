package udp

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/eqoa/server/internal/worlds"
)

// World ESF delivery over UDP, for the in-game HDD stager.
//
// WHY UDP AND NOT THE HTTP ROUTE NEXT DOOR
//
// The HTTP route (internal/network/tcp/worldserver.go) is driven by the
// PATCHER's own HTTP client, and that client is gone by the time our code
// exists. The SS stub applies S2/SD from a CLIENT hook, so the payload's first
// instruction runs at the moment CLIENT loads — and loading CLIENT is what
// unloads the patcher module. Measured in one boot: at the Update screen
// 0x003E5F58 is still retail and the ctor at 0x005DC568 is code; after pressing
// Play the hook is installed and the ctor is data, `modules` 6 -> 4. Not a
// timing problem to be tuned — the two events are the same event.
//
// What IS always resident is SLUS (never unloads) and SUPPORT (fixed base, the
// module our three VIFile hooks already patch). Between them they hold a whole
// socket API and, better, a client data block already bound to the IOP's INET
// RPC server:
//
//	cd  = 0x004E3890                    SifRpcClientData, bound, [cd+0x30]=2048
//	buf = [0x004E3800]                  the 2 KB RPC send/recv buffer
//	sceInetCreate (cd,buf,params)              -> handle   0x00124CD0
//	sceInetOpen   (cd,buf,h,-1)                            0x00124D40
//	sceInetSendTo (cd,buf,h,p,n,0,addr,port,t)             0x00125058
//	sceInetRecv   (cd,buf,h,dst,n,0,timeout)   -> bytes    0x00124DD0
//	sceInetClose  (cd,buf,h,-1)                            0x00124D88
//
// Proved from cold guest calls with no patcher resident: a datagram left the
// PS2 and a 64-byte reply came back into EE RAM byte-exact.
//
// So the transport is ours to define, and UDP costs the stager far less than
// TCP would: no connect, no HTTP request to format, no header to parse, no
// Content-Length arithmetic. The whole EE side is "patch two words into a baked
// request, sendto, recv, sceWrite".
//
// [cd+0x30] = 2048 is a hard ceiling on one RPC transfer, and sceInetRecv can
// ask for at most that minus 8. MaxChunk stays well under both, and under the
// Ethernet MTU, so a reply is never fragmented.

const (
	// ReqSize is fixed so the stager can bake the whole request as a template
	// and only patch the offset and length words before sending.
	ReqSize = 48
	// ReplyHeader precedes the payload in every data reply.
	ReplyHeader = 16
	// MaxChunk is the largest payload the stager may ask for. A request above
	// it is REFUSED, never clamped: the stager sizes its buffer before asking,
	// and it reads a short reply as end-of-file. Clamping would hand it a
	// truncated world of plausible length.
	MaxChunk = 1400
	// NameLen is the filename field, NUL-padded, at the end of a request.
	NameLen = ReqSize - 12
)

var (
	reqMagic   = [4]byte{'E', 'Q', 'W', 'R'}
	replyMagic = [4]byte{'E', 'Q', 'W', 'D'}
	errMagic   = [4]byte{'E', 'Q', 'W', 'E'}
)

// WorldServer answers world-chunk requests from the in-game stager.
type WorldServer struct {
	dir  string
	port int

	conn *net.UDPConn
	wg   sync.WaitGroup

	mu     sync.Mutex
	closed bool

	// OnStatus receives EQPREP outcomes reported by the payload, keyed by the
	// console's IP. Set by the game server, which is the only thing that knows
	// which session that is; nil here simply logs, so the world protocol keeps
	// working on a server that has not wired the technician up.
	OnStatus func(ip string, rep StatusReport)
}

// NewWorldServer builds a server over the configured worlds directory. The
// directory is resolved now so the log line names what will actually be served.
func NewWorldServer(worldsDir string, port int) *WorldServer {
	return &WorldServer{dir: worlds.Dir(worldsDir), port: port}
}

// Start binds the socket and serves in the background.
func (s *WorldServer) Start() error {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: s.port}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("world UDP listen on %d: %w", s.port, err)
	}
	s.conn = conn
	log.Printf("World server (UDP) listening on :%d, serving %s", s.port, s.dir)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.serve()
	}()
	return nil
}

// Stop closes the socket and waits for the serve loop to finish.
func (s *WorldServer) Stop() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.wg.Wait()
}

func (s *WorldServer) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *WorldServer) serve() {
	req := make([]byte, ReqSize+64) // slack, so an oversized datagram is seen as oversized
	for {
		n, from, err := s.conn.ReadFromUDP(req)
		if err != nil {
			if s.isClosed() {
				return
			}
			log.Printf("World server (UDP): read: %v", err)
			continue
		}
		// A report, not a request: it is answered by talking to the player
		// over the GAME session, not by a datagram, so nothing is written back.
		if rep, ok := ParseStatus(req[:n]); ok {
			if s.OnStatus != nil {
				s.OnStatus(from.IP.String(), rep)
			} else {
				log.Printf("World server (UDP): status from %v with no handler: %+v",
					from.IP, rep)
			}
			continue
		}
		reply := s.Handle(req[:n])
		if reply == nil {
			continue // not ours; stay silent rather than answer a stray packet
		}
		if _, err := s.conn.WriteToUDP(reply, from); err != nil {
			log.Printf("World server (UDP): write to %v: %v", from, err)
		}
	}
}

// Handle turns one request datagram into one reply, or nil to say nothing at
// all. Split out from serve so the wire format can be tested without a socket.
func (s *WorldServer) Handle(req []byte) []byte {
	// Anything that is not exactly our request shape gets no reply. This port
	// is reachable from the internet and a reply to a spoofed source is a
	// reflection amplifier; staying silent on garbage keeps the only
	// amplifying path behind a well-formed request.
	if len(req) != ReqSize || !hasMagic(req, reqMagic) {
		return nil
	}
	offset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])
	name := trimNUL(req[12 : 12+NameLen])

	if !worlds.SafeName(name) {
		return errorReply(offset)
	}
	if length > MaxChunk {
		return errorReply(offset)
	}

	path := filepath.Join(s.dir, name)
	f, err := os.Open(path)
	if err != nil {
		return errorReply(offset)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return errorReply(offset)
	}
	total := fi.Size()

	// length 0 is the size probe the stager sends before deciding whether to
	// download at all: it answers "how big is the finished file", which is what
	// the on-disk size has to match. It means the SD does not have to have the
	// size baked into it at build time.
	if length == 0 || int64(offset) >= total {
		return dataReply(offset, total, nil)
	}

	n := int64(length)
	if int64(offset)+n > total {
		n = total - int64(offset)
	}
	payload := make([]byte, n)
	if _, err := f.ReadAt(payload, int64(offset)); err != nil {
		return errorReply(offset)
	}
	return dataReply(offset, total, payload)
}

func hasMagic(b []byte, m [4]byte) bool {
	return len(b) >= 4 && b[0] == m[0] && b[1] == m[1] && b[2] == m[2] && b[3] == m[3]
}

func trimNUL(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func dataReply(offset uint32, total int64, payload []byte) []byte {
	out := make([]byte, ReplyHeader+len(payload))
	copy(out[0:4], replyMagic[:])
	binary.LittleEndian.PutUint32(out[4:8], offset)
	binary.LittleEndian.PutUint32(out[8:12], uint32(len(payload)))
	binary.LittleEndian.PutUint32(out[12:16], uint32(total))
	copy(out[ReplyHeader:], payload)
	return out
}

// errorReply is distinguishable from a legitimate empty data reply on purpose.
// An end-of-file reply and a "there is no such world" reply mean opposite
// things to the stager: one finishes the download, the other must abort it and
// leave the short file behind for the next boot to retry.
func errorReply(offset uint32) []byte {
	out := make([]byte, ReplyHeader)
	copy(out[0:4], errMagic[:])
	binary.LittleEndian.PutUint32(out[4:8], offset)
	return out
}

// ---------------------------------------------------------------------------
// The status channel.
//
// The world protocol is the payload asking for bytes. This is the payload
// ANSWERING: after it runs EQPREP for the Freeport technician it reports what
// happened, so the NPC can say so instead of "working…".
//
// It rides the same socket and the same 48-byte datagram size, because the
// payload already has code that builds and sends exactly that — a second size
// or a second port would be a second thing to get wrong on the console, where
// getting it wrong is silent.
// ---------------------------------------------------------------------------

// statusMagic marks a report rather than a request.
var statusMagic = [4]byte{'E', 'Q', 'W', 'S'}

// StatusReport is one EQPREP outcome, exactly as the console saw it.
type StatusReport struct {
	Verb   uint32 // which command produced it (1 check, 2 create, 3 format)
	Result uint32 // EQPREP's raw return word, 0x51<step><detail>01
	ID     int32  // sceSifLoadStartModule's return: negative means it never ran
	Token  uint64 // the nonce the command carried; 0 = it carried none
}

// TokenBits is the width of the nonce the payload echoes back, and the reason
// it is not 64: it travels to the console as UTF-16 code units carrying 12
// usable bits each (see CMD_TOKN in hdd_sd.S), and comes back as two 24-bit
// words. Everything that generates or compares one has to agree on the width,
// so it is named once here.
const TokenBits = 48

// tokenWordMax is the largest value either half can legitimately hold. The
// payload builds both from 12-bit chunks, so anything wider did not come from
// our encoder.
const tokenWordMax = 1<<24 - 1

// ParseStatus decodes a status datagram. Split out from serve for the same
// reason Handle is: the wire format is testable without a socket.
//
// A ZERO TOKEN IS NOT AN ERROR. It is what a console running a payload older
// than the nonce sends, and what our own payload sends when the command it
// acted on carried no token — the server falls back to matching the report
// against the recorded ask. Halves that are out of range are read the same
// way, because a token we cannot have issued is worth exactly as much as none.
func ParseStatus(req []byte) (StatusReport, bool) {
	if len(req) != ReqSize || !hasMagic(req, statusMagic) {
		return StatusReport{}, false
	}
	rep := StatusReport{
		Verb:   binary.LittleEndian.Uint32(req[4:8]),
		Result: binary.LittleEndian.Uint32(req[8:12]),
		ID:     int32(binary.LittleEndian.Uint32(req[12:16])),
	}
	hi := binary.LittleEndian.Uint32(req[16:20])
	lo := binary.LittleEndian.Uint32(req[20:24])
	if hi <= tokenWordMax && lo <= tokenWordMax {
		rep.Token = uint64(hi)<<24 | uint64(lo)
	}
	return rep, true
}

// PrepMagic is the byte EQPREP stamps its return word with. Anything else means
// the module did not run its own code — modload's own error, not a disk result.
const PrepMagic = 0x51

// PrepStep pulls the step out of a return word, and PrepDetail the errno that
// goes with a failure. Steps below 0x10 are successes.
func PrepStep(result uint32) int   { return int((result >> 16) & 0xFF) }
func PrepDetail(result uint32) int { return int((result >> 8) & 0xFF) }

// prepStepNames is the same table the setup scene prints, kept here because the
// server owns the wording — the payload deliberately reports the word RAW so a
// step it has never heard of still arrives intact.
//
// Every step eqprep.c defines must appear here, and TestPrepStepNamesCoverEQPREP
// reads the header to prove it. That test exists because the first successful
// in-game run returned OK_ABSENT (0x04) and the player was told "which this
// server does not know" — a correct answer the server could not say. A missing
// SUCCESS is the bad case: `Describe` sends failures through a numbered
// fallback that is at least legible, while an unknown success reads as a fault.
var prepStepNames = map[int]string{
	0x00: "the world partition is already there and mountable",
	0x01: "the world partition has been created and formatted",
	0x02: "the disk was formatted, and the world partition created",
	0x03: "the world partition was re-formatted",
	0x04: "the disk is ready, but the world partition is not there yet - " +
		"ask me to create it",
	0x05: "the world partition is there and the game is using it right now - " +
		"nothing needs doing",
	0x10: "the partition name or size was rejected",
	0x11: "there is NO HARD DISK in this console",
	0x12: "the disk is locked to another console",
	0x13: "the disk is not formatted - it needs FORMAT DISK first",
	0x14: "the disk status could not be read",
	0x15: "formatting the disk failed",
	0x16: "a system partition on the disk would not format",
	0x17: "there is not enough free space on the disk for the world partition",
	0x18: "the world partition was created but could not be formatted",
	0x19: "the world partition was created and formatted, but will not mount",
	0x1A: "the world partition is already there but will not mount - it needs " +
		"re-formatting, which erases what is in it",
	0x1B: "the game has the world partition open, so I left it alone - leave " +
		"this world first, then ask me again",
	0x20: "the game is using the world partition right now - leave this world " +
		"first, then ask me again",
}

// PrepSynthetic is the first step the PAYLOAD produces rather than EQPREP. They
// are refusals to run the module at all, so they carry no errno in <detail> and
// must not be decorated with one — "(error 255)" on a deliberate refusal reads
// as a fault in the thing that refused.
const PrepSynthetic = 0x20

// Describe turns a report into one sentence a player can act on.
func (r StatusReport) Describe() string {
	if r.ID < 0 {
		return fmt.Sprintf("the disk tool would not load (%d) - the console is "+
			"missing EQPREP.IRX", r.ID)
	}
	if (r.Result>>24)&0xFF != PrepMagic {
		return fmt.Sprintf("the disk tool returned %#08x, which is not its own "+
			"answer - it did not run", r.Result)
	}
	step := PrepStep(r.Result)
	name, known := prepStepNames[step]
	if !known {
		return fmt.Sprintf("the disk tool reported step %#02x (%#08x), which "+
			"this server does not know", step, r.Result)
	}
	if step >= PrepSynthetic {
		return name
	}
	if step >= 0x10 {
		return fmt.Sprintf("%s (error %d)", name, PrepDetail(r.Result))
	}
	return name
}
