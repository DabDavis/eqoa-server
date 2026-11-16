package gmweb

import (
	"fmt"
	"math"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// liveDyes puts a character's dyes, as chardyes holds them now, on it where
// it stands (entity/livelook.go; the change is committed: the database is
// free): every client of the live game's look draws them from the next tick.
// A row moved to another character puts both right.
func (s *Server) liveDyes(before, after map[string]any) string {
	if s.d.World == nil || s.d.DB == nil {
		return "at the character's next login"
	}
	ids := map[uint32]bool{}
	for _, row := range []map[string]any{before, after} {
		if row == nil {
			continue
		}
		if id, ok := asFloat(row["serverid"]); ok && id > 0 {
			ids[uint32(id)] = true
		}
	}
	var online []string
	for id := range ids {
		got, err := s.d.DB.CharDyes(id)
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		if name, ok := s.d.World.SetDyes(id, entity.Dyes(got)); ok {
			online = append(online, name)
		}
	}
	if len(online) == 0 {
		return "at the character's next login"
	}
	return fmt.Sprintf("at once: %v drawn with it by every client of the live game's look from the next tick", online)
}

// checkDyeRow holds a chardyes row to a slot the channel has and a colour it
// sends (database.ValidDye): alpha 0 would make the slot vanish.
func checkDyeRow(row map[string]any) error {
	if v, ok := row["slot"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f < 0 || f >= database.DyeSlots {
			return badRequest("slot is 0 chest, 1 bracer, 2 gloves, 3 legs, 4 boots, 5 helm, 6 primary, 7 secondary, 8 shield, 9 robe, 10 cape, 11-16 the trims of chest, bracer, gloves, legs, boots, helm")
		}
	} else {
		return badRequest("slot is the dye's slot: 0 chest ... 5 helm, 6 primary, 7 secondary, 8 shield, 9 robe, 10 cape, 11-16 the trims of chest, bracer, gloves, legs, boots, helm")
	}
	if v, ok := row["color"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || !database.ValidDye(int64(f)) {
			return badRequest("color is 0xRRGGBBAA (0xD10000FF red), its alpha (the low byte) not 0")
		}
	} else {
		return badRequest("color is the dye, 0xRRGGBBAA (0xD10000FF red)")
	}
	return nil
}

// livePetDyes puts a character's pet dyes, as petdyes holds them now, on it
// where it stands (entity/petdyes.go): every client that asks for pet dyes
// draws its pet in them from the next tick. A row moved to another character
// puts both right.
func (s *Server) livePetDyes(before, after map[string]any) string {
	if s.d.World == nil || s.d.DB == nil {
		return "at the character's next login"
	}
	ids := map[uint32]bool{}
	for _, row := range []map[string]any{before, after} {
		if row == nil {
			continue
		}
		if id, ok := asFloat(row["serverid"]); ok && id > 0 {
			ids[uint32(id)] = true
		}
	}
	var online []string
	for id := range ids {
		got, err := s.d.DB.PetDyesOf(id)
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		if name, ok := s.d.World.SetPetDyes(id, got); ok {
			online = append(online, name)
		}
	}
	if len(online) == 0 {
		return "at the character's next login"
	}
	return fmt.Sprintf("at once: %v's pet drawn with it by every client that asks for pet dyes from the next tick", online)
}

// livePetDyeAreas reads the pet bodies' areas again (entity/petdyes.go).
func (s *Server) livePetDyeAreas() string {
	if s.d.World == nil || s.d.DB == nil {
		return "at the next start"
	}
	n, err := s.d.World.LoadPetDyeAreas(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	return fmt.Sprintf("at once: %d pet bodies of two areas; every client that asks for pet dyes is told its view's pets from the next tick", n)
}

// checkPetDyeRow holds a petdyes row's colours to ones the server sends
// (database.ValidDye) or 0.
func checkPetDyeRow(row map[string]any) error {
	for _, col := range []string{"area1", "area2"} {
		v, ok := row[col]
		if !ok {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || (f != 0 && !database.ValidDye(int64(f))) {
			return badRequest("%s is 0xRRGGBBAA (0xD10000FF red), its alpha (the low byte) not 0; or 0, none", col)
		}
	}
	return nil
}

// checkPetDyeAreaRow holds a petdyeareas row's materials to a list the server
// reads (database.ParsePetDyeMaterials).
func checkPetDyeAreaRow(row map[string]any) error {
	if v, ok := row["materials"]; ok {
		str, _ := v.(string)
		if _, err := database.ParsePetDyeMaterials(str); err != nil {
			return badRequest("materials are the body's palette materials of area 2, 0-63 apart by commas: %v", err)
		}
	}
	return nil
}
