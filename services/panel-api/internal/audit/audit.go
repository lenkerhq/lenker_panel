package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
)

type Event struct {
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string
	Reason       string
	Changes      map[string]any
}

type Recorder interface {
	Record(ctx context.Context, event Event) error
}

type NoopRecorder struct {
}

func (NoopRecorder) Record(ctx context.Context, event Event) error {
	return nil
}

// AuditLog is a persisted audit entry.
type AuditLog struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actor_type"`
	ActorID      string         `json:"actor_id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Outcome      string         `json:"outcome"`
	Reason       string         `json:"reason,omitempty"`
	Changes      map[string]any `json:"changes,omitempty"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ListFilter defines query parameters for listing audit logs.
type ListFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
}

// Repository provides read access to audit logs.
type Repository interface {
	List(ctx context.Context, filter ListFilter) ([]AuditLog, error)
	FindByID(ctx context.Context, id string) (AuditLog, error)
	ResourceHistory(ctx context.Context, resourceType, resourceID string, limit, offset int) ([]AuditLog, error)
}

// PostgresRecorder writes audit events to PostgreSQL.
type PostgresRecorder struct {
	db *sql.DB
}

func NewPostgresRecorder(db *sql.DB) *PostgresRecorder {
	return &PostgresRecorder{db: db}
}

func (r *PostgresRecorder) Record(ctx context.Context, event Event) error {
	ip, ua := requestMeta(ctx)
	var changesJSON []byte
	if event.Changes != nil {
		var err error
		changesJSON, err = json.Marshal(event.Changes)
		if err != nil {
			changesJSON = nil
		}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, resource_type, resource_id, outcome, reason, changes, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''))
	`, event.ActorType, event.ActorID, event.Action, event.ResourceType,
		event.ResourceID, event.Outcome, event.Reason, changesJSON, ip, ua)
	return err
}

func (r *PostgresRecorder) List(ctx context.Context, f ListFilter) ([]AuditLog, error) {
	query := `SELECT id, actor_type, actor_id, action, resource_type, COALESCE(resource_id,''), outcome, COALESCE(reason,''), changes, COALESCE(ip_address,''), COALESCE(user_agent,''), created_at FROM audit_logs WHERE 1=1`
	args := []any{}
	n := 0

	if f.ActorID != "" {
		n++
		query += ` AND actor_id = $` + itoa(n)
		args = append(args, f.ActorID)
	}
	if f.Action != "" {
		n++
		query += ` AND action = $` + itoa(n)
		args = append(args, f.Action)
	}
	if f.ResourceType != "" {
		n++
		query += ` AND resource_type = $` + itoa(n)
		args = append(args, f.ResourceType)
	}
	if f.ResourceID != "" {
		n++
		query += ` AND resource_id = $` + itoa(n)
		args = append(args, f.ResourceID)
	}
	if !f.From.IsZero() {
		n++
		query += ` AND created_at >= $` + itoa(n)
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		n++
		query += ` AND created_at <= $` + itoa(n)
		args = append(args, f.To)
	}
	query += ` ORDER BY created_at DESC`
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	n++
	query += ` LIMIT $` + itoa(n)
	args = append(args, limit)
	if f.Offset > 0 {
		n++
		query += ` OFFSET $` + itoa(n)
		args = append(args, f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

func (r *PostgresRecorder) FindByID(ctx context.Context, id string) (AuditLog, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, actor_type, actor_id, action, resource_type, COALESCE(resource_id,''), outcome, COALESCE(reason,''), changes, COALESCE(ip_address,''), COALESCE(user_agent,''), created_at FROM audit_logs WHERE id = $1`, id)
	return scanAuditLog(row)
}

