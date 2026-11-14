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
	// XPPct is the share of a tradeskill's XP the player gets, XPTag its line's words: the towns'
	// boon and the GM's reward, as a kill's (gmrewardxp.go xpPctLocked; the owner, 2026-11-14: "the
	// boon 6x bonus xp for live and heirloom applies to crafting also"). 0 is 100.
	XPPct int
	XPTag string
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
	v.XPPct, v.XPTag = w.xpPctLocked(p) // a boon's, a GM's reward's (gmrewardxp.go)
	for i, st := range CraftStatOrder {
		v.Stats[i] = p.StatNow(st)
	}
	return v, true
}
