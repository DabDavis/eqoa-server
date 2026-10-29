package handlers

import (
	"encoding/binary"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Duels (entity/duel.go): how a player asks for one, answers and ends one.
//
// Every client duels the same way underneath; what it is shown differs:
//
//   - a PS2 (the Frontiers disc) and vanilla without the live window: a
//     challenge is /duel at the target or /duel <name> (a client that
//     forwards its /commands: vanilla, an S2) or "/tell <name> duel" (any
//     client: Sandstorm's way, "whisper "duel" to another player to
//     challenge"); the invitation is a box (0x0034) with Accept and Decline;
//     /duel cancel asks "Cancel duel?" and then withdraws, declines or
//     surrenders; /duel accept and /duel decline answer without the box.
//   - a client with SOE's live duel window (VIWndDuel), which says so by
//     answering the offer (protocol.OpcodeDuelWindow, 0x7E09): the live
//     game's own messages, 0x1501 up (handled here) and 0x1502 down
//     (entity/duelwire.go); its Community > Duel menu's Invite and Surrender.

// buildDuelWindowOffer is OpcodeDuelWindow from the server: a u32, 1.
func buildDuelWindowOffer() []byte {
	return build(protocol.OpcodeDuelWindow, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// buildLiveFeatures is 0x07F5 with the duel bit (entity.DuelFeatures): the
// live client's feature word, without which its duel part sends nothing. The
// disc has no case for it.
func buildLiveFeatures() []byte {
	return build(protocol.OpcodeLiveFeatures, 6, func(w *buffer.Writer) {
		w.WriteUint32(entity.DuelFeatures)
	})
}

// SendDuelOffer offers the session's client the live duel window's
// messages, and tells a live client duels are on (0x07F5). A PS2 ignores both.
func SendDuelOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeDuelWindow, buildDuelWindowOffer())
	queueReliable(sess, protocol.OpcodeLiveFeatures, buildLiveFeatures())
}

// DuelHandler takes the duel's messages: the live window's 0x1501 and the
// answer to the offer.
type DuelHandler struct {
	world *entity.World
}

// NewDuelHandler creates the duel handler.
func NewDuelHandler(w *entity.World) *DuelHandler { return &DuelHandler{world: w} }

// RegisterHandlers registers the duel handler.
func (h *DuelHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDuelRequest, h.HandleDuelRequest)
	r.Register(protocol.OpcodeDuelWindow, h.HandleDuelWindow)
}

