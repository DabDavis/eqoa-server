package database

// questpointerdrops.go — the next-giver pointers a character deleted from its
// journal (handlers/questpointers.go), kept over logins (Heirloom petition
// 3467: "ive been trying to remove this quest from my quest log for 3 days
// now, and everytime i log back in it shows back up"). A row a character and
// the quest the pointer pointed to (table charquestpointers); an NPC who sends
// the player on to that quest again takes the row away.

import "time"

// DroppedPointers is the quests whose pointers the character deleted.
func (s *QuestStore) DroppedPointers(serverID uint32) (map[int]bool, error) {
	rs, err := s.db.Query(`SELECT questid FROM charquestpointers WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[int]bool{}
	for rs.Next() {
		var id int
		if err := rs.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rs.Err()
}

// DropPointer keeps the character's delete of the pointer to quest.
func (s *QuestStore) DropPointer(serverID uint32, quest int, now time.Time) error {
	_, err := s.db.Exec(`INSERT INTO charquestpointers (serverid, questid, at) VALUES (?, ?, ?)
		ON CONFLICT(serverid, questid) DO UPDATE SET at = excluded.at`,
		serverID, quest, now.UTC().Format("2006-01-02T15:04:05Z"))
	return err
}

// RestorePointer forgets the character's delete of the pointer to quest.
func (s *QuestStore) RestorePointer(serverID uint32, quest int) error {
	_, err := s.db.Exec(`DELETE FROM charquestpointers WHERE serverid = ? AND questid = ?`, serverID, quest)
	return err
}
