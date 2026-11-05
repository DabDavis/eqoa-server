package session

import (
	"sync"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// What the session's client was shown lately, for a petition's server view
// (handlers/petitionview.go): every text line, dialog box, question, damage
// figure and death queued to it (MessageQueue.AddWithSequence), kept as
// queued, the newest TranscriptMax within TranscriptAge of the asking. Only
// the opcodes inTranscript names are kept; the rest of the traffic (state
// channels, movement, item records, everything else) never is. The lines are
// what the player's own screen showed, chat said to them among them, and are
// read only into a petition its player files.
const (
	TranscriptMax = 120
	TranscriptAge = 10 * time.Minute
)

// inTranscript is whether a transcript keeps a message of op.
func inTranscript(op protocol.GameOpcode) bool {
	switch op {
	case protocol.OpcodeClientMessage, // 0x0A7A, a white line
		protocol.OpcodeColoredChat,      // 0x0A7B, a coloured line (chat among them)
		protocol.OpcodeErrorMessage,     // 0x00CE, a red line
		protocol.OpcodeDialogueBox,      // 0x0046, a dialog box
		protocol.OpcodeQuestPopupMulti,  // 0x0034, a question and its answers
		protocol.OpcodeDamageIndication, // 0x00DB, a damage or heal figure
		protocol.OpcodeCharacterDied:    // 0x005E
		return true
	}
	return false
}

// Shown is one message of a transcript: when it was queued, its opcode and
// its payload as queued (the opcode first), a copy.
type Shown struct {
	At     time.Time
	Opcode protocol.GameOpcode
	Data   []byte
}

// transcript is the ring a session keeps (Session.shown).
type transcript struct {
	mu   sync.Mutex
	ring [TranscriptMax]Shown
	next int // where the next one goes
	n    int // how many are kept
}

// note keeps a copy of data, a message of op queued at now.
func (t *transcript) note(op protocol.GameOpcode, data []byte, now time.Time) {
	b := make([]byte, len(data))
	copy(b, data)
	t.mu.Lock()
	t.ring[t.next] = Shown{At: now, Opcode: op, Data: b}
	t.next = (t.next + 1) % TranscriptMax
	if t.n < TranscriptMax {
		t.n++
	}
	t.mu.Unlock()
}

// Transcript is what the client was shown within TranscriptAge of now,
// oldest first.
func (s *Session) Transcript(now time.Time) []Shown {
	t := &s.shown
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Shown, 0, t.n)
	for i := 0; i < t.n; i++ {
		e := t.ring[(t.next-t.n+i+2*TranscriptMax)%TranscriptMax]
		if now.Sub(e.At) <= TranscriptAge {
			out = append(out, e)
		}
	}
	return out
}
