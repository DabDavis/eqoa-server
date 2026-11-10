package entity

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// peteffects.go — the effects on a player's pet, for a client that draws them
// under its pet's bar (the user, 2026-11-09: "jibcro is showing pets buffs as if
// on him, put the buff icons under the pet name plate").
//
// CLIENT has no place for them: its pet bar (VIWndGame::DrawPet) shows the pet's
// name, its health and two status icons (channel byte 154's poison and disease),
// and its effects window (VIWndInfluence) is channel 0x43, the player's own. From
// petition 1041 (2026-11-06, Eatma: "Pet buffs for necro aren't showing up on buff
// display") the pet's effects were sent on that channel after the player's, so
// every client, a PS2 too, showed them as the player's own; they are not any more
// (statesync.go's SerializeBuffUpdate). A client that draws them where the pet is
// (eqoa-rust-client-vanilla's Pet Effects) asks for them as for others' power
// (powerbars.go): the server offers OpcodePetEffects at world entry
// (handlers/peteffects.go), the client answers 1, and from then on, every
// petEffectsTicks, it is sent the set (OpcodePetEffectsData) when it changed: each
// of the pet's effects that has a name and has not run out, in the order they went
// on (a recast goes last: putOnPetLocked), its icon, its time left and its name.
// Whatever is on the pet counts: its owner's spells on it (landOnPetLocked), its
// own buffs (petBuffItselfLocked), its drain's share (petDrainStatsLocked), and on
// a charmed creature what was put on it before. No pet, or a dead one, is an empty
// set, sent once as the pet goes. A PS2 never asks, and is sent none.

// PetEffectsMost is the most effects a set carries: more than a pet holds (a
// level 60 necromancer's has four or five), and with names of at most
// petEffectNameBytes the body stays under a message (protocol.DefaultMessageSize).
const PetEffectsMost = 16

// petEffectNameBytes is the most bytes of a name sent, cut on a character's edge,
// after its first MaxEffectName characters (channel 0x43's most).
const petEffectNameBytes = 63

// petEffectsTicks is how often the system looks: half a second, as the power
// bars' (powerSyncSystem).
const petEffectsTicks = 5

// SetPetEffects makes sess's player's client be sent its pet's effects (on) or
// stop (off); on, the set goes out at the next pass whatever was sent before.
// False when sess has no player.
func (w *World) SetPetEffects(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.PetEffects = on
	p.petEffectsSent, p.petEffectsDue = "", on
	return true
}

// petEffectSetLocked is p's pet's effects as the set shows them at now: each
// that has a name and has not run out, in the order they went on, at most
// PetEffectsMost; none with no living pet (petOfLocked). Must be called with the
// lock held.
func (w *World) petEffectSetLocked(p *Player, now time.Time) []*ActiveEffect {
	pet := w.petOfLocked(p)
	if pet == nil {
		return nil
	}
	var out []*ActiveEffect
	for _, e := range pet.Effects {
		if len(out) == PetEffectsMost {
			break
		}
		if e.Name != "" && now.Before(e.Until) {
			out = append(out, e)
		}
	}
	return out
}

// petEffectName is a name as the set carries it: its first MaxEffectName
// characters, at most petEffectNameBytes bytes, cut on a character's edge.
func petEffectName(s string) string {
	end := 0
	for n := 0; end < len(s) && n < MaxEffectName; n++ {
		_, size := utf8.DecodeRuneInString(s[end:])
		if end+size > petEffectNameBytes {
			break
		}
		end += size
	}
	return s[:end]
}

// petEffectsKey is what makes a set: each effect's icon, end and name, not its
// time left (less at every look), so a set goes out only when an effect goes on,
// runs out or is cast again, or the pet goes.
func petEffectsKey(fx []*ActiveEffect) string {
	var b strings.Builder
	for _, e := range fx {
		fmt.Fprintf(&b, "%d|%d|%s\x00", e.Icon, e.Until.UnixMilli(), e.Name)
	}
	return b.String()
}

// encodePetEffects is OpcodePetEffectsData's body for fx at now: a u8 count,
// then each effect's u32 icon, u32 milliseconds left and its name (a u8 byte
// count and UTF-8).
func encodePetEffects(fx []*ActiveEffect, now time.Time) []byte {
	if len(fx) > PetEffectsMost {
		fx = fx[:PetEffectsMost]
	}
	out := []byte{byte(len(fx))}
	for _, e := range fx {
		left := max(0, min(e.Until.Sub(now).Milliseconds(), math.MaxUint32))
		name := petEffectName(e.Name)
		out = binary.LittleEndian.AppendUint32(out, e.Icon)
		out = binary.LittleEndian.AppendUint32(out, uint32(left))
		out = append(out, byte(len(name)))
		out = append(out, name...)
	}
	return out
}

// petEffectsSystem sends each client that asked its pet's effects when they
// changed (petEffectSetLocked), and at once after the ask. Every
// petEffectsTicks, after petSystem took off what ran out. Must be called with the
// lock held.
func (w *World) petEffectsSystem(now time.Time) {
	if w.TickCount%petEffectsTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.PetEffects {
			return true
		}
		fx := w.petEffectSetLocked(p, now)
		key := petEffectsKey(fx)
		if !p.petEffectsDue && key == p.petEffectsSent {
			return true
		}
		p.petEffectsSent, p.petEffectsDue = key, false
		body := encodePetEffects(fx, now)
		sendGroupMsg(s, protocol.OpcodePetEffectsData, func(b *buffer.Writer) { b.WriteBytes(body) })
		return true
	})
}
