package handlers

import "strings"

// consoletext.go: what one player's text may be when it goes to another. A
// PS2's edit box types letters, digits, a space and ! " % ' ( ) * + , - . / : ;
// = ? @ _ ~ (VIWndEdit::OnKeyDown and its sKeys), and CLIENT draws what comes to
// it on those terms. Its text box takes `|x` for a button's glyph, with no width,
// so a line of them never wraps, and VIUIWindow::TextBox writes its terminator
// past its 256 characters and over its own stack: a gram of 360 characters of
// `|+` killed the game as the gram was opened (the tracer, 2026-11-07, through
// VIWndReadGram's CountLines; 340 survived, and 511 periods or letters do). No
// PS2's keyboard can type that, and vanilla's paste keeps to the same list, but a
// client of anyone's making can send anything, so the relay holds the line: every
// path that hands a player's text to another comes through here, once, as the
// text is relayed. What a keyboard can type passes byte for byte.

// The most of each a PS2 can type, less the terminator, off CLIENT: the chat
// line's editor (VIWndGame, SetMaxLength(128)), and the gram's subject and text
// as the read window keeps them (m_subject[64], m_text[512]; the compose window's
// subject is shorter, a reply's "Re:" comes on top of it).
const (
	chatLineMost    = 127
	gramSubjectMost = 63
	gramTextMost    = 511
)

// consoleTypable is whether a PS2's own keyboard can type c.
func consoleTypable(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return c < 0x80 && strings.ContainsRune(" !\"%'()*+,-./:;=?@_~", c)
}

// consoleLine is s as one line of at most most characters: a line end or a tab a
// space (a Windows line end one), whatever else a PS2's keyboard cannot type left
// out. It may come back empty: the caller relays nothing then.
func consoleLine(s string, most int) string {
	return consoleText(s, most, false)
}

// consoleBlock is consoleLine keeping its line ends, for what a PS2 types over
// several lines (a gram's text, a guild's charter).
func consoleBlock(s string, most int) string {
	return consoleText(s, most, true)
}

func consoleText(s string, most int, lines bool) string {
	clean := len(s) <= most
	if clean {
		for _, c := range s {
			if !consoleTypable(c) && !(lines && c == '\n') {
				clean = false
				break
			}
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	n := 0
	cr := false
	for _, c := range s {
		was := cr
		cr = c == '\r'
		switch {
		case n >= most:
			return b.String()
		case c == '\n' && was:
			continue
		case c == '\n' || c == '\r':
			if lines {
				c = '\n'
			} else {
				c = ' '
			}
		case c == '\t':
			c = ' '
		case !consoleTypable(c):
			continue
		}
		b.WriteRune(c)
		n++
	}
	return b.String()
}
