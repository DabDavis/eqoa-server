package entity

import (
	"math/rand"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// wereinfect.go — lycanthropy caught from a were's melee (the user,
// 2026-11-06: "there is a layer to the were grind, players must be melee
// attacked by the were animal to be infected. the oasis NPCs around the
// campfire offer cures for a price. so you can be accidently infected,
// though very rare"). EQOA-Data's were guides: a yellow-con were gives by far
// the best chance ("possible from light blues and reds, but the percentage
// chance of getting infected from a yellow is huge in comparison"), and
// players were infected before their society's 1000, while still turning in
// tails. Each melee hit a were lands on a player (a round's hit, a class
// template's special) rolls: 1% from one that cons yellow to it, 0.1% from one
// white, dark blue, light blue or red, never from one green (the user's
// choice; dark blue, nearer than light blue, at the light blues' figure, ours).
// A roll that infects is told to the hearers (OnInfection: the quest engine,
// which runs it off the lock), who decide whether the character may catch it
// (a werehunter's brand, a fever or a form already: handlers/wereinfect.go)
// and mark it.

// InfectionHit is a were's bite that infects, as its hearers are told it.
type InfectionHit struct {
	Session  *session.Session
	ServerID uint32 // the character's (characters.serverid)
	Name     string
	Line     string // the quests' line: wolfwere, bearwere, ratwere, lionwere, gatorwere
	NPCName  string
}

// InfectionFunc hears a bite that infects. It is called with the World's lock
// held, so it must not call back into the World, nor block.
type InfectionFunc func(InfectionHit)

// OnInfection adds a hearer of every bite that infects.
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

// WereLine is the line a creature's bite carries, by its name; "" for one
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

// infectionPer10000 is a bite's chance in ten thousand by the con the were
// shows the player (the user's choice: yellow 1%, the rest but green 0.1%).
func infectionPer10000(con byte) int {
	switch con {
	case ConYellow:
		return 100
	case ConWhite, ConDarkBlue, ConLightBlue, ConRed:
		return 10
	}
	return 0
}

// infectIntn is the bites' dice (a test's own).
var infectIntn = rand.Intn

// wereBiteLocked rolls a were's melee hit on p and tells the hearers of a
// bite that infects. Must be called with the lock held, p standing.
func (w *World) wereBiteLocked(sess *session.Session, p *Player, npc *NPC) {
	if len(w.onInfection) == 0 || sess == nil || p == nil || npc == nil || p.IsDead() || npc.Owner != 0 {
		return
	}
	line := WereLine(npc.Name)
	if line == "" {
		return
	}
	chance := infectionPer10000(ConColor(p.Level, npc.Level))
	if chance <= 0 || infectIntn(10000) >= chance {
		return
	}
	h := InfectionHit{Session: sess, ServerID: p.CharServerID, Name: p.Name, Line: line, NPCName: npc.Name}
	for _, fn := range w.onInfection {
		fn(h)
	}
}
