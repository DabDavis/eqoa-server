package entity

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// TestMain runs the package's tests with the NPC sets checked, then removes
// what they shared: the pets' database (pettemplate_test.go).
func TestMain(m *testing.M) {
	// every set-based system checks its set against the whole NPC map, so a
	// change of an NPC's state made without its hook fails here (npcsets.go;
	// EQOA_CHECK_NPC_SETS=log lists them all instead)
	npcSetsCheck = true
	// every NPC class key kept is checked against one made afresh (npckey.go)
	npcKeyCheck = true
	// every channel's kept compare with its base is checked against a fresh
	// one (session.StateUpdateData.GenerateXORDelta)
	session.SetBaseCheck(true)
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
