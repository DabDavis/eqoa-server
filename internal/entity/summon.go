package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// summon.go — a creature's summon (database.AbilitySummon; the GM Jibrish's petition 1440: "at 25% hp mr
// moogie should summon 3 adds to assist him. They are all level 65 "Servant of Moogie". They have mr
// moogies damage and health and abilities minus a flat 50%. So if moogie crits for 6k, they'd crit for
// 3k. etc.").
//
// When its hit points first fall below its summon's share (Below) in a fight, Count copies of the NPC its
// summon names (NPC: the spawn-only original, spawnonly.go, copied as a GM trigger's spawn is: a
// Temporary NPC, its own body, size and level) rise beside it, a ring round it on the ground's floor
// there, facing what it fights, and join its fight: its hate list is theirs as it stands (its foes,
// players, pets and NPCs alike, at its own figures), so each goes for whom it would. Once a fight: a
// fight that ends (its hate list empty, a leash home, its death) and begins again may call them again,
// the next time it falls below the share. With Share the adds fight by its own class template at that
// share of its figures (the data's 50 for the Servants):
//
//   - their hit points are that share of its most hit points at the call;
//   - every figure of theirs is that share of its own, rolled as it would roll it: their melee hits are
//     its hits on the same target (its level's roll, its template's melee, its level's balance and con)
//     at the share, their special (its crit) its level's at the share, their kick, procs, damage shield,
//     regeneration in a fight and hit heal its own at the share, their abilities' hits, ticks and heals
//     of a spell's figure at the share (a heal of its share of hit points, Lay Hands', is of their own
//     hit points, so already the share of its), a fixed hit's figure at the share, a fight's proc its
//     own at the share, and a veil's tap of their own hit points;
//   - its spells by its level and class; its armour (taken_pct), resists, run and preferred foes are
//     theirs as they are;
//   - its summons and its enrage are its fight's own: an add calls no adds and never enrages, and its
//     abilities' lines in /say are its creature's (an add says none).
//
// Without Share an add is the copy it is, its own template if its name has one (npctemplate).
//
// They die with their creature (masterdeath.go: the owner's "when the summoned pet's master dies, so does
// the pet"), laid down dead where they stand, their deaths played, a corpse of nothing a few seconds; and
// they go when its fight ends any other way (its hate list empty, a leash home, a GM's edit of the
// templates, as every fight by a template ends at one, or its going from the world): out of the world at
// once, no corpse. One killed in the fight lies its corpse's time, a corpse of nothing no one loots
// (handlers' loot: CorpseOfNothing: no coins, no items, no loot table or category roll); its kill earns
// what a creature's of its level does. A keep restart carries no Temporary NPC: the adds are gone, their
// creature whole on its spawn's terms (keepnpc.go). Ours: the ring, the lines and the rules of their
// going.

// addTie is a summoned add's tie to its creature (the file's doc): the creature, the fight it was
// called in (another, or none, and it goes), its share of the creature's figures (0: its own), and the
// creature's level and class at the call, the level and class its figures are rolled as.
type addTie struct {
	by           *NPC
	fight        *templateFight
	pct          int
	level, class int
}

// templateSummon is one of a template's summons.
type templateSummon struct {
	database.TemplateAbility
}

// summonRing is how far from its creature an add rises (ours: inside every creature's reach, so the
// adds stand in its fight from the first), summonFloor how far from its height the floor there may be.
const (
	summonRing  float32 = 4
	summonFloor float32 = 20
)

// summoner is the creature whose summon called n, nil for any other NPC.
func (n *NPC) summoner() *NPC {
	if n == nil || n.add == nil {
		return nil
	}
	return n.add.by
}

// shareOf is an add's share of its creature's figures in percent (summon.go), 100 for any other NPC.
func (n *NPC) shareOf() int {
	if n.add == nil || n.add.pct <= 0 {
		return 100
	}
	return n.add.pct
}

