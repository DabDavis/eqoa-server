package entity

import "github.com/eqoa/server/internal/session"

// CoinSharers is who shares a corpse's tunar with the one who opens it
// (petition 1228, Waldorf: "Currently only the player in the group that loots
// a mob gets any tunar on them. During live, the entire group shared a portion
// of it"): the opener and its group's members as share its kill
// (killSharersLocked: in the corpse's world, within KillShareRange of it,
// alive), the opener first, each with its session (nil: none).
func (w *World) CoinSharers(opener *Player, npc *NPC) ([]*Player, []*session.Session) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ps := w.killSharersLocked(opener, npc)
	ss := make([]*session.Session, len(ps))
	for i, p := range ps {
		ss[i] = w.sessionOfLocked(p)
	}
	return ps, ss
}
