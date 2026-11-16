package database

// bugboard.go — the counts behind the Bug Finder Leaderboard (handlers/bugboard.go): each account's bug and crash
// reports filed in the game, as the website's contributors page counts them (category 1, Bug Report; a crash is its
// subcategory crashSub; every other subcategory a bug, whatever became of the petition since: answered, closed or
// deleted by its player; the subcategories in skip, Feature Request, are no bug and not counted).

// BugTally is one account's reports.
type BugTally struct {
	AccountID int
	Username  string // accountinfo.username, "" when the account is gone
	// Player is the character that filed most of the account's reports (the latest on a tie), "" when none
	// named one: the board shows players, not accounts (the owner, 2026-11-16: "bug reports by player name
	// instead of account name").
	Player  string
	Bugs    int
	Crashes int
}

// BugReportTallies is every account's tally on q's petitions, an account with none left out.
func BugReportTallies(q Queryer, category, crashSub int64, skip ...int64) ([]BugTally, error) {
	args := []any{crashSub, crashSub, category}
	not := ""
	for _, s := range skip {
		not += " AND p.subcategory != ?"
		args = append(args, s)
	}
	rows, err := q.Query(`SELECT p.accountid, COALESCE(a.username, ''),
			COALESCE((SELECT p2.charname FROM petitions p2 WHERE p2.accountid = p.accountid AND p2.category = p.category
				AND p2.charname != '' GROUP BY p2.charname ORDER BY COUNT(*) DESC, MAX(p2.filed) DESC LIMIT 1), ''),
			SUM(CASE WHEN p.subcategory != ? THEN 1 ELSE 0 END), SUM(CASE WHEN p.subcategory = ? THEN 1 ELSE 0 END)
		FROM petitions p LEFT JOIN accountinfo a ON a.accountid = p.accountid
		WHERE p.category = ? AND p.accountid > 0`+not+`
		GROUP BY p.accountid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BugTally
	for rows.Next() {
		var t BugTally
		if err := rows.Scan(&t.AccountID, &t.Username, &t.Player, &t.Bugs, &t.Crashes); err != nil {
			return nil, err
		}
		if t.Bugs+t.Crashes > 0 {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}
