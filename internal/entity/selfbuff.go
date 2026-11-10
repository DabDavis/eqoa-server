package entity

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// selfbuff.go — a creature's buffs on itself, and its lines at a kill.
//
// A proc or a veil of a template that pins a spell (Innoruuk's Largesse of Plague, 700794, and Veil of
// Death, 901430) is that buff on its creature: the owner, 2026-11-10, "innoruuk is casting veil of death
// on moogie and the buffs, should be on innoruuk" (each proc and each tap had sent the buff's own cast
// visual at Mr. Moogie, which the client plays as a cast of it at him). Now its creature casts each on
// itself as its fight begins, a 0x00B1 from it at itself with the buff's own visual and no cast time, as
// a self buff shows on everyone's screen, and again at the buff's time (its record's duration: Largesse
// of Plague's 5 minutes) for as long as that fight lasts; a buff of no time once a fight. The proc and the
// veil are its template's whatever was cast (no effect is put on it: no client shows a creature's buffs,
// and Mr. Moogie's strip would take them); its procs' hits show the ability's own look (fx) at what they
// hit, the creature special's for none, and its veil's taps their fx or nothing.
//
// A killsay (database.AbilityKillSay; the GM Jibrish's petition 1458: "whenever he kills Innoruuk he says
// "Dab got dunked again"") is its creature's line in /say ("Name: text", heard as a say is, within a say's
// reach) when a hit of its own kills one of its foes by name: the killing blow, tracked exactly, of its
// melee, casts, specials, kicks, procs, veil, rampage or damage over time on a creature (an NPC: the fight's
// hit, castFoeHitLocked, and its melee, npcHitNPCLocked), or of its melee, spells, specials, procs, veil,
// damage shield or damage over time on a player. Not a death another's hit made, nor a pet's, nor an
// add's (a summon's copies fight by their creature's template and say none of its lines: summon.go).

// templateSelfBuff is a buff a templated creature casts on itself: its name, its look (a 0x00B1's
// visual), and how long it lasts (0: once a fight).
type templateSelfBuff struct {
	name  string
	fx    uint32
	every time.Duration
}

// selfBuffOf is the buff a pinned spell is: its name, its visual (its record's, else a buff's stand-in)
// and its time.
func selfBuffOf(s *CastSpell) *templateSelfBuff {
	return &templateSelfBuff{name: s.Name, fx: s.visual(), every: s.Def.Duration}
}

// templateKillSay is a template's line at a kill: the foes' names (lower case) and the line.
type templateKillSay struct {
	foes []string
	say  string
}

// killSayOf is a killsay ability as the world says it.
func killSayOf(a database.TemplateAbility) templateKillSay {
	k := templateKillSay{say: a.Say}
	for _, f := range a.Foes {
		k.foes = append(k.foes, strings.ToLower(strings.TrimSpace(f)))
	}
	return k
}

// selfBuffsLocked casts npc's buffs on itself that are due in its fight f (the file's doc): each as the
// fight begins, then at its time. Must be called with the lock held.
func (w *World) selfBuffsLocked(npc *NPC, f *templateFight, now time.Time) {
	buffs := npc.tmpl.selfBuffs
	if len(buffs) == 0 {
		return
	}
	if len(f.buffNext) != len(buffs) {
		f.buffNext = make([]time.Time, len(buffs))
	}
	for i := range buffs {
		b := &buffs[i]
		if !f.buffNext[i].IsZero() && now.Before(f.buffNext[i]) {
			continue
		}
		if b.every > 0 {
			f.buffNext[i] = now.Add(b.every)
		} else {
			f.buffNext[i] = now.Add(100 * 365 * 24 * time.Hour) // once a fight
		}
		w.broadcastNPCCastLocked(npc, npc.ServerID, 0, b.fx)
		log.Printf("Template: %s casts %q on itself (its proc's buff), again in %v", npc.Name, b.name, b.every)
	}
}

// killSaidLocked is killer's line in /say when its hit killed the one named victim (the file's doc), each
// killsay of its template's that names it. Must be called with the lock held.
func (w *World) killSaidLocked(killer *NPC, victim string) {
	if killer == nil || killer.add != nil || killer.tmpl == nil || len(killer.tmpl.killSays) == 0 {
		return
	}
	v := strings.ToLower(strings.TrimSpace(victim))
	for _, k := range killer.tmpl.killSays {
		for _, f := range k.foes {
			if f == v {
				n := w.sayNearLocked(killer.Name, killer.World, killer.X, killer.Z, k.say) // abilitytimes.go
				log.Printf("Template: %s says %q at %s's death (heard by %d)", killer.Name, k.say, victim, n)
				break
			}
		}
	}
}

// killedPlayerLocked is killSaidLocked for a player a creature's hit brought down: only when the player
// died of it (a GM's #testmode stands at 1 HP). Must be called with the lock held.
func (w *World) killedPlayerLocked(killer *NPC, p *Player) {
	if p != nil && p.IsDead() {
		w.killSaidLocked(killer, p.Name)
	}
}
