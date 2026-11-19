package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// werecrit.go — the weres' critical hit and its infection (work 1461, the GM
// a GM's of 2026-11-14 after checking the live game's rates).
//
// Every hostile were of the five lines (WereLine: wolfwere or werewolf, bearwere
// or werebear, wererat or ratwere, lionwere or werelion, gatorwere or weregator,
// any rank, never a pet) fights by a copy of its template of its own
// (wereTemplateLocked): its template's class, hit points, melee, abilities and
// all as they are, its Critical Hit on a clock of 30 s with no opening delay and
// no jitter; one with no template (Heirloom's 34 level 50-60 weres, 2026-11-15)
// by a template of nothing but that crit, its own class, hit points and melee;
// one whose template has no timed crit gets one at its special's damage a level,
// else the roamer's 7.4 (round(its level x 7.4), the normal special's figure).
// The roamer's row itself, which the other creatures share, is not changed. So a
// were's crit is a round of its own (critround.go, work 1340): its first attack
// of every fight, then every 30 s at its next round's start.
//
// Its infection ability rides its crit: ready at its spawn, spent by its first
// crit that lands on anything (a player, a pet), whatever the roll, and ready
// again WereInfectCooldown later, on the creature, not its fight: a fight left,
// a leash, another target, a player zoning or logging out leave it; only a death
// and a new spawn (respawnNPCLocked) make it ready again at once. A restart
// that keeps the sessions carries it with the creature (keepnpc.go). The crit it
// rides plays Disease Cloud's visual (spell 875's, the reference server's recorded cast,
// 0x052EF90F: a green cloud) in place of the special's, roll or none, and hits
// for the crit's damage; on a player that stands after it, one roll in ten
// thousand against wereInfectPer10000 of the con the were shows that player at
// that moment. A roll that infects is told to the hearers with its own key;
// every later crit while the ability waits is the plain crit. Nothing else of a
// were's (its quads, a kick, a proc) rolls.

// WereInfectCooldown is the infection ability's wait after its use.
const WereInfectCooldown = 24 * time.Hour

// WereCritEvery is a were's crit's clock, seconds; WereCritDamage its damage a
// level when its template gives none (the roamer's).
const (
	WereCritEvery  = 30
	WereCritDamage = 7.4
)

// WereInfectFX is the infection's visual on its crit: Disease Cloud's.
const WereInfectFX uint32 = 0x052EF90F

// wereTemplateName names a were's template made of none.
const wereTemplateName = "were"

// wereTemplateLocked is a were's copy of t (nil: of none), made once a template.
// Must be called with the lock held.
func (w *World) wereTemplateLocked(t *ClassTemplate) *ClassTemplate {
	if d := w.templates.weres[t]; d != nil {
		return d
	}
	d := wereTemplateOf(t)
	if w.templates.weres == nil {
		w.templates.weres = map[*ClassTemplate]*ClassTemplate{}
	}
	w.templates.weres[t] = d
	return d
}

// wereTemplateOf is a were's copy of t (the file's doc).
func wereTemplateOf(t *ClassTemplate) *ClassTemplate {
	var d ClassTemplate
	if t != nil {
		d = *t
	} else {
		d.ClassTemplate = database.ClassTemplate{Name: wereTemplateName, Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100}
	}
	if !hasSpecial(&d) {
		d.SpecialDamage = WereCritDamage
	}
	if d.Special == "" {
		d.Special = "Critical Hit"
	}
	d.SpecialEvery, d.SpecialFirst, d.SpecialJitter, d.SpecialFrom = WereCritEvery, 0, 0, 0
	d.were = true
	return &d
}

// wereAbilityReady reports a were whose infection ability its next crit carries.
func (n *NPC) wereAbilityReady(now time.Time) bool {
	return n.tmpl != nil && n.tmpl.were && n.Owner == 0 && !now.Before(n.wereReady)
}

// wereCritLocked lands a were's crit with its infection when the ability is
// ready, the ability spent first (whatever it lands on and whatever the roll);
// false when it is not ready (the plain crit is the caller's). Must be called
// with the lock held.
func (w *World) wereCritLocked(npc *NPC, target EntityID, now time.Time) bool {
	if !npc.wereAbilityReady(now) {
		return false
	}
	line := WereLine(npc.Name)
	if line == "" {
		return false
	}
	npc.wereReady = now.Add(WereInfectCooldown)
	t := npc.tmpl
	damage := npc.shared(specialFigure(t, npc.figureLevel()))
	struck := false
	w.templateStrikeFXLocked(npc, target, now, damage, specialName(t), WereInfectFX, func(sess *session.Session, p *Player) {
		struck = true
		w.wereInfectRollLocked(sess, p, npc, line, now)
	})
	if !struck {
		log.Printf("Weres: %s's infection spent on %s (no player standing to roll for); ready again %s", npc.Name,
			w.strikeNameLocked(target), npc.wereReady.UTC().Format(time.RFC3339))
	}
	return true
}

// wereInfectRollLocked is the one roll of a were's opening crit on p, standing
// after it. Must be called with the lock held.
func (w *World) wereInfectRollLocked(sess *session.Session, p *Player, npc *NPC, line string, now time.Time) {
	con := ConColor(p.Level, npc.Level)
	chance := wereInfectPer10000(con)
	roll := infectIntn(10000)
	took := roll < chance
	log.Printf("Weres: %s's infection on %s (con %s): rolled %d against %d in 10000, %v; ready again %s", npc.Name, p.Name,
		conWords(con), roll, chance, map[bool]string{true: "infects", false: "no infection"}[took], npc.wereReady.UTC().Format(time.RFC3339))
	if !took || sess == nil {
		return
	}
	h := InfectionHit{Session: sess, ServerID: p.CharServerID, Name: p.Name, Line: line, NPCName: npc.Name,
		NPCID: uint32(npc.ServerID), Gen: npc.spawnGen, At: now,
		Key: fmt.Sprintf("%d/%d/%d", npc.ServerID, npc.spawnGen, now.UnixMilli())}
	for _, fn := range w.onInfection {
		fn(h)
	}
}

// strikeNameLocked is what a crit struck, for a log line.
func (w *World) strikeNameLocked(id EntityID) string {
	if p := w.Players[id]; p != nil {
		return p.Name
	}
	if n := w.NPCs[id]; n != nil {
		return fmt.Sprintf("%q", n.Name)
	}
	return fmt.Sprint(id)
}

// conWords is a con colour in a word, for the log.
func conWords(con byte) string {
	switch con {
	case ConRed:
		return "red"
	case ConYellow:
		return "yellow"
	case ConWhite:
		return "white"
	case ConDarkBlue:
		return "dark blue"
	case ConLightBlue:
		return "light blue"
	case ConGreen:
		return "green"
	}
	return fmt.Sprint(con)
}

// wereWords is a were's infection for #stats; "" for no were.
func wereWords(npc *NPC, now time.Time) string {
	if npc.tmpl == nil || !npc.tmpl.were {
		return ""
	}
	if npc.wereAbilityReady(now) {
		return fmt.Sprintf("were infection (%s) ready on its next crit: 1%% green, 5%% other cons", InfectionName(WereLine(npc.Name)))
	}
	return fmt.Sprintf("were infection (%s) spent, ready again in %v", InfectionName(WereLine(npc.Name)),
		npc.wereReady.Sub(now).Round(time.Second))
}
