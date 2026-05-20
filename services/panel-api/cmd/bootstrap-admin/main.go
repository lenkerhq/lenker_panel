package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lenker/lenker/services/panel-api/internal/devbootstrap"
)

func generatePassword(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

func main() {
	email := flag.String("email", os.Getenv("ADMIN_EMAIL"), "admin email")
	password := flag.String("password", os.Getenv("ADMIN_PASSWORD"), "admin password")
	flag.Parse()

	// Generate password if not provided or is the default
	if *password == "" || *password == "change-me-now" {
		generated, err := generatePassword(16)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate password: %v\n", err)
			os.Exit(1)
		}
		password = &generated
	}

	dsn := os.Getenv("LENKER_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "LENKER_DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "connect database: %v\n", err)
		os.Exit(1)
	}

	result, err := devbootstrap.BootstrapAdmin(ctx, db, devbootstrap.AdminInput{
		Email:    *email,
		Password: *password,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap admin: %v\n", err)
		os.Exit(1)
	}

	devbootstrap.WriteResult(os.Stdout, result)
}
