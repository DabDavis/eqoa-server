package entity

import (
	"io"
	"log"
	"os"
	"testing"
)

// companionrelink_real_test.go — the companions' relink (linkCompanionsLocked)
// over a real server's NPCs (EQOA_REAL_DB): Live's roam turns spiked 300-600 ms
// on every relink (2026-11-18), each a spawn or a despawn away.

func BenchmarkRealCompanionRelink(b *testing.B) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		b.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	w := realCompanionWorld(b)
	w.linkCompanionsLocked()
	b.Logf("%d NPCs, %d pets paired", len(w.NPCs), len(w.companionOf))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.companionsAt = -1 // an NPC came or went
		w.linkCompanionsLocked()
	}
}

// TestRealRelinkPairsAsBefore holds the relink to the one before it over a
// real server's NPCs.
func TestRealRelinkPairsAsBefore(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	w := realCompanionWorld(t)
	want := linkCompanionsBefore(w)
	w.linkCompanionsLocked()
	sameCompanions(t, w, want)
	t.Logf("%d NPCs, %d pairs the same", len(w.NPCs), len(want))
}
