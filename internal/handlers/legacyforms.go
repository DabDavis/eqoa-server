package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// legacyforms.go — an ability's older form, owned through a class mastery
// (Heirloom petition 3184: "the ability to return abilities back to older
// versions with CMs, like when the paladin 51 was a weapon proc and Fervor was
// an HP proc damage shield"; the owner approved the mechanism; table
// legacyforms, database/legacyforms.go).
//
// The rank is an ordinary cmcatalog row, listed under its class in the Class
// Mastery window and bought, removed and reset as any. Owning it, a cast of the
// ability (0x0032 on its bar slot) casts the legacy spell instead: its
// spellpattern row (power, cast time, recast, scope, range, icon, name) and its
// spelleffect row (line, workbook effects, visual), from the ability's own place
// in the book, so its lockout (0x00CD) is told on the ability the player
// pressed. The book itself is not changed: removing the rank, a #cmreset or a
// Relive (whose masteries sleep) gives the present form back at the next cast,
// and nothing is lost or moved on the bars. An effect already on keeps its
// form until it ends.
//
// The quality-of-life lock holds (vanillaui.go, ogaccount.go): on a
// world.vanilla_ui shard and for an [OG] character no older form is cast, and
// such a rank is not sold (classmastery.go's buy).

// legacyStore is where the older forms and their spells are read
// (database.SpellbookStore).
type legacyStore interface {
	LegacyFormsOf(spellID int) ([]database.LegacyForm, error)
	SpellPattern(spellID int) (*database.SpellPatternRecord, error)
}

// legacyFormOf is the book entry e as the session's player casts it: the older
// form's spell in its place when the player owns a mastery that gives one (the
// file's doc), else e itself.
func (h *SpellbookHandler) legacyFormOf(sess *session.Session, p *entity.Player, e *database.CharSpellEntry) *database.CharSpellEntry {
	if h.store == nil || h.gameWorld == nil {
		return e
	}
	return legacyCast(h.store, h.gameWorld.OwnsCMAwake, sess, p, e)
}

// legacyCast is legacyFormOf with its store and the World's ownership test
// given (tests).
func legacyCast(store legacyStore, owns func(*session.Session, []int) (int, bool), sess *session.Session, p *entity.Player, e *database.CharSpellEntry) *database.CharSpellEntry {
	if !QoLOnFor(sess) {
		return e
	}
	forms, err := store.LegacyFormsOf(e.Pattern.SpellID)
	if err != nil {
		log.Printf("Cast: %q's older forms: %v", e.Pattern.SpellName, err)
		return e
	}
	if len(forms) == 0 {
		return e
	}
	ids := make([]int, len(forms))
	for i, f := range forms {
		ids[i] = f.MasteryID
	}
	id, ok := owns(sess, ids)
	if !ok {
		return e
	}
	for _, f := range forms {
		if f.MasteryID != id {
			continue
		}
		sp, err := store.SpellPattern(f.LegacySpellID)
		if err != nil || sp == nil {
			log.Printf("Cast: %q's older form %d (mastery %d): %v", e.Pattern.SpellName, f.LegacySpellID, id, err)
			return e
		}
		out := *e
		out.Pattern = *sp
		name := "?"
		if p != nil {
			name = p.Name
		}
		log.Printf("Cast: %s casts %q as its older form %q (%d, mastery %d)", name, e.Pattern.SpellName, sp.SpellName, sp.SpellID, id)
		return &out
	}
	return e
}
