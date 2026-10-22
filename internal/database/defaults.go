package database

import (
	"log"
	"sort"
	"sync"
	"sync/atomic"
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

	mu sync.Mutex
	// In-memory cache keyed by (tclass, race, humType)
	defaults map[defaultKey]*DefaultCharacterRecord
	// gen is defaultsGen when the cache was read
	gen int64
}

// defaultsGen counts InvalidateDefaultCharacters: every store reads the table
// again before its next answer once it has moved.
var defaultsGen atomic.Int64

// InvalidateDefaultCharacters makes every DefaultCharacterStore read the
// defaultcharacter table again before it next answers: the GM panel changed
// it. The game's creation and the account pages each hold a store.
func InvalidateDefaultCharacters() {
	defaultsGen.Add(1)
}

// fresh reads the table again if it changed since the cache was read. Must be
// called with mu held.
func (s *DefaultCharacterStore) fresh() {
	if g := defaultsGen.Load(); g != s.gen {
		s.gen = g
		s.defaults = make(map[defaultKey]*DefaultCharacterRecord)
		s.loadAll()
	}
}

type defaultKey struct {
	tclass, race, humType int
}

// NewDefaultCharacterStore creates and loads the default character store.
func NewDefaultCharacterStore(db *SQLiteDB) *DefaultCharacterStore {
	s := &DefaultCharacterStore{
		db:       db,
		defaults: make(map[defaultKey]*DefaultCharacterRecord),
		gen:      defaultsGen.Load(),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
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

// All returns every template, by race, city (humType) and class.
func (s *DefaultCharacterStore) All() []*DefaultCharacterRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	out := make([]*DefaultCharacterRecord, 0, len(s.defaults))
	for _, d := range s.defaults {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Race != b.Race {
			return a.Race < b.Race
		}
		if a.HumType != b.HumType {
			return a.HumType < b.HumType
		}
		return a.TClass < b.TClass
	})
	return out
}

// Count returns the number of default character templates.
func (s *DefaultCharacterStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	return len(s.defaults)
}
