package entity

import (
	"log"
	"strings"
	"time"
)

// petrotation.go — the necromancer's zombie, mummy and spectre fight as
// Sandstorm's do (recorded 2026-10-30 by lane-spellfx24: a necromancer of 51
// set Decaying Flesh, Putrid Flesh and Howling Death on a Training Dummy Lv25
// for 120 s each, `petattack`; its Ancient Death joined its owner's casts
// besides; ~/.cache/f3pass24/lane-spellfx24-notes.md "Pets"):
//
//   - Rounds: the zombie (Decaying Flesh, 24) and the mummy (Putrid Flesh, 34)
//     swing Attack2 (0xDA9BEA12, 4000 ms) every 4.0 s; the spectre (Howling
//     Death, 44) Attack3 (0xDA9BEA13, 5000 ms) every 5.2 s.
//   - An opener on its first swings: the zombie taps at once; the mummy drains
//     stamina (0x723913E6, Tap Fortitude's visual, 1000 ms); the spectre
//     casts 0xAAC52F03 (Cursed Hand's visual, 1000 ms) at its target, the
//     same at itself a round later, and the stamina drain two rounds after.
//   - Then a tap, 0xAF72B895 (Drain Spirit's) at cast 0, now and then
//     0xAF72B894 (Lifespike's) with 2000 ms, every 2-5 swings: the same visual
//     for the zombie of 24 as the spectre of 44.
//   - The spectre's instants, 0xFD069D13 at cast 0, half a second after a
//     swing (not with it), from about 20 s into the fight, 1-6 swings apart.
//   - Each spell's 0x00B1 came in the same millisecond as a swing's and
//     before it; nothing else said what any did, and no 0x00DB came in six
//     minutes of fights: the owner is told no pet hit.
//
// Here the taps do what ours did (pettap.go: the necromancer's own tap of
// the pet's level, its figure off the target and as much back on the pet),
// on Sandstorm's cadence and visual and untold; the openers and the
// instants are their visuals alone (nothing recorded what they did). The
// melee keeps the damage a second our round gave the pet (petHitRate): no
// hit of these pets was ever shown on Sandstorm, so only the cadence is
// recorded, and our warrior's round from level 35 (Attack4, four hits in 6
// s) out-hits Sandstorm's mummy (two in 4 s) by a third and its spectre (two
// in 5.2 s) by three quarters, which the balance's pets were tuned on
// (balancesim: a necromancer of 50 with its spectre won every +2 fight
// before, none at Sandstorm's round with our hits). A pet
// of any other kind (the necromancer's skeletons and Swarming pets, every
// other class's, a charmed creature) is ours: pet.go's rounds, pettap.go's
// 30 s tap, its hits told to its owner.

// petKind is what a summoned pet fights as.
type petKind int

const (
	petKindOurs    petKind = iota // not recorded: pet.go's and pettap.go's
	petKindZombie                 // Decaying and Rotting Flesh (Wilting Flesh by its line)
	petKindMummy                  // Putrid and Preserved Flesh
	petKindSpectre                // Howling and Ancient Death
)

func (k petKind) String() string {
	switch k {
	case petKindZombie:
		return "zombie"
	case petKindMummy:
		return "mummy"
	case petKindSpectre:
		return "spectre"
	}
	return "ours"
}

// petKindsByName are the necromancer's summonings by name, as Sandstorm's
// own records describe them ("Summons a zombie with weapons to aid you.",
// "... a mummy ...", "... a spectre with a weapon ..."); ours carry other
// lines for some (Putrid Flesh's "Summons a Zombie pet"), so the name
// decides. Wilting Flesh, which Sandstorm's characters lack, is a zombie by
// its line ("Summons a zombie to your aid").
var petKindsByName = map[string]petKind{
	"wilting flesh":   petKindZombie,
	"decaying flesh":  petKindZombie,
	"rotting flesh":   petKindZombie,
	"putrid flesh":    petKindMummy,
	"preserved flesh": petKindMummy,
	"howling death":   petKindSpectre,
	"ancient death":   petKindSpectre,
}

// petKindOf is the kind of the pet a necromancer's summoning makes; any
// other class's pet, or a summoning not named above, is ours.
func petKindOf(casterClass int, spellName string) petKind {
	if casterClass != classNecromancer {
		return petKindOurs
	}
	return petKindsByName[strings.ToLower(strings.TrimSpace(spellName))]
}

// The rotation's visuals, as Sandstorm sent them.
const (
	fxPetTap            uint32 = 0xAF72B895 // the necromancer's Drain Spirit's
	fxPetTapLong        uint32 = 0xAF72B894 // Lifespike's and Spirit Tap's, with 2000 ms
	fxPetStaDrain       uint32 = 0x723913E6 // Tap and Siphon Fortitude's
	fxPetSpectre        uint32 = 0xAAC52F03 // the shadow knight's Cursed Hand's
	fxPetSpecial        uint32 = 0xFD069D13 // an instant hit: the target's hit action, a shake
	petTapLongMs        uint32 = 2000
	petOpenerMs         uint32 = 1000
	petTapLongOne              = 6                       // one tap in so many is the long one
	petSpecialFrom             = 20 * time.Second        // the spectre's instants from so far into a fight
	petSpecialLag              = 500 * time.Millisecond  // after a swing,
	petSpecialLagJitter        = 200 * time.Millisecond  // up to this much more
	petSpecialStale            = time.Second             // dropped when this late
	spectreRound               = 5200 * time.Millisecond // its Attack3's round
)

