package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveAccount    = errors.New("account is inactive")
)

type Service struct {
	repo            Repository
	sessionDuration time.Duration
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, sessionDuration: 7 * 24 * time.Hour}
}

type RegisterInput struct {
	Email    string
	Password string
}

type RegisterResult struct {
	Account Account `json:"account"`
	Session Session `json:"session"`
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || input.Password == "" {
		return RegisterResult{}, ErrInvalidCredentials
	}
	if len(input.Password) < 8 {
		return RegisterResult{}, ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return RegisterResult{}, err
	}

	account, err := s.repo.Create(ctx, email, string(hash))
	if err != nil {
		return RegisterResult{}, err
	}

	token, tokenHash, err := newToken()
	if err != nil {
		return RegisterResult{}, err
	}

	session, err := s.repo.CreateSession(ctx, account.ID, tokenHash, time.Now().UTC().Add(s.sessionDuration))
	if err != nil {
		return RegisterResult{}, err
	}
	session.Token = token

	return RegisterResult{Account: account, Session: session}, nil
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	Account Account `json:"account"`
	Session Session `json:"session"`
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || input.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	account, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}

	if account.Status != "active" {
		return LoginResult{}, ErrInactiveAccount
	}

	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(input.Password)) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, tokenHash, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}

	session, err := s.repo.CreateSession(ctx, account.ID, tokenHash, time.Now().UTC().Add(s.sessionDuration))
	if err != nil {
		return LoginResult{}, err
	}
	session.Token = token

	return LoginResult{Account: account, Session: session}, nil
}

func (s *Service) Me(ctx context.Context, tokenHash string) (Account, error) {
	account, err := s.repo.FindByActiveSessionTokenHash(ctx, tokenHash, time.Now().UTC())
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (s *Service) Logout(ctx context.Context, tokenHash string) error {
	return s.repo.DeleteSession(ctx, tokenHash)
}

func newToken() (token string, tokenHash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(raw)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