// HandleDuelWindow makes the client's answer its player's: a u32, nonzero
// for the live window's messages, 0 for boxes.
func (h *DuelHandler) HandleDuelWindow(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("DuelWindow: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetDuelWindow(sess, on) {
		log.Printf("DuelWindow: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("DuelWindow: %s %s", sess.RemoteAddr(), map[bool]string{true: "has the live window", false: "takes boxes"}[on])
	return nil
}

// The live client's 0x1501 sub-ops (its duel part, live 0x012E5898..):
// {u8 sub, u32 the duel id it holds, ...}.
const (
	duelUpChallenge  = 1  // u32 target: Community > Duel > Invite, at the target
	duelUpDecline    = 2  // u32 invitation, string its own name
	duelUpAccept     = 3  // u32 invitation
	duelUpSurrender  = 4  // Surrender, and a logout with a duel on or set
	duelUpTunar      = 5  // u32 tunar: the wager's (the client sends at most its purse)
	duelUpItem       = 6  // u32 item key, u32 count: an item staked
	duelUpItemBack   = 7  // u32 item key: taken back
	duelUpAcceptBet  = 8  // two VIPool<int> of item keys: the wager accepted
	duelUpUnaccept   = 9  // the accept withdrawn
	duelUpCancelDuel = 10 // the wager window's "Cancel duel?" Yes
)

// HandleDuelRequest is a 0x1501 from a client with the live window.
func (h *DuelHandler) HandleDuelRequest(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 || h.world == nil {
		return nil
	}
	sub := data[0]
	r := buffer.NewReader(data[5:])
	now := time.Now()
	switch sub {
	case duelUpChallenge:
		target, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		h.world.DuelChallenge(sess, target, "", 0, now)
	case duelUpDecline, duelUpAccept:
		id, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		h.world.DuelAnswer(sess, id, sub == duelUpAccept, now)
	case duelUpSurrender, duelUpCancelDuel:
		h.world.DuelCancel(sess, now)
	case duelUpTunar:
		n, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		h.world.DuelWagerTunar(sess, int(min(n, uint32(entity.DuelMaxWager))))
	case duelUpItem:
		h.world.DuelWagerItem(sess)
	case duelUpAcceptBet, duelUpUnaccept:
		h.world.DuelWagerAccept(sess, sub == duelUpAcceptBet, now)
	default:
		log.Printf("Duel: %s sent 0x1501 sub %d", sess.RemoteAddr(), sub) // 7: no item is staked here
	}
	return nil
}

// duelCommand is /duel's words after the command: "" (the target, sent with
// the line), a name, either with a wager after it ("/duel 100", "/duel Bob
// 100"), cancel (and its spellings), accept or decline.
func duelCommand(gw *entity.World, sess *session.Session, rest string, target uint32) {
	arg := strings.ToLower(strings.TrimSpace(rest))
	now := time.Now()
	switch arg {
	case "cancel", "surrender", "yield", "forfeit", "end", "stop":
		gw.DuelAskCancel(sess)
		return
	case "accept", "yes":
		duelAnswerOwn(gw, sess, true, now)
		return
	case "decline", "no":
		duelAnswerOwn(gw, sess, false, now)
		return
	}
	name, wager := duelWords(rest)
	if name == "" {
		gw.DuelChallenge(sess, target, "", wager, now)
		return
	}
	gw.DuelChallenge(sess, 0, name, wager, now)
}

// duelWords splits a challenge's words: a name (none: the target) and a
// wager, the last word when it is a number.
func duelWords(rest string) (name string, wager int) {
	words := strings.Fields(rest)
	if n := len(words); n > 0 {
		if v, err := strconv.Atoi(words[n-1]); err == nil {
			wager, words = v, words[:n-1]
		}
	}
	return strings.Join(words, " "), wager
}

// duelAnswerOwn answers the invitation sess's player was sent, if any.
func duelAnswerOwn(gw *entity.World, sess *session.Session, accept bool, now time.Time) {
	gw.DuelAnswer(sess, 0, accept, now)
}

// duelTell reports a tell's text as Sandstorm's challenge: "duel", or
// "duel" and a wager ("duel 100").
func duelTell(text string) (wager int, ok bool) {
	words := strings.Fields(text)
	if len(words) == 0 || !strings.EqualFold(words[0], "duel") || len(words) > 2 {
		return 0, false
	}
	if len(words) == 2 {
		v, err := strconv.Atoi(words[1])
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, true
}

// duelStakesStore is where a duel's wager is held (database/duel.go).
type duelStakesStore interface {
	DuelHold(duel uint32, stakes []database.DuelStake, now time.Time) error
	DuelSettle(duel uint32, pays []database.DuelPay) error
	DuelForget(duel uint32) error
	DuelRefundHeld() (int, error)
}

// duelPurse is the world's duel purse hook (entity.DuelPurseFunc) on db: a
// hold or a payment written in one transaction, then each client in the
// world told its purse (0x0052).
func duelPurse(db duelStakesStore) entity.DuelPurseFunc {
	return func(op entity.DuelPurseOp) {
		if op.Hold {
			stakes := make([]database.DuelStake, 0, len(op.Moves))
			for _, m := range op.Moves {
				stakes = append(stakes, database.DuelStake{ServerID: m.CharID, Tunar: m.Tunar, PurseNow: m.PurseNow})
			}
			if err := db.DuelHold(op.Duel, stakes, time.Now()); err != nil {
				log.Printf("Duel %d: HOLDING THE WAGER FAILED (the purses keep the server's figures): %v", op.Duel, err)
			}
		} else {
			pays := make([]database.DuelPay, 0, len(op.Moves))
			for _, m := range op.Moves {
				pays = append(pays, database.DuelPay{ServerID: m.CharID, Tunar: m.Tunar, Online: m.P != nil, PurseNow: m.PurseNow})
			}
			if err := db.DuelSettle(op.Duel, pays); err != nil {
				// the server's figures are paid already: a refund at the next
				// start would pay the stakes twice
				log.Printf("Duel %d: PAYING THE WAGER FAILED: %v", op.Duel, err)
				if err := db.DuelForget(op.Duel); err != nil {
					log.Printf("Duel %d: and its stakes are still held, to be refunded at the next start: %v", op.Duel, err)
				}
			}
		}
		for _, m := range op.Moves {
			if m.Sess != nil && m.Tunar != 0 {
				queueReliable(m.Sess, protocol.OpcodePlayerTunar, tunarPayload(m.PurseNow))
			}
		}
	}
}

// StartDuelPurse gives back every stake a stopped server left held, then
// hooks the world's duels to db.
func StartDuelPurse(gw *entity.World, db duelStakesStore) {
	if n, err := db.DuelRefundHeld(); err != nil {
		log.Printf("Duel: refunding the stakes left held: %v", err)
	} else if n > 0 {
		log.Printf("Duel: %d stakes a stopped duel held went back to their owners", n)
	}
	gw.OnDuelPurse(duelPurse(db))
}
