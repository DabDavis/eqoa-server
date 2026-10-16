package database

import (
	"errors"
	"time"
)

var (
	ErrCharacterNotFound = errors.New("character not found")
	ErrCharacterExists   = errors.New("character name already exists")
	ErrMaxCharacters     = errors.New("maximum characters reached")
)

const MaxCharactersPerAccount = 8

// Character represents a player character.
type Character struct {
	ID              int
	AccountID       int
	ServerID        int
	CharName        string
	ModelID         int
	TLA             int
	Race            int
	Class           int
	HumanType       int
	Level           int
	HairColor       int
	HairLength      int
	HairStyle       int
	FaceOption      int
	X               float32
	Y               float32
	Z               float32
	Facing          float32
	CurrentHP       int
	MaxHP           int
	CurrentPower    int
	MaxPower        int
	Strength        int
	Stamina         int
	Agility         int
	Dexterity       int
	Wisdom          int
	Intelligence    int
	Charisma        int
	TotalXP         int64
	Debt            int64
	Breath          int
	Tunar           int64
	BankTunar       int64
	UnusedTP        int
	TotalTP         int
	World           int
	CreatedAt       time.Time
	LastPlayed      time.Time
	PlaytimeMinutes int
}
