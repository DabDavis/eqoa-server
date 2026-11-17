package handlers

import "sync/atomic"

// keepgm.go — what a restart that keeps the players carries of the GM
// channel's state for a session (keep.go; the GMs' modes on the player are
// the World's: entity/keepgm.go): whether a GM silenced its character. Before
// it a restart let every silenced player speak again.

// keepChat is the chat handler RegisterDefaultHandlers made, whose GM channel
// a resume gives back what was kept.
var keepChat atomic.Pointer[ChatHandler]

// silencedChar reports whether a GM silenced the character (Silence).
func (h *ChatHandler) silencedChar(char uint32) bool {
	h.gm.mu.Lock()
	defer h.gm.mu.Unlock()
	return h.gm.silenced[char]
}

// silenceChar silences the character, as Silence does, telling no one.
func (h *ChatHandler) silenceChar(char uint32) {
	h.gm.mu.Lock()
	defer h.gm.mu.Unlock()
	if h.gm.silenced == nil {
		h.gm.silenced = make(map[uint32]bool)
	}
	h.gm.silenced[char] = true
}
