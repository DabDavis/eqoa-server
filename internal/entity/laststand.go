package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// laststand.go — a creature's last stand (database.AbilityLastStand; the GM Jibrish's petition 1522:
// "Add a new ability to Mr. Moogie. If he reaches 2% hp he completely heals to full and doubles his
// damage if a non player character is hitting him").
//
// When its hit points are at its last stand's share (Below) or under and a non-player is hitting it, it
// stands: healed to its full, its damage DamagePct percent of what it was (200: doubled) until that
// fight ends (its death, its hate list empty, a leash home, a GM's edit of the templates), its line in
// /say (Say). A non-player hitting it is an NPC alive on its hate list in its world: a player's pet, a
// mercenary, a creature a GM's trigger set on it (Tunare, Lord Nagafen); a familiar draws no one's hate
// (familiar.go) and hits nothing, and its own adds fight for it. With players alone on its list nothing
// happens, at 2% or at its death; an NPC joining while it is at the share or under sets it off then.
//
// Once a fight, as its summons and its enrage are (summon.go, enrage.go): it stands once, the next fight
// may again. Every time would make it unkillable while any pet was on its list, and its damage would
// double again at each, four times, eight times; once is a second whole bar of hit points at twice the
// damage, which a raid that brought help must still beat. Its damage after it is its melee (its rounds,
// and so a rampage of them), its kick and its fixed hits (a lifetap's 4,000), not its procs.
//
// The check is the fight's tick's (templatePhasesLocked), as the other moments' are: a tenth of a
// second's damage past the whole share at once (40,000 of Mr. Moogie's two million) kills it first.

// lastStandOn is a last stand under way: when it stood and its ability.
type lastStandOn struct {
	at time.Time
	a  *database.TemplateAbility
}

// lastStandPhaseLocked is npc's last stand in its fight f this tick (the file's doc). Must be called with
// the lock held.
func (w *World) lastStandPhaseLocked(npc *NPC, f *templateFight, now time.Time) {
	a := npc.tmpl.lastStand
	if a == nil || f.lastStand != nil || npc.HP*100 > npc.MaxHP*a.Below {
		return
	}
	by := w.npcHittingLocked(npc)
	if by == 0 {
		return // players alone: nothing
	}
	was := npc.HP
	npc.HP = npc.MaxHP
	f.lastStand = &lastStandOn{at: now, a: a}
	if a.Say != "" {
		w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, a.Say) // abilitytimes.go
	}
	log.Printf("Template: %s stands (HP %d/%d, at or below %d%%, %s hitting it): healed to %d, its damage %d%% for this fight",
		npc.Name, was, npc.MaxHP, a.Below, w.entityNameLocked(by), npc.HP, a.DamageOr())
}

// npcHittingLocked is a non-player hitting npc (the file's doc), the lowest id of them; 0 for none. Must
// be called with the lock held.
func (w *World) npcHittingLocked(npc *NPC) EntityID {
	ids := make([]EntityID, 0, len(npc.Threats))
	for id, th := range npc.Threats {
		if th <= 0 || id == npc.ServerID {
			continue
		}
		if _, player := w.Players[id]; player {
			continue
		}
		o := w.NPCs[id]
		if o == nil || o.Dead || o.World != npc.World || o.Dummy != NotDummy || w.isFamiliarIDLocked(id) {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return 0
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids[0]
}

// lastStandPct is the percent of its damage npc deals now: its last stand's while one is on this fight,
// else 100.
func (n *NPC) lastStandPct() int {
	if n.tfight == nil || n.tfight.lastStand == nil {
		return 100
	}
	return n.tfight.lastStand.a.DamageOr()
}

// stoodOut is a figure of npc's damage (a melee hit, its kick, a fixed hit) as its last stand makes it,
// at least 1.
func (n *NPC) stoodOut(x int) int {
	pct := n.lastStandPct()
	if pct == 100 || x <= 0 {
		return x
	}
	return max(1, int(math.Round(float64(x)*float64(pct)/100)))
}

// lastStandWords is npc's last stand as the panel's words say it, "" for none.
func lastStandWords(npc *NPC) string {
	a := npc.tmpl.lastStand
	if a == nil {
		return ""
	}
	s := fmt.Sprintf("last stand at %d%% with a non-player hitting it: healed whole, its damage %d%%", a.Below, a.DamageOr())
	if f := npc.tfight; f != nil && f.lastStand != nil {
		s += " (standing now)"
	}
	return s
}
