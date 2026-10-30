package entity

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/session"
)

// A duel's wager (duel.go): tunar out of each side's purse, held while the
// duel is fought and paid to the winner at its end (both stakes; a draw gives
// each its own back).
//
// What the live client shows (eqoa-live-1500, claude-notes/duel-20261029.md):
// once an invitation is accepted the server opens VIWndDuel's wager half
// (0x1502 sub 13), a copy of the trade window titled "Wager"; each side puts
// in tunar (its deposit window in mode 4, whose limit is the PURSE, VIPlayer
// +0x34: sub 5 up, and the client sends at most what it carries) and items (up
// to four, sub 6 and 7 up, 10 and 11 down); 12 down carries both sides' tunar
// and accepts, Square accepts (8 up, with the two lists of item keys it saw),
// Triangle withdraws an accept (9) or calls the duel off ("Cancel duel?",
// 10), 15 down the accepts (both set: each window closes) and 14 closes it.
// How the live server paid out is in no client; the winner taking both stakes
// is ours.
//
// Here: the window for two clients that have it, tunar alone (an item put in
// is refused with a line: holding and delivering items is the auction's kind
// of work, not done here); a client without the window names its wager in the
// challenge (/duel <name> <tunar>, "/tell <name> duel <tunar>"), which both
// stake. Stakes are taken from the purses as the fight begins and are in
// table duel_stakes until the end, one transaction each way
// (database/duel.go); a server that stops mid-duel gives them back at its
// next start.

// DuelPurseMove is one purse's part in a hold or a payment: the player (nil:
// no longer in the world), its character, the tunar moved and its purse once
// moved (the server's figure, taken under the world lock).
type DuelPurseMove struct {
	P        *Player
	Sess     *session.Session
	CharID   uint32
	Tunar    int
	PurseNow int
}

// DuelPurseOp is a hold (the stakes out of the purses into the table) or a
// payment (out of the table into the purses).
type DuelPurseOp struct {
	Hold  bool
	Duel  uint32
	Moves []DuelPurseMove
}

// DuelPurseFunc writes a hold or a payment and tells each client in the world
// its purse (0x0052): handlers' (database/duel.go), run in order after the
// world lock is let go.
type DuelPurseFunc func(op DuelPurseOp)

// duelPurse is the ops waiting for the hook, in order.
type duelPurse struct {
	fn    DuelPurseFunc
	mu    sync.Mutex // the queue
	run   sync.Mutex // one runner at a time, so the ops go in order
	queue []DuelPurseOp
}

// OnDuelPurse sets the hook that writes the stakes.
func (w *World) OnDuelPurse(fn DuelPurseFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.duelPurse.mu.Lock()
	defer w.duelPurse.mu.Unlock()
	w.duelPurse.fn = fn
}

func (w *World) queueDuelPurseLocked(op DuelPurseOp) {
	w.duelPurse.mu.Lock()
	defer w.duelPurse.mu.Unlock()
	if w.duelPurse.fn != nil {
		w.duelPurse.queue = append(w.duelPurse.queue, op)
	}
}

// runDuelPurse runs the ops waiting, outside the world lock (deferred by the
// methods that queue them, and every tick).
func (w *World) runDuelPurse() {
	w.duelPurse.run.Lock()
	defer w.duelPurse.run.Unlock()
	for {
		w.duelPurse.mu.Lock()
		ops := w.duelPurse.queue
		w.duelPurse.queue = nil
		fn := w.duelPurse.fn
		w.duelPurse.mu.Unlock()
		if len(ops) == 0 || fn == nil {
			return
		}
		for _, op := range ops {
			fn(op)
		}
	}
}

// purseMoveLocked is p's part in an op (p alive in the world), its purse as
// it now is.
func (w *World) purseMoveLocked(p *Player, tunar int) DuelPurseMove {
	return DuelPurseMove{P: p, Sess: w.findSessionByEntityIDLocked(p.ServerID), CharID: p.CharServerID, Tunar: tunar, PurseNow: p.Tunar}
}

