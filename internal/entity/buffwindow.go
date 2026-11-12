package entity

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// buffwindow.go — the buff window (the GM Jibrish's petition 1761, 2026-11-12: "Give the
// buff window 2 horizontal bars for buffs on the player, and a third for buffs on their
// pet. Allow someone to mouse click off their buffs, and add a way for console to navigate
// to the buff bar and then use arrow keys to click off buffs, or check their name and
// remaining duration").
//
// CLIENT's effects window (VIWndInfluence) is channel 0x43: at most MaxEffects icons and
// names, no time left, and no way to take one off. A client that draws its own window
// (eqoa-rust-client-vanilla's `buffwnd`) answers the offer (protocol.OpcodeBuffWindow,
// handlers/buffwindow.go) and from then on, every buffWindowTicks, is sent the set
// (OpcodeBuffWindowData) when it changed: each of the player's effects that has a name and
// has not run out, in the order they went on (more than channel 0x43's eight: an effect past
// the eighth lands with no slot there, cast.go), then each slot of channel 0x43 no effect
// backs (a GM's #effect), then the pet's (petEffectSetLocked's), each with its time left,
// its name and whether the player may take it off. A removal (OpcodeBuffRemove,
// RemoveBuff) takes off a beneficial effect (beneficialEffect: what a creature's strip
// takes, a buff with its price, the Tester Pro's too) as its time running out would, and
// never a harmful one, the towns' Boon or a slot no effect backs. A PS2 never answers, and
// is sent nothing: its window is channel 0x43's, as it was.

// BuffWindowPlayerMost and BuffWindowPetMost are the most of the player's and of its pet's
// effects a set carries: two rows of eight and one of eight, the client's window.
const (
	BuffWindowPlayerMost = 16
	BuffWindowPetMost    = 8
)

// buffWindowTicks is how often the system looks: half a second, as the pet's effects'.
const buffWindowTicks = 5

// Whose a buff window's line is.
const (
	BuffOnPlayer = 0
	BuffOnPet    = 1
)

// buffNoEnd is a line's time left when it has no end the server keeps (a #effect slot).
const buffNoEnd = math.MaxUint32

// BuffWindowLine is one line of the set.
type BuffWindowLine struct {
	Whose     uint8
	Removable bool
	// ID is the effect's spell id, what a removal names (with Icon); 0 for a slot no
	// effect backs
	ID     uint32
	Icon   uint32
	LeftMS uint32
	Name   string
}

// SetBuffWindow makes sess's player's client be sent its buff window's set (on) or stop
// (off); on, the set goes out at the next pass whatever was sent before. False when sess
// has no player.
func (w *World) SetBuffWindow(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.BuffWindow = on
	p.buffWindowSent, p.buffWindowDue = "", on
	return true
}

// buffWindowSetLocked is p's set at now (the file's doc). Must be called with the lock
// held.
func (w *World) buffWindowSetLocked(p *Player, now time.Time) []BuffWindowLine {
	var out []BuffWindowLine
	left := func(e *ActiveEffect) uint32 {
		return uint32(max(0, min(e.Until.Sub(now).Milliseconds(), buffNoEnd-1)))
	}
	backed := make([]bool, len(p.Buffs))
	mine := 0
	for _, e := range p.Effects {
		if e.Name == "" || !now.Before(e.Until) {
			continue
		}
		for j, b := range p.Buffs {
			if !backed[j] && b.Name == e.Name && b.Icon == e.Icon {
				backed[j] = true
				break
			}
		}
		if mine == BuffWindowPlayerMost {
			continue
		}
		mine++
		out = append(out, BuffWindowLine{Whose: BuffOnPlayer, Removable: beneficialEffect(e), ID: uint32(e.SpellID), Icon: e.Icon, LeftMS: left(e), Name: petEffectName(e.Name)})
	}
	for j, b := range p.Buffs {
		if backed[j] || mine == BuffWindowPlayerMost {
			continue
		}
		mine++
		out = append(out, BuffWindowLine{Whose: BuffOnPlayer, Icon: b.Icon, LeftMS: buffNoEnd, Name: petEffectName(b.Name)})
	}
	for i, e := range w.petEffectSetLocked(p, now) {
		if i == BuffWindowPetMost {
			break
		}
		out = append(out, BuffWindowLine{Whose: BuffOnPet, Removable: beneficialEffect(e), ID: uint32(e.SpellID), Icon: e.Icon, LeftMS: left(e), Name: petEffectName(e.Name)})
	}
	return out
}

// buffWindowKey is what makes a set: each line but its time left (less at every look), and
// each effect's end, so a set goes out only when an effect goes on, runs out, is cast again
// or is taken off, or the pet goes.
func buffWindowKey(lines []BuffWindowLine, now time.Time) string {
	var b strings.Builder
	for _, l := range lines {
		end := int64(-1)
		if l.LeftMS != buffNoEnd {
			// to the second: the same end read at two looks rounds alike
			end = (now.UnixMilli() + int64(l.LeftMS) + 500) / 1000
		}
		fmt.Fprintf(&b, "%d|%v|%d|%d|%d|%s\x00", l.Whose, l.Removable, l.ID, l.Icon, end, l.Name)
	}
	return b.String()
}

