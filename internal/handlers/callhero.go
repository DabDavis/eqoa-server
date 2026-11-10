package handlers

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// callhero.go — Call of the Hero's question (entity/callhero.go; jib's PR #1,
// 7b79650): each far member of the caster's group it reaches is asked in a
// Yes/No box (0x0034) under heroCallBoxMark, as a Summoning Portal's call asks
// (summonportal.go), and HandleDialogueOption hands the 0x0035 here. Yes within
// entity.HeroCallWait lands the member beside its caster
// (World.HeroCallBring); No, or the box closed, is a no, and the caster is told
// each answer. A 0x0034 CLIENT declines at once because a menu is up (255
// within portalDeclinedWithin) is no answer: the member is told once in a line
// who calls, and the box is sent again every portalAskAgain until the call
// lapses. A member whose game cannot load the caster's world is not asked (the
// caster told), and one already answering a call is not asked a second (the
// caster told to try again). Every word is ours.

// heroCallBoxMark marks a call's box (beside summonportal.go's list and the
// welcome guide's 0x61 and the herald's 0x62; 0x60's byte is the pets'
// non-collide id, petNoCollideID, and is left alone). jib's PR used
// 0x55000000, which is the were advisors' on main (wereinfect.go).
const heroCallBoxMark = 0x63000000

// The call's words (ours).
const (
	heroAsk     = "%s calls you to their side. Go?"
	heroAskMenu = "%s calls you to their side. Close your menu to answer."
	heroPassed  = "That call has passed."
	heroYouDead = "You cannot answer a call while you are dead."
	heroEnded   = "%s's call has ended."
	heroLapsed  = "%s's call has passed."

	heroAnswered = "%s answers your call."
	heroDeclined = "%s declines your call."
	heroNoAnswer = "%s does not answer your call."
	heroLeft     = "%s left before answering your call."
	heroNoWorld  = "%s's game cannot load this world."
	heroBusy     = "%s is already answering a call. Try again in a moment."
)

var heroAnswers = []string{"Yes", "No"}

// IsHeroCallBox reports a box a call asked.
func IsHeroCallBox(id uint32) bool { return id&0xFF000000 == heroCallBoxMark }

// heroCallOpen is a call out: its box on the member's screen.
type heroCallOpen struct {
	box                    uint32
	call                   entity.HeroCall
	asked, until, again    time.Time // again: when a box CLIENT declined goes out again; zero: none
	told                   bool      // a declined box's line has been said
	memberName, casterName string
}

// HeroCalls runs the calls' questions: one open call a member, by its id.
type HeroCalls struct {
	gw  *entity.World
	now func() time.Time

	mu    sync.Mutex
	seq   uint32
	calls map[entity.EntityID]*heroCallOpen
}

// NewHeroCalls makes the calls' questions over the world. Start begins its
// sweep.
func NewHeroCalls(gw *entity.World) *HeroCalls {
	return &HeroCalls{gw: gw, now: time.Now, calls: map[entity.EntityID]*heroCallOpen{}}
}

// Start begins the sweep over the calls.
func (g *HeroCalls) Start() {
	go func() {
		t := time.NewTicker(portalSweepEvery)
		defer t.Stop()
		for range t.C {
			g.sweep()
		}
	}()
}

// Ask is the World's hero-call hook: the member asked, or the caster told why
// not.
func (g *HeroCalls) Ask(c entity.HeroCall) {
	if g == nil || c.Sess == nil {
		return
	}
	if !CanLoadWorld(c.Sess, c.World) {
		log.Printf("Call: %s's %q does not ask %s: its game cannot load world %d", c.CasterName, c.Spell, c.MemberName, c.World)
		portalRefuse(c.CasterSess, fmt.Sprintf(heroNoWorld, c.MemberName))
		return
	}
	now := g.now()
	g.mu.Lock()
	if open := g.calls[c.Member]; open != nil && now.Before(open.until) {
		g.mu.Unlock()
		portalRefuse(c.CasterSess, fmt.Sprintf(heroBusy, c.MemberName))
		return
	}
	g.seq++
	o := &heroCallOpen{box: heroCallBoxMark | (g.seq & 0x00FFFFFF), call: c, asked: now, until: now.Add(entity.HeroCallWait),
		memberName: c.MemberName, casterName: c.CasterName}
	g.calls[c.Member] = o
	g.mu.Unlock()
	log.Printf("Call: %s's %q asks %s", c.CasterName, c.Spell, c.MemberName)
	SendQuestPopupMulti(c.Sess, o.box, fmt.Sprintf(heroAsk, c.CasterName), heroAnswers)
}

