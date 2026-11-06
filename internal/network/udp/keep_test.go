//go:build unix

package udp

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// keepListener is a listener on a socket of its own with one lobby session
// (an account, no character: the handlers' part needs none), reading.
func keepListener(t *testing.T) (*Listener, *session.Manager, *net.UDPConn) {
	t.Helper()
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	sm := session.NewManager()
	cfg := config.DefaultConfig()
	l := NewListener(cfg, sm, nil, entity.NewWorld())
	l.Adopt(server)
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Stop)
	s := sm.CreateSession(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061, 0x2EC7D061, 0x1234, 0xFFFE, false)
	s.SetAccountID(110)
	s.SetCharListSent(true)
	for i := 0; i < 5; i++ {
		s.Connection().LastSentPacketSeq()
	}
	return l, sm, client
}

// The game socket goes across the exec: the copy handed over has its
// close-on-exec flag cleared, its number is in the next image's environment
// with the snapshot's file, and the next image (here a second listener in
// this process, given only that environment) takes the socket by that number,
// reads the snapshot back, and gets the session again under its key with its
// numbers, and the datagrams a client sent while nothing read them.
func TestTheGameSocketAndTheSessionsGoAcrossTheExec(t *testing.T) {
	l, _, client := keepListener(t)
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	var env []string
	execed := make(chan struct{})
	o := KeepOptions{File: file, Exe: "/bin/true", Env: os.Environ(), Exec: func(exe string, args, e []string) error {
		env = e
		close(execed)
		select {} // an exec does not return
	}}
	go l.KeepRestart(o)
	select {
	case <-execed:
	case <-time.After(5 * time.Second):
		t.Fatal("no exec")
	}
	if !l.frozen.Load() || !l.paused.Load() {
		t.Fatal("still reading or sending at the exec")
	}
	var fdv, filev string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, KeepFDEnv+"="); ok {
			fdv = v
		}
		if v, ok := strings.CutPrefix(e, KeepFileEnv+"="); ok {
			filev = v
		}
	}
	if fdv == "" || filev != file {
		t.Fatalf("the next image's environment: %s=%q %s=%q", KeepFDEnv, fdv, KeepFileEnv, filev)
	}
	var fd int
	for _, c := range fdv {
		fd = fd*10 + int(c-'0')
	}
	flags, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	if e != 0 || flags&syscall.FD_CLOEXEC != 0 {
		t.Fatalf("descriptor %d: flags %#x (%v): it would close at the exec", fd, flags, e)
	}

	// a client's datagram while no process reads the socket
	if _, err := client.WriteToUDP([]byte("waiting"), l.conn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}

	// the next image
	t.Setenv(KeepFDEnv, fdv)
	conn, err := InheritedConn()
	if err != nil || conn == nil {
		t.Fatalf("InheritedConn: %v %v", conn, err)
	}
	defer conn.Close()
	if os.Getenv(KeepFDEnv) != "" {
		t.Fatal("the variable is left for a later image")
	}
	if conn.LocalAddr().String() != l.conn.LocalAddr().String() {
		t.Fatalf("the socket handed over is %s, the game's %s", conn.LocalAddr(), l.conn.LocalAddr())
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the snapshot is left to be read again")
	}
	sm2 := session.NewManager()
	l2 := NewListener(config.DefaultConfig(), sm2, nil, entity.NewWorld())
	l2.Adopt(conn)
	if resumed, told := l2.ResumeKept(snap, time.Now()); resumed != 1 || told != 0 {
		t.Fatalf("resumed %d, told %d", resumed, told)
	}
	s := sm2.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061)
	if s == nil || s.AccountID() != 110 || !s.CharListSent() || !s.Resumed() {
		t.Fatalf("the session after the exec: %+v", s)
	}
	if got := s.Connection().LastSentPacketSeq(); got != 6 {
		t.Fatalf("its next packet %d, want 6", got)
	}
	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, from, err := conn.ReadFromUDP(buf)
	if err != nil || string(buf[:n]) != "waiting" || from.String() != client.LocalAddr().String() {
		t.Fatalf("the datagram sent across the exec: %q from %v (%v)", buf[:n], from, err)
	}
}

// An exec that fails leaves the server as it was: reading, ticking and
// sending again, its sessions untouched, no snapshot left behind; what was
// saved is saved.
func TestAFailedExecLeavesTheServerRunning(t *testing.T) {
	l, sm, client := keepListener(t)
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	err := l.KeepRestart(KeepOptions{File: file, Exe: "/nonexistent", Exec: func(string, []string, []string) error {
		if _, err := os.Stat(file); err != nil {
			t.Errorf("no snapshot at the exec: %v", err)
		}
		return errors.New("no such file")
	}})
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("KeepRestart: %v", err)
	}
	if l.paused.Load() || l.frozen.Load() {
		t.Fatal("still paused or frozen")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the snapshot is left behind")
	}
	if sm.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061) == nil {
		t.Fatal("the session is gone")
	}
	if err := l.SendTo([]byte("hello"), client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, _, err := client.ReadFromUDP(buf); err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("the listener does not send again: %q %v", buf[:n], err)
	}
}

// A snapshot is read once, and only by this process's next image, in its
// format, within KeepMaxAge: the controls for each.
func TestASnapshotIsReadOnlyFresh(t *testing.T) {
	dir := t.TempDir()
	write := func(s Snapshot) string {
		p := filepath.Join(dir, "k.json")
		if err := s.Write(p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	now := time.Now()
	good := Snapshot{Version: KeepVersion, PID: os.Getpid(), Written: now}
	if _, err := ReadSnapshot(write(good), now); err != nil {
		t.Fatalf("a fresh one: %v", err)
	}
	for name, s := range map[string]Snapshot{
		"another process's": {Version: KeepVersion, PID: os.Getpid() + 1, Written: now},
		"another format's":  {Version: KeepVersion + 1, PID: os.Getpid(), Written: now},
		"a stale one":       {Version: KeepVersion, PID: os.Getpid(), Written: now.Add(-KeepMaxAge - time.Second)},
	} {
		p := write(s)
		if _, err := ReadSnapshot(p, now); err == nil {
			t.Errorf("%s was read", name)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was left to be read again", name)
		}
	}
}