// EncodeBuffWindow is OpcodeBuffWindowData's body: a u8 count, then each line's u8 whose
// (BuffOnPlayer, BuffOnPet), u8 flags (bit 0: the player may take it off), u32 id, u32
// icon, u32 milliseconds left (0xFFFFFFFF: no end the server keeps) and its name (a u8
// byte count and UTF-8).
func EncodeBuffWindow(lines []BuffWindowLine) []byte {
	out := []byte{byte(len(lines))}
	for _, l := range lines {
		var flags byte
		if l.Removable {
			flags |= 1
		}
		out = append(out, l.Whose, flags)
		out = binary.LittleEndian.AppendUint32(out, l.ID)
		out = binary.LittleEndian.AppendUint32(out, l.Icon)
		out = binary.LittleEndian.AppendUint32(out, l.LeftMS)
		out = append(out, byte(len(l.Name)))
		out = append(out, l.Name...)
	}
	return out
}

// buffWindowSystem sends each client that asked its set when it changed, and at once after
// the ask. Every buffWindowTicks, after effectSystem and petSystem took off what ran out.
// Must be called with the lock held.
func (w *World) buffWindowSystem(now time.Time) {
	if w.TickCount%buffWindowTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.BuffWindow {
			return true
		}
		w.sendBuffWindowLocked(s, p, now, false)
		return true
	})
}

// sendBuffWindowLocked sends p's set to s when it changed, or whatever it is when force.
func (w *World) sendBuffWindowLocked(s *session.Session, p *Player, now time.Time, force bool) {
	lines := w.buffWindowSetLocked(p, now)
	key := buffWindowKey(lines, now)
	if !force && !p.buffWindowDue && key == p.buffWindowSent {
		return
	}
	p.buffWindowSent, p.buffWindowDue = key, false
	body := EncodeBuffWindow(lines)
	sendGroupMsg(s, protocol.OpcodeBuffWindowData, func(b *buffer.Writer) { b.WriteBytes(body) })
}

// RemoveBuff takes the effect of spell id with icon off sess's player (whose BuffOnPlayer)
// or its pet (BuffOnPet), as the player asked from its buff window, when it is beneficial
// (beneficialEffect): on the player as its time running out ends it (removeEffectLocked:
// its slot off channel 0x43, the stats summed again, its speed, body or invisibility
// gone); on the pet off its effects, its maximum hit points taken back (stripPetLocked's
// way). The player is told in a line either way, and a client that asked is sent its set
// at once. It reports whether one was taken off.
func (w *World) RemoveBuff(sess *session.Session, whose uint8, id, icon uint32) bool {
	return w.RemoveBuffAt(sess, whose, id, icon, time.Now())
}

// RemoveBuffAt is RemoveBuff at now.
func (w *World) RemoveBuffAt(sess *session.Session, whose uint8, id, icon uint32, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	ok, line := w.removeBuffLocked(p, whose, id, icon, now)
	if line != "" {
		w.sendChatToSession(sess, line)
	}
	if p.BuffWindow {
		w.sendBuffWindowLocked(sess, p, now, true)
	}
	return ok
}

// removeBuffLocked is RemoveBuff's rule: whether it was taken off, and the line to tell.
func (w *World) removeBuffLocked(p *Player, whose uint8, id, icon uint32, now time.Time) (bool, string) {
	find := func(fx []*ActiveEffect) *ActiveEffect {
		for _, e := range fx {
			if uint32(e.SpellID) == id && e.Icon == icon && e.Name != "" && now.Before(e.Until) {
				return e
			}
		}
		return nil
	}
	switch whose {
	case BuffOnPlayer:
		e := find(p.Effects)
		if e == nil || id == 0 {
			return false, "" // gone already (ran out as it was asked): the set says so
		}
		if !beneficialEffect(e) {
			return false, fmt.Sprintf("%s cannot be removed.", e.Name)
		}
		if p.IsDead() || !w.removeEffectLocked(p, e.SpellID) {
			return false, ""
		}
		log.Printf("BuffWindow: %s took %q off (spell %d)", p.Name, e.Name, e.SpellID)
		return true, fmt.Sprintf("You remove %s.", e.Name)
	case BuffOnPet:
		pet := w.petOfLocked(p)
		if pet == nil {
			return false, ""
		}
		e := find(pet.Effects)
		if e == nil || id == 0 {
			return false, ""
		}
		if !beneficialEffect(e) {
			return false, fmt.Sprintf("%s cannot be removed from %s.", e.Name, pet.Name)
		}
		keep := pet.Effects[:0]
		for _, x := range pet.Effects {
			if x != e {
				keep = append(keep, x)
			}
		}
		pet.Effects = keep
		pet.syncEffectMaxHP()
		log.Printf("BuffWindow: %s took %q off its pet %s (spell %d)", p.Name, e.Name, pet.Name, e.SpellID)
		return true, fmt.Sprintf("You remove %s from %s.", e.Name, pet.Name)
	}
	return false, ""
}
