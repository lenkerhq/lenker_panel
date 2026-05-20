package accounts

import (
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("account not found")
	ErrEmailTaken     = errors.New("email already registered")
	ErrInvalidSession = errors.New("invalid or expired session")
)

type Account struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
