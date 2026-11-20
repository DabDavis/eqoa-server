package database

import "testing"

// TestNPCClassKeyCached checks the kept and fast paths against the plain
// lowering, over names of every shape, asked twice (made, then kept).
func TestNPCClassKeyCached(t *testing.T) {
	names := []string{"", "a gnoll", "Lady Vox", "  Lady Vox  ", "orc pawn$", "Orc Pawn $", " x", "x ",
		"\tTab", "Ünicode Name", "ünicode", "$", " $ ", "UPPER", "mixed Case$", "a b", " a"}
	for round := 0; round < 2; round++ {
		for _, n := range names {
			if got, want := NPCClassKey(n), npcClassKeyOf(n); got != want {
				t.Errorf("round %d: NPCClassKey(%q) = %q, want %q", round, n, got, want)
			}
		}
	}
}

func BenchmarkNPCClassKey(b *testing.B) {
	names := []string{"a gnoll", "Lady Vox", "Fippy Darkpaw", "a froglok villager"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NPCClassKey(names[i%len(names)])
	}
}

// BenchmarkNPCClassKeyUncached is the lowering NPCClassKey kept before.
func BenchmarkNPCClassKeyUncached(b *testing.B) {
	names := []string{"a gnoll", "Lady Vox", "Fippy Darkpaw", "a froglok villager"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = npcClassKeyOf(names[i%len(names)])
	}
}
