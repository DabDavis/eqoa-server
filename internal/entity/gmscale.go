package entity

import "fmt"

// gmscale.go — a GM's /size (the owner, 2026-11-11: "lets add /scale command for GMs change
// size/scale of any PC/NPC like /scale 2.5 /scale 1(for default)", then "instead of /scale lets make it /size"). The factor is of the body's own
// size (its row's for an NPC, its race's for a player): /scale 2.5 makes it two and a half times
// that, /scale 1 gives it back. It is the running world's alone: an NPC's respawn or a restart, a
// player's next login, ends it (a row's size is the GM panel's to change for good).

// GMScaleMin and GMScaleMax bound a /size factor.
const (
	GMScaleMin = 0.1
	GMScaleMax = 10
)

// GMScale scales the GM gm's target (a player or an NPC it sees), else gm itself, by factor of
// its own size. It returns a line for the GM.
func (w *World) GMScale(gm *Player, factor float32) (string, bool) {
	if factor < GMScaleMin || factor > GMScaleMax {
		return fmt.Sprintf("A size is %g to %g (1 is the body's own size)", float32(GMScaleMin), float32(GMScaleMax)), false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if gm == nil {
		return "No GM", false
	}
	target := w.targetOfLocked(gm)
	if target == nil {
		target = gm
	}
	switch t := target.(type) {
	case *NPC:
		if t.gmScaleBase == 0 {
			t.gmScaleBase = t.Size
		}
		t.Size = t.gmScaleBase * factor
		if factor == 1 {
			t.gmScaleBase = 0
		}
		return fmt.Sprintf("%s: size %.2f (%gx its own %.2f)", t.Name, t.Size, factor, t.Size/factor), true
	case *Player:
		if t.gmScaleBase == 0 {
			t.gmScaleBase = t.Size
		}
		t.Size = t.gmScaleBase * factor
		if factor == 1 {
			t.gmScaleBase = 0
		}
		return fmt.Sprintf("%s: size %.2f (%gx its own %.2f)", t.Name, t.Size, factor, t.Size/factor), true
	}
	return "Nothing to scale", false
}
