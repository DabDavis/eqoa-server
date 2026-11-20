package entity

import "testing"

// rigChecksOff turns this package's tests' own checks off for a rig's
// figures (the sets' full scans), back at the end of the test.
func rigChecksOff(tb testing.TB) {
	sets := npcSetsCheck
	npcSetsCheck = false
	tb.Cleanup(func() { npcSetsCheck = sets })
}

// rigChecksOn turns back on the checks of the work the rig measures.
func rigChecksOn(tb testing.TB) {}
