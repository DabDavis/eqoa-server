package handlers

// noclip.go — /noclip, the server's own GMs' flight through walls (entity/noclip.go;
// the GM Jibrish's petition 1904, 2026-11-13: "Add a high level GM only command (Just
// for jibrish and dabs account) that is /noclip - this lets the player move through
// walls vertical or horizontal and default turns on /fly. Make it a toggle, hitting
// it again turns it off").
//
// A GM's (acctlevel 1 or more, as every # command) whose account is one of
// world.noclip_accounts (SetNoclipAccounts; the owner's dab and Jibrish's jibrish on
// eqoaclassic): any other GM is told it is not theirs, a player is answered as an
// unknown command. Alone it turns over, `on` and `off` say which. On, the flight comes
// on with it (OpcodeFly 1, then OpcodeNoclip 1), and the server's movement check
// judges none of its moves (a flyer's, movecheck.go); off, both go (OpcodeNoclip 0,
// OpcodeFly 0). #fly off ends it too. Only a client that flies (vanilla, which says
// so to the GM channel) is sent either; a client that does not know OpcodeNoclip
// flies with its own walls, so the line says what a client needs.

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What /noclip answers.
const (
	noclipOnLine   = "Noclip on: you fly through walls, floors and ceilings (/noclip again to turn it off). A client without noclip still meets its walls."
	noclipOffLine  = "Noclip off: walls hold you again, and you fall and walk as normal from where you are"
	noclipNotYours = "/noclip is for the server's own GMs alone."
	noclipCannot   = "Noclip stays off: this client cannot fly (a PS2 or an older build; made a GM just now? log in again)"
)

var noclipAccounts struct {
	mu    sync.RWMutex
	names map[string]bool
}

// SetNoclipAccounts sets world.noclip_accounts: the accounts' usernames, any case.
func SetNoclipAccounts(names []string) {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			m[n] = true
		}
	}
	noclipAccounts.mu.Lock()
	noclipAccounts.names = m
	noclipAccounts.mu.Unlock()
	if len(m) > 0 {
		log.Printf("Noclip: %d accounts may /noclip (world.noclip_accounts)", len(m))
	}
}

// noclipAccount reports whether the account of that username may /noclip.
func noclipAccount(username string) bool {
	noclipAccounts.mu.RLock()
	defer noclipAccounts.mu.RUnlock()
	return noclipAccounts.names[strings.ToLower(strings.TrimSpace(username))]
}

// usernames reads an account's username (database.SQLiteAccountStore).
type usernames interface {
	Username(accountID int) (string, error)
}

// mayNoclip reports whether sess's account may /noclip: a GM (handleGMCommand
// checked) of world.noclip_accounts, read on each use.
func (h *ChatHandler) mayNoclip(sess *session.Session) bool {
	u, ok := h.access.(usernames)
	if !ok || sess == nil || sess.AccountID() == 0 {
		return false
	}
	name, err := u.Username(sess.AccountID())
	return err == nil && noclipAccount(name)
}

// buildNoclip is protocol.OpcodeNoclip from the server: a u32, 1 on and 0 off.
func buildNoclip(on bool) []byte {
	return build(protocol.OpcodeNoclip, 6, func(w *buffer.Writer) {
		if on {
			w.WriteUint32(1)
		} else {
			w.WriteUint32(0)
		}
	})
}

// SendNoclip tells sess's client whether its flight goes through walls, a client
// that flies alone. It reports whether it told it.
func SendNoclip(sess *session.Session, on bool) bool {
	if !sess.FlyClient() {
		return false
	}
	queueReliable(sess, protocol.OpcodeNoclip, buildNoclip(on))
	return true
}

// noclipCommand is /noclip (a GM's: handleGMCommand gates it; the accounts here).
func (h *ChatHandler) noclipCommand(sess *session.Session, args []string) {
	if !h.mayNoclip(sess) {
		h.gmReply(sess, noclipNotYours)
		return
	}
	on, ok := h.gameWorld.Noclip(sess)
	if !ok {
		return
	}
	want := !on
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "on":
			want = true
		case "off":
			want = false
		default:
			h.gmReply(sess, "Usage: /noclip [on|off] (alone: on, or off again)")
			return
		}
	}
	if want && !sess.FlyClient() {
		h.gmReply(sess, noclipCannot)
		return
	}
	if !h.gameWorld.SetNoclip(sess, want, time.Now()) {
		return
	}
	if want {
		SendFlying(sess, true)
		SendNoclip(sess, true)
		h.gmReply(sess, noclipOnLine)
		return
	}
	SendNoclip(sess, false)
	SendFlying(sess, false)
	h.gmReply(sess, noclipOffLine)
}
