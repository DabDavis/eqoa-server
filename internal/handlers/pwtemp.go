package handlers

import (
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A temporary password (the owner, 2026-11-11: a forgotten password is reset
// by a GM, "the player must change it at next login"; tcp/accountpassword.go,
// gmweb/ops_pwreset.go). The game's login takes it, since no game client has a
// screen that changes a password on this server (STATION's PassChng sends
// 0x2A, which the auth port answers without changing anything), so the world
// says it instead: each time a character of the account enters it, in red,
// until the password is changed on the website, which clears the flag.

// PasswordTempFlag reads whether an account's password is temporary
// (database.SQLiteDB.PasswordTemporary).
type PasswordTempFlag interface {
	PasswordTemporary(accountID int) (bool, error)
}

// PasswordTempText is the line the world says; where is the website's
// password page as players reach it (auth.password_url), "" for none named.
func PasswordTempText(where string) string {
	if where == "" {
		where = "the account website's Change password page"
	}
	return "Your password is a temporary one a GM gave you. Set your own now at " + where + "."
}

// PasswordTempNotice is the OnPlayerEntered hook that says it.
func PasswordTempNotice(db PasswordTempFlag, where string) func(*session.Session, *entity.Player) {
	text := PasswordTempText(where)
	return func(sess *session.Session, _ *entity.Player) {
		if db == nil || sess == nil || sess.AccountID() == 0 {
			return
		}
		if temp, err := db.PasswordTemporary(sess.AccountID()); err == nil && temp {
			SendErrorMessage(sess, text)
		}
	}
}
