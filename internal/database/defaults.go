package database

import (
	"log"
)

// DefaultCharacterRecord represents a row from the defaultcharacter table.
type DefaultCharacterRecord struct {
	ID                  int
	TClass              int
	Race                int
	HumType             int
	Level               int
	Tunar               int
	BankTunar           int
	UnusedTP            int
	TotalTP             int
	X, Y, Z             float32
	Facing              float32
	DefaultSpeed        float32
	World               int
	DefaultStrength     int
	DefaultStamina      int
	DefaultAgility      int
	DefaultDexterity    int
	DefaultWisdom       int
	DefaultIntelligence int
	DefaultCharisma     int
}

// DefaultCharacterStore provides lookup for default character templates.
type DefaultCharacterStore struct {
	db *SQLiteDB
	// In-memory cache keyed by (tclass, race, humType)
	defaults map[defaultKey]*DefaultCharacterRecord
}

type defaultKey struct {
	tclass, race, humType int
}

// NewDefaultCharacterStore creates and loads the default character store.
func NewDefaultCharacterStore(db *SQLiteDB) *DefaultCharacterStore {
	s := &DefaultCharacterStore{
		db:       db,
		defaults: make(map[defaultKey]*DefaultCharacterRecord),
	}
	s.loadAll()
	return s
}

// loadAll loads all default character records into memory.
func (s *DefaultCharacterStore) loadAll() {
	rows, err := s.db.Query(`
		SELECT defaultCharacterID, tclass, race, humType, level, tunar, bankTunar,
			unusedTP, totalTP, x, y, COALESCE(z, 0), facing,
			COALESCE(defaultSpeed, 8), COALESCE(world, 0),
			COALESCE(defaultStrength, 0), COALESCE(defaultStamina, 0),
			COALESCE(defaultAgility, 0), COALESCE(defaultDexterity, 0),
			COALESCE(defaultWisdom, 0), COALESCE(defaultIntelligence, 0),
			COALESCE(defaultCharisma, 0)
		FROM defaultcharacter
	`)
	if err != nil {
		log.Printf("DefaultCharacterStore: failed to load defaults: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var d DefaultCharacterRecord
		err := rows.Scan(
			&d.ID, &d.TClass, &d.Race, &d.HumType, &d.Level, &d.Tunar, &d.BankTunar,
			&d.UnusedTP, &d.TotalTP, &d.X, &d.Y, &d.Z, &d.Facing,
			&d.DefaultSpeed, &d.World,
			&d.DefaultStrength, &d.DefaultStamina,
			&d.DefaultAgility, &d.DefaultDexterity,
			&d.DefaultWisdom, &d.DefaultIntelligence,
			&d.DefaultCharisma,
		)
		if err != nil {
			log.Printf("DefaultCharacterStore: scan error: %v", err)
			continue
		}
		key := defaultKey{d.TClass, d.Race, d.HumType}
		s.defaults[key] = &d
		count++
	}
	log.Printf("DefaultCharacterStore: loaded %d default character templates", count)
}

// GetDefaults returns the default character template for a given class, race, humType.
func (s *DefaultCharacterStore) GetDefaults(tclass, race, humType int) *DefaultCharacterRecord {
	key := defaultKey{tclass, race, humType}
	if d, ok := s.defaults[key]; ok {
		return d
	}

	// Try with humType=0 or humType=1 as fallback
	for _, ht := range []int{0, 1} {
		key := defaultKey{tclass, race, ht}
		if d, ok := s.defaults[key]; ok {
			return d
		}
	}

	// Try any matching class+race
	for k, d := range s.defaults {
		if k.tclass == tclass && k.race == race {
			return d
		}
	}

	return nil
}

// Count returns the number of default character templates.
func (s *DefaultCharacterStore) Count() int {
	return len(s.defaults)
}
