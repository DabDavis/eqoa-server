package session_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// receiver is the receive half of connection.cpp for one state channel: each
// message is kept under its sequence number, and one with a reference (its
// refnum the distance back to it) is XORed with that message over the shorter
// of the two, DRDP_MIN of their sizes, keeping its own size.
type receiver map[uint16][]byte

func (c receiver) receive(t *testing.T, data []byte, seq uint16, ref byte) []byte {
	t.Helper()
	v := append([]byte(nil), data...)
	if ref != 0 {
		base, ok := c[seq-uint16(ref)]
		if !ok {
			t.Fatalf("seq %d refers to %d, which the client does not hold", seq, seq-uint16(ref))
		}
		for i := 0; i < len(v) && i < len(base); i++ {
			v[i] ^= base[i]
		}
	}
	c[seq] = v
	return v
}

// effects is a value of the effects channel's shape: the count, then 132
// bytes a record, filled from seed.
func effects(n int, seed byte) []byte {
	v := make([]byte, 4+132*n)
	binary.LittleEndian.PutUint32(v, uint32(n))
	for i := 4; i < len(v); i++ {
		v[i] = seed + byte(i*7)
	}
	return v
}

// The effects channel changes length with its count, and every value reaches
// the client whole: growing, shrinking to none and growing again. The old base
// update XORed in place at the base's old length, so after a shrink it kept
// the tail the client had dropped, and the next longer value went out XORed
// against bytes the client no longer held (the 1-after-0 step fails on it).
func TestEffectsChannelGrowsAndShrinks(t *testing.T) {
	st := session.NewConnectionData(nil, 0, 0).GetClientBuffUpdate()
	st.SetActive(true)
	c := receiver{}
	for k, n := range []int{1, 2, 0, 1, 3, 1, 0, 2} {
		want := effects(n, byte(k+1))
		st.SetSizedData(want)
		data, seq, ref := st.GenerateXORDelta()
		if data == nil {
			t.Fatalf("step %d (%d effects): nothing sent", k, n)
		}
		if got := c.receive(t, data, seq, ref); !bytes.Equal(got, want) {
			t.Fatalf("step %d (%d effects): the client holds %d bytes that differ from the %d set", k, n, len(got), len(want))
		}
		st.UpdateBaseXor(seq)
		if again, _, _ := st.GenerateXORDelta(); again != nil {
			t.Fatalf("step %d: an acknowledged value went out again", k)
		}
	}
}

// Unacknowledged, the values still arrive whole: each goes out against the
// newest acknowledged one, which the client holds.
func TestEffectsChannelUnacknowledged(t *testing.T) {
	st := session.NewConnectionData(nil, 0, 0).GetClientBuffUpdate()
	st.SetActive(true)
	c := receiver{}
	send := func(want []byte) uint16 {
		st.SetSizedData(want)
		data, seq, ref := st.GenerateXORDelta()
		if got := c.receive(t, data, seq, ref); !bytes.Equal(got, want) {
			t.Fatalf("seq %d: the client holds a different value", seq)
		}
		return seq
	}
	first := send(effects(2, 1))
	st.UpdateBaseXor(first)
	send(effects(0, 2))
	send(effects(3, 3))
	last := send(effects(1, 4))
	st.UpdateBaseXor(last)
	send(effects(2, 5))
}

// An unacknowledged change is sent again after ResendInterval quiet visits,
// one that only shortens the value included: a value that is a prefix of the
// base used to count as unchanged, so its loss was never repaired.
func TestShorterValueIsResent(t *testing.T) {
	st := session.NewConnectionData(nil, 0, 0).GetClientBuffUpdate()
	st.SetActive(true)
	long := effects(1, 9)
	st.SetSizedData(long)
	_, seq, _ := st.GenerateXORDelta()
	st.UpdateBaseXor(seq)
	st.SetSizedData(long[:40])
	_, sent, _ := st.GenerateXORDelta()
	var again []byte
	var at uint16
	for i := 0; i < session.ResendInterval && again == nil; i++ {
		again, at, _ = st.GenerateXORDelta()
	}
	if again == nil || at != sent || len(again) != 40 {
		t.Fatalf("the shortened value was not resent under its sequence (%d bytes, seq %d, first sent as %d)", len(again), at, sent)
	}
}
