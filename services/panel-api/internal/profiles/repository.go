package profiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Repository interface {
	List(ctx context.Context) ([]*NodeProfile, error)
	FindByID(ctx context.Context, id string) (*NodeProfile, error)
	Create(ctx context.Context, input CreateInput) (*NodeProfile, error)
	Update(ctx context.Context, id string, input UpdateInput) (*NodeProfile, error)
	Delete(ctx context.Context, id string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context) ([]*NodeProfile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, name, description, is_system, config, created_at, updated_at
		FROM node_profiles ORDER BY is_system DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*NodeProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*NodeProfile, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id::text, name, description, is_system, config, created_at, updated_at
		FROM node_profiles WHERE id = $1::uuid
	`, id)
	return scanProfileRow(row)
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (*NodeProfile, error) {
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO node_profiles (name, description, config)
		VALUES ($1, $2, $3)
		RETURNING id::text, name, description, is_system, config, created_at, updated_at
	`, input.Name, nullableString(input.Description), cfg)
	return scanProfileRow(row)
}

func (r *PostgresRepository) Update(ctx context.Context, id string, input UpdateInput) (*NodeProfile, error) {
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = nil
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE node_profiles SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			config = COALESCE($4, config),
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, name, description, is_system, config, created_at, updated_at
	`, id, nullableString(input.Name), nullableString(input.Description), nullableJSON(cfg))
	return scanProfileRow(row)
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM node_profiles WHERE id = $1::uuid AND is_system = false`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProfile(s scanner) (*NodeProfile, error) {
	var p NodeProfile
	var desc sql.NullString
	var cfg []byte
	err := s.Scan(&p.ID, &p.Name, &desc, &p.IsSystem, &cfg, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if desc.Valid {
		p.Description = &desc.String
	}
	p.Config = json.RawMessage(cfg)
	return &p, nil
}

func scanProfileRow(row *sql.Row) (*NodeProfile, error) {
	var p NodeProfile
	var desc sql.NullString
	var cfg []byte
	err := row.Scan(&p.ID, &p.Name, &desc, &p.IsSystem, &cfg, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if desc.Valid {
		p.Description = &desc.String
	}
	p.Config = json.RawMessage(cfg)
	return &p, nil
}

func nullableString(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return []byte(data)
}
