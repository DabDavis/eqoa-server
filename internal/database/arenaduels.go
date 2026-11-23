package database

func (db *SQLiteDB) RecentArenaDuels() ([][2]string, error) {
	rows, err := db.Query("SELECT winner,loser FROM arena_duel_results ORDER BY id DESC LIMIT 10")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var pair [2]string
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, err
		}
		out = append(out, pair)
	}
	return out, rows.Err()
}
