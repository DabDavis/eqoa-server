package protocol

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
)

// A client's reliable message past 1,300 bytes comes in pieces
// (connection_send_guaranteed): 0xFA pieces of 1,300, only the first starting
// with the opcode, then a last 0xFB as short as one byte. Each piece is read
// with its bytes whole (Unsplit), and a one-byte last piece is read as one
// byte: an opcode read off it ran into the next message. The control: a
// message whole has its opcode taken off its data, as always.
func TestPiecesAreReadWhole(t *testing.T) {
	first := make([]byte, 1300)
	first[0], first[1] = 0xA1, 0x0A // the opcode, 0x0AA1
	for i := 2; i < len(first); i++ {
		first[i] = byte(i)
	}
	type wireMsg struct {
		typ  byte
		seq  uint16
		data []byte
	}
	msgs := []wireMsg{
		{0xFA, 7, first},
		{0xFB, 8, []byte{0x2E}}, // the last piece of 1,301 bytes
		{0xFB, 9, []byte{0x0B, 0x00, 'h', 'i'}},
		{0xF9, 10, []byte{1, 2, 3}},
	}
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	w.WriteByte(0)   // body flags
	w.WriteUint16(1) // arrival number
	for _, m := range msgs {
		w.WriteByte(m.typ)
		w.WriteSize(len(m.data))
		w.WriteUint16(m.seq)
		w.WriteBytes(m.data)
	}
	n := w.Position()
	body := NewSegmentBody()
	if err := body.Read(buffer.NewReader(buf[:n]), buf[:n], &SegmentHeader{Size: n}); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != len(msgs) {
		t.Fatalf("%d messages read, want %d", len(body.Messages), len(msgs))
	}
	for i, m := range body.Messages {
		if byte(m.Type) != msgs[i].typ || m.Sequence != msgs[i].seq || !bytes.Equal(m.Wire(), msgs[i].data) {
			t.Errorf("message %d: type %#x seq %d wire % x, want %#x %d % x", i, m.Type, m.Sequence, m.Wire(), msgs[i].typ, msgs[i].seq, msgs[i].data)
		}
	}
	if !body.Messages[0].Unsplit || len(body.Messages[0].Data()) != 1300 {
		t.Errorf("the first piece: unsplit %v, %d bytes of data", body.Messages[0].Unsplit, len(body.Messages[0].Data()))
	}
	if !body.Messages[1].Unsplit || len(body.Messages[1].Data()) != 1 {
		t.Errorf("the one-byte last piece: unsplit %v, %d bytes", body.Messages[1].Unsplit, len(body.Messages[1].Data()))
	}
	whole := body.Messages[2]
	if whole.Unsplit || whole.Opcode != 0x000B || string(whole.Data()) != "hi" {
		t.Errorf("the control, a message whole: unsplit %v, opcode %#x, data %q", whole.Unsplit, whole.Opcode, whole.Data())
	}
	if !body.Messages[3].Unsplit || body.Messages[3].Opcode != 0 {
		t.Errorf("a system message has no opcode: unsplit %v, opcode %#x", body.Messages[3].Unsplit, body.Messages[3].Opcode)
	}
}
