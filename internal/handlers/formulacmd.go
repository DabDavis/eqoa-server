package handlers

// formulacmd.go — /formula (and #formula), a shard's damage formula mode (the
// owner, 2026-11-14: "give GMs /formula jibs heirloom/live, /formula default
// heirloom/live so we can switch between the two"; internal/formulamode):
//
//   - /formula: each shard's mode (any GM);
//   - /formula jibs <shard>: that shard applies its operator's formula file
//     (mode live); refused when the shard has no formula loaded;
//   - /formula default <shard>: the built-in damage (mode off);
//   - /formula shadow <shard>: the built-in damage, the formula's figure
//     recorded beside it in the shard's private shadow log.
//
// A switch is an admin's (gm.admin_level, as #servdown), at once, kept over a
// restart, audited (gm_audit on the shard switched and on this one) and told
// on the GM channel of both. <shard> is a name of combat.formula_shard or
// combat.formula_peers; left out, this shard.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/formulamode"
	"github.com/eqoa/server/internal/session"
)

var formulaCtl struct {
	mu    sync.RWMutex
	c     *formulamode.Control
	level int
}

// SetFormulaControl gives /formula its switch and the acctlevel a switch
// needs (gm.admin_level).
func SetFormulaControl(c *formulamode.Control, adminLevel int) {
	formulaCtl.mu.Lock()
	formulaCtl.c, formulaCtl.level = c, adminLevel
	formulaCtl.mu.Unlock()
}

// formulaWords are /formula's mode words.
var formulaWords = map[string]entity.FormulaMode{
	"jibs": entity.FormulaLive, "live": entity.FormulaLive,
	"default": entity.FormulaOff, "off": entity.FormulaOff, "builtin": entity.FormulaOff,
	"shadow": entity.FormulaShadow,
}

const formulaUsage = "Usage: /formula (each shard's mode) | /formula jibs|default|shadow <shard>"

// formulaCommand is /formula (a GM's: handleGMCommand gates it; a switch an admin's).
func (h *ChatHandler) formulaCommand(sess *session.Session, args []string) {
	formulaCtl.mu.RLock()
	c, level := formulaCtl.c, formulaCtl.level
	formulaCtl.mu.RUnlock()
	if c == nil {
		h.gmReply(sess, "Formula: this server has no damage formula switch")
		return
	}
	if len(args) == 0 {
		for _, st := range c.Status() {
			h.gmReply(sess, formulaStatusLine(st))
		}
		h.gmReply(sess, formulaUsage)
		return
	}
	m, ok := formulaWords[strings.ToLower(args[0])]
	if !ok || len(args) > 2 {
		h.gmReply(sess, formulaUsage)
		return
	}
	if have, err := h.access.AccessLevel(sess.AccountID()); err != nil || have < level {
		h.gmReply(sess, fmt.Sprintf("Formula: a switch is an admin's (acctlevel %d)", level))
		return
	}
	shard := ""
	if len(args) == 2 {
		shard = args[1]
	}
	line, err := c.Set(shard, m, gmEditOf(sess))
	if err != nil {
		h.gmReply(sess, "Formula: not switched: "+err.Error())
		return
	}
	h.gmReply(sess, line)
}

// formulaStatusLine is a shard's line of /formula.
func formulaStatusLine(st formulamode.Status) string {
	name := st.Shard
	if name == "" {
		name = "this shard"
	}
	if st.Here {
		name += " (here)"
	}
	line := fmt.Sprintf("%s: %s; %d formula(s) loaded", name, formulamode.ModeWord(st.Mode), st.Loaded)
	if st.ShadowLog {
		line += ", shadow log set"
	}
	if st.By != "" {
		line += fmt.Sprintf("; set by %s at %s", st.By, st.At)
	}
	if st.Err != "" {
		line += " (" + st.Err + ")"
	}
	return line
}
