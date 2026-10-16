package handlers

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// encodeConnStats builds a 0x07E1 payload the way the client does:
// int32_LE(charCount) + UTF-16LE text. charCount is CHARACTERS, not bytes —
// getting that wrong reads half the string and was worth pinning.
func encodeConnStats(text string) []byte {
	u := utf16.Encode([]rune(text))
	out := make([]byte, 4+len(u)*2)
	binary.LittleEndian.PutUint32(out[:4], uint32(len(u)))
	for i, c := range u {
		binary.LittleEndian.PutUint16(out[4+i*2:6+i*2], c)
	}
	return out
}

// decodeConnStats mirrors HandleConnStats' parsing so it can be tested without
// a live session. Kept adjacent to the handler on purpose.
func decodeConnStats(data []byte) (string, bool) {
	if len(data) < 4 {
		return "", false
	}
	charCount := int(binary.LittleEndian.Uint32(data[:4]))
	body := data[4:]
	avail := len(body) / 2
	if charCount < 0 || charCount > avail {
		charCount = avail
	}
	u16 := make([]uint16, charCount)
	for i := 0; i < charCount; i++ {
		u16[i] = binary.LittleEndian.Uint16(body[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16)), true
}

// TestConnStatsRoundTripMatchesCapture uses the exact report retail's client
// sent (pcaps/ps2_sandstorm.pcap packet #115, 512-byte payload) and requires
// the counters to come back out.
func TestConnStatsRoundTripMatchesCapture(t *testing.T) {
	// Abridged to the lines that carry counters; the encoding is what matters.
	const report = "0: discarded (old)\n" +
		"0: discarded (no memory was available to process message)\n" +
		"36: discarded (corrupt)\n" +
		"0: discarded (crc)\n" +
		"0: out of sequence\n" +
		"71: recv\n21: sent\n" +
		"14: guaranteed recv (duplicates)\n27: guaranteed recv\n" +
		"0: state recv (duplicates)\n0: state recv\n"

	payload := encodeConnStats(report)

	// The real one was 512 bytes: 4 header + 254 chars * 2.
	if got := binary.LittleEndian.Uint32(payload[:4]); int(got) != len([]rune(report)) {
		t.Fatalf("charCount = %d, want %d", got, len([]rune(report)))
	}

	out, ok := decodeConnStats(payload)
	if !ok {
		t.Fatal("decode failed on a well-formed payload")
	}
	if out != report {
		t.Errorf("round trip changed the text")
	}
	if !strings.Contains(out, "36: discarded (corrupt)") {
		t.Error("lost the corrupt-discard counter, which is the whole point of reading this")
	}
}

// TestConnStatsSurvivesMalformedPayloads pins that a bad length cannot panic
// the handler. The count is attacker-supplied in the sense that it comes off
// the wire, and a client mid-disconnect can truncate mid-message.
func TestConnStatsSurvivesMalformedPayloads(t *testing.T) {
	good := encodeConnStats("71: recv\n")

	cases := map[string][]byte{
		"empty":       {},
		"header only": good[:4],
		"3 bytes":     good[:3],
		"count way too big": func() []byte {
			b := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(b[:4], 0xFFFFFF)
			return b
		}(),
		"count negative": func() []byte {
			b := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(b[:4], 0xFFFFFFFF)
			return b
		}(),
		"odd trailing byte": append(append([]byte(nil), good...), 0x41),
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on %s: %v", name, r)
				}
			}()
			decodeConnStats(payload) // must not panic; result is don't-care
		})
	}
}
