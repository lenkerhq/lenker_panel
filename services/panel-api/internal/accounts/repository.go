package accounts

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, email, passwordHash string) (Account, error)
	FindByEmail(ctx context.Context, email string) (Account, error)
	FindByActiveSessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (Account, error)
	CreateSession(ctx context.Context, accountID, tokenHash string, expiresAt time.Time) (Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}
