package routing

import (
	"context"
	"database/sql"
	"errors"
)

type Repository interface {
	List(ctx context.Context, nodeID *string) ([]*Rule, error)
	FindByID(ctx context.Context, id string) (*Rule, error)
	Create(ctx context.Context, input CreateInput) (*Rule, error)
	Update(ctx context.Context, id string, input UpdateInput) (*Rule, error)
	Delete(ctx context.Context, id string) error
	Reorder(ctx context.Context, entries []ReorderEntry) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context, nodeID *string) ([]*Rule, error) {
	var rows *sql.Rows
	var err error
	if nodeID == nil {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id::text, node_id::text, rule_type, target, action, outbound_tag, priority, enabled, description, created_at, updated_at
			FROM routing_rules WHERE node_id IS NULL ORDER BY priority, created_at
		`)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id::text, node_id::text, rule_type, target, action, outbound_tag, priority, enabled, description, created_at, updated_at
			FROM routing_rules WHERE node_id IS NULL OR node_id = $1::uuid ORDER BY priority, created_at
		`, *nodeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*Rule, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id::text, node_id::text, rule_type, target, action, outbound_tag, priority, enabled, description, created_at, updated_at
		FROM routing_rules WHERE id = $1::uuid
	`, id)
	return scanRuleRow(row)
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (*Rule, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO routing_rules (node_id, rule_type, target, action, outbound_tag, priority, enabled, description)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, node_id::text, rule_type, target, action, outbound_tag, priority, enabled, description, created_at, updated_at
	`, nullableString(input.NodeID), input.RuleType, input.Target, input.Action, nullableString(input.OutboundTag), input.Priority, enabled, nullableString(input.Description))
	return scanRuleRow(row)
}

func (r *PostgresRepository) Update(ctx context.Context, id string, input UpdateInput) (*Rule, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE routing_rules SET
			rule_type = COALESCE($2, rule_type),
			target = COALESCE($3, target),
			action = COALESCE($4, action),
			outbound_tag = CASE WHEN $5::text IS NOT NULL THEN $5 ELSE outbound_tag END,
			priority = COALESCE($6, priority),
			enabled = COALESCE($7, enabled),
			description = CASE WHEN $8::text IS NOT NULL THEN $8 ELSE description END,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, node_id::text, rule_type, target, action, outbound_tag, priority, enabled, description, created_at, updated_at
	`, id, nullableString(input.RuleType), nullableString(input.Target), nullableString(input.Action), nullableString(input.OutboundTag), nullableInt(input.Priority), nullableBool(input.Enabled), nullableString(input.Description))
	return scanRuleRow(row)
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Reorder(ctx context.Context, entries []ReorderEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, entry := range entries {
		_, err := tx.ExecContext(ctx, `UPDATE routing_rules SET priority = $2, updated_at = now() WHERE id = $1::uuid`, entry.ID, entry.Priority)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRule(s scanner) (*Rule, error) {
	var rule Rule
	var nodeID, outboundTag, description sql.NullString
	err := s.Scan(&rule.ID, &nodeID, &rule.RuleType, &rule.Target, &rule.Action, &outboundTag, &rule.Priority, &rule.Enabled, &description, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if nodeID.Valid {
		rule.NodeID = &nodeID.String
	}
	if outboundTag.Valid {
		rule.OutboundTag = &outboundTag.String
	}
	if description.Valid {
		rule.Description = &description.String
	}
	return &rule, nil
}

func scanRuleRow(row *sql.Row) (*Rule, error) {
	var rule Rule
	var nodeID, outboundTag, description sql.NullString
	err := row.Scan(&rule.ID, &nodeID, &rule.RuleType, &rule.Target, &rule.Action, &outboundTag, &rule.Priority, &rule.Enabled, &description, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if nodeID.Valid {
		rule.NodeID = &nodeID.String
	}
	if outboundTag.Valid {
		rule.OutboundTag = &outboundTag.String
	}
	if description.Valid {
		rule.Description = &description.String
	}
	return &rule, nil
}

func nullableString(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullableInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func nullableBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}