// shared is a figure x of the NPC's at its share of its creature's (an add's), x for any other: at
// least 1 for an x above 0.
func (n *NPC) shared(x int) int {
	pct := n.shareOf()
	if pct == 100 || x <= 0 {
		return x
	}
	return max(1, int(math.Round(float64(x)*float64(pct)/100)))
}

// figures is the NPC its figures are rolled as: an add of a share, a copy of it at its creature's level
// and class at the call (its own template, effects and place: the creature's are its own, at the
// creature's level), itself for any other NPC.
func (n *NPC) figures() *NPC {
	if n.add == nil || n.add.pct <= 0 {
		return n
	}
	c := *n
	c.Level, c.Class = n.add.level, n.add.class
	return &c
}

// figureLevel is the level the NPC's figures are rolled at: an add's creature's, else its own.
func (n *NPC) figureLevel() int {
	if n.add != nil && n.add.pct > 0 {
		return n.add.level
	}
	return n.Level
}

// Summoned reports an add a creature's summon called (or its corpse): its corpse gives its own items
// alone (handlers' loot: no coins, no loot table of every creature's, no category roll).
func (n *NPC) Summoned() bool { return n != nil && n.add != nil }

// templatePhasesLocked is a templated NPC's moments in its fight this tick: each summon of its
// template's once a fight below its share of hit points, its enrage once (enrage.go), and its
// enrage's pulls on their clock. Never an add's, a pet's or a dummy's. Must be called with the lock
// held.
func (w *World) templatePhasesLocked(npc *NPC, now time.Time) {
	t := npc.tmpl
	if t == nil || npc.Dead || npc.add != nil || npc.Owner != 0 || npc.Dummy != NotDummy || npc.MaxHP <= 0 ||
		(len(t.summons) == 0 && t.enrage == nil) {
		return
	}
	f := npc.fight()
	for i := range t.summons {
		s := &t.summons[i]
		if i >= 32 || f.summoned&(1<<i) != 0 || npc.HP*100 >= npc.MaxHP*s.Below {
			continue
		}
		f.summoned |= 1 << i
		w.summonLocked(npc, f, s, now)
	}
	w.enragePhaseLocked(npc, f, now) // enrage.go
}

// summonLocked calls a summon's adds beside npc into its fight f (the file's doc). Must be called with the
// lock held.
func (w *World) summonLocked(npc *NPC, f *templateFight, s *templateSummon, now time.Time) {
	orig := w.npcByNameLocked(s.NPC)
	if orig == nil {
		log.Printf("Template: %s's summon below %d%% calls %q, and no NPC has the name: none comes", npc.Name, s.Below, s.NPC)
		return
	}
	facing := npc.Facing
	if x, _, z, world, ok := w.fightSpotLocked(npc.CurrentEnemy); ok && world == npc.World {
		facing = headingTo(x-npc.X, z-npc.Z)
	}
	called := make([]string, 0, s.Count)
	for k := 0; k < s.Count; k++ {
		a := facing + float32(2*math.Pi*float64(k)/float64(s.Count)) + math.Pi/float32(max(s.Count, 1))
		x, y, z := npc.X+summonRing*float32(math.Sin(float64(a))), npc.Y, npc.Z+summonRing*float32(math.Cos(float64(a)))
		if w.ground != nil {
			if fy, ok := w.ground.Floor(npc.World, x, y+3, z); ok && fy-y <= summonFloor && y-fy <= summonFloor {
				y = fy
			}
		}
		c := w.spawnCopyLocked(orig, x, y, z, facing, npc.World)
		c.NPCType &^= 0x80 // a fighter, attackable
		c.EventAggro, c.QuestSpawn = false, false
		c.add = &addTie{by: npc, fight: f, pct: s.Share, level: npc.Level, class: npc.Class}
		if s.Share > 0 {
			// its creature's template at its share: its hit points the share of the creature's most
			c.tmpl, c.tfight = npc.tmpl, nil
			c.tbase = templateBase{class: c.Class, maxHP: c.MaxHP}
			c.Class = npc.Class
			c.endRound()
			c.MaxHP = max(1, int(math.Round(float64(npc.MaxHP)*float64(s.Share)/100)))
			c.HP = c.MaxHP
		}
		w.joinAddLocked(c, npc)
		f.adds = append(f.adds, c.ServerID)
		w.adds = append(w.adds, c)
		called = append(called, fmt.Sprintf("%d", c.ServerID))
	}
	if s.Say != "" {
		w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, s.Say) // abilitytimes.go
	}
	log.Printf("Template: %s (HP %d/%d, below %d%%) calls %d %s (%s), level %d, %s, into its fight", npc.Name, npc.HP, npc.MaxHP,
		s.Below, s.Count, orig.Name, strings.Join(called, ", "), orig.Level, shareWords(s.Share, npc))
}

