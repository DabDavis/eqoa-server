package entity

// A creature's hate for someone is never below 0 (petition 3254: a ranger cast Concussion, a hate
// of -144, on a creature that was not fighting, then nuked it without ever becoming its enemy: the
// -144 sat on its list and every later hit paid it off first). A hate taken away (the rogues'
// evades, Concussion and the clashes: EffectHate with a negative amount) takes from what the
// creature holds for the caster, down to 0 and no further; from one not on its list it takes
// nothing, starts no fight and calls no one (updateAggro, addThreatLocked).

// addHate is hate held plus delta, never below 0.
func addHate(held int64, delta int) int64 {
	return max(held+int64(delta), 0)
}
