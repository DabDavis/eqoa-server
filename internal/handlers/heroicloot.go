package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
)

func (h *NPCWindowHandler) rollHeroicMarks(boss string) []database.CharInventoryEntry {
	set := database.HeroicMarksFor(boss)
	if set == nil {
		return nil
	}
	marks := make([]database.CharInventoryEntry, 0, len(set.Roles))
	for _, role := range set.Roles {
		mark, ok, err := h.inv.ItemPatternByID(role.MarkID)
		if err != nil || !ok {
			log.Printf("Heroic loot: %s's mark %d unavailable: %v", set.Name, role.MarkID, err)
			return nil
		}
		marks = append(marks, database.CharInventoryEntry{Pattern: mark, StackLeft: 1, EquipLoc: -1})
	}
	count := 2 + h.raidIntn(3)
	drops := make([]database.CharInventoryEntry, 0, count)
	for i := 0; i < count; i++ {
		drops = append(drops, marks[h.raidIntn(len(marks))])
	}
	return drops
}
