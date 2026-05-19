package settings

import (
	"context"
	"database/sql"
	"encoding/json"
)

type Repository interface {
	List(ctx context.Context) ([]*Setting, error)
	Get(ctx context.Context, key string) (*Setting, error)
	Set(ctx context.Context, key string, value json.RawMessage, adminID string) (*Setting, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context) ([]*Setting, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT key, value, description, updated_by::text, updated_at
		FROM global_settings ORDER BY key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, key string) (*Setting, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT key, value, description, updated_by::text, updated_at
		FROM global_settings WHERE key = $1
	`, key)
	return scanSettingRow(row)
}

func (r *PostgresRepository) Set(ctx context.Context, key string, value json.RawMessage, adminID string) (*Setting, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO global_settings (key, value, updated_by)
		VALUES ($1, $2, $3::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING key, value, description, updated_by::text, updated_at
	`, key, value, adminID)
	return scanSettingRow(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSetting(s scanner) (*Setting, error) {
	var setting Setting
	var desc, updatedBy sql.NullString
	err := s.Scan(&setting.Key, &setting.Value, &desc, &updatedBy, &setting.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		setting.Description = &desc.String
	}
	if updatedBy.Valid {
		setting.UpdatedBy = &updatedBy.String
	}
	return &setting, nil
}

func scanSettingRow(row *sql.Row) (*Setting, error) {
	var setting Setting
	var desc, updatedBy sql.NullString
	err := row.Scan(&setting.Key, &setting.Value, &desc, &updatedBy, &setting.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUnknownKey
		}
		return nil, err
	}
	if desc.Valid {
		setting.Description = &desc.String
	}
	if updatedBy.Valid {
		setting.UpdatedBy = &updatedBy.String
	}
	return &setting, nil
}
