package entity

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// recastgroup.go — spells that share one recast (ticket 2011, Brother, 2026-11-13:
// "should Not be able to cast Hunter Form or Wolf form while one or the other is on
// Cooldown"). The ranger's Wolf's Instinct and its no-form upgrade Hunter's Instinct
// both stay in the book (ticket 1241 asked the upgrade to keep the old ability), each
// with a 30 minute recast of its own, so a ranger cast one and then the other.
//
// A spell's recast group is data (database/recastgroup.go, table spellrecastgroup),
// handed to the cast with the spells of the group the caster's book holds
// (CastSpell.RecastGroup, GroupMates: handlers/cast.go). Casting one of a group locks
// out every one of them for the cast spell's own lockout (its cast time, its recast
// and castLockoutExtra), each told by its own 0x00CD so the client greys its button
// too; a cast of any of them while locked is refused as a recast is (castWaitFor
// finds the mate's lockout), and each lockout ends with its 0x00CD 0 and is kept
// over a logout and a restart as any other (lockoutsave.go, keep.go). A spell of the
// group not in the book (bought later) is locked as well, shown on no ability.
//
// The spells of a group are alternatives of each other, so their lasting effects
// are one buff line (BuffLine: "recast:<group> <bearer>"), whatever they do: the
// newer takes the older off (one rank, the line's rule), and Wolf's Instinct, an
// illusion with no line of its own (a form's), no longer stacks with Hunter's
// Instinct's same stats.

// GroupMate is a spell of the cast spell's recast group, with its place in the
// caster's book (-1 for one the book lacks).
type GroupMate struct {
	SpellID      int
	DisplayIndex int
}

// recastLinePrefix begins a recast group's buff line.
const recastLinePrefix = "recast:"

// recastGroupLine is the buff line of a spell of recast group grp borne by bearer.
func recastGroupLine(grp, bearer string) string {
	return recastLinePrefix + strings.Join(strings.Fields(grp), "_") + " " + bearer
}

// lockRecastGroupLocked locks out every other spell of the cast spell's recast
// group until until (the cast spell's own lockout end), each told to the client.
// A mate already locked out longer keeps its end. Must be called with the lock held.
func (w *World) lockRecastGroupLocked(sess *session.Session, p *Player, spell *CastSpell, now, until time.Time) {
	if spell.RecastGroup == "" {
		return
	}
	for _, m := range spell.GroupMates {
		if m.SpellID == spell.SpellID {
			continue
		}
		end, had := until, false
		for i := range p.Lockouts {
			if l := &p.Lockouts[i]; l.spellID == m.SpellID {
				had = true
				l.displayIndex = m.DisplayIndex
				if until.After(l.until) {
					l.until = until
				}
				if l.until.After(end) {
					end = l.until
				}
			}
		}
		if !had {
			p.Lockouts = append(p.Lockouts, castLockout{spellID: m.SpellID, displayIndex: m.DisplayIndex, until: until})
		}
		if sess != nil {
			sendCooldown(sess, m.DisplayIndex, end.Sub(now))
		}
		log.Printf("Cast: %s's %q locks out spell %d (recast group %q) for %v", p.Name, spell.Name, m.SpellID,
			spell.RecastGroup, end.Sub(now).Round(time.Millisecond))
	}
}
