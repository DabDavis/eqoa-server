package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// spellfx.go — a spell's visual by hand, for GMs (F3 pass 16, the user:
// "allow GMs to change the animation hash, so we can fix them manually by
// hand", then "add a visual animation playback"). A visual is what 0x00B1
// names: a 0xC200 record of SPELLFX.CSF on the player's own disc, by its
// DictID (238 on the Frontiers disc). A spell's own is spelleffect.fxdict;
// 0, or no row, plays the stand-in of its kind (entity.StandInVisual), and
// most spells have no visual recorded off Sandstorm. #fx plays one in the
// game so it can be seen; #spellfx gives one to a spell, the change logged
// in the GM panel's audit log as a panel edit is (database.SetSpellVisual),
// so the panel lists it with the spell and can revert it. The panel does the
// same (gmweb's play_visual and spell_visual) and lists the disc's visuals.

// spellVisualStore is where #fx and #spellfx read the spells and write a
// visual (database.SpellbookStore).
type spellVisualStore interface {
	SpellNames() ([]database.SpellName, error)
	spellVisualReader
	SetSpellVisual(spellID int, fx uint32, who database.GMEdit, detail string) (uint32, int64, error)
}

// spellVisualReader reads one spell's record and effect row.
type spellVisualReader interface {
	SpellPattern(spellID int) (*database.SpellPatternRecord, error)
	SpellEffect(spellID int) (line string, fx uint32, ok bool, err error)
}

// SetSpellVisuals gives #fx and #spellfx the spells (database.SpellbookStore).
func (h *ChatHandler) SetSpellVisuals(s spellVisualStore) {
	h.visuals = s
}

// SpellVisual is what a spell plays: its own visual, or the stand-in of its
// kind when it has none.
type SpellVisual struct {
	SpellID  int
	Name     string
	Level    int
	CastTime time.Duration
	// Own is spelleffect.fxdict: 0 for none
	Own uint32
	// Plays is what 0x00B1 carries for it
	Plays uint32
	// StandIn is the stand-in's kind ("heal", "damage", "buff") when it
	// plays one, else ""
	StandIn string
}

// Describe is the spell's visual in a line: "Ward Death (333): visual
// 0x12AB34CD", or the stand-in it plays.
func (v SpellVisual) Describe() string {
	if v.StandIn != "" {
		return fmt.Sprintf("%s (%d): no visual of its own; it plays the %s stand-in %s", v.Name, v.SpellID, v.StandIn, Hex32(v.Plays))
	}
	return fmt.Sprintf("%s (%d): visual %s", v.Name, v.SpellID, Hex32(v.Own))
}

// SpellVisualOf is a spell's visual as a cast reads it (cast.go's castSpell:
// its effect line, else its name and description, says which stand-in).
func SpellVisualOf(r spellVisualReader, spellID int) (SpellVisual, error) {
	sp, err := r.SpellPattern(spellID)
	if err != nil {
		return SpellVisual{}, fmt.Errorf("no spell %d", spellID)
	}
	line, fx, _, err := r.SpellEffect(spellID)
	if err != nil {
		return SpellVisual{}, err
	}
	v := SpellVisual{SpellID: sp.SpellID, Name: sp.SpellName, Level: sp.AbilityLvl, CastTime: time.Duration(sp.CastTime) * time.Second, Own: fx, Plays: fx}
	if fx == 0 {
		v.Plays, v.StandIn = entity.StandInVisual(entity.ParseSpellNamed(sp.SpellName, line, sp.SpellDesc))
	}
	return v, nil
}

// Hex32 is a DictID as the panel and these commands write one: 0x1DAEB5B6.
func Hex32(v uint32) string {
	return fmt.Sprintf("0x%08X", v)
}

