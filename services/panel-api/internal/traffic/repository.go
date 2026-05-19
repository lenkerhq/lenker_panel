package traffic

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

type Repository interface {
	CreateLog(ctx context.Context, log TrafficLog) (*TrafficLog, error)
	GetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error)
	SetQuota(ctx context.Context, input SetQuotaInput, bytesUsed int64) (*TrafficQuota, error)
	UpdateQuota(ctx context.Context, subscriptionID string, bytesUsed int64) error
	IncrementQuota(ctx context.Context, subscriptionID string, bytesDelta int64) (*TrafficQuota, error)
	ResetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error)
	GetUsageBySubscription(ctx context.Context, subscriptionID string, from, to time.Time) (*TrafficUsage, error)
	GetUsageByDevice(ctx context.Context, deviceID string, from, to time.Time) (*TrafficUsage, error)
	GetUsageByNode(ctx context.Context, nodeID string, from, to time.Time) (*TrafficUsage, error)
}

type PostgresRepository struct {
	db queryer
}

type NodeResolver interface {
	NodeIDByToken(ctx context.Context, nodeToken string) (string, error)
}

type PostgresNodeResolver struct {
	db *sql.DB
}

func NewPostgresNodeResolver(db *sql.DB) *PostgresNodeResolver {
	return &PostgresNodeResolver{db: db}
}

