package entity

import (
	"math/rand"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// wereinfect.go — lycanthropy caught from a were (the user, 2026-11-06: "there
// is a layer to the were grind, players must be melee attacked by the were
// animal to be infected. the oasis NPCs around the campfire offer cures for a
// price. so you can be accidently infected, though very rare"), since work 1461
// (the GM Jibrish, 2026-11-14) by the were's opening critical hit alone: each
// spawn's infection ability rides its first crit and then waits a day
// (werecrit.go), rolling once at the crit on the player it strikes, 1% from a
// were that cons green to the player and 5% from every other con. A roll that
// infects is told to the hearers (OnInfection: the quest engine, which runs it
// off the lock), who decide whether the character may catch it (a werehunter's
// brand, an infection already, a form already: handlers/wereinfect.go), give it
// as the seven-day infection (work 1463) and show it. Until 1461 every melee hit
// a were landed rolled: 1% yellow, 0.1% the other non-green cons.

// InfectionHit is a were's crit that infects, as its hearers are told it.
type InfectionHit struct {
	Session  *session.Session
	ServerID uint32 // the character's (characters.serverid)
	Name     string
	Line     string // the quests' line: wolfwere, bearwere, ratwere, lionwere, gatorwere
	NPCName  string
	// NPCID and Gen are the were's id and its spawn's generation (werecrit.go),
	// At the crit's time, Key the roll's own key: a grant retried after a
	// failure to keep it is the same grant (handlers/wereinfect.go)
	NPCID uint32
	Gen   uint32
	At    time.Time
	Key   string
}

// InfectionFunc hears a crit that infects. It is called with the World's lock
// held, so it must not call back into the World, nor block.
type InfectionFunc func(InfectionHit)

// OnInfection adds a hearer of every crit that infects.
func (w *World) OnInfection(fn InfectionFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onInfection = append(w.onInfection, fn)
}

// wereKinds are the weres by the words their names carry ("elder wolfwere",
// "werewolf", "Ancient Weregator"), each with its line's key.
var wereKinds = []struct{ a, b, line string }{
	{"wolfwere", "werewolf", "wolfwere"},
	{"bearwere", "werebear", "bearwere"},
	{"wererat", "ratwere", "ratwere"},
	{"lionwere", "werelion", "lionwere"},
	{"gatorwere", "weregator", "gatorwere"},
}

// WereLines are the five lines, in order.
var WereLines = []string{"wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"}

// WereLine is the line a creature's crit carries, by its name; "" for one
// that is no were.
func WereLine(name string) string {
	n := strings.ToLower(name)
	for _, k := range wereKinds {
		if strings.Contains(n, k.a) || strings.Contains(n, k.b) {
			return k.line
		}
	}
	return ""
}

// wereInfectPer10000 is the opening crit's chance in ten thousand by the con
// the were shows the player (Jibrish, 2026-11-14: "Set green con to 1%", every
// other con 5%).
func wereInfectPer10000(con byte) int {
	if con == ConGreen {
		return 100
	}
	return 500
}

// infectIntn is the infection's dice (a test's own).
var infectIntn = rand.Intn

// The infection as a player carries it (work 1463): an effect of its own on the
// effects channel, a line's id and name, Disease Cloud's icon (spell 875's front
// icon, a disease's: Plague's and Innothule Fever's too, the same on both
// shards), harmful (a strip leaves it) and past every cure but the campfire's
// and the antidote (NoCure), for seven days of real time from its catching. Its
// time is the handlers' charinfection row's absolute expiry: the effect is put on
// at the catching and at every login with that expiry (never kept in charbuffs),
// and effectSystem takes it off at its end, as the quest engine's sweep ends the
// row. CLIENT's effects record carries an icon and a name and no time
// (EffectRecordSize: a u32 icon and a 64-byte name), so seven days fits it; the
// time left is said in words at the catching and at each login.
const (
	InfectionIcon     uint32 = 0xCF3C577B
	InfectionDuration        = 7 * 24 * time.Hour
	infectionEffectID        = -1021 // to -1025, a line each: WereLines' order
)

// InfectionEffectID is the line's infection's effect id; 0 for no line.
func InfectionEffectID(line string) int {
	for i, l := range WereLines {
		if l == line {
			return infectionEffectID - i
		}
	}
	return 0
}

// infectionEffect reports an infection's effect id.
func infectionEffect(spellID int) bool {
	return spellID <= infectionEffectID && spellID > infectionEffectID-len(WereLines)
}

// InfectionName is the line's infection as the effects window names it:
// "were-wolf infection".
func InfectionName(line string) string {
	return "were-" + strings.TrimSuffix(line, "were") + " infection"
}

// PutInfection puts the line's infection on the session's player until its end,
// in place of any infection on it, its icon in a slot even when the eight are
// taken (the last of them gives its slot up: its effect goes on unseen, as a
// ninth's would). False with no player, no line or an end gone by.
func (w *World) PutInfection(sess *session.Session, line string, until, now time.Time) bool {
	id := InfectionEffectID(line)
	if id == 0 || !now.Before(until) {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	w.takeInfectionLocked(p)
	if len(p.Buffs) >= MaxEffects {
		p.Buffs = p.Buffs[:MaxEffects-1]
	}
	w.addEffectLocked(p, &ActiveEffect{SpellID: id, Name: InfectionName(line), Icon: InfectionIcon, Until: until,
		Rules: EffectRules{Harm: true, NoCure: true}}, now)
	return true
}

// TakeInfection takes any infection off the session's player; false with none
// on it.
func (w *World) TakeInfection(sess *session.Session) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && w.takeInfectionLocked(p)
}

func (w *World) takeInfectionLocked(p *Player) bool {
	took := false
	for _, line := range WereLines {
		took = w.removeEffectLocked(p, InfectionEffectID(line)) || took
	}
	return took
}

// Infection is the infection on the session's player: its line and end, "" for
// none.
func (w *World) Infection(sess *session.Session) (line string, until time.Time) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", time.Time{}
	}
	for _, e := range p.Effects {
		if infectionEffect(e.SpellID) {
			return WereLines[infectionEffectID-e.SpellID], e.Until
		}
	}
	return "", time.Time{}
}
