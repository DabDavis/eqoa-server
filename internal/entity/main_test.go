package entity

import (
	"os"
	"testing"
)

// TestMain runs the package's tests, then removes what they shared: the
// pets' database (pettemplate_test.go).
func TestMain(m *testing.M) {
	code := m.Run()
	if petFixture.db != nil {
		petFixture.db.Close()
	}
	if petFixture.dir != "" {
		os.RemoveAll(petFixture.dir)
	}
	os.Exit(code)
}
