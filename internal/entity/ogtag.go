package entity

import (
	"strings"
	"sync/atomic"

	"github.com/eqoa/server/internal/session"
)

// ogtag.go — an [OG] account's characters drawn "[OG] Name" (the owner,
// 2026-11-18, for the Vanilla Test server: "characters [OG] Meshuggah for
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

// OGTag goes before an [OG] account's character's name where it is drawn.
const OGTag = "[OG] "

// ShownNameMost is the most bytes of a drawn name: the channel's 24 less a
// NUL. A character's own name is 15 at most (isValidName), so only a GM's
// [GM] [OG] with a full one is cut.
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
var nameTags = []string{GMTag, OGTag, CTFCarrierTag}

// PlainName is a name as typed or sent back, without the tags a drawn name
// carries before it ("[GM] [OG] Name" is "Name"), and trimmed.
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

// IsNameTag reports whether word is one of the tags alone ("[OG]"): a tell's
// line "/t [OG] Name text" splits on it.
func IsNameTag(word string) bool {
	for _, t := range nameTags {
		if strings.EqualFold(word, strings.TrimSpace(t)) {
			return true
		}
	}
	return false
}

// ShownNames is every name a character of an account (gm, og) may be drawn by
// on this shard, its own first: the lists vanilla keys by the drawn name.
func ShownNames(name string, gm, og bool) []string {
	out := []string{name}
	og = og && OGTagsOn()
	if og {
		out = append(out, cutShownName(OGTag+name))
	}
	if gm {
		out = append(out, cutShownName(GMTag+name))
		if og {
			out = append(out, cutShownName(GMTag+OGTag+name))
		}
	}
	return out
}

// SetPlayerOG records whether the session's character's account is an [OG]
// one (read at its entry, handlers/ogaccount.go): its name drawn with OGTag
// where world.og_accounts is on.
func (w *World) SetPlayerOG(sess *session.Session, og bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.OG = og
	}
}
