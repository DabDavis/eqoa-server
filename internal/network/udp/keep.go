package udp

// keep.go — a restart of the server that keeps the players in the world
// (claude-notes/session-keep-restart-20261106.md; the user, 2026-11-06: "is
// there a way to keep folks in world and not boot with a restart", "lets
// implement").
//
// Why a restart booted everyone: while the process was down the game port was
// closed, so each client's next datagram was answered "port unreachable", and
// CLIENT's DRDP library ends the route on that at once (drdp.cpp 3709,
// drdp_route_terminate). The new process then knew none of the connections.
//
// So the process re-execs itself in place (the same PID: systemd sees no
// restart) with the game's UDP socket open across the exec, and nothing answers
// unreachable: datagrams wait in the socket. Before the exec it stops reading
// and ticking (Pause), saves everyone as a shutdown does (SaveAll), writes each
// session down (Snapshot: the connection's numbers and unacknowledged messages,
// the character's extras, the client's entity slots, the groups), and stops
// sending (frozen). The new process takes the socket (InheritedConn), loads
// the world as always, and before it reads a datagram puts each session back
// (ResumeKept): the character built from its row as a login builds it, the
// slots and groups as they were. Once a client has reported where it stands it
// is sent a MemoryDump there (ResumeDump), as a respawn is, which brings its
// copy of the character in line. CLIENT waits about 30 s for a silent server
// (connection_set_failure(m_conn, 1000, 30000, 60000), viclient.cpp 1707); a
// code-only restart takes ~5 s. Anything that cannot be put back is told the
// server went down (0x07C0) and goes to its login, as before.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// KeepVersion is the snapshot's format: a process reads only its own.
const KeepVersion = 1

// KeepMaxAge is the oldest snapshot a process resumes from: past it the
// clients have given up (CLIENT's 30 s).
const KeepMaxAge = 60 * time.Second

// The environment a process passes its next image: the socket's descriptor
// and the snapshot's file.
const (
	KeepFDEnv   = "EQOA_KEEP_FD"
	KeepFileEnv = "EQOA_KEEP_FILE"
)

const (
	// resumeReportWait is how long after a resumed client's first report
	// its dump goes: the reports it sent while the server was away come in
	// together, the newest last.
	resumeReportWait = 300 * time.Millisecond
	// resumeDumpMax is the longest a resumed client waits for its dump when
	// no report comes (one standing still sends few).
	resumeDumpMax = 2 * time.Second
)

// Snapshot is what a restart that keeps the sessions writes down.
type Snapshot struct {
	Version  int                `json:"version"`
	PID      int                `json:"pid"`
	Written  time.Time          `json:"written"`
	Sessions []KeptSession      `json:"sessions"`
	Groups   []entity.KeptGroup `json:"groups,omitempty"`
}

// KeptSession is one session in a Snapshot: its connection, and with a
// character the World's and the handlers' parts of it.
type KeptSession struct {
	Session session.Kept         `json:"session"`
	Player  *entity.KeptPlayer   `json:"player,omitempty"`
	View    *entity.KeptView     `json:"view,omitempty"`
	Extras  *handlers.KeptExtras `json:"extras,omitempty"`
}

// pauseState is one Pause: the read loop closes parked when it stops.
type pauseState struct {
	once   sync.Once
	parked chan struct{}
}

var pauses sync.Map // *Listener -> *pauseState

// Pause stops the listener reading and ticking without closing its socket,
// and returns once the read loop has stopped, every packet it read has been
// handled and no tick is running; or an error past wait.
func (l *Listener) Pause(wait time.Duration) error {
	ps := &pauseState{parked: make(chan struct{})}
	pauses.Store(l, ps)
	l.paused.Store(true)
	deadline := time.After(wait)
	if l.reading.Load() {
		select {
		case <-ps.parked:
		case <-deadline:
			return errors.New("the read loop did not stop")
		}
	}
	done := make(chan struct{})
	go func() { l.packets.Wait(); close(done) }()
	select {
	case <-done:
	case <-deadline:
		return errors.New("packets still being handled")
	}
	l.tickMu.Lock() // a tick running finishes; the next sees paused
	l.tickMu.Unlock()
	return nil
}

// Unpause lets a paused listener read, tick and send again.
func (l *Listener) Unpause() {
	l.frozen.Store(false)
	l.paused.Store(false)
}

