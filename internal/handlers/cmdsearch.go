package handlers

import (
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// cmdsearch.go — "/tan ?" lists the commands that begin with "/tan" (the GM Jibrish's petition 1865, 2026-11-12: "add a
// fuzzy search marker to commands so if we go /tan ? - it will pull up all the commands we can do that start with
// /tan"). A word that begins with / or #, a space and a question mark: every line of /help (playerHelpLines) and, for a
// GM, of #help (gmHelpLines) naming a command that begins with the word, either sign alike ("/tan ?" finds
// #tankprofile1). A PS2 keeps an unknown /command to itself, so the search comes from a client that forwards it.

// cmdSearchAsk is the search's form: the word (with its sign) and a lone "?".
var cmdSearchAsk = regexp.MustCompile(`^\s*([/#][^\s?]+)\s*\?\s*$`)

// cmdNames is every command a help line names: a / or # and its word.
var cmdNames = regexp.MustCompile(`[/#]([A-Za-z][A-Za-z0-9_-]*)`)

// commandSearch answers a search and reports whether message was one.
func (h *ChatHandler) commandSearch(sess *session.Session, message string) bool {
	m := cmdSearchAsk.FindStringSubmatch(message)
	if m == nil {
		return false
	}
	word := m[1]
	SendClientMessage(sess, "Commands that begin with "+word+":")
	for _, l := range commandsBeginning(word, h.isGM(sess)) {
		SendClientMessage(sess, l)
	}
	return true
}

// commandsBeginning is the help lines (the GM's too when gm) naming a command that begins with word (its / or #
// either way, any case), or one line saying there is none.
func commandsBeginning(word string, gm bool) []string {
	prefix := strings.ToLower(strings.TrimLeft(word, "/#"))
	lines := playerHelpLines
	if gm {
		lines = append(append([]string(nil), playerHelpLines...), gmHelpLines...)
	}
	var out []string
	for _, l := range lines {
		for _, c := range cmdNames.FindAllStringSubmatch(l, -1) {
			if strings.HasPrefix(strings.ToLower(c[1]), prefix) {
				out = append(out, l)
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{"  none (/help lists every command)"}
	}
	return out
}
