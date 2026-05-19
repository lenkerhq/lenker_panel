package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/configrender"
)

type Subscription struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	PlanID            string    `json:"plan_id"`
	Status            string    `json:"status"`
	StartsAt          time.Time `json:"starts_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	TrafficLimitBytes *int64    `json:"traffic_limit_bytes"`
	TrafficUsedBytes  int64     `json:"traffic_used_bytes"`
	DeviceLimit       int       `json:"device_limit"`
	PreferredRegion   *string   `json:"preferred_region"`
}

type SubscriptionAccess struct {
	ExportKind     string                     `json:"export_kind"`
	SubscriptionID string                     `json:"subscription_id"`
	UserID         string                     `json:"user_id"`
	UserLabel      string                     `json:"user_label"`
	PlanID         string                     `json:"plan_id"`
	PlanName       string                     `json:"plan_name"`
	Status         string                     `json:"status"`
	Protocol       string                     `json:"protocol"`
	ProtocolPath   string                     `json:"protocol_path"`
	Node           SubscriptionAccessNode     `json:"node"`
	Endpoint       SubscriptionAccessEndpoint `json:"endpoint"`
	Client         SubscriptionAccessClient   `json:"client"`
	DisplayName    string                     `json:"display_name"`
	URI            string                     `json:"uri"`
}

type SubscriptionAccessToken struct {
	SubscriptionID string    `json:"subscription_id"`
	Token          string    `json:"access_token,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type SubscriptionAccessTokenStatus struct {
	SubscriptionID string     `json:"subscription_id"`
	Status         string     `json:"status"`
	Issued         bool       `json:"issued"`
	IssuedAt       *time.Time `json:"issued_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	Generation     int        `json:"generation"`
}

type SubscriptionHandoffInvite struct {
	SubscriptionID string    `json:"subscription_id"`
	HandoffToken   string    `json:"handoff_token,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type SubscriptionHandoffInviteStatus struct {
	SubscriptionID string     `json:"subscription_id"`
	Status         string     `json:"status"`
	Issued         bool       `json:"issued"`
	IssuedAt       *time.Time `json:"issued_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	Generation     int        `json:"generation"`
}

type SubscriptionHandoffClaim struct {
	SubscriptionID       string             `json:"subscription_id"`
	AccessToken          string             `json:"access_token"`
	AccessTokenExpiresAt time.Time          `json:"access_token_expires_at"`
	ClaimedAt            time.Time          `json:"claimed_at"`
	Access               SubscriptionAccess `json:"access"`
}

type SubscriptionAccessNode struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Region         string `json:"region"`
	CountryCode    string `json:"country_code"`
	Hostname       string `json:"hostname"`
	Status         string `json:"status"`
	DrainState     string `json:"drain_state"`
	ActiveRevision int    `json:"active_revision"`
}

type SubscriptionAccessEndpoint struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Network     string `json:"network"`
	Security    string `json:"security"`
	SNI         string `json:"sni"`
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id"`
	Fingerprint string `json:"fingerprint"`
	SpiderX     string `json:"spider_x"`
}

type SubscriptionAccessClient struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Flow   string `json:"flow"`
	Level  int    `json:"level"`
	PlanID string `json:"plan_id"`
}

type CreateSubscriptionInput struct {
	UserID          string
	PlanID          string
	PreferredRegion *string
}

type UpdateSubscriptionInput struct {
	Status               *string
	TrafficLimitBytes    *int64
	ClearTrafficLimit    bool
	DeviceLimit          *int
	PreferredRegion      *string
	ClearPreferredRegion bool
}

type SubscriptionsRepository interface {
	List(ctx context.Context) ([]Subscription, error)
	Create(ctx context.Context, input CreateSubscriptionInput) (Subscription, error)
	FindByID(ctx context.Context, id string) (Subscription, error)
	Access(ctx context.Context, id string) (SubscriptionAccess, error)
	AccessTokenStatus(ctx context.Context, id string) (SubscriptionAccessTokenStatus, error)
	CreateAccessToken(ctx context.Context, id string) (SubscriptionAccessToken, error)
	RotateAccessToken(ctx context.Context, id string) (SubscriptionAccessToken, error)
	RevokeAccessToken(ctx context.Context, id string) (SubscriptionAccessTokenStatus, error)
	AccessByToken(ctx context.Context, token string) (SubscriptionAccess, error)
	CreateHandoffInvite(ctx context.Context, id string) (SubscriptionHandoffInvite, error)
	HandoffInviteStatus(ctx context.Context, id string) (SubscriptionHandoffInviteStatus, error)
	RevokeHandoffInvite(ctx context.Context, id string) (SubscriptionHandoffInviteStatus, error)
	ClaimHandoffInvite(ctx context.Context, token string) (SubscriptionHandoffClaim, error)
	Update(ctx context.Context, id string, input UpdateSubscriptionInput) (Subscription, error)
	Renew(ctx context.Context, id string, extendDays int) (Subscription, error)
	SubscriptionIDByToken(ctx context.Context, token string) (string, error)
}

