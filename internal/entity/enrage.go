package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// enrage.go — a creature's enrage (database.AbilityEnrage; the GM Jibrish's petition 1443: "At 10% health
// mr moogie enrages at grows in size by 200% and should start summoning players on his agro list to him
// every 3 seconds, unless they are already in melee distance. ... where he attacks them with rampage. This
// should work as if a tank is main tanking moogie, he summons number 2 on the agro list and does rampage
// damage to them starting on his next combat round. Rampage should do 50% of the damage it is doing to the
// main threat target to the rampage target.").
//
// When its hit points first fall below its enrage's share (Below) in a fight, it enrages, once a fight,
// until that fight ends (its death, its hate list empty, a leash home, a GM's edit of the templates): a
// heal above the share leaves it enraged, and the next fight may enrage again. Its line in /say (Say),
// and:
//
//   - it grows Grow percent (200: three times its size, as "20% bigger" is 1.2 times), every client
//     drawing it so from the next update (its channel's size), its reach the bigger body's (reach.go);
//     its own size again at the fight's end;
//   - at once and every Every seconds after, every player of its hate list in its world, alive and not
//     loading a world, who does not stand within its melee reach is brought beside it, as a Call of the
//     Hero brings a target to its caster (gates.go's travel: the teleport, a place round it), told so in
//     a line; never one in its reach;
//   - Rampage percent of each of its melee hits on the one it fights (whom its melee rounds are at)
//     also lands on the second of its hate list (its highest threat after that one: a player, a pet, or
//     the NPC of a scripted fight), from the first round it begins after it enraged: fixed, through any
//     armour, as its special lands (templateStrikeLocked: the special's visual, damage shields answering a
//     player's), when that second stands within its reach; one farther off (the pulls bring a player in)
//     takes none of that hit. Its procs and specials are not rampaged.
//
// The pulls' words and their place round it are ours.

// enrageOn is an enrage under way: when it began (a rampage from the round begun after it), its size
// before it, and its next pull (zero: no pulls).
type enrageOn struct {
	at   time.Time
	size float32
	next time.Time
	e    *database.TemplateAbility
}

// The enrage's words (ours).
const (
	enragePulledLine = "%s pulls you to it!"
	enrageRampage    = database.DefaultRampageName
)

// enragePhaseLocked is npc's enrage in its fight f this tick: begun below its share, then its pulls on
// their clock. Must be called with the lock held.
func (w *World) enragePhaseLocked(npc *NPC, f *templateFight, now time.Time) {
	e := npc.tmpl.enrage
	if e == nil {
		return
	}
	if f.enraged == nil {
		if npc.HP*100 >= npc.MaxHP*e.Below {
			return
		}
		on := &enrageOn{at: now, size: npc.Size, e: e}
		f.enraged = on
		if e.Grow > 0 {
			npc.Size = npc.Size * float32(100+e.Grow) / 100
		}
		if e.Every > 0 {
			on.next = now
		}
		if e.Say != "" {
			w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, e.Say) // abilitytimes.go
		}
		log.Printf("Template: %s enrages (HP %d/%d, below %d%%): size %.2f -> %.2f, pulls every %gs, rampage %d%%", npc.Name,
			npc.HP, npc.MaxHP, e.Below, on.size, npc.Size, e.Every, e.Rampage)
	}
	on := f.enraged
	if on.next.IsZero() || now.Before(on.next) {
		return
	}
	on.next = now.Add(time.Duration(on.e.Every * float64(time.Second)))
	w.enragePullLocked(npc, now)
}

// inMeleeReachLocked reports a body at (x, y, z) within npc's melee reach, as its chase judges it: across
// the ground within its reach (npcReach), on its floor (not over or under it). Must be called with the
// lock held.
func inMeleeReach(npc *NPC, x, y, z float32) bool {
	return distXZ(npc.X, npc.Z, x, z) <= npcReach(npc) && absf(y-npc.Y) <= routeLevel
}

