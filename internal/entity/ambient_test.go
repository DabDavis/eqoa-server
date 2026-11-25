package entity

import (
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

func TestBusyAmbientReadLeavesWorldTicksFree(t *testing.T) {
	r := newAmbientRig(t)
	r.player("Near", 1000, 1020)
	started, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	r.w.SetAmbientLines(func() ([]database.NPCDialogLine, error) {
		close(started)
		<-release
		return nil, errors.New("old reader")
	})
	now := time.Now()
	returned := make(chan struct{})
	go func() { r.w.Tick(now); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("a blocked dialogue read blocked the game tick")
	}
	<-started
	old := r.w.ambient.pending
	r.w.Tick(now.Add(ambientEvery))
	if r.w.ambient.pending != old {
		t.Fatal("another read started before the first returned")
	}
	r.w.SetAmbientLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{ambientRow(1, r.npc.Name, database.DialogUseAmbient, "Here.")}, nil
	})
	close(release)
	<-old.done
	r.w.runAmbient(now.Add(2 * ambientEvery))
	fresh := r.w.ambient.pending
	if fresh == nil || fresh == old {
		t.Fatal("replacement reader did not start")
	}
	<-fresh.done
	r.w.runAmbient(now.Add(2*ambientEvery + time.Millisecond))
	if r.w.ambient.speakers[r.npc.ServerID] == nil {
		t.Fatal("fresh dialogue was not applied on the world tick")
	}
}

// ambientRig is a world with one speaker in a library and the players added near or far.
type ambientRig struct {
	t    *testing.T
	w    *World
	mgr  *session.Manager
	npc  *NPC
	next int
}

func newAmbientRig(t *testing.T) *ambientRig {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	npc := &NPC{ServerID: 50, Name: "Researcher Taruna", NPCType: 0x80, X: 1000, Z: 1000, HP: 100, MaxHP: 100}
	w.NPCs[npc.ServerID] = npc
	w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	return &ambientRig{t: t, w: w, mgr: mgr, npc: npc}
}

func (r *ambientRig) player(name string, x, z float32) *session.Session {
	r.next++
	id := EntityID(1000 + r.next)
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 7, byte(r.next)), Port: 7100 + r.next}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, Name: name, Level: 10, HP: 100, MaxHP: 100, X: x, Z: z}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, x, z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	return sess
}

func ambientRow(id int, name, use, line string) database.NPCDialogLine {
	return database.NPCDialogLine{ID: id, Name: name, Line: line, Weight: 1, Use: use, Race: -1, SpeakerRace: -1,
		Class: -1, NotRace: -1}
}

