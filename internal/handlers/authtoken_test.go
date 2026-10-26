package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// madeUpToken is the token of the made-up password "made-up pass" (internal/
// auth's vector, encrypted by openssl).
var madeUpToken, _ = hex.DecodeString("aff1cac9dfbc7b1799fd85409063a16cb71c12ae76c498fdf3ceeb444a0a49b5")

// loginTailOf is Authenticate's payload as STATION writes it: the version,
// two u32s, then "EQOA" and the name as counted strings, a byte 1 and the
// token.
func loginTailOf(name string, token []byte) []byte {
	b := []byte{0x00}
	b = binary.LittleEndian.AppendUint32(b, 3)
	b = binary.LittleEndian.AppendUint32(b, 4)
	b = append(b, "EQOA"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(name)))
	b = append(b, name...)
	b = append(b, 1)
	return append(b, token...)
}

// fakePasswords is a PasswordChecker that says what it was asked.
type fakePasswords struct {
	asked   []string
	id      int
	refusal string
}

func (f *fakePasswords) CheckUDPPassword(user, pw string, _ *net.UDPAddr) (int, string) {
	f.asked = append(f.asked, user+"/"+pw)
	return f.id, f.refusal
}

// queuedOps is the opcodes of the messages queued for s, in order.
func queuedOps(s *session.Session) []protocol.GameOpcode {
	var out []protocol.GameOpcode
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, protocol.GameOpcode(binary.LittleEndian.Uint16(d)))
		}
	}
	return out
}

// A username with no ticket signs in by the password its token carries (a
// client back at the server list after a restart, its Station login gone);
// a refusal is told to the client as CLIENT shows one, where it was told
// nothing and waited on "Connecting..." for good. The controls: a ticket
// signs in with no token and without asking, and a token alone is not enough
// with no checker.
func TestAUsernameWithNoTicketSignsInByItsToken(t *testing.T) {
	user, tok := loginTail(loginTailOf("dave", madeUpToken))
	if user != "dave" || !bytes.Equal(tok, madeUpToken) {
		t.Fatalf("the tail read as %q, token %x", user, tok)
	}
	if _, tok := loginTail(loginTailOf("dave", madeUpToken[:10])); tok != nil {
		t.Errorf("a tail ending short of the token gave %x", tok)
	}

	h := &AuthHandler{loginTickets: auth.NewTicketStore(0)}
	s := testSession()
	if _, ok := h.authorizeUsername(s, "Authenticate", "dave", madeUpToken); ok {
		t.Error("the control: a token signed in with no checker")
	}
	if got := queuedOps(s); !slices.Equal(got, []protocol.GameOpcode{protocol.OpcodeBadLoginPassword}) {
		t.Errorf("a refusal with no checker told the client %v", got)
	}

	chk := &fakePasswords{id: 7}
	h.SetPasswordChecker(chk)
	h.loginTickets.Grant("erin", 9)
	s = testSession()
	if id, ok := h.authorizeUsername(s, "Authenticate", "erin", nil); !ok || id != 9 || len(chk.asked) != 0 {
		t.Errorf("the control: a ticket signed in as %d (%v), asked %v", id, ok, chk.asked)
	}

	s = testSession()
	id, ok := h.authorizeUsername(s, "Authenticate", "dave", madeUpToken)
	if !ok || id != 7 || !slices.Equal(chk.asked, []string{"dave/made-up pass"}) || len(queuedOps(s)) != 0 {
		t.Errorf("no ticket, the token's password: account %d (%v), asked %v, told %v", id, ok, chk.asked, queuedOps(s))
	}

	chk.refusal = "password"
	s = testSession()
	if _, ok := h.authorizeUsername(s, "Authenticate2", "dave", madeUpToken); ok {
		t.Error("a refused password signed in")
	}
	if got := queuedOps(s); !slices.Equal(got, []protocol.GameOpcode{protocol.OpcodeBadLoginPassword}) {
		t.Errorf("a refused password told the client %v, want Bad Login/Password (0x0729)", got)
	}

	asked := len(chk.asked)
	s = testSession()
	if _, ok := h.authorizeUsername(s, "Authenticate", "dave", nil); ok || len(chk.asked) != asked {
		t.Errorf("no token: signed in %v, asked %v", ok, chk.asked[asked:])
	}
	if got := queuedOps(s); !slices.Equal(got, []protocol.GameOpcode{protocol.OpcodeBadLoginPassword}) {
		t.Errorf("no token told the client %v", got)
	}

	chk.refusal = "address"
	s = testSession()
	if _, ok := h.authorizeUsername(s, "Authenticate", "dave", madeUpToken); ok {
		t.Error("another account at the address signed in")
	}
	if got := queuedOps(s); !slices.Equal(got, []protocol.GameOpcode{protocol.OpcodeForceDisconnect}) {
		t.Errorf("another account at the address told the client %v, want 0x00EC", got)
	}
}
