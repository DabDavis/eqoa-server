package entity

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
)

// bestiary.go — each character's bestiary (Heirloom petition 3192: "some type of
// bestiary that shows mobs you encounter and the zones they are from"), kept in table
// charbestiary (database/bestiary.go) and shown by handlers/bestiary.go.
//
// What counts, two places only:
//
//   - a meeting: a creature takes a player into its hate, the player's own or its
//     pet's (noteHateLocked, hcrules.go: everything that starts or feeds a fight, a hit
//     either way, a harmful spell, a taunt, an aggro on sight). Written once a login
//     for each kind and zone (Player.bestiaryMet), its time the first seen;
//   - a kill: every player a kill is credited to (creditKillLocked's KillCredit: the
//     killer and the group members it shares with), and the killer itself whatever
//     the Hardcore rules give it.
//
// A kind is the creature's name; the zone is where it spawns (where it is "from"):
// a heroic dungeon's name in the dungeons' world, else the handlers' zone namer
// (SetBestiaryZoner: the map's zones), else the world's number. Not counted: a
// player's pet or a charmed creature (an owner), a mercenary or its pet, a training
// dummy, and a creature no one may attack (the NPC bit, 0x80).
//
// The notes are written after the world lock is let go, as the combat records are:
// queued here and handed in a batch to the BestiaryFunc each tick (runBestiary), and by
// SaveAll before a restart or a shutdown (FlushBestiary).

// BestiaryFunc writes a batch of notes (handlers': database.RecordBestiary).
type BestiaryFunc func([]database.BestiaryNote)

// BestiaryZoner names the zone of a place in a world ("" for none known).
type BestiaryZoner func(world uint8, x, z float32) string

// bestiaryState is the notes waiting for the hook, in order, and the zone namer.
type bestiaryState struct {
	fn    BestiaryFunc
	zoner BestiaryZoner
	mu    sync.Mutex // the queue
	run   sync.Mutex // one runner at a time
	queue []database.BestiaryNote
}

// OnBestiary sets the hook that writes the notes; nil keeps none.
func (w *World) OnBestiary(fn BestiaryFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bestiary.mu.Lock()
	defer w.bestiary.mu.Unlock()
	w.bestiary.fn = fn
}

// SetBestiaryZoner sets the bestiary's zone namer.
func (w *World) SetBestiaryZoner(z BestiaryZoner) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bestiary.zoner = z
}

// bestiaryKind is the kind a creature counts as, false for one the bestiary leaves out.
func bestiaryKind(npc *NPC) (string, bool) {
	if npc == nil || npc.Owner != 0 || npc.isMerc() || npc.isMercPet() || npc.Dummy != NotDummy || npc.NPCType&0x80 != 0 {
		return "", false
	}
	name := strings.TrimSpace(npc.Name)
	return name, name != ""
}

// BestiaryZoneLocked is the zone the bestiary names for a creature: its spawn's. Must be
// called with the lock held.
func (w *World) bestiaryZoneLocked(npc *NPC) string {
	x, z := npc.SpawnX, npc.SpawnZ
	if x == 0 && z == 0 {
		x, z = npc.X, npc.Z
	}
	if w.dungeons.on && npc.World == w.dungeons.world {
		for i := range w.dungeons.defs {
			if w.dungeons.defs[i].has(x, z) {
				return w.dungeons.defs[i].Name + " (Heroic)"
			}
		}
	}
	if w.bestiary.zoner != nil {
		if n := strings.TrimSpace(w.bestiary.zoner(npc.World, x, z)); n != "" {
			return n
		}
	}
	return fmt.Sprintf("World %d", npc.World)
}

// bestiaryOn is whether a hook takes the notes.
func (w *World) bestiaryOn() bool {
	w.bestiary.mu.Lock()
	defer w.bestiary.mu.Unlock()
	return w.bestiary.fn != nil
}

func (w *World) queueBestiaryLocked(n database.BestiaryNote) {
	w.bestiary.mu.Lock()
	defer w.bestiary.mu.Unlock()
	if w.bestiary.fn != nil {
		w.bestiary.queue = append(w.bestiary.queue, n)
	}
}

// bestiaryMetLocked notes p meeting npc: once a login for its kind and zone. Must be
// called with the lock held.
func (w *World) bestiaryMetLocked(npc *NPC, p *Player, now time.Time) {
	if p == nil || p.CharServerID == 0 || !w.bestiaryOn() {
		return
	}
	kind, ok := bestiaryKind(npc)
	if !ok {
		return
	}
	zone := w.bestiaryZoneLocked(npc)
	key := strings.ToLower(kind) + "\x00" + strings.ToLower(zone)
	if p.bestiaryMet[key] {
		return
	}
	if p.bestiaryMet == nil {
		p.bestiaryMet = map[string]bool{}
	}
	p.bestiaryMet[key] = true
	w.queueBestiaryLocked(database.BestiaryNote{ServerID: p.CharServerID, Kind: kind, Zone: zone, At: now})
}

// bestiaryKillLocked notes npc's kill for each character, once each. Must be called
// with the lock held.
func (w *World) bestiaryKillLocked(npc *NPC, chars []uint32, now time.Time) {
	if len(chars) == 0 || !w.bestiaryOn() {
		return
	}
	kind, ok := bestiaryKind(npc)
	if !ok {
		return
	}
	zone := w.bestiaryZoneLocked(npc)
	seen := map[uint32]bool{}
	for _, c := range chars {
		if c == 0 || seen[c] {
			continue
		}
		seen[c] = true
		w.queueBestiaryLocked(database.BestiaryNote{ServerID: c, Kind: kind, Zone: zone, At: now, Kill: true})
	}
}

// runBestiary writes the notes waiting, outside the world lock (every tick).
func (w *World) runBestiary() {
	w.bestiary.run.Lock()
	defer w.bestiary.run.Unlock()
	w.bestiary.mu.Lock()
	notes := w.bestiary.queue
	w.bestiary.queue = nil
	fn := w.bestiary.fn
	w.bestiary.mu.Unlock()
	if len(notes) == 0 || fn == nil {
		return
	}
	fn(notes)
}

// FlushBestiary writes every note still waiting: SaveAll's, before a restart or a
// shutdown.
func (w *World) FlushBestiary() { w.runBestiary() }
