package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// spellarea.go — a GM's row of table spellarea over what an area spell's line
// reads (entity/area.go; jib's review, F3 pass 19): read at every cast, as the
// spell's line is, so a GM's change counts at the next cast.

// areaOf is spell with the GM's spellarea row over its area, if it has one.
func (h *SpellbookHandler) areaOf(spell entity.CastSpell) entity.CastSpell {
	row, ok, err := h.store.SpellArea(spell.SpellID)
	if err != nil {
		log.Printf("Cast: %q's area: %v", spell.Name, err)
	}
	return withAreaRow(spell, row, ok)
}

// withAreaRow is spell with a spellarea row over its area; ok false for none.
func withAreaRow(spell entity.CastSpell, row database.SpellAreaRow, ok bool) entity.CastSpell {
	if ok {
		spell.Area = spell.Area.With(entity.AreaRule{Centre: row.Centre, Radius: float32(row.Radius), Max: row.MaxTargets})
	}
	return spell
}
