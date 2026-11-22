package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A place text opens as a player walks into its area (placetexts.go, work
// board 773): once, not again while it stays, again after it left and came
// back; not on the floor under the area, not when it was already standing
// there at its first look (a login), not while it loads a world. The control:
// a player walking about outside it is shown nothing.
func TestAPlaceTextOpensAsAPlayerWalksIn(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP = 47
	hall := database.PlaceText{ID: 3, World: 0, X1: 90, X2: 110, Z1: 90, Z2: 110, Y1: 5, Y2: 20,
		Text: "Organized commotion comes to a close as you enter.", Answer: "Done"}
	f.w.SetPlaceTexts([]database.PlaceText{hall})
	boxes := func() [][]byte {
		f.w.mu.Lock()
		f.w.TickCount = 10
		f.w.placeTextSystem()
		f.w.mu.Unlock()
		return ofOp(duelDrain(f.sess), protocol.OpcodeQuestPopupMulti)
	}
	want := BuildDuelBox(placeTextBoxMark|3, hall.Text, []string{"Done"})
	// the first look: already inside (a login there), nothing
	if got := boxes(); len(got) != 0 {
		t.Fatalf("at the login inside: %d boxes", len(got))
	}
	f.p.X = 200 // out
	if got := boxes(); len(got) != 0 {
		t.Fatalf("outside: %d boxes", len(got))
	}
	f.p.X, f.p.Y = 100, 0 // under the hall's floor
	if got := boxes(); len(got) != 0 {
		t.Fatalf("under the floor: %d boxes", len(got))
	}
	f.p.Y = 10
	if got := boxes(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("walking in: %d boxes", len(got))
	}
	f.p.X = 105 // still inside
	if got := boxes(); len(got) != 0 {
		t.Fatalf("inside still: %d boxes", len(got))
	}
	f.p.X = 200
	boxes()
	f.sess.HoldStateChannels(time.Minute) // a trip back in, loading
	f.p.X = 100
	if got := boxes(); len(got) != 0 {
		t.Fatalf("loading: %d boxes", len(got))
	}
	f.sess.ReleaseStateChannels()
	if got := boxes(); len(got) != 1 {
		t.Fatalf("back in: %d boxes, want 1", len(got))
	}
	if !IsPlaceTextBox(placeTextBoxMark|3) || IsPlaceTextBox(0x7A000003) {
		t.Error("IsPlaceTextBox")
	}
}

// The rows' checks: low edges first, words a PS2 shows.
func TestPlaceTextProblems(t *testing.T) {
	ok := database.PlaceText{X1: 1, X2: 2, Z1: 1, Z2: 2, Y1: 0, Y2: 1, Text: "Words.", Answer: "Done"}
	if p := database.PlaceTextProblem(0, ok); p != "" {
		t.Fatalf("a good row: %s", p)
	}
	for _, bad := range []func(p *database.PlaceText){
		func(p *database.PlaceText) { p.X1 = 3 },
		func(p *database.PlaceText) { p.Text = "" },
		func(p *database.PlaceText) { p.Text = "a |x" },
		func(p *database.PlaceText) { p.Text = string(bytes.Repeat([]byte("a"), 128)) },
	} {
		p := ok
		bad(&p)
		if database.PlaceTextProblem(0, p) == "" {
			t.Errorf("%+v taken", p)
		}
	}
}