// Answer is HandleDialogueOption's question for a call's box. It reports
// whether the box was a call's (an old one does nothing but say so).
func (g *HeroCalls) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsHeroCallBox(box) {
		return false
	}
	if g == nil || g.gw == nil || p == nil {
		return true
	}
	now := g.now()
	g.mu.Lock()
	o := g.calls[p.ServerID]
	if o == nil || o.box != box {
		g.mu.Unlock()
		if option == 0 {
			portalTell(sess, heroPassed)
		}
		return true
	}
	if option == 255 && now.Sub(o.asked) < portalDeclinedWithin {
		// CLIENT declined it itself, a menu up: no answer; asked again
		o.again = now.Add(portalAskAgain)
		first := !o.told
		o.told = true
		name := o.casterName
		g.mu.Unlock()
		if first {
			portalTell(sess, fmt.Sprintf(heroAskMenu, name))
		}
		return true
	}
	delete(g.calls, p.ServerID)
	g.mu.Unlock()
	cs := g.inWorld(o.call.Caster)
	if option != 0 {
		log.Printf("Call: %s declines %s's %q", o.memberName, o.casterName, o.call.Spell)
		portalTell(cs, fmt.Sprintf(heroDeclined, o.memberName))
		return true
	}
	if now.After(o.until) {
		portalTell(sess, heroPassed)
		return true
	}
	ok, callerGone := g.gw.HeroCallBring(o.call.Caster, p.ServerID, sess, o.call.Ring, o.call.Spell)
	switch {
	case ok:
		portalTell(cs, fmt.Sprintf(heroAnswered, o.memberName))
	case callerGone:
		portalRefuse(sess, fmt.Sprintf(heroEnded, o.casterName))
	default:
		portalRefuse(sess, heroYouDead)
	}
	return true
}

// heroLine is a line the sweep says once its mutex is let go.
type heroLine struct {
	sess *session.Session
	line string
}

// sweep ends the calls whose time is up, or whose people are gone, and sends
// again the boxes CLIENT declined. Every portalSweepEvery. The mutex is taken
// before the World's lock here, and never the other way.
func (g *HeroCalls) sweep() {
	if g == nil || g.gw == nil {
		return
	}
	now := g.now()
	var lines []heroLine
	type askAgain struct {
		sess   *session.Session
		box    uint32
		caster string
	}
	var again []askAgain
	g.mu.Lock()
	for id, o := range g.calls {
		ms, cs := g.inWorld(id), g.inWorld(o.call.Caster)
		switch {
		case ms == nil:
			lines = append(lines, heroLine{cs, fmt.Sprintf(heroLeft, o.memberName)})
		case cs == nil:
			lines = append(lines, heroLine{ms, fmt.Sprintf(heroEnded, o.casterName)})
		case now.After(o.until):
			lines = append(lines, heroLine{ms, fmt.Sprintf(heroLapsed, o.casterName)},
				heroLine{cs, fmt.Sprintf(heroNoAnswer, o.memberName)})
		default:
			if !o.again.IsZero() && !now.Before(o.again) {
				o.again, o.asked = time.Time{}, now
				again = append(again, askAgain{ms, o.box, o.casterName})
			}
			continue
		}
		delete(g.calls, id)
	}
	g.mu.Unlock()
	for _, l := range lines {
		portalTell(l.sess, l.line)
	}
	for _, a := range again {
		SendQuestPopupMulti(a.sess, a.box, fmt.Sprintf(heroAsk, a.caster), heroAnswers)
	}
}

// inWorld is the player's session while it is in the world, else nil.
func (g *HeroCalls) inWorld(id entity.EntityID) *session.Session {
	if _, ok := g.gw.PlayerSnapshot(id); !ok {
		return nil
	}
	return g.gw.FindSessionByEntityID(id)
}
