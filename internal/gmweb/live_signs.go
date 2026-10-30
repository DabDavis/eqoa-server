package gmweb

import (
	"math"

	"github.com/eqoa/server/internal/database"
)

// checkSignTextRow holds a signtext row to database.SignTextProblem: a
// sign's npcs row or the news row (npcid 0, kind news), a kind the server
// knows, words that fit a few boxes.
func checkSignTextRow(row map[string]any) error {
	id, ok := asFloat(row["npcid"])
	if !ok || id != math.Trunc(id) {
		return badRequest("npcid is the sign's npcs row (0 for the news row)")
	}
	kind, _ := row["kind"].(string)
	if _, set := row["kind"]; !set {
		kind = database.SignKindGM // the column's default
	}
	text, _ := row["text"].(string)
	if p := database.SignTextProblem(int(id), kind, text); p != "" {
		return badRequest("%s", p)
	}
	return nil
}
