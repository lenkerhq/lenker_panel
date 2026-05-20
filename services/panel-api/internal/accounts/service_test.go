package accounts

import (
	"context"
	"testing"
	"time"
)

type mockRepo struct {
	accounts map[string]Account
	sessions map[string]Session
}

func newMockRepo() *mockRepo {
	return &mockRepo{accounts: make(map[string]Account), sessions: make(map[string]Session)}
}

func (m *mockRepo) Create(_ context.Context, email, passwordHash string) (Account, error) {
	for _, a := range m.accounts {
		if a.Email == email {
			return Account{}, ErrEmailTaken
		}
	}
	a := Account{
		ID:           "acc-" + email,
		Email:        email,
		PasswordHash: passwordHash,
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	m.accounts[a.ID] = a
	return a, nil
}

func (m *mockRepo) FindByEmail(_ context.Context, email string) (Account, error) {
	for _, a := range m.accounts {
		if a.Email == email {
			return a, nil
		}
	}
	return Account{}, ErrNotFound
}

func (m *mockRepo) FindByActiveSessionTokenHash(_ context.Context, tokenHash string, now time.Time) (Account, error) {
	s, ok := m.sessions[tokenHash]
	if !ok || s.ExpiresAt.Before(now) {
		return Account{}, ErrNotFound
	}
	a, ok := m.accounts[s.AccountID]
	if !ok || a.Status != "active" {
		return Account{}, ErrNotFound
	}
	return a, nil
}

func (m *mockRepo) CreateSession(_ context.Context, accountID, tokenHash string, expiresAt time.Time) (Session, error) {
	s := Session{ID: "sess-1", AccountID: accountID, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
	m.sessions[tokenHash] = s
	return s, nil
}

func (m *mockRepo) DeleteSession(_ context.Context, tokenHash string) error {
	delete(m.sessions, tokenHash)
	return nil
}

func TestRegister(t *testing.T) {
	svc := NewService(newMockRepo())

	result, err := svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Account.Email != "user@test.com" {
		t.Errorf("expected email user@test.com, got %s", result.Account.Email)
	}
	if result.Session.Token == "" {
		t.Error("expected session token")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	svc := NewService(newMockRepo())

	_, _ = svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password123"})
	_, err := svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password456"})
	if err != ErrEmailTaken {
		t.Errorf("expected ErrEmailTaken, got %v", err)
	}
}

func TestRegisterShortPassword(t *testing.T) {
	svc := NewService(newMockRepo())

	_, err := svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "short"})
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password123"})

	result, err := svc.Login(context.Background(), LoginInput{Email: "user@test.com", Password: "password123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.Token == "" {
		t.Error("expected session token")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password123"})

	_, err := svc.Login(context.Background(), LoginInput{Email: "user@test.com", Password: "wrongpass"})
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginNotFound(t *testing.T) {
	svc := NewService(newMockRepo())

	_, err := svc.Login(context.Background(), LoginInput{Email: "nobody@test.com", Password: "password123"})
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestMeAndLogout(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	result, _ := svc.Register(context.Background(), RegisterInput{Email: "user@test.com", Password: "password123"})
	tokenHash := HashToken(result.Session.Token)

	account, err := svc.Me(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.Email != "user@test.com" {
		t.Errorf("expected user@test.com, got %s", account.Email)
	}

	if err := svc.Logout(context.Background(), tokenHash); err != nil {
		t.Fatalf("logout error: %v", err)
	}

	_, err = svc.Me(context.Background(), tokenHash)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after logout, got %v", err)
	}
}
