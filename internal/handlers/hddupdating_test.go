package handlers

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// testSession is a session with a real address, which is what the guard keys
// on. A nil session deliberately reports "not updating", so a test that used one
// would exercise the fallback rather than the guard.
func testSession() *session.Session {
	return session.NewSession(
		&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 6000}, 1, 1, 0, 0, true)
}

// The technician must not send a control string to a console whose payload it
// knows is absent.
//
// A command is a run of Unicode private-use runes that the payload's display
// hook is supposed to swallow. With no payload nothing swallows it and the
// client DRAWS it — the runes have no glyphs, so the player gets a line of
// question marks and then permanent silence. Observed on hardware 2026-08-12
// between the two SD passes, and it is what this whole guard exists to stop.
func TestTechnicianRefusesWhileConsoleIsUpdating(t *testing.T) {
	old := ConsoleMidSDRefresh
	t.Cleanup(func() { ConsoleMidSDRefresh = old })
	ConsoleMidSDRefresh = func(string) bool { return true }

	for _, arg := range []string{"check", "create", "format", "format confirm"} {
		var said []string
		// A real session: sendHDDCommand would queue a control string on it, so
		// reaching the switch at all is the failure this is looking for.
		hddSetupCommand(testSession(), &entity.Player{ServerID: 1, Name: "T"}, arg,
			func(s string) { said = append(said, s) }, "")

		if len(said) != 1 || said[0] != HDDUpdatingReply {
			t.Errorf("%q: said %q, want the updating explanation", arg, said)
		}
	}
}

// hddConsoleUpdating answers off the SESSION's host, and a nil session has none.
// Guarding on that keeps the refusal from firing in the one place it would be
// pure damage: a caller with no console to ask about.
func TestUpdatingGuardIsOffWithoutAWiring(t *testing.T) {
	old := ConsoleMidSDRefresh
	t.Cleanup(func() { ConsoleMidSDRefresh = old })

	ConsoleMidSDRefresh = nil
	if hddConsoleUpdating(nil) {
		t.Error("an unwired server must behave exactly as it did before this existed")
	}

	ConsoleMidSDRefresh = func(string) bool { return true }
	if hddConsoleUpdating(nil) {
		t.Error("a session with no address must not be reported as mid-update")
	}
}

// help still works while updating: it talks to the PLAYER, never to the payload,
// so refusing it would remove the one thing that still functions.
func TestHelpSurvivesTheUpdatingGuard(t *testing.T) {
	old := ConsoleMidSDRefresh
	t.Cleanup(func() { ConsoleMidSDRefresh = old })
	ConsoleMidSDRefresh = func(string) bool { return true }

	var said []string
	hddSetupCommand(testSession(), &entity.Player{ServerID: 1, Name: "T"}, "help",
		func(s string) { said = append(said, s) }, "")

	if len(said) == 0 || said[0] == HDDUpdatingReply {
		t.Fatalf("help was refused: %q", said)
	}
}

// The refusal has to tell the player what to DO. The state is real and
// self-correcting — one more patch pass fixes it — so a bare "not available"
// would leave them staring at a technician that looks broken, which is exactly
// the impression the question marks already gave.
func TestUpdatingReplyNamesTheRemedy(t *testing.T) {
	low := strings.ToLower(HDDUpdatingReply)
	for _, want := range []string{"title screen", "update"} {
		if !strings.Contains(low, want) {
			t.Errorf("the refusal never mentions %q: %s", want, HDDUpdatingReply)
		}
	}
}

// An armed format must not survive the refusal. The arm is a 30-second licence
// to erase a disk; carrying one across a console that is mid-update means the
// next thing the player does could consume an arm they no longer remember
// setting.
func TestRefusalClearsAnArmedFormat(t *testing.T) {
	old := ConsoleMidSDRefresh
	t.Cleanup(func() { ConsoleMidSDRefresh = old })

	ConsoleMidSDRefresh = func(string) bool { return false }
	player := &entity.Player{ServerID: 42, Name: "T"}
	hddSetupCommand(testSession(), player, "format", func(string) {}, "")

	ConsoleMidSDRefresh = func(string) bool { return true }
	hddSetupCommand(testSession(), player, "check", func(string) {}, "")

	if takeFormatArm(player.ServerID) {
		t.Error("a format arm survived a refusal; it must be cleared with everything else")
	}
}