// park is the read loop's stop while paused.
func (l *Listener) park() {
	if v, ok := pauses.Load(l); ok {
		ps := v.(*pauseState)
		ps.once.Do(func() { close(ps.parked) })
	}
	for l.paused.Load() {
		select {
		case <-l.ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Snapshot writes down every session worth keeping: one with an account,
// not on its way out (replaced, terminating, or link-dead after a quit). The
// listener is paused.
func (l *Listener) Snapshot(now time.Time) *Snapshot {
	snap := &Snapshot{Version: KeepVersion, PID: os.Getpid(), Written: now}
	seen := map[*session.Session]bool{}
	l.sessionManager.ForEach(func(sess *session.Session) bool {
		if seen[sess] {
			return true // filed under two keys (GetOrCreateSession's routing)
		}
		seen[sess] = true
		if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() || sess.AccountID() == 0 {
			return true
		}
		if _, quit := sess.QuittingSince(); quit {
			return true
		}
		ks := KeptSession{Session: sess.Keep()}
		if sess.InGame() && l.gameWorld != nil {
			kp, ok := l.gameWorld.KeepPlayer(sess, now)
			if !ok {
				return true // in the game with no character: nothing to put back
			}
			ks.Player = &kp
			ex := handlers.KeepExtras(sess, l.gameWorld.SessionPlayer(sess))
			ks.Extras = &ex
			if sess.CharacterInWorld() {
				v := l.gameWorld.KeepView(sess)
				ks.View = &v
			}
		}
		snap.Sessions = append(snap.Sessions, ks)
		return true
	})
	if l.gameWorld != nil {
		snap.Groups = l.gameWorld.KeepGroups()
	}
	return snap
}

// Write puts the snapshot in path, whole or not at all, readable by its owner
// alone (it holds addresses and account ids).
func (s *Snapshot) Write(path string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadSnapshot is the snapshot at path when this process's last image wrote
// it (its PID: an exec keeps it), in this format, and under KeepMaxAge old; the
// file is removed either way, so it is read once.
func ReadSnapshot(path string, now time.Time) (*Snapshot, error) {
	b, err := os.ReadFile(path)
	os.Remove(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case s.Version != KeepVersion:
		return nil, fmt.Errorf("%s: format %d, this server reads %d", path, s.Version, KeepVersion)
	case s.PID != os.Getpid():
		return nil, fmt.Errorf("%s: written by process %d, this is %d", path, s.PID, os.Getpid())
	case now.Sub(s.Written) > KeepMaxAge:
		return nil, fmt.Errorf("%s: written %v ago", path, now.Sub(s.Written).Round(time.Second))
	}
	return &s, nil
}

// KeepOptions is how a restart that keeps the sessions goes.
type KeepOptions struct {
	File string // the snapshot
	// Notice is said to every player in the world before it starts.
	Notice string
	// Hold and Saved are the swap's handshake (tools/swap_keep.sh): with Hold
	// there, Saved is written once everyone is saved and written down, and
	// the exec waits (at most HoldMax) until Hold is gone, while the swap
	// runs its data step and puts the new binary in place.
	Hold, Saved string
	HoldMax     time.Duration
	// Exec becomes the next image (syscall.Exec; a test's own).
	Exe  string
	Args []string
	Env  []string
	Exec func(exe string, args, env []string) error
}

// KeepRestart restarts the process keeping its sessions: the notice, the
// pause, everyone saved, the sessions written down, sending stopped, the
// socket made to survive the exec, and the exec. It returns only when it
// failed, the listener going on as it was (its sessions untouched: what was
// saved is saved).
func (l *Listener) KeepRestart(o KeepOptions) error {
	start := time.Now()
	if l.conn == nil {
		return errors.New("no game socket")
	}
	if o.Exec == nil || o.File == "" {
		return errors.New("no exec or no snapshot file")
	}
	f, fd, err := inheritable(l.conn)
	if err != nil {
		return fmt.Errorf("the game socket: %w", err)
	}
	if o.Notice != "" {
		log.Printf("Keep: %q told to %d players", o.Notice, l.tellInWorld(o.Notice))
	}
	if err := l.Pause(5 * time.Second); err != nil {
		l.Unpause()
		f.Close()
		return err
	}
	undo := func(err error) error {
		l.Unpause()
		f.Close()
		return err
	}
	l.SaveAll()
	snap := l.Snapshot(time.Now())
	l.frozen.Store(true)
	if err := snap.Write(o.File); err != nil {
		return undo(fmt.Errorf("the snapshot: %w", err))
	}
	in := 0
	for _, s := range snap.Sessions {
		if s.Session.CharacterInWorld {
			in++
		}
	}
	log.Printf("Keep: %d sessions (%d in the world, %d groups) written to %s, %v after the signal",
		len(snap.Sessions), in, len(snap.Groups), o.File, time.Since(start).Round(time.Millisecond))
	if o.Hold != "" {
		if _, err := os.Stat(o.Hold); err == nil {
			if o.Saved != "" {
				os.WriteFile(o.Saved, []byte(time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0o644)
			}
			holdUntil := time.Now().Add(o.HoldMax)
			for time.Now().Before(holdUntil) {
				if _, err := os.Stat(o.Hold); err != nil {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if _, err := os.Stat(o.Hold); err == nil {
				log.Printf("Keep: %s still there after %v: going on", o.Hold, o.HoldMax)
			}
		}
	}
	env := []string{KeepFDEnv + "=" + strconv.Itoa(fd), KeepFileEnv + "=" + o.File}
	for _, e := range o.Env {
		if !strings.HasPrefix(e, KeepFDEnv+"=") && !strings.HasPrefix(e, KeepFileEnv+"=") {
			env = append(env, e)
		}
	}
	log.Printf("Keep: becoming %s, the game socket as descriptor %d, %v after the signal", o.Exe, fd, time.Since(start).Round(time.Millisecond))
	err = o.Exec(o.Exe, o.Args, env)
	os.Remove(o.File)
	return undo(fmt.Errorf("exec %s: %w", o.Exe, err))
}

// tellInWorld says text to every player in the world and sends it at once.
func (l *Listener) tellInWorld(text string) int {
	var told []*session.Session
	l.sessionManager.ForEach(func(sess *session.Session) bool {
		if sess.CharacterInWorld() {
			told = append(told, sess)
		}
		return true
	})
	for _, sess := range told {
		handlers.SendClientMessage(sess, text)
		l.flushSession(sess)
	}
	return len(told)
}

// InheritedConn is the game socket the last image of this process handed over
// (KeepFDEnv), nil when there is none; the variable is cleared so no later
// image takes a descriptor it does not hold.
func InheritedConn() (*net.UDPConn, error) {
	v := os.Getenv(KeepFDEnv)
	os.Unsetenv(KeepFDEnv)
	if v == "" {
		return nil, nil
	}
	fd, err := strconv.Atoi(v)
	if err != nil || fd < 0 {
		return nil, fmt.Errorf("%s=%q", KeepFDEnv, v)
	}
	f := os.NewFile(uintptr(fd), "eqoa-game-udp")
	if f == nil {
		return nil, fmt.Errorf("%s=%d: no such descriptor", KeepFDEnv, fd)
	}
	defer f.Close()
	pc, err := net.FilePacketConn(f)
	if err != nil {
		return nil, fmt.Errorf("%s=%d: %w", KeepFDEnv, fd, err)
	}
	c, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return nil, fmt.Errorf("%s=%d is not a UDP socket", KeepFDEnv, fd)
	}
	return c, nil
}

// Adopt makes c the listener's socket: Start reads it rather than binding
// one (a restart's, InheritedConn).
func (l *Listener) Adopt(c *net.UDPConn) {
	l.conn = c
}

// ResumeKept puts the snapshot's sessions back, before the listener reads a
// datagram or ticks: each session under its key with its connection as it
// was; each character built from its row and given what was kept; then the
// groups, each client's entity slots, and its dump due once it reports (at
// once for one that was dead: it respawns). A session whose character cannot
// be built is told the server went down (0x07C0) and goes once that is sent.
// It returns how many sessions were put back and how many were told.
func (l *Listener) ResumeKept(snap *Snapshot, now time.Time) (resumed, told int) {
	type back struct {
		sess *session.Session
		ks   KeptSession
	}
	var in []back
	for _, ks := range snap.Sessions {
		sess, err := l.sessionManager.Resume(ks.Session)
		if err != nil {
			log.Printf("Keep: %v", err)
			continue
		}
		addr := sess.RemoteAddr()
		sess.SetSendFunc(func(data []byte) error { return l.SendTo(data, addr) })
		if ks.Session.InGame && ks.Player != nil {
			ex := handlers.KeptExtras{}
			if ks.Extras != nil {
				ex = *ks.Extras
			}
			if _, err := handlers.ResumeCharacter(sess, ex, *ks.Player, now); err != nil {
				log.Printf("Keep: %s ('%s'): %v; told the server went down", ks.Session.Addr, ks.Player.Name, err)
				handlers.SendDisconnect(sess, protocol.OpcodeServerUpdatingDC)
				sess.MarkReplaced() // kept only to deliver it (ReplacedGrace)
				told++
				continue
			}
		}
		in = append(in, back{sess, ks})
		resumed++
	}
	if l.gameWorld != nil && len(snap.Groups) > 0 {
		n, err := l.gameWorld.RestoreGroups(snap.Groups)
		if err != nil {
			log.Printf("Keep: groups: %v", err)
		}
		log.Printf("Keep: %d of %d groups formed again", n, len(snap.Groups))
	}
	for _, b := range in {
		if l.gameWorld != nil && b.ks.View != nil && b.ks.Session.CharacterInWorld {
			kept, gone := l.gameWorld.RestoreView(b.sess, *b.ks.View)
			log.Printf("Keep: %s ('%s'): %d entities kept in view, %d taken away", b.ks.Session.Addr, b.ks.Player.Name, kept, gone)
		}
		if b.ks.Session.CharacterInWorld && b.ks.Player != nil {
			wait := resumeDumpMax
			if b.ks.Player.Dead {
				wait = 0
			}
			b.sess.SetResumeDump(now.Add(wait), b.ks.Player.Dead)
		}
	}
	return resumed, told
}

// resumeDump sends a resumed session its dump (handlers.ResumeDump).
func (l *Listener) resumeDump(sess *session.Session, dead bool) {
	if !handlers.ResumeDump(sess, dead) {
		log.Printf("Keep: %s has no character for its dump", sess.RemoteAddr())
		return
	}
	what := "where it stands"
	if dead {
		what = "respawned home (it was dead)"
	}
	log.Printf("Keep: %s's dump sent, %s", sess.RemoteAddr(), what)
}