// enragePullLocked brings every player of npc's hate list who is not within its reach beside it (the file's
// doc), each a place of its own round it. Must be called with the lock held.
func (w *World) enragePullLocked(npc *NPC, now time.Time) {
	ids := make([]EntityID, 0, len(npc.Threats))
	for id, th := range npc.Threats {
		if th > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	ring := 0
	for _, id := range ids {
		p := w.Players[id]
		if p == nil || p.IsDead() || p.respawnPending() || p.World != npc.World || inMeleeReach(npc, p.X, p.Y, p.Z) || w.travelDueLocked(p) {
			continue
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess == nil || sess.StateChannelsHeld() {
			continue // loading a world, or no client to land
		}
		ring++
		w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, Ring: 1 + (ring-1)%4, Spell: npc.Name + "'s pull",
			To: GateSpot{World: npc.World, X: npc.X, Y: npc.Y, Z: npc.Z, Place: "beside " + npc.Name}})
		w.sendColoredLocked(sess, fmt.Sprintf(enragePulledLine, npc.Name), duelOrange)
		log.Printf("Template: %s's enrage pulls %s from %.1f off (reach %.1f)", npc.Name, p.Name,
			dist3(npc.X, npc.Y, npc.Z, p.X, p.Y, p.Z), npcReach(npc))
	}
}

// travelDueLocked reports a travel of p's on its way (a gate's, a call's, a pull's), not yet landed. Must
// be called with the lock held.
func (w *World) travelDueLocked(p *Player) bool {
	for _, t := range w.gates.due {
		if t.P == p {
			return true
		}
	}
	return false
}

// rampageLocked is npc's rampage after one of its melee hits of n landed on main, whom its round is at
// (the file's doc): its share of n on the second of its hate list within its reach. Must be called with
// the lock held.
func (w *World) rampageLocked(npc *NPC, main EntityID, n int, now time.Time) {
	f := npc.tfight
	if f == nil || f.enraged == nil || f.enraged.e.Rampage <= 0 || n <= 0 || npc.Dead || !npc.RoundStart.After(f.enraged.at) {
		return
	}
	second, x, y, z := w.secondFoeLocked(npc, main)
	if second == 0 || !inMeleeReach(npc, x, y, z) {
		return
	}
	hit := max(1, int(math.Round(float64(n)*float64(f.enraged.e.Rampage)/100)))
	log.Printf("Template: %s's %s: %d%% of its %d on %s lands on %s", npc.Name, enrageRampage, f.enraged.e.Rampage, n,
		w.entityNameLocked(main), w.entityNameLocked(second))
	w.templateStrikeLocked(npc, second, now, hit, enrageRampage)
}

// secondFoeLocked is the second of npc's hate list after main: its highest threat of the rest (the lower id
// of equal ones) that it may hit, a living player in its world (not across the water's edge from it), a
// living pet of a player's there, or the NPC of a scripted fight (scriptedFoes); and where it stands. 0
// for none. Must be called with the lock held.
func (w *World) secondFoeLocked(npc *NPC, main EntityID) (id EntityID, x, y, z float32) {
	var top int64
	for other, th := range npc.Threats {
		if other == main || th <= 0 || (id != 0 && (th < top || th == top && other > id)) {
			continue
		}
		if p := w.Players[other]; p != nil {
			if p.IsDead() || p.respawnPending() || p.World != npc.World || w.apartLocked(p, npc) {
				continue
			}
			id, top, x, y, z = other, th, p.X, p.Y, p.Z
			continue
		}
		n := w.NPCs[other]
		if n == nil || n.Dead || n.World != npc.World || (n.Owner == 0 && w.castFoeLocked(npc, other) == nil) {
			continue
		}
		id, top, x, y, z = other, th, n.X, n.Y, n.Z
	}
	return id, x, y, z
}

// enrageWords is #stats' words for a template's enrage: what it does, and whether it is on.
func enrageWords(npc *NPC) string {
	e := npc.tmpl.enrage
	if e == nil {
		return ""
	}
	s := fmt.Sprintf("enrage below %d%%: %d%% bigger, pulls every %gs, rampage %d%% on the second", e.Below, e.Grow, e.Every, e.Rampage)
	if f := npc.tfight; f != nil && f.enraged != nil {
		s += " (enraged now)"
	}
	return s
}