// petKindSwing is the swing a recorded kind fights with whatever its level:
// the zombie's and the mummy's Attack2 (swing kind 4, a 4 s round), the
// spectre's Attack3 (kind 3, 5000 ms) in a 5.2 s round; ok false for ours.
func petKindSwing(k petKind) (kind int, round time.Duration, ok bool) {
	switch k {
	case petKindZombie, petKindMummy:
		return 4, 0, true
	case petKindSpectre:
		return 3, spectreRound, true
	}
	return 0, 0, false
}

// petHitRate is a pet's melee hits a second as its round makes them.
func petHitRate(pet *NPC) float64 {
	length, _, _, hits := npcRound(pet)
	return float64(len(hits)) / length.Seconds()
}

// petNextTapIn is how many swings until the next tap: 2 to 5.
func petNextTapIn() int { return 2 + petTapIntn(4) }

// petRoundSpellLocked casts the spell a recorded kind's round n (1-based, on
// its target) carries, before the round's swing: its opener's, else a tap
// when one is due. It reports a kill (a tap's).
func (w *World) petRoundSpellLocked(ps *petState, t *NPC, n int, now time.Time) bool {
	pet := ps.npc
	switch ps.kind {
	case petKindZombie:
		if n == 1 {
			return w.petRotationTapLocked(ps, t, now)
		}
	case petKindMummy:
		if n == 1 {
			w.broadcastNPCSwingLocked(pet, t.ServerID, petOpenerMs, fxPetStaDrain)
			ps.tapIn = petNextTapIn()
			return false
		}
	case petKindSpectre:
		switch n {
		case 1:
			w.broadcastNPCSwingLocked(pet, t.ServerID, petOpenerMs, fxPetSpectre)
			ps.fightStart = now
			ps.specialIn = 1 + petTapIntn(6)
			return false
		case 2:
			w.broadcastNPCSwingLocked(pet, pet.ServerID, petOpenerMs, fxPetSpectre) // at itself
			return false
		case 4:
			w.broadcastNPCSwingLocked(pet, t.ServerID, petOpenerMs, fxPetStaDrain)
			ps.tapIn = petNextTapIn()
			return false
		case 3:
			return false
		}
	default:
		return false
	}
	if ps.tapIn--; ps.tapIn > 0 {
		return false
	}
	return w.petRotationTapLocked(ps, t, now)
}

// petRotationTapLocked is a recorded kind's tap: its visual (now and then the
// long one), the necromancer's tap of the pet's level done as pettap.go
// does it but told to no one, and the next one 2-5 swings on. It reports a
// kill.
func (w *World) petRotationTapLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	ps.tapIn = petNextTapIn()
	fx, ms := fxPetTap, uint32(0)
	if petTapIntn(petTapLongOne) == 0 {
		fx, ms = fxPetTapLong, petTapLongMs
	}
	w.broadcastNPCSwingLocked(pet, t.ServerID, ms, fx)
	tap := w.petTapForLocked(pet.Level)
	if tap == nil {
		return false
	}
	n := -tap.Hit.AmountMax
	if tap.Hit.AmountMax > tap.Hit.Amount {
		n += petTapIntn(tap.Hit.AmountMax - tap.Hit.Amount + 1)
	}
	log.Printf("Pet: %s's %q (%s) casts %s on %s for %d", ps.owner.Name, pet.Name, ps.kind, tap.Name, t.Name, n)
	killed := w.petDamageLocked(ps, t, n, now)
	if pet.HP < pet.MaxHP {
		pet.HP = min(pet.HP+n, pet.MaxHP)
	}
	return killed
}

// petSpecialAfterSwingLocked lines up a spectre's instant half a second (and
// up to 0.2 s more) after the swing just sent, when its fight is 20 s old and
// 1-6 swings have passed since the last.
func (w *World) petSpecialAfterSwingLocked(ps *petState, now time.Time) {
	if ps.kind != petKindSpectre || ps.fightStart.IsZero() || now.Sub(ps.fightStart) < petSpecialFrom {
		return
	}
	if ps.specialIn--; ps.specialIn > 0 {
		return
	}
	ps.specialIn = 1 + petTapIntn(6)
	ps.specialAt = now.Add(petSpecialLag + time.Duration(petTapIntn(int(petSpecialLagJitter/time.Millisecond)+1))*time.Millisecond)
}

// petSpecialDueLocked sends a spectre's instant once it is due; one more
// than petSpecialStale past it (the pet off chasing, out of reach) is
// dropped, not sent late.
func (w *World) petSpecialDueLocked(ps *petState, t *NPC, now time.Time) {
	if ps.specialAt.IsZero() || now.Before(ps.specialAt) {
		return
	}
	late := now.Sub(ps.specialAt)
	ps.specialAt = time.Time{}
	if late > petSpecialStale {
		return
	}
	w.broadcastNPCSwingLocked(ps.npc, t.ServerID, 0, fxPetSpecial)
}
