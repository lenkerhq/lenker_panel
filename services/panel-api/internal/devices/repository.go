package devices

import (
	"context"
	"database/sql"
	"time"
)

type Repository interface {
	Create(ctx context.Context, d Device) (*Device, error)
	Update(ctx context.Context, id string, d Device) (*Device, error)
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*Device, error)
	FindByFingerprint(ctx context.Context, subscriptionID, fingerprint string) (*Device, error)
	ListBySubscription(ctx context.Context, subscriptionID string) ([]*Device, error)
	CountActiveBySubscription(ctx context.Context, subscriptionID string) (int, error)
	MarkInactive(ctx context.Context, id string) error
	UpdateLastSeen(ctx context.Context, id string, ip string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, d Device) (*Device, error) {
	var dev Device
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO devices (subscription_id, device_fingerprint, device_name, platform, app_version, last_ip)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, subscription_id, device_fingerprint, device_name, platform, app_version,
		          first_seen_at, last_seen_at, last_ip::TEXT, is_active, created_at, updated_at`,
		d.SubscriptionID, d.DeviceFingerprint, d.DeviceName, d.Platform, d.AppVersion, nilIfEmpty(d.LastIP),
	).Scan(&dev.ID, &dev.SubscriptionID, &dev.DeviceFingerprint, &dev.DeviceName, &dev.Platform,
		&dev.AppVersion, &dev.FirstSeenAt, &dev.LastSeenAt, &dev.LastIP, &dev.IsActive, &dev.CreatedAt, &dev.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &dev, nil
}

func (r *PostgresRepository) Update(ctx context.Context, id string, d Device) (*Device, error) {
	var dev Device
	err := r.db.QueryRowContext(ctx, `
		UPDATE devices SET device_name = $2, platform = $3, app_version = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, subscription_id, device_fingerprint, device_name, platform, app_version,
		          first_seen_at, last_seen_at, last_ip::TEXT, is_active, created_at, updated_at`,
		id, d.DeviceName, d.Platform, d.AppVersion,
	).Scan(&dev.ID, &dev.SubscriptionID, &dev.DeviceFingerprint, &dev.DeviceName, &dev.Platform,
		&dev.AppVersion, &dev.FirstSeenAt, &dev.LastSeenAt, &dev.LastIP, &dev.IsActive, &dev.CreatedAt, &dev.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &dev, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*Device, error) {
	var dev Device
	err := r.db.QueryRowContext(ctx, `
		SELECT id, subscription_id, device_fingerprint, device_name, platform, app_version,
		       first_seen_at, last_seen_at, last_ip::TEXT, is_active, created_at, updated_at
		FROM devices WHERE id = $1`, id,
	).Scan(&dev.ID, &dev.SubscriptionID, &dev.DeviceFingerprint, &dev.DeviceName, &dev.Platform,
		&dev.AppVersion, &dev.FirstSeenAt, &dev.LastSeenAt, &dev.LastIP, &dev.IsActive, &dev.CreatedAt, &dev.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &dev, nil
}

func (r *PostgresRepository) FindByFingerprint(ctx context.Context, subscriptionID, fingerprint string) (*Device, error) {
	var dev Device
	err := r.db.QueryRowContext(ctx, `
		SELECT id, subscription_id, device_fingerprint, device_name, platform, app_version,
		       first_seen_at, last_seen_at, last_ip::TEXT, is_active, created_at, updated_at
		FROM devices WHERE subscription_id = $1 AND device_fingerprint = $2`, subscriptionID, fingerprint,
	).Scan(&dev.ID, &dev.SubscriptionID, &dev.DeviceFingerprint, &dev.DeviceName, &dev.Platform,
		&dev.AppVersion, &dev.FirstSeenAt, &dev.LastSeenAt, &dev.LastIP, &dev.IsActive, &dev.CreatedAt, &dev.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &dev, nil
}

func (r *PostgresRepository) ListBySubscription(ctx context.Context, subscriptionID string) ([]*Device, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, subscription_id, device_fingerprint, device_name, platform, app_version,
		       first_seen_at, last_seen_at, last_ip::TEXT, is_active, created_at, updated_at
		FROM devices WHERE subscription_id = $1 ORDER BY last_seen_at DESC`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*Device
	for rows.Next() {
		var dev Device
		if err := rows.Scan(&dev.ID, &dev.SubscriptionID, &dev.DeviceFingerprint, &dev.DeviceName, &dev.Platform,
			&dev.AppVersion, &dev.FirstSeenAt, &dev.LastSeenAt, &dev.LastIP, &dev.IsActive, &dev.CreatedAt, &dev.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, &dev)
	}
	return devices, rows.Err()
}

func (r *PostgresRepository) CountActiveBySubscription(ctx context.Context, subscriptionID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE subscription_id = $1 AND is_active = true`, subscriptionID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) MarkInactive(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE devices SET is_active = false, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateLastSeen(ctx context.Context, id string, ip string) error {
	now := time.Now()
	var ipArg any
	if ip != "" {
		ipArg = ip
	}
	res, err := r.db.ExecContext(ctx, `UPDATE devices SET last_seen_at = $2, last_ip = $3, updated_at = $2 WHERE id = $1`, id, now, ipArg)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func nilIfEmpty(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
