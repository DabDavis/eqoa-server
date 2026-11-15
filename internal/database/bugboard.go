package database

// bugboard.go — the counts behind the Bug Finder Leaderboard (handlers/bugboard.go): each account's bug and crash
// reports filed in the game, as the website's contributors page counts them (category 1, Bug Report; a crash is its
// subcategory crashSub; every other subcategory a bug, whatever became of the petition since: answered, closed or
// deleted by its player; the subcategories in skip, Feature Request, are no bug and not counted).

// BugTally is one account's reports.
type BugTally struct {
	AccountID int
	Username  string // accountinfo.username, "" when the account is gone
	Bugs      int
	Crashes   int
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
		if err := rows.Scan(&t.AccountID, &t.Username, &t.Bugs, &t.Crashes); err != nil {
			return nil, err
		}
		if t.Bugs+t.Crashes > 0 {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}
