package entity

// merc1 is the owner's first merc out (dead or not), nil for none: the tests
// of one merc a player.
func (w *World) merc1(owner EntityID) *mercState {
	if list := w.mercs[owner]; len(list) > 0 {
		return list[0]
	}
	return nil
}