func (r *PostgresRecorder) ResourceHistory(ctx context.Context, resourceType, resourceID string, limit, offset int) ([]AuditLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, actor_type, actor_id, action, resource_type, COALESCE(resource_id,''), outcome, COALESCE(reason,''), changes, COALESCE(ip_address,''), COALESCE(user_agent,''), created_at FROM audit_logs WHERE resource_type = $1 AND resource_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`, resourceType, resourceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

func scanAuditLogs(rows *sql.Rows) ([]AuditLog, error) {
	var logs []AuditLog
	for rows.Next() {
		log, err := scanAuditLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAuditLog(row *sql.Row) (AuditLog, error) {
	var l AuditLog
	var changesJSON []byte
	err := row.Scan(&l.ID, &l.ActorType, &l.ActorID, &l.Action, &l.ResourceType, &l.ResourceID, &l.Outcome, &l.Reason, &changesJSON, &l.IPAddress, &l.UserAgent, &l.CreatedAt)
	if err != nil {
		return AuditLog{}, err
	}
	if len(changesJSON) > 0 {
		_ = json.Unmarshal(changesJSON, &l.Changes)
	}
	return l, nil
}

func scanAuditLogRow(rows *sql.Rows) (AuditLog, error) {
	var l AuditLog
	var changesJSON []byte
	err := rows.Scan(&l.ID, &l.ActorType, &l.ActorID, &l.Action, &l.ResourceType, &l.ResourceID, &l.Outcome, &l.Reason, &changesJSON, &l.IPAddress, &l.UserAgent, &l.CreatedAt)
	if err != nil {
		return AuditLog{}, err
	}
	if len(changesJSON) > 0 {
		_ = json.Unmarshal(changesJSON, &l.Changes)
	}
	return l, nil
}

func requestMeta(ctx context.Context) (string, string) {
	return httpapi.GetRequestMeta(ctx)
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

const (
	ActionAdminLogin                 = "admin.login"
	ActionAdminSessionValidation     = "admin.session_validation"
	ActionUserCreate                 = "user.create"
	ActionUserUpdate                 = "user.update"
	ActionUserSuspend                = "user.suspend"
	ActionUserActivate               = "user.activate"
	ActionPlanCreate                 = "plan.create"
	ActionPlanUpdate                 = "plan.update"
	ActionPlanArchive                = "plan.archive"
	ActionSubscriptionCreate         = "subscription.create"
	ActionSubscriptionUpdate         = "subscription.update"
	ActionSubscriptionRenew          = "subscription.renew"
	ActionNodeBootstrapToken         = "node.bootstrap_token.create"
	ActionNodeRegister               = "node.register"
	ActionNodeHeartbeat              = "node.heartbeat"
	ActionNodeDrain                  = "node.drain"
	ActionNodeUndrain                = "node.undrain"
	ActionNodeDisable                = "node.disable"
	ActionNodeEnable                 = "node.enable"
	ActionNodeConfigRevisionCreate   = "node.config_revision.create"
	ActionNodeConfigRevisionFetch    = "node.config_revision.fetch"
	ActionNodeConfigRevisionReport   = "node.config_revision.report"
	ActionNodeConfigRevisionRollback = "node.config_revision.rollback"

	ActionDeviceDelete     = "device.delete"
	ActionDeviceDeactivate = "device.deactivate"

	ActionTrafficQuotaSet   = "traffic.quota.set"
	ActionTrafficQuotaReset = "traffic.quota.reset"

	ActionRoutingRuleCreate  = "routing_rule.create"
	ActionRoutingRuleUpdate  = "routing_rule.update"
	ActionRoutingRuleDelete  = "routing_rule.delete"
	ActionRoutingRuleReorder = "routing_rule.reorder"

	ActionSettingUpdate = "setting.update"

	ActionWarpSet    = "warp.set"
	ActionWarpDelete = "warp.delete"

	ActionProfileCreate = "node_profile.create"
	ActionProfileUpdate = "node_profile.update"
	ActionProfileDelete = "node_profile.delete"
	ActionProfileApply  = "node_profile.apply"

	ActionTemplateCreate = "subscription_template.create"
	ActionTemplateUpdate = "subscription_template.update"
	ActionTemplateDelete = "subscription_template.delete"

	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)
