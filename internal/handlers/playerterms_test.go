package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/protocol"
)

func TestPlayerTermsRejectsOldTicketPasswordAndCharacterSelection(t *testing.T) {
	h := ticketAuthHandler(t)
	h.SetPasswordChecker(&fakePasswords{id: 7})
	h.loginTickets.Grant("erin", "made-up pass", auth.LoginCredential{AccountID: 9, PasswordHash: "testhash"})
	deny := func(int) bool { return false }
	h.playerTermsAllowed = deny
	for _, name := range []string{"erin", "dave"} {
		s := testSession()
		if _, ok := h.authorizeUsername(s, "Authenticate", name, madeUpToken); ok {
			t.Fatal("admitted before agreement")
		}
		if !slices.Equal(queuedOps(s), []protocol.GameOpcode{protocol.OpcodeBadLoginPassword}) {
			t.Fatal("missing refusal")
		}
	}
	s := testSession()
	characters := CharacterHandler{playerTermsAllowed: deny}
	if err := characters.HandleSelectedChar(s, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(queuedOps(s), []protocol.GameOpcode{protocol.OpcodeBadLoginPassword}) {
		t.Fatal("old character-selection session did not recheck")
	}
}
