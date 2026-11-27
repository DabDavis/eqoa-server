package entity

import (
	"fmt"
	"sort"
	"time"

	"github.com/eqoa/server/internal/session"
)

const testReturnCooldown = 5 * time.Second

type testReturn struct {
	mark  dungeonReturn
	ready time.Time
}

// TestReturnRead uses the GM-issued scroll. The two scrolls keep independent
// marks. Only a successful trip starts the character's test scroll cooldown.
// spend validates the destination and takes one scroll before a return is queued.
func (w *World) TestReturnRead(sess *session.Session, now time.Time, home *GateSpot,
	spend func(GateSpot) bool) (DungeonReturnResult, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() || w.duelingLocked(p.ServerID) {
		return ReturnRefused, returnLineNow
	}
	heroic := w.inDungeonsLocked(p)
	if !heroic && Instanced(p.World) {
		return ReturnRefused, "You cannot use the test scroll in this instance."
	}
	state := w.testReturns[p.CharServerID]
	if wait := state.ready.Sub(now); wait > 0 {
		secs := int((wait + time.Second - 1) / time.Second)
		return ReturnRefused, fmt.Sprintf("You can read the test scroll again in %d sec.", secs)
	}
	for _, travel := range w.gates.due {
		if travel.P == p {
			return ReturnRefused, returnLineNow
		}
	}

	var res DungeonReturnResult
	var line string
	switch {
	case heroic:
		if home == nil {
			return ReturnRefused, returnLineNow
		}
		res, line = w.dungeonReturnOutLocked(sess, p, now, &state.mark, home)
	case state.mark.inst != 0:
		res, line = w.dungeonReturnBackLocked(sess, p, now, &state.mark, spend)
	default:
		res, line = w.openWorldReturnLocked(sess, p, now, &state.mark, home, spend)
	}
	if res != ReturnRefused {
		state.ready = now.Add(testReturnCooldown)
	}
	if state.mark.at.IsZero() && !state.ready.After(now) {
		delete(w.testReturns, p.CharServerID)
	} else {
		if w.testReturns == nil {
			w.testReturns = make(map[uint32]testReturn)
		}
		w.testReturns[p.CharServerID] = state
	}
	return res, line
}

func (w *World) openWorldReturnLocked(sess *session.Session, p *Player, now time.Time, mark *dungeonReturn,
	home *GateSpot, spend func(GateSpot) bool) (DungeonReturnResult, string) {
	const spell = "the test Scroll of Returning"
	if mark.at.IsZero() || now.Sub(mark.at) > DungeonReturnWindow {
		if home == nil {
			return ReturnRefused, returnLineNow
		}
		*mark = dungeonReturn{world: p.World, x: p.X, y: p.Y, z: p.Z, facing: p.Facing, at: now}
		w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: *home, Spell: spell})
		return ReturnOut, fmt.Sprintf("The test scroll marks your place and carries you home. Read it again within %d minutes to return.",
			int(DungeonReturnWindow/time.Minute))
	}
	to := GateSpot{World: mark.world, X: mark.x, Y: mark.y, Z: mark.z, Place: "the marked spot"}
	if Instanced(to.World) || !spend(to) {
		return ReturnRefused, returnLineNow
	}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: spell, Facing: mark.facing, HasFacing: true})
	*mark = dungeonReturn{}
	return ReturnBack, "The test scroll carries you back to the marked spot and crumbles to dust."
}

type KeptTestReturn struct {
	KeptDungeonReturn
	World uint8     `json:"world,omitempty"`
	Ready time.Time `json:"ready"`
}

func (w *World) pruneTestReturnsLocked(now time.Time) {
	for char, state := range w.testReturns {
		if !state.ready.After(now) && (state.mark.at.IsZero() || now.Sub(state.mark.at) > DungeonReturnWindow) {
			delete(w.testReturns, char)
		}
	}
}

func (w *World) keepTestReturnsLocked(now time.Time) []KeptTestReturn {
	w.pruneTestReturnsLocked(now)
	var kept []KeptTestReturn
	for char, state := range w.testReturns {
		m := state.mark
		kept = append(kept, KeptTestReturn{
			KeptDungeonReturn: KeptDungeonReturn{Char: char, Inst: m.inst, Opened: m.opened,
				Place: [4]float32{m.x, m.y, m.z, m.facing}, At: m.at},
			World: m.world,
			Ready: state.ready,
		})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Char < kept[j].Char })
	return kept
}

func (w *World) restoreTestReturnsLocked(kept []KeptTestReturn) {
	if len(kept) == 0 {
		return
	}
	w.testReturns = make(map[uint32]testReturn, len(kept))
	for _, r := range kept {
		w.testReturns[r.Char] = testReturn{ready: r.Ready, mark: dungeonReturn{
			inst: r.Inst, opened: r.Opened, world: r.World, at: r.At,
			x: r.Place[0], y: r.Place[1], z: r.Place[2], facing: r.Place[3],
		}}
	}
}
