package gmweb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func TestAccountTOTPWithoutPanelListener(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if enrolled, ok := AccountTOTP(db, 1, ""); enrolled || ok {
		t.Fatal("unexpected enrollment on a fresh database")
	}
	mustExec(t, db, panelSchema)
	secret := newTOTPSecret()
	mustExec(t, db, "INSERT INTO gm_totp(accountid, secret, enabled) VALUES (1, ?, '')", secret)
	if enrolled, ok := AccountTOTP(db, 1, ""); !enrolled || ok {
		t.Fatal("enrolled account accepted without a code")
	}
	key, err := b32.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	code := hotp(key, uint64(time.Now().Unix()/totpStep))
	if enrolled, ok := AccountTOTP(db, 1, code); !enrolled || !ok {
		t.Fatal("current code refused")
	}
	if _, ok := AccountTOTP(db, 1, code); ok {
		t.Fatal("spent code accepted")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if enrolled, ok := AccountTOTP(db, 1, ""); !enrolled || ok {
		t.Fatal("failed database access must require an authenticator")
	}
}
