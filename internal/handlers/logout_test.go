package handlers

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A client's 0x09B0 (Quit) ends its session at the next tick, and the
// opcode is routed to the handler. The control: a session that sent nothing
// is not ended.
func TestLogoutEndsTheSession(t *testing.T) {
	mgr := session.NewManager()
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 6000}, 1, 1, 1, 2, true)
	sess.SetCharacterInWorld(true)
	if sess.PendingTermination() {
		t.Fatal("a fresh session is already ending")
	}
	r := NewRegistry()
	(&CharacterHandler{sessionManager: mgr}).RegisterHandlers(r)
	h, ok := r.Get(protocol.OpcodeDisconnectClient)
	if !ok {
		t.Fatal("0x09B0 has no handler")
	}
	if err := h(sess, protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDisconnectClient)); err != nil {
		t.Fatal(err)
	}
	if !sess.PendingTermination() {
		t.Fatal("the session goes on after its logout")
	}
}
