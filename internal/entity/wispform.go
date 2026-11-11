package entity

import (
	"encoding/binary"
	"slices"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/database"
)

// wispform.go — a Wisp in its own form shows nothing it wears but its cape (the
// owner, 2026-11-11, a proposal forwarded as theirs: "If all gear could be
// invisible besides cape. Wisp with a cape seems funny af, one of the racial cms
// I mentioned would give a wisp form, it would be cool if one of the forms just
// let all the gear be shown like this").
//
// A Wisp (race 31, raceoptions.go: a human's body with the will o' wisp laid on
// it always, the race option row "wisp") is in its own form while it shows its
// own body: no illusion (a glamour of Trickster Flame, its racial: "allowing the
// wisp to take mortal form"), no GM's Change Sprite, no race armour body. Then
// every client is sent it with nothing worn: the entity channel's held items,
// armour sets, their tints and robe none (SerializePlayerUpdate), and the
// character list's gear as bare (handlers' characterGear), so vanilla, which
// draws a Wisp's body as its worn slots alone (its wisp.rs), shows the wisp in
// the helm's place and nothing else. The cape is its own message (capes.go,
// 0x7E25-0x7E27) and still drawn. The held items' attack types stay, so the
// swings are a weapon's still, with nothing in the hand. The ctf banner (an
// override item, ctf.go) stays: the flag is the game's. Nothing of the
// character changes: its inventory, its stats and its weapon's rolls are as
// worn; only what the channel shows.
//
// Under its Wisp's Raiment (inRaiment, a self buff of Trickster Flame's) a Wisp
// is in its own form, the wisp in its helm, and shows all it wears.
//
// In a mortal form all of it shows as on anyone. An illusion of a human is a
// human's body, the Wisp's own model, which a client cannot tell from the
// Wisp's own form: so a Wisp in another form is sent without its wisp (the race
// looks, OpcodeRaceLooks, leave the wisp row out for it: wispLooks below), and
// vanilla draws it as the body it shows, whole. A PS2 never draws a wisp: it
// shows a Wisp in its own form as a bare human with empty hands (a human in its
// armour before 2026-11-11), and in a mortal form as the illusion's body in its
// gear, as everyone.

// wispRowKey is the race option row that makes a race's characters Wisps
// (raceOptionRows: the will o' wisp, laid always).
const wispRowKey = "wisp"

// IsWispRace is whether race's characters are Wisps: its options have the wisp
// row, laid always.
func IsWispRace(race int) bool {
	for _, r := range RaceOptions(race) {
		if r.Key == wispRowKey && r.Always {
			return true
		}
	}
	return false
}

// inOtherBody is whether the player shows a body that is not its own: an
// illusion, a GM's Change Sprite, its race's armour body (ownBody's order).
func (p *Player) inOtherBody() bool {
	if p.GMForm != 0 || p.ArmorBody != 0 {
		return true
	}
	for _, e := range p.Effects {
		if p.shownForm(e) {
			return true
		}
	}
	return false
}

// WispForm is whether the player is a Wisp in its own form shown with nothing
// worn but its cape: not under its Raiment (inRaiment).
func (p *Player) WispForm() bool {
	return IsWispRace(p.Race) && !p.inOtherBody() && !p.inRaiment()
}

// inRaiment is whether the player wears a Wisp's Raiment (EffectRaiment: the
// Trickster Flame's form of the wisp in all its gear, the owner, 2026-11-11:
// "there isn't a form that shows the wisp with all the gear as an orb"): a
// Wisp in its own form, its wisp in the race looks still, shown in all it
// wears, as before the kit hid it; vanilla draws it so, the human's body
// invisible but for its armour, its weapons in its hands and the wisp in its
// helm (its wisp.rs). It is a form: it ends as an illusion does (Remove
// Illusion, a glamour cast over it: endIllusionsLocked), at its time, or
// clicked off.
func (p *Player) inRaiment() bool {
	for _, e := range p.Effects {
		if e.Rules.Raiment {
			return true
		}
	}
	return false
}

// wispMortal is whether the player is a Wisp in another form (a mortal one):
// drawn whole, its wisp left out of the looks.
func (p *Player) wispMortal() bool {
	return IsWispRace(p.Race) && p.inOtherBody()
}

// hideWornGear makes an entity channel buffer (the disc's 162 bytes) show
// nothing worn: no held item (their attack types kept), no armour set, no tint,
// no robe. The override items (79-90: a ctf banner) stay.
func hideWornGear(buf []byte) {
	for i := 67; i < 79; i++ { // the primary, the secondary, the shield
		buf[i] = 0
	}
	for i := 93; i < 105; i++ { // the six armour sets and their tints
		buf[i] = 0
	}
	buf[109], buf[110] = 0xFF, 0 // no robe, its tint none
}

// HideCharacterGear makes a 0x002C record's 32 gear bytes (handlers'
// characterGear) show nothing worn, as the character list sends a bare slot:
// no robe (0xFFFFFFFF), no held item (their attack types kept), every armour
// slot 0xFF, every tint 0.
func HideCharacterGear(g []byte) {
	if len(g) < 32 {
		return
	}
	binary.LittleEndian.PutUint32(g[0:4], 0xFFFFFFFF)
	g[4] = 0
	for i := 5; i < 17; i++ {
		g[i] = 0
	}
	for i := 20; i < 26; i++ {
		g[i] = 0xFF
	}
	for i := 26; i < 32; i++ {
		g[i] = 0
	}
}

// wispLooks is what the race looks need of the Wisps in the world: the names of
// those in a mortal form (their wisp left out), and the characters the last
// looks were made of, to make them again when one changes form without asking
// the database (the tick holds the world's lock).
type wispLooks struct {
	mu     sync.Mutex
	mortal map[string]bool
	key    string                   // the names, sorted and joined: a change is a new key
	chars  []database.LookCharacter // RefreshRaceLooks' last
	have   bool                     // chars was read once
	send   sync.Mutex               // one making and sending at a time, in order
}

// mortalNames is the names of the Wisps in a mortal form (a map never changed
// once made: a change makes a new one).
func (l *wispLooks) mortalNames() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.mortal
}

// noteWispFormsLocked is the tick's look at the Wisps in the world (names, those
// in a mortal form): when they are not those of the last tick, the race looks
// are made again from the characters last read and sent (on a goroutine of its
// own: the world's lock is held). Must be called with the world's lock held.
func (w *World) noteWispFormsLocked(names []string) {
	slices.Sort(names)
	key := strings.Join(names, "\x00")
	l := &w.wispLooks
	l.mu.Lock()
	if key == l.key {
		l.mu.Unlock()
		return
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	l.key, l.mortal = key, m
	have := l.have
	l.mu.Unlock()
	if have {
		go w.resendRaceLooks()
	}
}

// resendRaceLooks makes the race looks again from the characters last read
// (RefreshRaceLooks) and sends them to every client that takes them and has
// not had them so.
func (w *World) resendRaceLooks() {
	l := &w.wispLooks
	l.send.Lock()
	defer l.send.Unlock()
	l.mu.Lock()
	chars := l.chars
	l.mu.Unlock()
	w.sendRaceLooks(chars)
}