func (r *PostgresNodeResolver) NodeIDByToken(ctx context.Context, nodeToken string) (string, error) {
	var nodeID string
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text
		FROM nodes
		WHERE auth_token_hash = $1
		  AND registered_at IS NOT NULL
		  AND status != 'disabled'
		LIMIT 1
	`, storage.HashNodeToken(nodeToken)).Scan(&nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return nodeID, nil
}

type queryer interface {
	queryRow(ctx context.Context, query string, args ...any) scanner
	exec(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type scanner interface {
	Scan(dest ...any) error
}

type sqlQueryer struct {
	db *sql.DB
}

func (q sqlQueryer) queryRow(ctx context.Context, query string, args ...any) scanner {
	return q.db.QueryRowContext(ctx, query, args...)
}

func (q sqlQueryer) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return q.db.ExecContext(ctx, query, args...)
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: sqlQueryer{db: db}}
}

func newRepositoryWithQueryer(db queryer) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateLog(ctx context.Context, log TrafficLog) (*TrafficLog, error) {
	row := r.db.queryRow(ctx, `
		INSERT INTO traffic_logs (subscription_id, device_id, node_id, bytes_up, bytes_down, recorded_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, COALESCE($6::timestamptz, now()))
		RETURNING id::text, subscription_id::text, device_id::text, node_id::text,
		          bytes_up, bytes_down, recorded_at, created_at
	`, log.SubscriptionID, optionalString(log.DeviceID), log.NodeID, log.BytesUp, log.BytesDown, optionalTime(log.RecordedAt))
	return scanTrafficLog(row)
}

func (r *PostgresRepository) GetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error) {
	row := r.db.queryRow(ctx, `
		SELECT
		    COALESCE(tq.id::text, '') AS id,
		    s.id::text AS subscription_id,
		    COALESCE(tq.bytes_limit, s.traffic_limit_bytes) AS bytes_limit,
		    COALESCE(tq.bytes_used, s.traffic_used_bytes) AS bytes_used,
		    tq.reset_at,
		    COALESCE(tq.created_at, s.created_at) AS created_at,
		    COALESCE(tq.updated_at, s.updated_at) AS updated_at
		FROM subscriptions s
		LEFT JOIN traffic_quotas tq ON tq.subscription_id = s.id
		WHERE s.id = $1::uuid
	`, subscriptionID)
	return scanTrafficQuota(row)
}

func (r *PostgresRepository) SetQuota(ctx context.Context, input SetQuotaInput, bytesUsed int64) (*TrafficQuota, error) {
	row := r.db.queryRow(ctx, `
		INSERT INTO traffic_quotas (subscription_id, bytes_limit, bytes_used, reset_at)
		SELECT s.id, $2::bigint, $3::bigint, $4::timestamptz
		FROM subscriptions s
		WHERE s.id = $1::uuid
		ON CONFLICT (subscription_id) DO UPDATE
		SET bytes_limit = EXCLUDED.bytes_limit,
		    bytes_used = EXCLUDED.bytes_used,
		    reset_at = EXCLUDED.reset_at,
		    updated_at = now()
		RETURNING id::text, subscription_id::text, bytes_limit, bytes_used, reset_at, created_at, updated_at
	`, input.SubscriptionID, optionalInt64(input.BytesLimit), bytesUsed, optionalTimePtr(input.ResetAt))
	quota, err := scanTrafficQuota(row)
	if err != nil {
		return nil, err
	}
	if err := r.syncSubscriptionQuota(ctx, input.SubscriptionID, input.BytesLimit, bytesUsed); err != nil {
		return nil, err
	}
	return quota, nil
}

func (r *PostgresRepository) UpdateQuota(ctx context.Context, subscriptionID string, bytesUsed int64) error {
	quota, err := scanTrafficQuota(r.db.queryRow(ctx, `
		UPDATE traffic_quotas
		SET bytes_used = $2::bigint,
		    updated_at = now()
		WHERE subscription_id = $1::uuid
		RETURNING id::text, subscription_id::text, bytes_limit, bytes_used, reset_at, created_at, updated_at
	`, subscriptionID, bytesUsed))
	if err != nil {
		return err
	}
	return r.syncSubscriptionQuota(ctx, subscriptionID, quota.BytesLimit, quota.BytesUsed)
}

func (r *PostgresRepository) IncrementQuota(ctx context.Context, subscriptionID string, bytesDelta int64) (*TrafficQuota, error) {
	row := r.db.queryRow(ctx, `
		INSERT INTO traffic_quotas (subscription_id, bytes_limit, bytes_used)
		SELECT s.id, s.traffic_limit_bytes, $2::bigint
		FROM subscriptions s
		WHERE s.id = $1::uuid
		ON CONFLICT (subscription_id) DO UPDATE
		SET bytes_used = traffic_quotas.bytes_used + EXCLUDED.bytes_used,
		    updated_at = now()
		RETURNING id::text, subscription_id::text, bytes_limit, bytes_used, reset_at, created_at, updated_at
	`, subscriptionID, bytesDelta)
	quota, err := scanTrafficQuota(row)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.exec(ctx, `
		UPDATE subscriptions
		SET traffic_used_bytes = traffic_used_bytes + $2::bigint,
		    updated_at = now()
		WHERE id = $1::uuid
	`, subscriptionID, bytesDelta); err != nil {
		return nil, err
	}
	return quota, nil
}

func (r *PostgresRepository) ResetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error) {
	row := r.db.queryRow(ctx, `
		INSERT INTO traffic_quotas (subscription_id, bytes_limit, bytes_used, reset_at)
		SELECT s.id, s.traffic_limit_bytes, 0, NULL
		FROM subscriptions s
		WHERE s.id = $1::uuid
		ON CONFLICT (subscription_id) DO UPDATE
		SET bytes_used = 0,
		    reset_at = NULL,
		    updated_at = now()
		RETURNING id::text, subscription_id::text, bytes_limit, bytes_used, reset_at, created_at, updated_at
	`, subscriptionID)
	quota, err := scanTrafficQuota(row)
	if err != nil {
		return nil, err
	}
	if err := r.syncSubscriptionQuota(ctx, subscriptionID, quota.BytesLimit, 0); err != nil {
		return nil, err
	}
	return quota, nil
}

func (r *PostgresRepository) GetUsageBySubscription(ctx context.Context, subscriptionID string, from, to time.Time) (*TrafficUsage, error) {
	return r.getUsage(ctx, "subscription", subscriptionID, `
		SELECT COALESCE(SUM(bytes_up), 0), COALESCE(SUM(bytes_down), 0)
		FROM traffic_logs
		WHERE subscription_id = $1::uuid
		  AND ($2::timestamptz IS NULL OR recorded_at >= $2::timestamptz)
		  AND ($3::timestamptz IS NULL OR recorded_at <= $3::timestamptz)
	`, from, to)
}

func (r *PostgresRepository) GetUsageByDevice(ctx context.Context, deviceID string, from, to time.Time) (*TrafficUsage, error) {
	return r.getUsage(ctx, "device", deviceID, `
		SELECT COALESCE(SUM(bytes_up), 0), COALESCE(SUM(bytes_down), 0)
		FROM traffic_logs
		WHERE device_id = $1::uuid
		  AND ($2::timestamptz IS NULL OR recorded_at >= $2::timestamptz)
		  AND ($3::timestamptz IS NULL OR recorded_at <= $3::timestamptz)
	`, from, to)
}

func (r *PostgresRepository) GetUsageByNode(ctx context.Context, nodeID string, from, to time.Time) (*TrafficUsage, error) {
	return r.getUsage(ctx, "node", nodeID, `
		SELECT COALESCE(SUM(bytes_up), 0), COALESCE(SUM(bytes_down), 0)
		FROM traffic_logs
		WHERE node_id = $1::uuid
		  AND ($2::timestamptz IS NULL OR recorded_at >= $2::timestamptz)
		  AND ($3::timestamptz IS NULL OR recorded_at <= $3::timestamptz)
	`, from, to)
}

func (r *PostgresRepository) getUsage(ctx context.Context, resourceType, resourceID, query string, from, to time.Time) (*TrafficUsage, error) {
	usage := TrafficUsage{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		From:         timePtrIfSet(from),
		To:           timePtrIfSet(to),
	}
	row := r.db.queryRow(ctx, query, resourceID, optionalTime(from), optionalTime(to))
	if err := row.Scan(&usage.BytesUp, &usage.BytesDown); err != nil {
		return nil, err
	}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (r *PostgresRepository) syncSubscriptionQuota(ctx context.Context, subscriptionID string, bytesLimit *int64, bytesUsed int64) error {
	result, err := r.db.exec(ctx, `
		UPDATE subscriptions
		SET traffic_limit_bytes = $2::bigint,
		    traffic_used_bytes = $3::bigint,
		    updated_at = now()
		WHERE id = $1::uuid
	`, subscriptionID, optionalInt64(bytesLimit), bytesUsed)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanTrafficLog(row scanner) (*TrafficLog, error) {
	var log TrafficLog
	var deviceID sql.NullString
	err := row.Scan(
		&log.ID,
		&log.SubscriptionID,
		&deviceID,
		&log.NodeID,
		&log.BytesUp,
		&log.BytesDown,
		&log.RecordedAt,
		&log.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if deviceID.Valid {
		log.DeviceID = &deviceID.String
	}
	log = log.WithDerivedFields()
	return &log, nil
}

func scanTrafficQuota(row scanner) (*TrafficQuota, error) {
	var quota TrafficQuota
	var bytesLimit sql.NullInt64
	var resetAt sql.NullTime
	err := row.Scan(
		&quota.ID,
		&quota.SubscriptionID,
		&bytesLimit,
		&quota.BytesUsed,
		&resetAt,
		&quota.CreatedAt,
		&quota.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if bytesLimit.Valid {
		quota.BytesLimit = &bytesLimit.Int64
	}
	if resetAt.Valid {
		quota.ResetAt = &resetAt.Time
	}
	quota = quota.WithDerivedFields()
	return &quota, nil
}

func optionalString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func optionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func optionalTimePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return *value
}

func timePtrIfSet(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
