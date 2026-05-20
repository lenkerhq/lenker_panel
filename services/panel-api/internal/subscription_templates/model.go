package subscription_templates

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("subscription template not found")
	ErrInvalidInput   = errors.New("invalid subscription template input")
	ErrSystemTemplate = errors.New("cannot modify system template")
)

type TemplateConfig struct {
	DurationDays      int    `json:"duration_days"`
	TrafficLimitBytes *int64 `json:"traffic_limit_bytes"`
	DeviceLimit       int    `json:"device_limit"`
}

type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	PlanID      *string         `json:"plan_id"`
	Config      json.RawMessage `json:"config"`
	IsSystem    bool            `json:"is_system"`
	CreatedAt   time.Time       `json:"created_at"`
}

type CreateInput struct {
	Name        string
	Description *string
	PlanID      *string
	Config      json.RawMessage
}

type UpdateInput struct {
	Name        *string
	Description *string
	PlanID      *string
	Config      json.RawMessage
}
