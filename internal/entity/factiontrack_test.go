package entity

import (
	"encoding/binary"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// trackSent is one OpcodeFactionTrackData read back.
type trackSent struct {
	known    bool
	name     string
	standing int
}

// trackData is every OpcodeFactionTrackData queued for sess so far.
func trackData(t *testing.T, sess *session.Session) []trackSent {
	t.Helper()
	var out []trackSent
	for _, m := range sess.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeFactionTrackData {
			continue
		}
		r := buffer.NewReader(d[2:])
		layout, _ := r.ReadByte()
		known, _ := r.ReadByte()
		n, _ := r.ReadUint32()
		u := make([]uint16, n)
		for i := range u {
			u[i], _ = r.ReadUint16()
		}
		v, err := r.Read7BitEncodedInt64()
		if err != nil || layout != FactionTrackLayout {
			t.Fatalf("data unreadable: % x", d)
		}
		out = append(out, trackSent{known == 1, string(utf16.Decode(u)), int(v)})
	}
	return out
}

// pass runs the tracker's pass as a tick on its beat would.
func trackPass(w *World) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TickCount = (w.TickCount/factionTrackTicks + 1) * factionTrackTicks
	w.factionTrackSystem()
}

// A client that names a faction (in any case, or by a part only it holds) is
// sent its standing at the next pass, then nothing while it holds; a kill's
// hit moves it and it is sent again, once. The control: a client that named
// none is sent nothing, before or after the kill.
func TestAFollowedFactionIsSentAsItChanges(t *testing.T) {
	f := newGuardFight(t)
	trackPass(f.w)
	if got := trackData(t, f.sess); len(got) != 0 {
		t.Fatalf("followed nothing, sent %v", got)
	}
	if name, ok := f.w.SetFactionTrack(f.sess, "freeport militia"); !ok || name != "The Freeport Militia" {
		t.Fatalf("named %q %v", name, ok)
	}
	trackPass(f.w)
	trackPass(f.w)
	if got := trackData(t, f.sess); len(got) != 1 || got[0] != (trackSent{true, "The Freeport Militia", 1}) {
		t.Fatalf("after the answer: %v, want one send of the start (1)", got)
	}
	f.w.mu.Lock()
	f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	trackPass(f.w)
	trackPass(f.w)
	got := trackData(t, f.sess)
	if len(got) != 2 || got[1] != (trackSent{true, "The Freeport Militia", -49}) {
		t.Fatalf("after the kill: %v, want -49 sent once", got)
	}
	// a GM's set is followed too
	f.w.SetFaction(f.sess, "The Freeport Militia", 700)
	trackPass(f.w)
	if got := trackData(t, f.sess); len(got) != 3 || got[2].standing != 700 {
		t.Fatalf("after a GM's set: %v", got)
	}
	// none: nothing more
	f.w.SetFactionTrack(f.sess, "")
	f.w.SetFaction(f.sess, "The Freeport Militia", 5)
	trackPass(f.w)
	if got := trackData(t, f.sess); len(got) != 3 {
		t.Fatalf("followed none, still sent: %v", got)
	}
}

// Words that name no faction, or several, are sent back unknown, standing 0,
// as asked; the Testing Heroes' faction is unknown on a server without them.
func TestAnUnknownFactionIsSaidSo(t *testing.T) {
	f := newFight(t)
	for _, asked := range []string{"zzz", "citizens", HeroesFaction} {
		if name, _ := f.w.SetFactionTrack(f.sess, asked); name != "" {
			t.Errorf("%q named %q", asked, name)
		}
		trackPass(f.w)
		got := trackData(t, f.sess)
		if last := got[len(got)-1]; last != (trackSent{false, asked, 0}) {
			t.Errorf("%q: sent %v", asked, last)
		}
	}
}

// A keep restart carries the words followed, and the standing is sent again at
// the resume's first pass.
func TestAFollowedFactionSurvivesAKeepRestart(t *testing.T) {
	f := newFight(t)
	f.w.SetFactionTrack(f.sess, "Citizens of Freeport")
	k, ok := f.w.KeepPlayer(f.sess, time.Now())
	if !ok || k.FactionTrack != "Citizens of Freeport" {
		t.Fatalf("kept %q %v", k.FactionTrack, ok)
	}
	f.w.SetFactionTrack(f.sess, "")
	if !f.w.RestoreKeptPlayer(f.sess, k, time.Now()) {
		t.Fatal("not restored")
	}
	if got := f.w.FactionTrack(f.sess); got != "Citizens of Freeport" {
		t.Fatalf("restored %q", got)
	}
	trackPass(f.w)
	if got := trackData(t, f.sess); len(got) != 1 || got[0].name != "Citizens of Freeport" || !got[0].known {
		t.Fatalf("sent %v", got)
	}
}