// holdStakesLocked takes the duel's stakes out of a's and b's purses (both
// or neither) and queues the hold. It returns why it could not, or "".
func (w *World) holdStakesLocked(d *Duel, a, b *Player, now time.Time) string {
	if d.Stake[0] == 0 && d.Stake[1] == 0 {
		return ""
	}
	for i, p := range []*Player{a, b} {
		if d.Stake[i] > p.Tunar {
			return fmt.Sprintf("%s does not have the %d tunar wagered.", p.Name, d.Stake[i])
		}
	}
	op := DuelPurseOp{Hold: true, Duel: d.ID}
	for i, p := range []*Player{a, b} {
		if d.Stake[i] == 0 {
			continue
		}
		p.Tunar -= d.Stake[i]
		op.Moves = append(op.Moves, w.purseMoveLocked(p, d.Stake[i]))
	}
	d.held = true
	w.queueDuelPurseLocked(op)
	log.Printf("Duel %d: %s stakes %d, %s %d", d.ID, a.Name, d.Stake[0], b.Name, d.Stake[1])
	return ""
}

// settleStakesLocked pays a duel's held stakes at its end: both to the winner
// (loser not 0), else each its own; told to the two, and queued.
func (w *World) settleStakesLocked(d *Duel, loser EntityID) {
	if !d.held {
		return
	}
	d.held = false
	pot := d.Stake[0] + d.Stake[1]
	pays := [2]int{d.Stake[0], d.Stake[1]}
	if loser == d.A {
		pays = [2]int{0, pot}
	} else if loser == d.B {
		pays = [2]int{pot, 0}
	}
	op := DuelPurseOp{Duel: d.ID}
	for i, id := range []EntityID{d.A, d.B} {
		p := w.duelistLocked(d, id)
		if p == nil {
			// gone: paid onto its row
			op.Moves = append(op.Moves, DuelPurseMove{CharID: d.who[i].CharServerID, Tunar: pays[i]})
			continue
		}
		p.Tunar += pays[i]
		op.Moves = append(op.Moves, w.purseMoveLocked(p, pays[i]))
		switch {
		case pays[i] == pot && pot > d.Stake[i]:
			w.duelSayLocked(p, fmt.Sprintf("You win the wager: %d tunar.", pot), duelOrange)
		case pays[i] > 0 && loser == 0:
			w.duelSayLocked(p, fmt.Sprintf("Your wager of %d tunar is returned.", pays[i]), duelOrange)
		case pays[i] == 0 && d.Stake[i] > 0:
			w.duelSayLocked(p, fmt.Sprintf("You lose your wager of %d tunar.", d.Stake[i]), duelOrange)
		}
	}
	w.queueDuelPurseLocked(op)
	log.Printf("Duel %d: the wager (%d and %d) pays %d and %d", d.ID, d.Stake[0], d.Stake[1], pays[0], pays[1])
}

// openWagerLocked opens the live wager window for two clients that have it:
// each side's tunar 0, nothing accepted. The live client's sub 13 clears the
// offers and its inventory copy but not the tunar or the accepts (+0x40..0x49
// of its duel part), so a sub 12 of zeroes follows it: without one a second
// wager in a login opens with the last one's accepts on, and its Accept sends
// nothing (lane-duelwnd24, on the rig).
func (w *World) openWagerLocked(d *Duel, a, b *Player, now time.Time) {
	d.Phase = duelWager
	d.windowed = true
	d.Stake = [2]int{}
	d.accept = [2]bool{}
	d.until = now.Add(DuelWagerTime)
	log.Printf("Duel %d: %s and %s set their wagers", d.ID, a.Name, b.Name)
	for _, p := range []*Player{a, b} {
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			if p == a {
				w.sendDuelAcceptedLocked(sess, d.ID) // the live client declines its own queue
			}
			queueDuel(sess, BuildDuel(duelSubWagerOpen, d.ID, nil))
		}
	}
	w.sendWagerLocked(d)
}

