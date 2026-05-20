package devbootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailRequired    = errors.New("admin email is required")
	ErrEmailInvalid     = errors.New("admin email must contain @")
	ErrPasswordRequired = errors.New("admin password is required")
	ErrPasswordTooShort = errors.New("admin password must be at least 8 characters")
)

type AdminInput struct {
	Email    string
	Password string
}

type AdminResult struct {
	ID       string
	Email    string
	Password string
	Created  bool
}

func NormalizeAdminInput(input AdminInput) (AdminInput, error) {
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	if input.Email == "" {
		return AdminInput{}, ErrEmailRequired
	}
	if !strings.Contains(input.Email, "@") {
		return AdminInput{}, ErrEmailInvalid
	}
	if input.Password == "" {
		return AdminInput{}, ErrPasswordRequired
	}
	if len(input.Password) < 8 {
		return AdminInput{}, ErrPasswordTooShort
	}
	return input, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func BootstrapAdmin(ctx context.Context, db *sql.DB, input AdminInput) (AdminResult, error) {
	input, err := NormalizeAdminInput(input)
	if err != nil {
		return AdminResult{}, err
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return AdminResult{}, err
	}

	var result AdminResult
	err = db.QueryRowContext(ctx, `
		INSERT INTO admins (email, password_hash, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (email) DO NOTHING
		RETURNING id::text, email
	`, input.Email, passwordHash).Scan(&result.ID, &result.Email)
	if err == nil {
		result.Created = true
		result.Password = input.Password
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AdminResult{}, err
	}

	err = db.QueryRowContext(ctx, `
		SELECT id::text, email
		FROM admins
		WHERE email = $1
	`, input.Email).Scan(&result.ID, &result.Email)
	if err != nil {
		return AdminResult{}, err
	}
	return result, nil
}

func WriteResult(out io.Writer, result AdminResult) {
	if result.Created {
		fmt.Fprintf(out, "created admin %s (%s)\n", result.Email, result.ID)
		fmt.Fprintf(out, "password: %s\n", result.Password)
		return
	}
	fmt.Fprintf(out, "admin %s already exists (%s); password was not changed\n", result.Email, result.ID)
}
