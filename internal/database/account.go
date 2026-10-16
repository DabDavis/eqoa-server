package database

import (
	"errors"
	"time"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrAccountExists       = errors.New("account already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrAccountBanned       = errors.New("account is banned")
	ErrAccountNotActivated = errors.New("account not activated")
)

// Account represents a user account.
type Account struct {
	ID           int
	Username     string
	PasswordHash string
	Email        string
	CreatedAt    time.Time
	LastLogin    time.Time
	Banned       bool
	BanReason    string
	Activated    bool
	AccessLevel  int
}
