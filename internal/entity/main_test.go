package entity

import (
	"os"
	"testing"
)

// TestMain runs the package's tests with the NPC sets checked, then removes
// what they shared: the pets' database (pettemplate_test.go).
func TestMain(m *testing.M) {
	// every set-based system checks its set against the whole NPC map, so a
	// change of an NPC's state made without its hook fails here (npcsets.go;
	// EQOA_CHECK_NPC_SETS=log lists them all instead)
	npcSetsCheck = true
	code := m.Run()
	if petFixture.db != nil {
		petFixture.db.Close()
	}
	if petFixture.dir != "" {
		os.RemoveAll(petFixture.dir)
	}
	if keepFixture.db != nil {
		keepFixture.db.Close()
	}
	if keepFixture.dir != "" {
		os.RemoveAll(keepFixture.dir)
	}
	os.Exit(code)
}
