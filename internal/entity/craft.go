package entity

import "github.com/eqoa/server/internal/session"

// CraftView is a player as the tradeskills see it (handlers/craft.go): who it
// is, and the seven stats a craft's roll reads, as the stat channel carries
// them (StatNow: the base and what items and spells add, capped).
type CraftView struct {
	Name         string
	CharServerID uint32
	Level        int
	Dead         bool
	// Stats is STR, STA, AGI, DEX, WIS, INT and CHA, in that order
	// (CraftStatOrder).
	Stats [7]int
}

// CraftStatOrder is the stats of a CraftView's Stats, in their order.
var CraftStatOrder = [7]Stat{StatStr, StatSta, StatAgi, StatDex, StatWis, StatInt, StatCha}

// CraftView is the session's player as the tradeskills see it, false when
// the session has none in the world.
func (w *World) CraftView(sess *session.Session) (CraftView, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CraftView{}, false
	}
	v := CraftView{Name: p.Name, CharServerID: p.CharServerID, Level: p.Level, Dead: p.IsDead()}
	for i, st := range CraftStatOrder {
		v.Stats[i] = p.StatNow(st)
	}
	return v, true
}
