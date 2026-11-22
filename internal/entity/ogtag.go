package entity

import (
	"strings"
	"sync/atomic"

	"github.com/eqoa/server/internal/session"
)

// ogtag.go — an [OG] character drawn "[OG] Name" (the owner,
// 2026-11-18, for the Vanilla Test server: "characters [OG] a player for
// example. this [OG] tag shows players you opt-out of the QoL changes").
//
// The tag is the GM tag's way (GMTag): it goes before the name where every
// client draws a character's name, the entity channel's 24 bytes at 116 (over
// its head, in a target's window: a PS2's CLIENT and vanilla alike), and
// nowhere else. The name stays the character's own everywhere a name is a key:
// the character list (0x002C), chat lines and tells, /r's name (0x07A3), a
// group's, a raid's and a guild's lists, mail, the auction, trade, quests,
// the GM tools, the database. A name typed or sent back with a tag before it
// (a target's name copied into "/t [OG] Name ...", "/invite [OG] Name") finds
// the character all the same (PlainName). The lists vanilla keys by the name
// it draws (race labels, race looks) carry both forms (ShownNames).

// OGTag goes before an [OG] character's name where it is drawn.
const OGTag = "[OG] "

// ShownNameMost is the most bytes of a drawn name: the channel's 24 less a
// NUL. A character's own name is 15 at most (isValidName); tags that would not
// fit with it are fitted by composeTags (hardcore.go), never the name.
const ShownNameMost = 23

// ogTags is world.og_accounts: the [OG] tag drawn (handlers.SetOGAccounts).
var ogTags atomic.Bool

// SetOGTags turns the [OG] tag on or off (world.og_accounts).
func SetOGTags(on bool) { ogTags.Store(on) }

// OGTagsOn reports world.og_accounts.
func OGTagsOn() bool { return ogTags.Load() }

// cutShownName cuts a drawn name to ShownNameMost bytes (names are ASCII).
func cutShownName(name string) string {
	if len(name) > ShownNameMost {
		return name[:ShownNameMost]
	}
	return name
}

// nameTags are the tags a drawn name may begin with.
var nameTags = []string{GMTag, TestTag, OGTag, HCTag, CTFCarrierTag, AFKTag}

// PlainName is a name as typed or sent back, without the tags a drawn name
// carries before it ("[GM] [OG] Name", "[OG][HC]Name" are "Name"), and trimmed.
func PlainName(name string) string {
	name = strings.TrimSpace(name)
	for again := true; again; {
		again = false
		for _, t := range nameTags {
			tag := strings.TrimSpace(t)
			if len(name) > len(tag) && strings.EqualFold(name[:len(tag)], tag) {
				name = strings.TrimSpace(name[len(tag):])
				again = true
			}
		}
	}
	return name
}

// IsNameTag reports whether word is the tags alone ("[OG]", "[OG][HC]"): a
// tell's line "/t [OG][HC] Name text" splits on it.
func IsNameTag(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	for word != "" {
		found := false
		for _, t := range nameTags {
			tag := strings.TrimSpace(t)
			if len(word) >= len(tag) && strings.EqualFold(word[:len(tag)], tag) {
				word = strings.TrimSpace(word[len(tag):])
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ShownNames is every name a character of an account (gm, og) that is or is
// not Hardcore (hc) may be drawn by on this shard, its own first: the lists
// vanilla keys by the drawn name. A GM may hide its tag, so both forms.
func ShownNames(name string, gm, og, hc bool) []string {
	return ShownNamesOf(name, gm, false, og, hc)
}

// ShownNamesOf is ShownNames for an account that may be a sub GM's (sub: its
// [Test] tag, testtag.go), drawn only where it is no GM's. Any character may be
// away (afk.go), so each form comes with its [AFK] too, where it fits.
func ShownNamesOf(name string, gm, sub, og, hc bool) []string {
	og = og && OGTagsOn()
	hc = hc && HardcoreOn()
	sub = sub && !gm
	var more []string
	for _, g := range []bool{false, gm} {
		for _, o := range []bool{false, og} {
			for _, h := range []bool{false, hc} {
				for _, t := range []bool{false, sub} {
					for _, a := range []bool{false, true} {
						if s := shownWith(name, g, t, false, o, h, a); s != name {
							more = append(more, s)
						}
					}
				}
			}
		}
	}
	return append([]string{name}, uniqueStrings(more)...)
}

// SetPlayerOG records whether the session's character is an [OG]
// one (read at its entry, handlers/ogaccount.go): its name drawn with OGTag
// where world.og_accounts is on.
func (w *World) SetPlayerOG(sess *session.Session, og bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.OG = og
	}
}
