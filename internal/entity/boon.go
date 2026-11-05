package entity

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// boon.go — the towns' boon (the user, 2026-11-05: "players want the boon at
// the Freeport Announcement, and other city announcements. The boon is an xp
// boost for an amount of time. Let's make it 2x Xp for 4 hours").
//
// Sandstorm's boon is an event of its own: "Server Message: A great wind
// stirs across Freeport.", every player in Freeport "blessed by Sandstorm
// Boon" and flung into the sky (claude-notes/f3-pass-20261031.md), and its
// players shout to ask when the next is. Here a town's announcement board
// gives one to whoever talks to it (handlers/signs.go): an effect named for
// the board's town ("Boon of Freeport") on the effects channel, with the
// Blessing spells' icon, that makes every experience award (a kill's, a
// quest's: awardXP) Boon.Percent of itself while it lasts, its lines saying
// so. A talk to a board with a boon on puts it back to its whole time (it
// never stacks), another town's in its place. Its time runs only in the
// world and it is kept over a logout, as every effect's (buffsave.go); a
// death does not end it (clearSpellsLocked), a GM's #clearbuffs does. The
// percent is read at each award, so a GM's change (world.boon_xp_pct, the GM
// panel's settings) is every boon's at once; the time is a boon's from its
// giving. None of it is a record's: the user's 2x and 4 hours; the name, the
// icon, the visual and the lines ours.

// BoonEffectID keys the boon among a player's effects: one at a time.
// Spells' ids are positive; food and drink are -1001 and -1002
// (consumable.go), potions -1,000,000 less their item (potion.go).
const BoonEffectID = -1003

// BoonIcon is the boon's icon on the effects channel: the Blessing spells'
// (Blessing, Nature's Blessing, Blessing of Valentina, ...), a DictID that
// VIWndInfluence blits whole.
const BoonIcon uint32 = 0xDA6F7DE9

// Boon is what a town's boon gives (world.boon_xp_pct, world.boon_duration,
// the GM panel's settings): Percent of every experience award while it lasts
// (100: the boards give none, and one on gives no more), for Duration of play
// from its giving.
type Boon struct {
	Percent  int
	Duration time.Duration
}

// DefaultBoon is the user's: double experience for four hours.
var DefaultBoon = Boon{Percent: 200, Duration: 4 * time.Hour}

// On is whether the boards give a boon.
func (b Boon) On() bool { return b.Percent > 100 && b.Duration > 0 }

// What is what the boon gives, in words: "double experience".
func (b Boon) What() string {
	switch b.Percent {
	case 200:
		return "double experience"
	case 300:
		return "triple experience"
	}
	return fmt.Sprintf("%d%% experience", b.Percent)
}

func (b Boon) String() string {
	if !b.On() {
		return "the boards give no boon (boon_xp_pct 100)"
	}
	return fmt.Sprintf("a town's board gives a boon of %s for %s of play", b.What(), spanWords(b.Duration))
}

// boonTag is what an award's line says of a boon's share: "boon x2".
func boonTag(pct int) string {
	return "boon x" + strconv.FormatFloat(float64(pct)/100, 'f', -1, 64)
}

// spanWords is a time in hours and minutes, a part of a minute counted
// whole: "4 hours", "1 hour 30 minutes".
func spanWords(d time.Duration) string {
	m := int((d + time.Minute - 1) / time.Minute)
	if m <= 0 {
		return "no time"
	}
	var parts []string
	for _, u := range []struct {
		n    int
		word string
	}{{m / 60, "hour"}, {m % 60, "minute"}} {
		switch {
		case u.n == 1:
			parts = append(parts, "1 "+u.word)
		case u.n > 1:
			parts = append(parts, strconv.Itoa(u.n)+" "+u.word+"s")
		}
	}
	return strings.Join(parts, " ")
}

// BoonName is the boon a town's board gives: "Boon of Freeport", "Boon" for
// a board of no town.
func BoonName(town string) string {
	if town = strings.TrimSpace(town); town == "" {
		return "Boon"
	}
	return "Boon of " + town
}

// BoonGiven is a board's boon as given: its name, what it gives and for how
// long, and the boon on before it ("" for none) with what was left of it.
type BoonGiven struct {
	Name     string
	Rule     Boon
	Replaced string
	Left     time.Duration
}

// Line is the player's line of chat for it.
func (g BoonGiven) Line() string {
	what := fmt.Sprintf("%s for %s of play.", g.Rule.What(), spanWords(g.Rule.Duration))
	if g.Replaced != "" {
		return fmt.Sprintf("The %s renews your blessing: %s", g.Name, what)
	}
	return fmt.Sprintf("You have been blessed by the %s: %s", g.Name, what)
}

// GiveBoon puts a town's boon on the session's player for the boon's whole
// time, in place of one on it; a new one (none on before) plays the buff
// visual on it to those round it, so a board talked to over and over is no
// spell cast at everyone near. False with no living player, or when the
// boards give none (Boon.On).
func (w *World) GiveBoon(sess *session.Session, town string, now time.Time) (BoonGiven, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	b := w.boon
	if p == nil || p.IsDead() || !b.On() {
		return BoonGiven{}, false
	}
	g := BoonGiven{Name: BoonName(town), Rule: b}
	for _, e := range p.Effects {
		if e.SpellID == BoonEffectID {
			g.Replaced, g.Left = e.Name, max(0, e.Until.Sub(now))
		}
	}
	w.addEffectLocked(p, &ActiveEffect{SpellID: BoonEffectID, Name: g.Name, Icon: BoonIcon, Caster: p.ServerID,
		Until: now.Add(b.Duration)}, now)
	if g.Replaced == "" {
		w.broadcastCastLocked(p, p.ServerID, 0, fxBuff)
	}
	if g.Replaced != "" {
		log.Printf("Boon: %s has the %s (%d%% for %v) in place of the %s (%v left)", p.Name, g.Name, b.Percent, b.Duration,
			g.Replaced, g.Left.Round(time.Second))
	} else {
		log.Printf("Boon: %s has the %s (%d%% for %v)", p.Name, g.Name, b.Percent, b.Duration)
	}
	return g, true
}

// BoonNotice is what a player entering the world is told of the boon (the
// user, 2026-11-05: "say Boon will be available at all City Announcements"):
// where it is and what it gives; false when the boards give none, or the
// player is not in the world or has one on.
func (w *World) BoonNotice(sess *session.Session) (string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.boon.On() || p.hasEffect(BoonEffectID) {
		return "", false
	}
	return fmt.Sprintf("The Boon is available at all City Announcements: %s for %s of play.", w.boon.What(),
		spanWords(w.boon.Duration)), true
}

// boonPctLocked is the share of an experience award a player gets: the
// boon's percent while one is on it, else 100. Must be called with the lock
// held.
func (w *World) boonPctLocked(p *Player) int {
	if w.boon.Percent > 100 && p.hasEffect(BoonEffectID) {
		return w.boon.Percent
	}
	return 100
}

// SetBoon sets what the towns' boon gives (world.boon_*, the GM panel's
// settings): the percent counts at once for every boon on, the time for the
// boons given after. A percent under 100 is 100 (no boon takes experience
// away), a time of 0 the default's.
func (w *World) SetBoon(b Boon) {
	b.Percent = max(100, b.Percent)
	if b.Duration <= 0 {
		b.Duration = DefaultBoon.Duration
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.boon = b
}

// Boon is what the towns' boon gives now.
func (w *World) Boon() Boon {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.boon
}
