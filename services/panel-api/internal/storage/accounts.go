package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/accounts"
)

type accountsRepository struct {
	db *sql.DB
}

func NewAccountsRepository(db *sql.DB) accounts.Repository {
	return &accountsRepository{db: db}
}

func (r *accountsRepository) Create(ctx context.Context, email, passwordHash string) (accounts.Account, error) {
	var a accounts.Account
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO consumer_accounts (email, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, email, status, created_at, updated_at
	`, email, passwordHash).Scan(&a.ID, &a.Email, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return accounts.Account{}, accounts.ErrEmailTaken
		}
		return accounts.Account{}, err
	}
	return a, nil
}

func (r *accountsRepository) FindByEmail(ctx context.Context, email string) (accounts.Account, error) {
	var a accounts.Account
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, email, password_hash, status, created_at, updated_at
		FROM consumer_accounts
		WHERE email = $1
	`, email).Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return accounts.Account{}, accounts.ErrNotFound
		}
		return accounts.Account{}, err
	}
	return a, nil
}

func (r *accountsRepository) FindByActiveSessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (accounts.Account, error) {
	var a accounts.Account
	err := r.db.QueryRowContext(ctx, `
		SELECT a.id::text, a.email, a.status, a.created_at, a.updated_at
		FROM consumer_sessions s
		JOIN consumer_accounts a ON a.id = s.account_id
		WHERE s.session_token_hash = $1
		  AND s.expires_at > $2
		  AND a.status = 'active'
	`, tokenHash, now).Scan(&a.ID, &a.Email, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return accounts.Account{}, accounts.ErrNotFound
		}
		return accounts.Account{}, err
	}

	_, _ = r.db.ExecContext(ctx, `
		UPDATE consumer_sessions SET last_seen_at = $1 WHERE session_token_hash = $2
	`, now, tokenHash)

	return a, nil
}

func (r *accountsRepository) CreateSession(ctx context.Context, accountID, tokenHash string, expiresAt time.Time) (accounts.Session, error) {
	var s accounts.Session
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO consumer_sessions (account_id, session_token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id::text, account_id::text, expires_at, created_at
	`, accountID, tokenHash, expiresAt).Scan(&s.ID, &s.AccountID, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return accounts.Session{}, err
	}
	return s, nil
}

func (r *accountsRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM consumer_sessions WHERE session_token_hash = $1
	`, tokenHash)
	return err
}
