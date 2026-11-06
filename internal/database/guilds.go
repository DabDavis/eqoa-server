package database

import "fmt"

// guilds.go — the guilds kept. entity/guild.go held them in memory alone
// until 2026-11-06, so every restart ended every guild (the user: "did
// larrybirds guild not get saved"; the 04:47 UTC restart of that night took
// "Bad Boys" and "Prophets of Zion"). The tables are sqlite.go's guilds and
// guild_members, which nothing wrote before: a guild's id, name, leader and
// charter (motd), and each member's character id and rank. A member's name,
// class, race and level are its characters row's, read with it. Written at
// every change as it is made (entity.World's guild store), read at the start.

// GuildRow is a guild as kept.
type GuildRow struct {
	ID      uint32
	Name    string
	Charter string
	Leader  uint32 // its leader's character id
	Members []GuildMemberRow
}

// GuildMemberRow is a member as kept, with its character's name, class, race
// and level.
type GuildMemberRow struct {
	ID                 uint32
	Rank               int
	Name               string
	Class, Race, Level int
}

// LoadGuilds is every guild kept with its members whose characters still
// exist, by id; a guild none of whose members exist is left out.
func (db *SQLiteDB) LoadGuilds() ([]GuildRow, error) {
	rows, err := db.Query(`SELECT guild_id, name, COALESCE(motd, ''), leader_id FROM guilds ORDER BY guild_id`)
	if err != nil {
		return nil, fmt.Errorf("guilds: %w", err)
	}
	var out []GuildRow
	at := map[uint32]int{}
	for rows.Next() {
		var g GuildRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Charter, &g.Leader); err != nil {
			rows.Close()
			return nil, err
		}
		at[g.ID] = len(out)
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := db.Query(`SELECT m.guild_id, m.player_id, COALESCE(m.rank, 0), c.charName, c.tclass, c.race, c.level
		FROM guild_members m JOIN characters c ON c.serverid = m.player_id ORDER BY m.guild_id, m.player_id`)
	if err != nil {
		return nil, fmt.Errorf("guild members: %w", err)
	}
	defer members.Close()
	for members.Next() {
		var gid uint32
		var m GuildMemberRow
		if err := members.Scan(&gid, &m.ID, &m.Rank, &m.Name, &m.Class, &m.Race, &m.Level); err != nil {
			return nil, err
		}
		if i, ok := at[gid]; ok {
			out[i].Members = append(out[i].Members, m)
		}
	}
	if err := members.Err(); err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, g := range out {
		if len(g.Members) > 0 {
			kept = append(kept, g)
		}
	}
	return kept, nil
}

// SaveGuild writes g whole, in one transaction: its row (made or changed)
// and its members (those whose characters exist). A row of another id under
// the same name is a guild the world no longer has (its name was free to
// take), and goes with its members.
func (db *SQLiteDB) SaveGuild(g GuildRow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM guild_members WHERE guild_id IN
		(SELECT guild_id FROM guilds WHERE name = ? COLLATE NOCASE AND guild_id != ?)`, g.Name, g.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM guilds WHERE name = ? COLLATE NOCASE AND guild_id != ?`, g.Name, g.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO guilds (guild_id, name, leader_id, motd) VALUES (?, ?, ?, ?)
		ON CONFLICT(guild_id) DO UPDATE SET name = excluded.name, leader_id = excluded.leader_id, motd = excluded.motd`,
		g.ID, g.Name, g.Leader, g.Charter); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM guild_members WHERE guild_id = ?`, g.ID); err != nil {
		return err
	}
	for _, m := range g.Members {
		if _, err := tx.Exec(`INSERT INTO guild_members (guild_id, player_id, rank)
			SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM characters WHERE serverid = ?)`, g.ID, m.ID, m.Rank, m.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteGuild takes a guild and its members out (disbanded, or its last
// member gone).
func (db *SQLiteDB) DeleteGuild(id uint32) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM guild_members WHERE guild_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM guilds WHERE guild_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