// ParseVisual reads a visual's DictID: 0x and 1 to 8 hex digits, or exactly
// eight hex digits ("1DAEB5B6"), as a DictID is written; a word that merely
// could be hex ("Fade") is not one.
func ParseVisual(s string) (uint32, bool) {
	t := strings.TrimSpace(s)
	switch {
	case len(t) > 2 && (t[:2] == "0x" || t[:2] == "0X"):
		t = t[2:]
		if len(t) > 8 {
			return 0, false
		}
	case len(t) != 8:
		return 0, false
	}
	v, err := strconv.ParseUint(t, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// FindSpells is the spells q names: the spell of that id, else those of that
// name (without case), else those whose name q begins, then those with a
// later word q begins (searchNames), each list in id order.
func FindSpells(all []database.SpellName, q string) []database.SpellName {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	if id, err := strconv.Atoi(q); err == nil {
		for _, s := range all {
			if s.SpellID == id {
				return []database.SpellName{s}
			}
		}
		return nil
	}
	var exact []database.SpellName
	for _, s := range all {
		if strings.EqualFold(s.Name, q) {
			exact = append(exact, s)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	names := make([]string, 0, len(all))
	for _, s := range all {
		names = append(names, s.Name)
	}
	var out []database.SpellName
	seen := map[string]bool{}
	for _, n := range searchNames(names, q) {
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, s := range all {
			if s.Name == n {
				out = append(out, s)
			}
		}
	}
	return out
}

// pickSpell is the one spell q names, or nil having said why: none, or
// several, listed with their ids (a name several ranks share is picked by id).
func (h *ChatHandler) pickSpell(sess *session.Session, q string) *database.SpellName {
	all, err := h.visuals.SpellNames()
	if err != nil {
		h.gmReply(sess, "The spells could not be read: "+err.Error())
		return nil
	}
	found := FindSpells(all, q)
	switch len(found) {
	case 0:
		h.gmReply(sess, fmt.Sprintf("No spell %q.", q))
		return nil
	case 1:
		return &found[0]
	}
	h.gmReply(sess, fmt.Sprintf("%d spells match %q; name one by its id:", len(found), q))
	for i, s := range found {
		if i == 10 {
			h.gmReply(sess, fmt.Sprintf("  ...and %d more: type more of the name", len(found)-10))
			break
		}
		h.gmReply(sess, fmt.Sprintf("  %d %s (level %d)", s.SpellID, s.Name, s.Level))
	}
	return nil
}

// defaultVisualCast is the cast a visual named by its DictID plays with: a
// visual's casting part waits for the cast time before it goes on.
const defaultVisualCast = 2 * time.Second

// fxCommand is #fx: a visual played now from the GM onto the GM's target (the
// GM with none), seen by everyone near, nothing saved. `#fx <visual>
// [seconds]` plays a DictID with that cast time (2 s); `#fx <spell>` the
// visual a spell plays, its stand-in when it has none, with its cast time.
func (h *ChatHandler) fxCommand(sess *session.Session, args []string) {
	const usage = "Usage: #fx <visual> [seconds] | #fx <spell id or name> - a visual (a SPELLFX DictID, 0x1DAEB5B6) played from you onto your target, nothing saved"
	if len(args) == 0 {
		h.gmReply(sess, usage)
		return
	}
	if fx, ok := ParseVisual(args[0]); ok {
		cast := defaultVisualCast
		if len(args) > 1 {
			sec, err := strconv.ParseFloat(args[1], 64)
			if err != nil || len(args) > 2 || sec < 0 || sec > entity.MaxVisualCast.Seconds() {
				h.gmReply(sess, usage)
				return
			}
			cast = time.Duration(sec * float64(time.Second))
		}
		on, ok := h.gameWorld.PlayVisual(sess, fx, cast)
		if !ok {
			return
		}
		h.gmReply(sess, fmt.Sprintf("Visual %s on %s, a %.1f s cast. Nothing shows if your disc has no visual of that id.", Hex32(fx), on, cast.Seconds()))
		log.Printf("GM: %s played visual %s on %s", gmName(h.gameWorld, sess), Hex32(fx), on)
		return
	}
	if h.visuals == nil {
		h.gmReply(sess, "This server keeps no spells: #fx takes a visual's DictID only.")
		return
	}
	sp := h.pickSpell(sess, strings.Join(args, " "))
	if sp == nil {
		return
	}
	v, err := SpellVisualOf(h.visuals, sp.SpellID)
	if err != nil {
		h.gmReply(sess, err.Error())
		return
	}
	on, ok := h.gameWorld.PlayVisual(sess, v.Plays, v.CastTime)
	if !ok {
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s, on %s, a %.1f s cast.", v.Describe(), on, v.CastTime.Seconds()))
}

// spellFXCommand is #spellfx: a spell's visual said, or set. `#spellfx
// <spell>` says what it plays; `#spellfx <spell> <visual>` gives it that one
// from its next cast and `#spellfx <spell> 0` puts it back on its stand-in,
// either logged in the GM panel's audit log as the GM's (via "game"), where
// it can be reverted.
func (h *ChatHandler) spellFXCommand(sess *session.Session, args []string) {
	const usage = "Usage: #spellfx <spell id or name> [<visual> | 0] - say a spell's visual, or give it one (0: its stand-in again)"
	if len(args) == 0 {
		h.gmReply(sess, usage)
		return
	}
	if h.visuals == nil {
		h.gmReply(sess, "This server keeps no spells.")
		return
	}
	last := args[len(args)-1]
	fx, set := ParseVisual(last)
	if !set && last == "0" {
		fx, set = 0, true
	}
	if set && len(args) == 1 {
		h.gmReply(sess, usage)
		return
	}
	spellArgs := args
	if set {
		spellArgs = args[:len(args)-1]
	}
	sp := h.pickSpell(sess, strings.Join(spellArgs, " "))
	if sp == nil {
		return
	}
	v, err := SpellVisualOf(h.visuals, sp.SpellID)
	if err != nil {
		h.gmReply(sess, err.Error())
		return
	}
	if !set {
		h.gmReply(sess, v.Describe()+fmt.Sprintf(". #spellfx %d <visual> gives it one; #fx <visual> plays one first.", v.SpellID))
		return
	}
	was := Hex32(v.Own)
	if v.StandIn != "" {
		was = "the " + v.StandIn + " stand-in"
	}
	detail := fmt.Sprintf("#spellfx in the game: %s (%d) visual %s -> %s", v.Name, v.SpellID, Hex32(v.Own), Hex32(fx))
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if a := sess.RemoteAddr(); a != nil {
		who.IP = a.IP.String()
	}
	_, audit, err := h.visuals.SetSpellVisual(v.SpellID, fx, who, detail)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("%s (%d): not changed: %v", v.Name, v.SpellID, err))
		return
	}
	if audit == 0 {
		h.gmReply(sess, fmt.Sprintf("%s (%d) has that visual already.", v.Name, v.SpellID))
		return
	}
	now, _ := SpellVisualOf(h.visuals, v.SpellID)
	if fx == 0 {
		h.gmReply(sess, fmt.Sprintf("%s (%d): back on the %s stand-in %s from its next cast (it was %s). In the GM panel's log as entry %d.", v.Name, v.SpellID, now.StandIn, Hex32(now.Plays), was, audit))
	} else {
		h.gmReply(sess, fmt.Sprintf("%s (%d): visual %s from its next cast (it was %s). In the GM panel's log as entry %d.", v.Name, v.SpellID, Hex32(fx), was, audit))
	}
	log.Printf("GM: %s: %s (audit %d)", gmName(h.gameWorld, sess), detail, audit)
}

// gmName is the session's player's name, for the log.
func gmName(gw *entity.World, sess *session.Session) string {
	if p := gw.SessionPlayer(sess); p != nil {
		return p.Name
	}
	return fmt.Sprintf("account %d", sess.AccountID())
}