var (
	ErrSubscriptionAccessUnavailable   = errors.New("subscription access unavailable")
	ErrInvalidSubscriptionAccessToken  = errors.New("invalid subscription access token")
	ErrInvalidSubscriptionHandoffToken = errors.New("invalid subscription handoff token")
)

type subscriptionsRepository struct {
	db      *sql.DB
	reality configrender.RealityConfig
}

const createSubscriptionSQL = `
	INSERT INTO subscriptions (
		user_id,
		plan_id,
		status,
		starts_at,
		expires_at,
		traffic_limit_bytes,
		device_limit,
		preferred_region
	)
	SELECT u.id, p.id, 'active', $3::timestamptz, $3::timestamptz + (p.duration_days * INTERVAL '1 day'),
	       p.traffic_limit_bytes, p.device_limit, $4
	FROM users u
	JOIN plans p ON p.id = $2
	            AND p.status = 'active'
	WHERE u.id = $1
	RETURNING id::text, user_id::text, plan_id::text, status, starts_at, expires_at,
	          traffic_limit_bytes, traffic_used_bytes, device_limit, preferred_region
`

func NewSubscriptionsRepository(db *sql.DB) SubscriptionsRepository {
	return NewSubscriptionsRepositoryWithReality(db, configrender.DefaultRealityConfig())
}

func NewSubscriptionsRepositoryWithReality(db *sql.DB, reality configrender.RealityConfig) SubscriptionsRepository {
	return &subscriptionsRepository{db: db, reality: reality.WithDefaults()}
}

