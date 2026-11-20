package entity

import "testing"

// rigChecksOff turns this package's tests' own checks off for a rig's
// figures (the sets' full scans, the class keys made again), back at the
// end of the test.
func rigChecksOff(tb testing.TB) {
	sets, keys := npcSetsCheck, npcKeyCheck
	npcSetsCheck, npcKeyCheck = false, false
	tb.Cleanup(func() { npcSetsCheck, npcKeyCheck = sets, keys })
}

// rigChecksOn turns back on the checks of the work the rig measures (not
// the sets' full scans, which would only cost).
func rigChecksOn(tb testing.TB) {
	npcKeyCheck = true
}
