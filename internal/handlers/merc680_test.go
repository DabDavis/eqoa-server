package handlers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Board entry 680: a liaison tells what a merc heeds before its first
// question: every line of mercLiaisonCommands, in dialogue boxes, the
// liaison's name first, none cut, and only then the role's question. The
// control: a Hardcore-bound talk (its own words) and mercs off say none.
func TestMercLiaisonTellsTheCommandsFirst(t *testing.T) {
	r, _, _ := newMercFixture(t)
	lia := r.liaison()
	r.drain()
	r.talk(r.anna, lia)
	got := r.msgs(r.anna)
	if len(got) < 2 || !strings.HasPrefix(got[len(got)-1], "ask#") {
		t.Fatalf("the talk: %q, want boxes then the question", got)
	}
	boxes := strings.Join(got[:len(got)-1], "\n")
	if !strings.HasPrefix(got[0], "box:"+MercLiaisonName+": A mercenary is yours") {
		t.Errorf("the first box: %q", got[0])
	}
	for _, l := range mercLiaisonCommands {
		if !strings.Contains(boxes, l) {
			t.Errorf("no line %q in %q", l, boxes)
		}
	}
	for _, b := range got[:len(got)-1] {
		if !strings.HasPrefix(b, "box:") || len([]rune(strings.TrimPrefix(b, "box:"))) > questBoxChars {
			t.Errorf("a box past its size or no box: %q", b)
		}
	}
	SetMercs(false)
	r.drain()
	r.talk(r.anna, lia)
	if got := r.msgs(r.anna); strings.Contains(strings.Join(got, "\n"), "/merc dismiss") {
		t.Errorf("the control, mercs off: told %q", got)
	}
}

// hireWarrior hires Anna a warrior and returns its NPC.
func hireWarrior(t *testing.T, r *portalRig) *entity.NPC {
	t.Helper()
	lia := r.liaison()
	r.drain()
	r.talk(r.anna, lia)
	got := r.answer(r.msgs(r.anna), "A tank")
	got = r.answer(got, "Warrior")
	got = r.answer(got, "Pillar")
	r.answer(got, "Like me: Human")
	for _, n := range r.gw.NPCs {
		if n.IsMerc() && n.Owner == r.pa.ServerID {
			r.drain()
			return n
		}
	}
	t.Fatal("no merc hired")
	return nil
}

// Q on the player's own merc lists its commands under its name and level: in a
// PS2's dialogue box, in layout 2's extra lines, and in the dialogue box for a
// client on layout 1 (never lost). Another player's merc and a monster show the
// name and level alone, in every layout.
func TestMobInfoOwnMercListsItsCommands(t *testing.T) {
	r, _, _ := newMercFixture(t)
	mobInfoSwitch(t)
	merc := hireWarrior(t, r)
	snake := &entity.NPC{ServerID: 7791, Name: "moss snake", Level: 3, HP: 40, MaxHP: 40}
	r.gw.NPCs[snake.ServerID] = snake
	name := strings.TrimSpace(merc.Name)
	cmds := "\n" + strings.Join(mercOwnCommands, "\n")

	// a PS2 (never answered)
	if !showMobInfo(r.anna, r.pa, merc, 9) {
		t.Fatal("own merc not answered")
	}
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:"+name+"\nLevel 30"+cmds {
		t.Fatalf("a PS2's own merc: %q", got)
	}
	// another player's merc, a monster: name and level alone
	if showMobInfo(r.cora, r.pc, merc, 9); true {
		if got := r.msgs(r.cora); len(got) != 1 || got[0] != "box:"+name+"\nLevel 30" {
			t.Fatalf("another's merc: %q", got)
		}
	}
	showMobInfo(r.anna, r.pa, snake, 8)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:moss snake\nLevel 3" {
		t.Fatalf("a monster: %q", got)
	}

	// layout 1: the box for its own merc, the message for the rest
	r.anna.SetMobInfoLayout(1)
	r.cora.SetMobInfoLayout(1)
	showMobInfo(r.anna, r.pa, merc, 9)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:"+name+"\nLevel 30"+cmds {
		t.Fatalf("layout 1, own merc: %q", got)
	}
	showMobInfo(r.cora, r.pc, merc, 9)
	if got := r.cora.Queue().GetPending(); len(got) != 1 || !bytes.Equal(got[0].Data(), buildMobInfo(9, name, 30)) {
		t.Fatalf("layout 1, another's merc: not the layout 1 message")
	}

	// layout 2: the commands as extra lines; another's merc and a monster none
	r.anna.SetMobInfoLayout(2)
	showMobInfo(r.anna, r.pa, merc, 9)
	got := r.anna.Queue().GetPending()
	want := buildMobInfoLayout(2, 9, name, 30, append([]string(nil), mercOwnCommands...))
	if len(got) != 1 || !bytes.Equal(got[0].Data(), want) {
		t.Fatalf("layout 2, own merc: %d messages, want one %x", len(got), want)
	}
	showMobInfo(r.anna, r.pa, snake, 8)
	got = r.anna.Queue().GetPending()
	if want := buildMobInfoLayout(2, 8, "moss snake", 3, nil); len(got) != 1 || !bytes.Equal(got[0].Data(), want) {
		t.Fatalf("layout 2, a monster: %d messages, want one %x", len(got), want)
	}
}

// Layout 2 on the wire: layout 1's fields with a 2, then the count and each
// line's length and bytes; no lines is a count of 0. The offer is 1 then 2, so
// a client reading only its first byte answers 1 as before; an answer of 2
// takes layout 2, past 2 is 2, 0 the box.
func TestMobInfoLayout2OnTheWire(t *testing.T) {
	want := []byte{0x4E, 0x7E, 2, 0xA1, 0x86, 0x01, 0x00, 3, 3, 'r', 'a', 't', 2, 2, '/', 'm', 1, 'x'}
	if got := buildMobInfoLayout(2, 0x000186A1, "rat", 3, []string{"/m", "x"}); !bytes.Equal(got, want) {
		t.Errorf("layout 2: %x, want %x", got, want)
	}
	if got, want := buildMobInfoLayout(2, 1, "rat", 3, nil), []byte{0x4E, 0x7E, 2, 1, 0, 0, 0, 3, 3, 'r', 'a', 't', 0}; !bytes.Equal(got, want) {
		t.Errorf("layout 2, no lines: %x, want %x", got, want)
	}
	if got, want := buildMobInfoOffer(), []byte{0x4C, 0x7E, 1, 2}; !bytes.Equal(got, want) {
		t.Errorf("the offer %x, want %x", got, want)
	}
	sess := entitySlotsSession()
	mh := NewMobInfoHandler()
	for _, c := range []struct {
		b    byte
		want uint8
	}{{2, 2}, {1, 1}, {9, 2}, {0, 0}} {
		mh.HandleMobInfoAnswer(sess, incoming(0x7E4D, []byte{c.b}))
		if got := sess.MobInfoLayout(); got != c.want {
			t.Errorf("answer %d: layout %d, want %d", c.b, got, c.want)
		}
	}
}
