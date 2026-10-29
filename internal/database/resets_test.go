package database

import (
	"errors"
	"testing"
)

// resetDB is a database with two characters, A (1) and B (2).
func resetDB(t *testing.T) *SQLiteCharacterStore {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/resets.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewSQLiteCharacterStore(db)
	for _, name := range []string{"A", "B"} {
		if err := s.Create(&CharacterRecord{CharName: name, AccountID: 1, Level: 30}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// A training reset out of the world gives every point the tp columns held
// back to the unspent ones, the total never below them, and leaves a
// negative column (a hand's edit) as it is. The control: the other
// character's row is untouched, and an unknown one is ErrCharacterNotFound.
func TestResetTrainingOffline(t *testing.T) {
	s := resetDB(t)
	a, _ := s.GetByServerID(1)
	a.TPStrength, a.TPAgility, a.TPCharisma, a.TPWisdom, a.UnusedTP, a.TotalTP = 10, 4, -2, 6, 3, 15
	b, _ := s.GetByServerID(2)
	b.TPStamina, b.UnusedTP, b.TotalTP = 7, 1, 8
	for _, r := range []*CharacterRecord{a, b} {
		if err := s.Update(r); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.ResetTraining(1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Spent != [7]int{10, 0, 4, 0, 6, 0, 0} || r.Refund() != 20 || r.Unused != 23 || r.Total != 23 {
		t.Fatalf("reset %+v (refund %d)", r, r.Refund())
	}
	a, _ = s.GetByServerID(1)
	if a.TPStrength != 0 || a.TPAgility != 0 || a.TPWisdom != 0 || a.TPCharisma != -2 || a.UnusedTP != 23 || a.TotalTP != 23 {
		t.Errorf("A kept %+v", a)
	}
	if b2, _ := s.GetByServerID(2); b2.TPStamina != 7 || b2.UnusedTP != 1 || b2.TotalTP != 8 {
		t.Errorf("the control: B is %+v", b2)
	}
	if _, err := s.ResetTraining(99); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("no character 99: %v", err)
	}
}

// TakeTraining lowers the columns by what was given back, not to zero (points
// the trainer took meanwhile stay), and writes the game's unspent and total.
func TestTakeTrainingLowers(t *testing.T) {
	s := resetDB(t)
	a, _ := s.GetByServerID(1)
	a.TPStrength, a.TPDexterity = 9, 2 // 2 of the STR taken after the game's reset read 7
	if err := s.Update(a); err != nil {
		t.Fatal(err)
	}
	if err := s.TakeTraining(1, TrainingReset{Spent: [7]int{7, 0, 0, 2}, Unused: 40, Total: 44}); err != nil {
		t.Fatal(err)
	}
	a, _ = s.GetByServerID(1)
	if a.TPStrength != 2 || a.TPDexterity != 0 || a.UnusedTP != 40 || a.TotalTP != 44 {
		t.Errorf("kept %+v", a)
	}
	if err := s.TakeTraining(99, TrainingReset{}); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("no character 99: %v", err)
	}
}

// AddBankTunar adds and takes away, never below 0 nor past MaxBankTunar.
func TestAddBankTunar(t *testing.T) {
	s := resetDB(t)
	for _, c := range []struct{ n, want int }{{500, 500}, {-200, 300}, {-1000, 0}, {MaxBankTunar, MaxBankTunar}, {5, MaxBankTunar}} {
		got, err := s.AddBankTunar(2, c.n)
		if err != nil || got != c.want {
			t.Fatalf("%+d: %d %v, want %d", c.n, got, err, c.want)
		}
		if b, _ := s.GetByServerID(2); b.BankTunar != c.want {
			t.Fatalf("%+d: the row holds %d", c.n, b.BankTunar)
		}
	}
	if a, _ := s.GetByServerID(1); a.BankTunar != 0 {
		t.Errorf("the control: A's bank %d", a.BankTunar)
	}
	if _, err := s.AddBankTunar(99, 1); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("no character 99: %v", err)
	}
}
