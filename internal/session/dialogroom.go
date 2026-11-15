package session

// The dialog room: how many answers a client shows in one 0x0034 box (the
// owner, 2026-11-15: "lets increase the dialog box for heroic dungeons,
// coachmen etc. to stack higher before pagination").
//
// CLIENT's Msg0034 (0x00BDBD58) reads the answers into VIDialogBox's six
// m_options, on its stack, with no check of the count: a seventh runs past
// them. So every box is cut to RetailDialogRoom, and a paged list holds
// RetailDialogRoom-1 and "More...", unless the client said it shows more
// (protocol.OpcodeDialogRoom, handlers/dialogroom.go): then the session's
// room, at most MaxDialogRoom. A PS2 never says, so it keeps six.
const (
	// RetailDialogRoom is CLIENT's VIDialogBox m_options[6].
	RetailDialogRoom = 6
	// MaxDialogRoom is the most answers the server puts in one box: vanilla's
	// response window's page (eqoa-rust-client-vanilla's dialog::PAGE).
	MaxDialogRoom = 12
)

// ClampDialogRoom is n within RetailDialogRoom and limit, limit itself within
// RetailDialogRoom and MaxDialogRoom.
func ClampDialogRoom(n, limit int) int {
	return max(RetailDialogRoom, min(n, limit, MaxDialogRoom))
}

// SetDialogRoom makes n (within RetailDialogRoom and MaxDialogRoom) the
// answers the session's client shows in one box.
func (s *Session) SetDialogRoom(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dialogRoom = uint8(ClampDialogRoom(n, MaxDialogRoom))
}

// DialogRoom is the answers the session's client shows in one box:
// RetailDialogRoom unless it said more (SetDialogRoom).
func (s *Session) DialogRoom() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.dialogRoom == 0 {
		return RetailDialogRoom
	}
	return int(s.dialogRoom)
}
