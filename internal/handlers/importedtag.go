package handlers

import (
	"sync/atomic"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// importedtag.go — "(Imported)" after an imported copy's name as a client is
// shown it (database/imported.go; the user, 2026-11-09: "add Imported Tag to
// gear that was imported", then "the imported add a note is only temporary
// for testing purposes"), behind world.imported_tag, so it comes off with
// the switch and no record changes.

// importedTagOn is world.imported_tag.
var importedTagOn atomic.Bool

// SetImportedTag turns the tag on or off.
func SetImportedTag(on bool) { importedTagOn.Store(on) }

// itemNameMost is the most characters a record's name may be sent with:
// CLIENT keeps it in VIItem::m_name, 64 UTF-16 units with its end.
const itemNameMost = 63

// itemNameShown is a record's name as a client is shown it: an imported
// copy's with the tag when it is on and fits.
func itemNameShown(p *database.ItemPatternRecord) string {
	if !importedTagOn.Load() || !entity.IsImported(p.PatternID) {
		return p.ItemName
	}
	if utf8.RuneCountInString(p.ItemName)+utf8.RuneCountInString(database.ImportedTag) > itemNameMost {
		return p.ItemName
	}
	return p.ItemName + database.ImportedTag
}
