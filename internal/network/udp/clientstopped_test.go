package udp

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A session in the world whose client is silent session.ClientStoppedSilence
// with no logout is said to have stopped, once, naming its character, and
// kept for its next crash report; heard from again it was hung. The
// controls: one silent a little less, one whose world is loading (a PS2 reads
// one off the disc in up to 22 s) and one that quit are said nothing.
func TestAClientThatStopsInTheWorldIsSaid(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r, sess, _ := slotsTick(t, 4)
	sess.SetInGame(true)
	sess.SetCharacterName("Lonewulf")
	sess.SetCharacterServerID(136)
	said := func() int {
		return strings.Count(out.String(), "'Lonewulf''s client stopped in the world with no logout")
	}
	sess.SetLastActivity(time.Now().Add(-session.ClientStoppedSilence + 5*time.Second))
	r.TickSession(sess)
	if said() != 0 {
		t.Fatalf("a client silent under %v was said to stop:\n%s", session.ClientStoppedSilence, out.String())
	}
	sess.HoldStateChannels(time.Minute) // its world loading
	sess.SetLastActivity(time.Now().Add(-session.ClientStoppedSilence - time.Second))
	r.TickSession(sess)
	if said() != 0 {
		t.Fatal("a client loading its world was said to stop")
	}
	sess.ReleaseStateChannels()
	sess.SetLastActivity(time.Now().Add(-session.ClientStoppedSilence - time.Second))
	r.TickSession(sess)
	r.TickSession(sess)
	if said() != 1 {
		t.Fatalf("said %d times:\n%s", said(), out.String())
	}
	if c, ok := session.ClientStopOf(136); !ok || c.Name != "Lonewulf" || c.How != "silent in the world" {
		t.Fatalf("kept: %+v %v", c, ok)
	}
	sess.UpdateActivity()
	r.TickSession(sess)
	if !strings.Contains(out.String(), "'Lonewulf''s client is heard from again: it was hung") {
		t.Errorf("heard again, not said:\n%s", out.String())
	}
	if _, ok := session.ClientStopOf(136); ok {
		t.Error("a hang's stop was kept")
	}
	// a client that quit in a fight is silent by its own choice
	sess.SetQuitting(time.Now())
	sess.SetLastActivity(time.Now().Add(-session.ClientStoppedSilence - time.Second))
	r.TickSession(sess)
	if said() != 1 {
		t.Error("a client that quit was said to stop")
	}
}