// shareWords says an add's share for the log.
func shareWords(share int, by *NPC) string {
	if share <= 0 {
		return "each as itself"
	}
	return fmt.Sprintf("each %d%% of %s's figures (%d hit points)", share, by.Name, max(1, int(math.Round(float64(by.MaxHP)*float64(share)/100))))
}

// joinAddLocked puts its creature's hate list on add, as it stands: its foes, its enemy whom it would
// fight of them (prefer.go). Must be called with the lock held.
func (w *World) joinAddLocked(add, by *NPC) {
	if add.Threats == nil {
		add.Threats = make(map[EntityID]int64, len(by.Threats))
	}
	for id, th := range by.Threats {
		if th > 0 && id != add.ServerID && add.Threats[id] < th {
			add.Threats[id] = th
		}
	}
	if id := w.pickEnemyLocked(add); id != 0 {
		add.CurrentEnemy = id
	}
	add.LastCombat = time.Now()
	w.wakeNPCLocked(add)
}

// addsSystem keeps each add standing to its creature's fight (the file's doc): one whose creature died
// dies with it (killNPC laid it down already, masterdeath.go); one whose creature is gone, or whose fight
// is over, leaves the world at once; one left with no enemy while its creature fights takes its
// creature's hate list up again. A dead add lies its corpse's time, its tie kept. Every tick; must be
// called with the lock held.
func (w *World) addsSystem(now time.Time) {
	if len(w.adds) == 0 {
		return
	}
	keep := w.adds[:0]
	for _, a := range w.adds {
		if w.NPCs[a.ServerID] != a || a.Dead {
			continue // gone (a GM's #despawn), or a corpse lying its time
		}
		by := a.add.by
		if by != nil && by.Dead {
			w.killWithMasterLocked(a, now) // it dies with its creature (masterdeath.go)
			continue
		}
		if by == nil || w.NPCs[by.ServerID] != by || by.tfight != a.add.fight {
			w.dropAggroLocked(a.ServerID)
			w.removeNPCLocked(a)
			log.Printf("Template: %s (%d) goes with its creature's fight", a.Name, a.ServerID)
			continue
		}
		if a.CurrentEnemy == 0 && by.CurrentEnemy != 0 && len(by.Threats) > 0 {
			w.joinAddLocked(a, by)
		}
		keep = append(keep, a)
	}
	for i := len(keep); i < len(w.adds); i++ {
		w.adds[i] = nil
	}
	w.adds = keep
}

// addWords is #stats' word on an add, "" for any other NPC.
func addWords(npc *NPC) string {
	a := npc.add
	if a == nil || a.by == nil {
		return ""
	}
	if a.pct <= 0 {
		return fmt.Sprintf("An add of %s's summon, as itself; it goes with its fight", a.by.Name)
	}
	return fmt.Sprintf("An add of %s's summon at %d%% of its figures (as its level %d); it goes with its fight", a.by.Name, a.pct, a.level)
}
