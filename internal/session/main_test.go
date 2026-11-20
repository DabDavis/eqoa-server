package session

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// every kept compare of a channel with its base is checked against a
	// fresh one (StateUpdateData.GenerateXORDelta)
	baseCheck = true
	os.Exit(m.Run())
}
