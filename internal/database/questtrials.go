package database

// questtrials.go — what a character's trial holds between its moments (Live
// petition 5663, the owner 2026-11-26: "fix these"; handlers/questtrials.go):
// a clock running (a step's clock event: Trainer Iruko's ten minutes, Master
// Landaar's fifteen) and a standing owed (a setback with regain: the 75 with
// the WereHunters of Tunaria a wrong answer took, "re-farm those 75 faction
// points back in order to take the test over"). A row a character and quest
// (table charquesttrials), gone when it holds neither.

import "time"

// QuestTrial is a character's trial state in one quest.
type QuestTrial struct {
	ServerID uint32
	QuestID  int
	// Deadline is when the clock's time is up (zero: no clock); Until the step
	// that stops it, reached in time; Back, Factions, Regain and Says the
	// setback when it is not (QuestEvent's)
	Deadline time.Time
	Until    int
	Back     int
	Factions string
	Regain   bool
	Says     string
	// Owes is the standing that must be earned back ("Werehunters of
	// Tunaria:1075", as a factionreq) before step OweStep is done again
	Owes    string
	OweStep int
}

// Empty is whether it holds neither a clock nor a standing owed.
func (t *QuestTrial) Empty() bool { return t.Deadline.IsZero() && t.Owes == "" }

// QuestTrials is a character's trial states, by quest.
func (s *QuestStore) QuestTrials(serverID uint32) (map[int]*QuestTrial, error) {
	rs, err := s.db.Query(`SELECT questid, deadline, until, back, factions, regain, says, owes, owestep
		FROM charquesttrials WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[int]*QuestTrial{}
	for rs.Next() {
		t := &QuestTrial{ServerID: serverID}
		var deadline int64
		var regain int
		if err := rs.Scan(&t.QuestID, &deadline, &t.Until, &t.Back, &t.Factions, &regain, &t.Says, &t.Owes, &t.OweStep); err != nil {
			return nil, err
		}
		if deadline > 0 {
			t.Deadline = time.Unix(deadline, 0)
		}
		t.Regain = regain != 0
		out[t.QuestID] = t
	}
	return out, rs.Err()
}

// SaveQuestTrial writes a trial state, or takes its row away when it is empty.
func (s *QuestStore) SaveQuestTrial(t *QuestTrial) error {
	if t.Empty() {
		_, err := s.db.Exec(`DELETE FROM charquesttrials WHERE serverid = ? AND questid = ?`, t.ServerID, t.QuestID)
		return err
	}
	var deadline int64
	if !t.Deadline.IsZero() {
		deadline = t.Deadline.Unix()
	}
	regain := 0
	if t.Regain {
		regain = 1
	}
	_, err := s.db.Exec(`INSERT INTO charquesttrials (serverid, questid, deadline, until, back, factions, regain, says, owes, owestep)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid, questid) DO UPDATE SET deadline = excluded.deadline, until = excluded.until,
			back = excluded.back, factions = excluded.factions, regain = excluded.regain, says = excluded.says,
			owes = excluded.owes, owestep = excluded.owestep`,
		t.ServerID, t.QuestID, deadline, t.Until, t.Back, t.Factions, regain, t.Says, t.Owes, t.OweStep)
	return err
}