// An NPC with an ambient line and a player 20 away says it, as a say ("Name: line"), within AmbientMax and not
// before AmbientMin; a player 150 off (within a say's reach) hears it too; the next comes AmbientMin to AmbientMax
// later and is not the same line when another fits. The controls: with no player within AmbientNear nothing is
// said, ever; a talk row of its name is never said unprompted; a player loading a world hears nothing.
func TestAnNPCSaysItsAmbientLinesUnprompted(t *testing.T) {
	lines := []database.NPCDialogLine{
		ambientRow(1, "Researcher Taruna", database.DialogUseAmbient, "All scribes are to catalogue these volumes. Be sure it is done."),
		ambientRow(2, "Researcher Taruna", database.DialogUseAmbient, "Ah ha! Here it is..."),
		ambientRow(3, "Researcher Taruna", database.DialogUseTalk, "Welcome to the library."),
	}
	t0 := time.Now() // the session's world load is marked on the clock

	// nobody near: nothing, however long
	r := newAmbientRig(t)
	far := r.player("Far", 1000, 1100) // 100 off: past AmbientNear, within a say's reach
	for d := time.Duration(0); d <= 2*AmbientMax; d += 5 * time.Second {
		if n := r.w.AmbientSay(lines, t0.Add(d)); n != 0 {
			t.Fatalf("with no one within %v: %d lines said", AmbientNear, n)
		}
	}
	if got := ownerLines(far); len(got) != 0 {
		t.Fatalf("with no one near, the far player heard %q", got)
	}

	near := r.player("Near", 1000, 1020)
	if n := r.w.AmbientSay(lines, t0); n != 0 {
		t.Fatalf("at once: %d lines said, want its wait first", n)
	}
	if n := r.w.AmbientSay(lines, t0.Add(AmbientMin-time.Second)); n != 0 {
		t.Fatalf("before AmbientMin: %d lines said", n)
	}
	if n := r.w.AmbientSay(lines, t0.Add(AmbientMax)); n != 1 {
		t.Fatalf("by AmbientMax: %d lines said, want 1", n)
	}
	first := ownerLines(near)
	if len(first) != 1 || !slices.Contains([]string{"Researcher Taruna: " + lines[0].Line, "Researcher Taruna: " + lines[1].Line}, first[0]) {
		t.Fatalf("the near player heard %q, want one of her ambient lines", first)
	}
	if got := ownerLines(far); !slices.Equal(got, first) {
		t.Errorf("the player 100 off heard %q, want %q (a say's reach)", got, first)
	}
	// the next: not before AmbientMin, by AmbientMax, the other line
	if n := r.w.AmbientSay(lines, t0.Add(AmbientMax+AmbientMin-time.Second)); n != 0 {
		t.Fatalf("the second before AmbientMin after the first: %d", n)
	}
	if n := r.w.AmbientSay(lines, t0.Add(2*AmbientMax)); n != 1 {
		t.Fatalf("the second by AmbientMax after the first: %d", n)
	}
	second := ownerLines(near)
	if len(second) != 1 || second[0] == first[0] {
		t.Errorf("the second line %q after %q: want the other", second, first)
	}

	// a talk row alone is never said unprompted
	r2 := newAmbientRig(t)
	s2 := r2.player("Near", 1000, 1020)
	for d := time.Duration(0); d <= 2*AmbientMax; d += 5 * time.Second {
		r2.w.AmbientSay(lines[2:], t0.Add(d))
	}
	if got := ownerLines(s2); len(got) != 0 {
		t.Errorf("a talk row alone: heard %q", got)
	}

	// a player loading a world hears nothing, and brings no one on
	r3 := newAmbientRig(t)
	r3.w.ambient.speakers = map[EntityID]*ambientSpeaker{r3.npc.ServerID: {next: t0}} // her time has come
	s3 := r3.player("Loading", 1000, 1020)
	s3.HoldStateChannels(time.Minute)
	for d := time.Duration(0); d < ambientLoadMost-5*time.Second; d += 5 * time.Second {
		r3.w.AmbientSay(lines, t0.Add(d))
	}
	if got := ownerLines(s3); len(got) != 0 {
		t.Errorf("a player loading a world heard %q", got)
	}
}

// A talk picks only a talk's rows (npcdialog.use empty): an NPC with an ambient and a shop row of its own and one
// talk row says the talk row, every time; with no talk row, it says nothing of its own name (the control: its
// shop greeting is ShopLine's).
func TestATalkPicksOnlyTalkRows(t *testing.T) {
	r := newAmbientRig(t)
	sess := r.player("Talker", 1000, 1010)
	p := sess.ECSEntity().(*Player)
	lines := []database.NPCDialogLine{
		ambientRow(1, "Researcher Taruna", database.DialogUseAmbient, "Ah ha! Here it is..."),
		ambientRow(2, "Researcher Taruna", database.DialogUseShop, "Books for sale."),
		ambientRow(3, "Researcher Taruna", database.DialogUseTalk, "Welcome to the library."),
	}
	for i := 0; i < 20; i++ {
		_, line, _, ok := r.w.TownLine(p, r.npc, lines, 0)
		if !ok || line != "Welcome to the library." {
			t.Fatalf("talk %d: %q %v, want the talk row", i, line, ok)
		}
	}
	if _, line, _, ok := r.w.TownLine(p, r.npc, lines[:2], 0); ok {
		t.Errorf("no talk row: said %q", line)
	}
	if line, ok := r.w.ShopLine(p, r.npc, lines); !ok || line != "Books for sale." {
		t.Errorf("ShopLine: %q %v", line, ok)
	}
}