// closeWagerLocked calls a duel in its wager window off: both windows closed
// (sub 14), the duel's state ended (sub 7 {0, 0, -1, -1}, as a fight over:
// the live client keeps the last sub 15's "set" through a 14, and its Quit
// would then surrender a duel that is not there), both told line.
func (w *World) closeWagerLocked(d *Duel, line string) {
	w.duels.remove(d)
	log.Printf("Duel %d: off in its wager (%s)", d.ID, line)
	for _, id := range []EntityID{d.A, d.B} {
		p := w.duelistLocked(d, id)
		if p == nil {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			queueDuel(sess, BuildDuel(duelSubWagerClose, d.ID, nil))
			w.sendDuelEndLocked(sess, d)
			w.sendColoredLocked(sess, line, duelOrange)
		}
	}
}

// duelWagerLocked is sess's player's duel in its wager window, and its side.
func (w *World) duelWagerLocked(sess *session.Session) (*Duel, *Player, int) {
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil, nil, 0
	}
	d := w.duels.of[p.ServerID]
	if d == nil || d.Phase != duelWager {
		return nil, p, 0
	}
	return d, p, d.side(p.ServerID)
}

// sendWagerLocked tells both windows the tunar and the accepts (sub 12), each
// its own first.
func (w *World) sendWagerLocked(d *Duel) {
	for i, id := range []EntityID{d.A, d.B} {
		p := w.duelistLocked(d, id)
		if p == nil {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			mine, theirs := i, 1-i
			queueDuel(sess, BuildDuel(duelSubWagerTunar, d.ID, func(wr *buffer.Writer) {
				wr.WriteUint32(uint32(d.Stake[mine]))
				wr.WriteUint32(uint32(d.Stake[theirs]))
				wr.WriteByte(b2u8(d.accept[mine]))
				wr.WriteByte(b2u8(d.accept[theirs]))
			}))
		}
	}
}

// sendAcceptsLocked tells both windows the accepts (sub 15): set 1, the duel
// is arranged; both accepted, each window closes itself.
func (w *World) sendAcceptsLocked(d *Duel) {
	for i, id := range []EntityID{d.A, d.B} {
		p := w.duelistLocked(d, id)
		if p == nil {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			mine, theirs := i, 1-i
			queueDuel(sess, BuildDuel(duelSubWagerAccepts, d.ID, func(wr *buffer.Writer) {
				wr.WriteByte(1)
				wr.WriteByte(b2u8(d.accept[mine]))
				wr.WriteByte(b2u8(d.accept[theirs]))
			}))
		}
	}
}

func b2u8(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// DuelWagerTunar is the live window's sub 5: sess's player stakes tunar (at
// most what it carries); both accepts cleared.
func (w *World) DuelWagerTunar(sess *session.Session, tunar int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, p, i := w.duelWagerLocked(sess)
	if d == nil || d.accept[i] {
		return // the live client sends none once it accepted
	}
	tunar = max(0, min(tunar, p.Tunar, DuelMaxWager))
	d.Stake[i] = tunar
	d.accept = [2]bool{}
	w.sendWagerLocked(d)
}

// DuelWagerItem is the live window's sub 6: an item put in, which this server
// does not take.
func (w *World) DuelWagerItem(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if d, p, _ := w.duelWagerLocked(sess); d != nil {
		w.duelSayLocked(p, "Only tunar may be wagered on a duel here.", duelRed)
	}
}

// DuelWagerAccept is the live window's sub 8 (on) and 9 (off): sess's
// player's accept. Both accepted, the stakes are held and the fight begins;
// a side that no longer carries its stake clears both.
func (w *World) DuelWagerAccept(sess *session.Session, on bool, now time.Time) {
	defer w.runDuelPurse()
	w.mu.Lock()
	defer w.mu.Unlock()
	d, _, i := w.duelWagerLocked(sess)
	if d == nil {
		return
	}
	d.accept[i] = on
	a, b := w.duelistLocked(d, d.A), w.duelistLocked(d, d.B)
	if !d.accept[0] || !d.accept[1] || a == nil || b == nil {
		w.sendAcceptsLocked(d)
		return
	}
	if why := w.holdStakesLocked(d, a, b, now); why != "" {
		d.accept = [2]bool{}
		w.sendAcceptsLocked(d)
		w.duelSayLocked(a, why, duelRed)
		w.duelSayLocked(b, why, duelRed)
		return
	}
	w.sendAcceptsLocked(d) // both set: each window closes
	w.startDuelLocked(d, a, b, now)
}
