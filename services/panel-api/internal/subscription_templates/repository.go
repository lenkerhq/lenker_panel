package subscription_templates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Repository interface {
	List(ctx context.Context) ([]*Template, error)
	FindByID(ctx context.Context, id string) (*Template, error)
	Create(ctx context.Context, input CreateInput) (*Template, error)
	Update(ctx context.Context, id string, input UpdateInput) (*Template, error)
	Delete(ctx context.Context, id string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context) ([]*Template, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, name, description, plan_id::text, config, is_system, created_at
		FROM subscription_templates ORDER BY is_system DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*Template, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id::text, name, description, plan_id::text, config, is_system, created_at
		FROM subscription_templates WHERE id = $1::uuid
	`, id)
	return scanTemplateRow(row)
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (*Template, error) {
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO subscription_templates (name, description, plan_id, config)
		VALUES ($1, $2, $3::uuid, $4)
		RETURNING id::text, name, description, plan_id::text, config, is_system, created_at
	`, input.Name, nullableString(input.Description), nullableString(input.PlanID), cfg)
	return scanTemplateRow(row)
}

func (r *PostgresRepository) Update(ctx context.Context, id string, input UpdateInput) (*Template, error) {
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = nil
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE subscription_templates SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			plan_id = COALESCE($4::uuid, plan_id),
			config = COALESCE($5, config)
		WHERE id = $1::uuid AND is_system = false
		RETURNING id::text, name, description, plan_id::text, config, is_system, created_at
	`, id, nullableString(input.Name), nullableString(input.Description), nullableString(input.PlanID), nullableJSON(cfg))
	return scanTemplateRow(row)
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM subscription_templates WHERE id = $1::uuid AND is_system = false`, id)
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

func scanTemplate(s scanner) (*Template, error) {
	var t Template
	var desc, planID sql.NullString
	var cfg []byte
	err := s.Scan(&t.ID, &t.Name, &desc, &planID, &cfg, &t.IsSystem, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if desc.Valid {
		t.Description = &desc.String
	}
	if planID.Valid {
		t.PlanID = &planID.String
	}
	t.Config = json.RawMessage(cfg)
	return &t, nil
}

func scanTemplateRow(row *sql.Row) (*Template, error) {
	var t Template
	var desc, planID sql.NullString
	var cfg []byte
	err := row.Scan(&t.ID, &t.Name, &desc, &planID, &cfg, &t.IsSystem, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if desc.Valid {
		t.Description = &desc.String
	}
	if planID.Valid {
		t.PlanID = &planID.String
	}
	t.Config = json.RawMessage(cfg)
	return &t, nil
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
