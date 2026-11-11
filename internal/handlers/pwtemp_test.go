package handlers

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

type tempFlags map[int]bool

func (f tempFlags) PasswordTemporary(id int) (bool, error) {
	if id == 99 {
		return false, errors.New("no such account")
	}
	return f[id], nil
}

// A character of an account whose password is a temporary one a GM set is
// told at its world entry, in red, to set its own on the website; one of an
// account with its own password is told nothing (the control).
func TestATemporaryPasswordIsSaidInTheWorld(t *testing.T) {
	c := newChatWorld(t)
	c.gw.OnPlayerEntered(PasswordTempNotice(tempFlags{1: true}, "eqoaclassic.com/eqoa/password"))
	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	as.SetAccountID(1)
	bs.SetAccountID(2)
	lines := func(sessQueue [][]byte) []string {
		var out []string
		for _, d := range sessQueue {
			if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeErrorMessage {
				text, _ := readWideLine(d[2:])
				out = append(out, text)
			}
		}
		return out
	}
	as.Queue().GetPending()
	bs.Queue().GetPending()
	c.gw.PlayerEntered(as)
	c.gw.PlayerEntered(bs)
	a := lines(queued(as, protocol.OpcodeErrorMessage))
	if len(a) != 1 || !strings.Contains(a[0], "temporary") || !strings.Contains(a[0], "eqoaclassic.com/eqoa/password") {
		t.Fatalf("Ann was told %q", a)
	}
	if b := lines(queued(bs, protocol.OpcodeErrorMessage)); len(b) != 0 {
		t.Fatalf("Bob was told %q", b)
	}
	if w := PasswordTempText(""); !strings.Contains(w, "the account website's Change password page") {
		t.Fatalf("with no page named: %q", w)
	}
}