func (r *subscriptionsRepository) List(ctx context.Context) ([]Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, user_id::text, plan_id::text, status, starts_at, expires_at,
		       traffic_limit_bytes, traffic_used_bytes, device_limit, preferred_region
		FROM subscriptions
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Subscription
	for rows.Next() {
		var subscription Subscription
		if err := rows.Scan(
			&subscription.ID,
			&subscription.UserID,
			&subscription.PlanID,
			&subscription.Status,
			&subscription.StartsAt,
			&subscription.ExpiresAt,
			&subscription.TrafficLimitBytes,
			&subscription.TrafficUsedBytes,
			&subscription.DeviceLimit,
			&subscription.PreferredRegion,
		); err != nil {
			return nil, err
		}
		result = append(result, subscription)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *subscriptionsRepository) Create(ctx context.Context, input CreateSubscriptionInput) (Subscription, error) {
	var subscription Subscription
	now := time.Now().UTC()
	err := r.db.QueryRowContext(ctx, createSubscriptionSQL, input.UserID, input.PlanID, now, input.PreferredRegion).Scan(
		&subscription.ID,
		&subscription.UserID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.StartsAt,
		&subscription.ExpiresAt,
		&subscription.TrafficLimitBytes,
		&subscription.TrafficUsedBytes,
		&subscription.DeviceLimit,
		&subscription.PreferredRegion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *subscriptionsRepository) FindByID(ctx context.Context, id string) (Subscription, error) {
	var subscription Subscription
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, user_id::text, plan_id::text, status, starts_at, expires_at,
		       traffic_limit_bytes, traffic_used_bytes, device_limit, preferred_region
		FROM subscriptions
		WHERE id = $1
	`, id).Scan(
		&subscription.ID,
		&subscription.UserID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.StartsAt,
		&subscription.ExpiresAt,
		&subscription.TrafficLimitBytes,
		&subscription.TrafficUsedBytes,
		&subscription.DeviceLimit,
		&subscription.PreferredRegion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *subscriptionsRepository) Access(ctx context.Context, id string) (SubscriptionAccess, error) {
	var row struct {
		subscription Subscription
		userEmail    string
		displayName  string
		userStatus   string
		planName     string
		nodeID       sql.NullString
		nodeName     sql.NullString
		nodeRegion   sql.NullString
		countryCode  sql.NullString
		hostname     sql.NullString
		nodeStatus   sql.NullString
		drainState   sql.NullString
		activeRev    sql.NullInt64
	}
	err := r.db.QueryRowContext(ctx, `
		SELECT s.id::text,
		       s.user_id::text,
		       s.plan_id::text,
		       s.status,
		       s.starts_at,
		       s.expires_at,
		       s.traffic_limit_bytes,
		       s.traffic_used_bytes,
		       s.device_limit,
		       s.preferred_region,
		       u.email,
		       u.display_name,
		       u.status,
		       p.name,
		       n.id::text,
		       n.name,
		       n.region,
		       n.country_code,
		       n.hostname,
		       n.status,
		       n.drain_state,
		       n.active_revision
		FROM subscriptions s
		JOIN users u ON u.id = s.user_id
		JOIN plans p ON p.id = s.plan_id
		LEFT JOIN LATERAL (
			SELECT id, name, region, country_code, hostname, status, drain_state, active_revision
			FROM nodes
			WHERE status = 'active'
			  AND drain_state = 'active'
			  AND hostname <> ''
			  AND (s.preferred_region IS NULL OR s.preferred_region = '' OR region = s.preferred_region)
			ORDER BY CASE WHEN s.preferred_region IS NOT NULL AND s.preferred_region <> '' AND region = s.preferred_region THEN 0 ELSE 1 END,
			         region ASC,
			         name ASC,
			         id::text ASC
			LIMIT 1
		) n ON true
		WHERE s.id = $1
	`, id).Scan(
		&row.subscription.ID,
		&row.subscription.UserID,
		&row.subscription.PlanID,
		&row.subscription.Status,
		&row.subscription.StartsAt,
		&row.subscription.ExpiresAt,
		&row.subscription.TrafficLimitBytes,
		&row.subscription.TrafficUsedBytes,
		&row.subscription.DeviceLimit,
		&row.subscription.PreferredRegion,
		&row.userEmail,
		&row.displayName,
		&row.userStatus,
		&row.planName,
		&row.nodeID,
		&row.nodeName,
		&row.nodeRegion,
		&row.countryCode,
		&row.hostname,
		&row.nodeStatus,
		&row.drainState,
		&row.activeRev,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionAccess{}, ErrNotFound
		}
		return SubscriptionAccess{}, err
	}
	if row.subscription.Status != "active" || row.userStatus != "active" || !row.subscription.ExpiresAt.After(time.Now().UTC()) {
		return SubscriptionAccess{}, ErrSubscriptionAccessUnavailable
	}
	if !row.nodeID.Valid || !row.hostname.Valid {
		return SubscriptionAccess{}, ErrSubscriptionAccessUnavailable
	}

	preferredRegion := ""
	if row.subscription.PreferredRegion != nil {
		preferredRegion = *row.subscription.PreferredRegion
	}
	accessEntry := configrender.BuildAccessEntry(configrender.SubscriptionInput{
		SubscriptionID:     row.subscription.ID,
		UserID:             row.subscription.UserID,
		PlanID:             row.subscription.PlanID,
		UserStatus:         row.userStatus,
		SubscriptionStatus: row.subscription.Status,
		PreferredRegion:    preferredRegion,
		PlanName:           row.planName,
		DeviceLimit:        row.subscription.DeviceLimit,
		TrafficLimitBytes:  row.subscription.TrafficLimitBytes,
		StartsAt:           row.subscription.StartsAt.UTC().Format(time.RFC3339),
		ExpiresAt:          row.subscription.ExpiresAt.UTC().Format(time.RFC3339),
	})
	node := SubscriptionAccessNode{
		ID:             row.nodeID.String,
		Name:           row.nodeName.String,
		Region:         row.nodeRegion.String,
		CountryCode:    row.countryCode.String,
		Hostname:       row.hostname.String,
		Status:         row.nodeStatus.String,
		DrainState:     row.drainState.String,
		ActiveRevision: int(row.activeRev.Int64),
	}
	reality := r.reality.WithDefaults()
	endpoint := SubscriptionAccessEndpoint{
		Address:     node.Hostname,
		Port:        reality.VLESSPort,
		Network:     "tcp",
		Security:    "reality",
		SNI:         reality.SNI,
		PublicKey:   reality.PublicKey,
		ShortID:     reality.ShortID,
		Fingerprint: reality.Fingerprint,
		SpiderX:     reality.SpiderX,
	}
	userLabel := row.userEmail
	if strings.TrimSpace(row.displayName) != "" {
		userLabel = row.displayName
	}
	displayName := fmt.Sprintf("Lenker %s %s", node.Region, row.planName)
	client := SubscriptionAccessClient{
		ID:     accessEntry.VLESSClientID,
		Email:  accessEntry.Email,
		Flow:   accessEntry.Flow,
		Level:  0,
		PlanID: accessEntry.PlanID,
	}
	return SubscriptionAccess{
		ExportKind:     "subscription_access.v1alpha1",
		SubscriptionID: row.subscription.ID,
		UserID:         row.subscription.UserID,
		UserLabel:      userLabel,
		PlanID:         row.subscription.PlanID,
		PlanName:       row.planName,
		Status:         row.subscription.Status,
		Protocol:       configrender.ProtocolVLESS,
		ProtocolPath:   configrender.ProtocolVLESS,
		Node:           node,
		Endpoint:       endpoint,
		Client:         client,
		DisplayName:    displayName,
		URI:            buildVLESSRealityURI(endpoint, client, displayName),
	}, nil
}

func (r *subscriptionsRepository) CreateAccessToken(ctx context.Context, id string) (SubscriptionAccessToken, error) {
	return r.replaceAccessToken(ctx, id)
}

func (r *subscriptionsRepository) AccessTokenStatus(ctx context.Context, id string) (SubscriptionAccessTokenStatus, error) {
	if _, err := r.FindByID(ctx, id); err != nil {
		return SubscriptionAccessTokenStatus{}, err
	}

	var row struct {
		status     sql.NullString
		createdAt  sql.NullTime
		updatedAt  sql.NullTime
		generation int
	}
	err := r.db.QueryRowContext(ctx, `
		WITH token_counts AS (
			SELECT COUNT(*)::int AS generation
			FROM subscription_access_tokens
			WHERE subscription_id = $1
		),
		current_token AS (
			SELECT status, created_at, updated_at
			FROM subscription_access_tokens
			WHERE subscription_id = $1
			ORDER BY CASE WHEN status = 'active' THEN 0 ELSE 1 END,
			         created_at DESC
			LIMIT 1
		)
		SELECT current_token.status,
		       current_token.created_at,
		       current_token.updated_at,
		       token_counts.generation
		FROM token_counts
		LEFT JOIN current_token ON true
	`, id).Scan(&row.status, &row.createdAt, &row.updatedAt, &row.generation)
	if err != nil {
		return SubscriptionAccessTokenStatus{}, err
	}

	status := SubscriptionAccessTokenStatus{
		SubscriptionID: id,
		Status:         "never_issued",
		Generation:     row.generation,
	}
	if !row.status.Valid {
		return status, nil
	}

	status.Issued = true
	status.Status = row.status.String
	if row.createdAt.Valid {
		issuedAt := row.createdAt.Time
		status.IssuedAt = &issuedAt
	}
	if row.status.String == "revoked" && row.updatedAt.Valid {
		revokedAt := row.updatedAt.Time
		status.RevokedAt = &revokedAt
	}
	return status, nil
}

func (r *subscriptionsRepository) RotateAccessToken(ctx context.Context, id string) (SubscriptionAccessToken, error) {
	return r.replaceAccessToken(ctx, id)
}

func (r *subscriptionsRepository) replaceAccessToken(ctx context.Context, id string) (SubscriptionAccessToken, error) {
	token, err := newSubscriptionAccessToken()
	if err != nil {
		return SubscriptionAccessToken{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SubscriptionAccessToken{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := ensureSubscriptionEligibleForAccessToken(ctx, tx, id); err != nil {
		return SubscriptionAccessToken{}, err
	}

	result, err := replaceAccessTokenInTx(ctx, tx, id, token)
	if err != nil {
		return SubscriptionAccessToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return SubscriptionAccessToken{}, err
	}
	result.Token = token
	return result, nil
}

func replaceAccessTokenInTx(ctx context.Context, tx *sql.Tx, id string, token string) (SubscriptionAccessToken, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE subscription_access_tokens
		SET status = 'revoked',
		    updated_at = now()
		WHERE subscription_id = $1
		  AND status = 'active'
	`, id); err != nil {
		return SubscriptionAccessToken{}, err
	}

	var result SubscriptionAccessToken
	err := tx.QueryRowContext(ctx, `
		INSERT INTO subscription_access_tokens (subscription_id, token_hash, expires_at, status)
		SELECT id, $2, expires_at, 'active'
		FROM subscriptions
		WHERE id = $1
		RETURNING subscription_id::text, expires_at, created_at
	`, id, HashSubscriptionAccessToken(token)).Scan(
		&result.SubscriptionID,
		&result.ExpiresAt,
		&result.CreatedAt,
	)
	if err != nil {
		return SubscriptionAccessToken{}, err
	}
	return result, nil
}

func (r *subscriptionsRepository) RevokeAccessToken(ctx context.Context, id string) (SubscriptionAccessTokenStatus, error) {
	if _, err := r.FindByID(ctx, id); err != nil {
		return SubscriptionAccessTokenStatus{}, err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE subscription_access_tokens
		SET status = 'revoked',
		    updated_at = now()
		WHERE subscription_id = $1
		  AND status = 'active'
	`, id); err != nil {
		return SubscriptionAccessTokenStatus{}, err
	}
	return r.AccessTokenStatus(ctx, id)
}

func (r *subscriptionsRepository) AccessByToken(ctx context.Context, token string) (SubscriptionAccess, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return SubscriptionAccess{}, ErrInvalidSubscriptionAccessToken
	}

	var subscriptionID string
	err := r.db.QueryRowContext(ctx, `
		SELECT subscription_id::text
		FROM subscription_access_tokens
		WHERE token_hash = $1
		  AND status = 'active'
		  AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
	`, HashSubscriptionAccessToken(token)).Scan(&subscriptionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionAccess{}, ErrInvalidSubscriptionAccessToken
		}
		return SubscriptionAccess{}, err
	}
	return r.Access(ctx, subscriptionID)
}

func (r *subscriptionsRepository) CreateHandoffInvite(ctx context.Context, id string) (SubscriptionHandoffInvite, error) {
	token, err := newSubscriptionHandoffToken()
	if err != nil {
		return SubscriptionHandoffInvite{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SubscriptionHandoffInvite{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := ensureSubscriptionEligibleForAccessToken(ctx, tx, id); err != nil {
		return SubscriptionHandoffInvite{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE subscription_handoff_invites
		SET status = 'revoked',
		    updated_at = now()
		WHERE subscription_id = $1
		  AND status = 'active'
	`, id); err != nil {
		return SubscriptionHandoffInvite{}, err
	}

	var invite SubscriptionHandoffInvite
	err = tx.QueryRowContext(ctx, `
		INSERT INTO subscription_handoff_invites (subscription_id, token_hash, expires_at, status)
		SELECT id, $2, LEAST(expires_at, now() + INTERVAL '24 hours'), 'active'
		FROM subscriptions
		WHERE id = $1
		RETURNING subscription_id::text, expires_at, created_at
	`, id, HashSubscriptionHandoffToken(token)).Scan(
		&invite.SubscriptionID,
		&invite.ExpiresAt,
		&invite.CreatedAt,
	)
	if err != nil {
		return SubscriptionHandoffInvite{}, err
	}
	if err := tx.Commit(); err != nil {
		return SubscriptionHandoffInvite{}, err
	}
	invite.HandoffToken = token
	return invite, nil
}

func (r *subscriptionsRepository) HandoffInviteStatus(ctx context.Context, id string) (SubscriptionHandoffInviteStatus, error) {
	if _, err := r.FindByID(ctx, id); err != nil {
		return SubscriptionHandoffInviteStatus{}, err
	}

	var row struct {
		status     sql.NullString
		createdAt  sql.NullTime
		expiresAt  sql.NullTime
		claimedAt  sql.NullTime
		updatedAt  sql.NullTime
		generation int
	}
	err := r.db.QueryRowContext(ctx, `
		WITH invite_counts AS (
			SELECT COUNT(*)::int AS generation
			FROM subscription_handoff_invites
			WHERE subscription_id = $1
		),
		current_invite AS (
			SELECT status, created_at, expires_at, claimed_at, updated_at
			FROM subscription_handoff_invites
			WHERE subscription_id = $1
			ORDER BY CASE WHEN status = 'active' AND expires_at > now() THEN 0 ELSE 1 END,
			         created_at DESC
			LIMIT 1
		)
		SELECT current_invite.status,
		       current_invite.created_at,
		       current_invite.expires_at,
		       current_invite.claimed_at,
		       current_invite.updated_at,
		       invite_counts.generation
		FROM invite_counts
		LEFT JOIN current_invite ON true
	`, id).Scan(&row.status, &row.createdAt, &row.expiresAt, &row.claimedAt, &row.updatedAt, &row.generation)
	if err != nil {
		return SubscriptionHandoffInviteStatus{}, err
	}

	status := SubscriptionHandoffInviteStatus{
		SubscriptionID: id,
		Status:         "never_issued",
		Generation:     row.generation,
	}
	if !row.status.Valid {
		return status, nil
	}

	status.Issued = true
	status.Status = row.status.String
	if row.status.String == "active" && row.expiresAt.Valid && !row.expiresAt.Time.After(time.Now().UTC()) {
		status.Status = "expired"
	}
	if row.createdAt.Valid {
		issuedAt := row.createdAt.Time
		status.IssuedAt = &issuedAt
	}
	if row.expiresAt.Valid {
		expiresAt := row.expiresAt.Time
		status.ExpiresAt = &expiresAt
	}
	if row.claimedAt.Valid {
		claimedAt := row.claimedAt.Time
		status.ClaimedAt = &claimedAt
	}
	if row.status.String == "revoked" && row.updatedAt.Valid {
		revokedAt := row.updatedAt.Time
		status.RevokedAt = &revokedAt
	}
	return status, nil
}

func (r *subscriptionsRepository) RevokeHandoffInvite(ctx context.Context, id string) (SubscriptionHandoffInviteStatus, error) {
	if _, err := r.FindByID(ctx, id); err != nil {
		return SubscriptionHandoffInviteStatus{}, err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE subscription_handoff_invites
		SET status = 'revoked',
		    updated_at = now()
		WHERE subscription_id = $1
		  AND status = 'active'
	`, id); err != nil {
		return SubscriptionHandoffInviteStatus{}, err
	}
	return r.HandoffInviteStatus(ctx, id)
}

func (r *subscriptionsRepository) ClaimHandoffInvite(ctx context.Context, token string) (SubscriptionHandoffClaim, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return SubscriptionHandoffClaim{}, ErrInvalidSubscriptionHandoffToken
	}

	accessToken, err := newSubscriptionAccessToken()
	if err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SubscriptionHandoffClaim{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var inviteID string
	var subscriptionID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, subscription_id::text
		FROM subscription_handoff_invites
		WHERE token_hash = $1
		  AND status = 'active'
		  AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, HashSubscriptionHandoffToken(token)).Scan(&inviteID, &subscriptionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionHandoffClaim{}, ErrInvalidSubscriptionHandoffToken
		}
		return SubscriptionHandoffClaim{}, err
	}

	if err := ensureSubscriptionEligibleForAccessToken(ctx, tx, subscriptionID); err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	access, err := r.Access(ctx, subscriptionID)
	if err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	var claimedAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE subscription_handoff_invites
		SET status = 'claimed',
		    claimed_at = now(),
		    updated_at = now()
		WHERE id = $1
		RETURNING claimed_at
	`, inviteID).Scan(&claimedAt)
	if err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	issuedToken, err := replaceAccessTokenInTx(ctx, tx, subscriptionID, accessToken)
	if err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	if err := tx.Commit(); err != nil {
		return SubscriptionHandoffClaim{}, err
	}

	return SubscriptionHandoffClaim{
		SubscriptionID:       subscriptionID,
		AccessToken:          accessToken,
		AccessTokenExpiresAt: issuedToken.ExpiresAt,
		ClaimedAt:            claimedAt,
		Access:               access,
	}, nil
}

type subscriptionQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ensureSubscriptionEligibleForAccessToken(ctx context.Context, queryer subscriptionQueryer, id string) error {
	var subscriptionID string
	err := queryer.QueryRowContext(ctx, `
		SELECT s.id::text
		FROM subscriptions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1
		  AND s.status = 'active'
		  AND u.status = 'active'
		  AND s.expires_at > now()
		FOR UPDATE OF s
	`, id).Scan(&subscriptionID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var exists bool
	if existsErr := queryer.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM subscriptions WHERE id = $1)`, id).Scan(&exists); existsErr != nil {
		return existsErr
	}
	if !exists {
		return ErrNotFound
	}
	return ErrSubscriptionAccessUnavailable
}

func buildVLESSRealityURI(endpoint SubscriptionAccessEndpoint, client SubscriptionAccessClient, displayName string) string {
	values := url.Values{}
	values.Set("encryption", "none")
	values.Set("flow", client.Flow)
	values.Set("fp", endpoint.Fingerprint)
	values.Set("pbk", endpoint.PublicKey)
	values.Set("security", endpoint.Security)
	values.Set("sid", endpoint.ShortID)
	values.Set("sni", endpoint.SNI)
	values.Set("spx", endpoint.SpiderX)
	values.Set("type", endpoint.Network)

	uri := url.URL{
		Scheme:   "vless",
		User:     url.User(client.ID),
		Host:     fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port),
		RawQuery: values.Encode(),
		Fragment: displayName,
	}
	return uri.String()
}

func newSubscriptionAccessToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "lnksa_" + hex.EncodeToString(raw), nil
}

func newSubscriptionHandoffToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "lnkhi_" + hex.EncodeToString(raw), nil
}

func HashSubscriptionAccessToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashSubscriptionHandoffToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *subscriptionsRepository) Update(ctx context.Context, id string, input UpdateSubscriptionInput) (Subscription, error) {
	current, err := r.FindByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	if input.Status != nil {
		current.Status = *input.Status
	}
	if input.ClearTrafficLimit {
		current.TrafficLimitBytes = nil
	} else if input.TrafficLimitBytes != nil {
		current.TrafficLimitBytes = input.TrafficLimitBytes
	}
	if input.DeviceLimit != nil {
		current.DeviceLimit = *input.DeviceLimit
	}
	if input.ClearPreferredRegion {
		current.PreferredRegion = nil
	} else if input.PreferredRegion != nil {
		current.PreferredRegion = input.PreferredRegion
	}

	var subscription Subscription
	err = r.db.QueryRowContext(ctx, `
		UPDATE subscriptions
		SET status = $2,
		    traffic_limit_bytes = $3,
		    device_limit = $4,
		    preferred_region = $5,
		    updated_at = now()
		WHERE id = $1
		RETURNING id::text, user_id::text, plan_id::text, status, starts_at, expires_at,
		          traffic_limit_bytes, traffic_used_bytes, device_limit, preferred_region
	`, id, current.Status, current.TrafficLimitBytes, current.DeviceLimit, current.PreferredRegion).Scan(
		&subscription.ID,
		&subscription.UserID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.StartsAt,
		&subscription.ExpiresAt,
		&subscription.TrafficLimitBytes,
		&subscription.TrafficUsedBytes,
		&subscription.DeviceLimit,
		&subscription.PreferredRegion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *subscriptionsRepository) Renew(ctx context.Context, id string, extendDays int) (Subscription, error) {
	var subscription Subscription
	err := r.db.QueryRowContext(ctx, `
		UPDATE subscriptions
		SET status = 'active',
		    expires_at = GREATEST(expires_at, now()) + ($2 * INTERVAL '1 day'),
		    updated_at = now()
		WHERE id = $1
		RETURNING id::text, user_id::text, plan_id::text, status, starts_at, expires_at,
		          traffic_limit_bytes, traffic_used_bytes, device_limit, preferred_region
	`, id, extendDays).Scan(
		&subscription.ID,
		&subscription.UserID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.StartsAt,
		&subscription.ExpiresAt,
		&subscription.TrafficLimitBytes,
		&subscription.TrafficUsedBytes,
		&subscription.DeviceLimit,
		&subscription.PreferredRegion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *subscriptionsRepository) SubscriptionIDByToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrInvalidSubscriptionAccessToken
	}
	var subscriptionID string
	err := r.db.QueryRowContext(ctx, `
		SELECT subscription_id::text
		FROM subscription_access_tokens
		WHERE token_hash = $1
		  AND status = 'active'
		  AND expires_at > now()
		LIMIT 1
	`, HashSubscriptionAccessToken(token)).Scan(&subscriptionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidSubscriptionAccessToken
		}
		return "", err
	}
	return subscriptionID, nil
}
