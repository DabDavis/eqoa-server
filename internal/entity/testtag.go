package entity

import "github.com/eqoa/server/internal/session"

// testtag.go — a sub GM's character drawn "[Test] Name" (the owner,
// 2026-11-21: "lets give sub gms a tag [Test]"). A sub GM is a tester's account
// whose tester row says subgm (database/subgm.go), read at each world
// entry (handlers' SendGMChannel, a keep restart's too).
//
// The tag is the GM tag's way (GMTag, ogtag.go): before the name where every
// client draws a character's name, the entity channel's 24 bytes, and nowhere
// else; a name typed or sent back with it finds the character all the same
// (PlainName, IsNameTag), and the lists vanilla keys by a drawn name carry its
// tagged forms (ShownNamesOf). The tags' order is "[GM][Test][Banner][OG][HC]
// Name", but a GM's account is drawn with [GM] alone, never [Test] beside it. A
// long name keeps itself whole: the space goes first, then [OG], then [Test],
// then [HC] (tagDropOrder).

// TestTag goes before a sub GM's name where it is drawn.
const TestTag = "[Test] "

// SetPlayerSubGM records whether the session's character's account is a sub
// GM's (read at its world entry): its name drawn with TestTag where it is no
// GM's.
func (w *World) SetPlayerSubGM(sess *session.Session, sub bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.SubGM = sub
	}
}
