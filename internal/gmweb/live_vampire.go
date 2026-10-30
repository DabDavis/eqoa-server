package gmweb

import "fmt"

// liveVampire makes a character a vampire, or not, as charvampire holds it
// now, where it stands (entity/vampire.go; the change is committed: the
// database is free): the sun's next tick reads it. A row moved to another
// character puts both right.
func (s *Server) liveVampire(before, after map[string]any) string {
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
		on, err := s.d.DB.IsVampire(id)
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		if name, ok := s.d.World.SetVampire(id, on); ok {
			online = append(online, name)
		}
	}
	if len(online) == 0 {
		return "at the character's next login"
	}
	return fmt.Sprintf("at once: %v, from the sun's next tick", online)
}
