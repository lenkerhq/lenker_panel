package warp

import (
	"context"
	"database/sql"
	"errors"
)

type Repository interface {
	Get(ctx context.Context, nodeID string) (*Credentials, error)
	Upsert(ctx context.Context, input SetInput) (*Credentials, error)
	Delete(ctx context.Context, nodeID string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Get(ctx context.Context, nodeID string) (*Credentials, error) {
	var c Credentials
	err := r.db.QueryRowContext(ctx, `
		SELECT node_id::text, private_key, public_key, address, endpoint, enabled, created_at
		FROM node_warp_credentials WHERE node_id = $1::uuid
	`, nodeID).Scan(&c.NodeID, &c.PrivateKey, &c.PublicKey, &c.Address, &c.Endpoint, &c.Enabled, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepository) Upsert(ctx context.Context, input SetInput) (*Credentials, error) {
	endpoint := input.Endpoint
	if endpoint == "" {
		endpoint = "engage.cloudflareclient.com:2408"
	}
	var c Credentials
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO node_warp_credentials (node_id, private_key, public_key, address, endpoint, enabled)
		VALUES ($1::uuid, $2, $3, $4, $5, true)
		ON CONFLICT (node_id) DO UPDATE SET
			private_key = EXCLUDED.private_key,
			public_key = EXCLUDED.public_key,
			address = EXCLUDED.address,
			endpoint = EXCLUDED.endpoint,
			enabled = true
		RETURNING node_id::text, private_key, public_key, address, endpoint, enabled, created_at
	`, input.NodeID, input.PrivateKey, input.PublicKey, input.Address, endpoint).Scan(
		&c.NodeID, &c.PrivateKey, &c.PublicKey, &c.Address, &c.Endpoint, &c.Enabled, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, nodeID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM node_warp_credentials WHERE node_id = $1::uuid`, nodeID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
