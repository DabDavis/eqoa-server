package database

// SubGM reports whether the account is a sub GM (the user, 2026-11-08: "make
// his account a sub gm new category above tester than can fly"): a tester
// (gm_testers, internal/gmweb/tester.go) whose row says subgm. In the panel it
// is a tester; in the game a player but for a sub GM's commands
// (handlers/devcmds.go: #fly). Read on each use, as the account's level is; an
// error where the table or its column is missing (a server with no panel's
// tables), which the game reads as no.
func (s *SQLiteAccountStore) SubGM(accountID int) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM gm_testers WHERE accountid = ? AND subgm != 0`, accountID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
