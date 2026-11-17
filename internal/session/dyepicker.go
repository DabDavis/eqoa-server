package session

// The dyers' colour picker (protocol.OpcodeDyePicker, handlers/dyepicker.go):
// the version a client answered the offer with. A PS2 never answers, so it is
// never offered "Pick any colour..." and keeps the palette.

// DyePickerVersion is the newest version of the picker's messages the server
// speaks.
const DyePickerVersion = 1

// SetDyePicker records the client's answer to the picker's offer: its
// version (at most DyePickerVersion), 0 none.
func (s *Session) SetDyePicker(version uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dyePicker = min(version, DyePickerVersion)
}

// DyePicker is the picker's version the client said it speaks; 0 (a client
// that never said) none.
func (s *Session) DyePicker() uint8 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dyePicker
}
