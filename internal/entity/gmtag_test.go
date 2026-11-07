package entity

import (
	"bytes"
	"testing"
)

// The user, 2026-11-07: a GM's name carries [GM] where clients draw it (the channel's 24
// bytes at 116); everyone else's is as it was.
func TestAGMsNameCarriesTheTag(t *testing.T) {
	for _, c := range []struct {
		gm   bool
		want string
	}{{true, "[GM] Jibrish"}, {false, "Jibrish"}} {
		p := &Player{Name: "Jibrish", GM: c.gm, Level: 60}
		buf := SerializePlayerUpdate(1, p)
		if len(buf) < 140 {
			t.Fatalf("channel %d bytes", len(buf))
		}
		name := string(bytes.TrimRight(buf[116:140], "\x00"))
		if name != c.want {
			t.Errorf("GM %v: the channel's name %q, want %q", c.gm, name, c.want)
		}
	}
}
