package profiles

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound       = errors.New("node profile not found")
	ErrInvalidProfile = errors.New("invalid node profile")
	ErrSystemProfile  = errors.New("cannot delete system profile")
)

type ProfileConfig struct {
	RoutingRules []ProfileRule `json:"routing_rules"`
}

type ProfileRule struct {
	RuleType    string  `json:"rule_type"`
	Target      string  `json:"target"`
	Action      string  `json:"action"`
	OutboundTag *string `json:"outbound_tag,omitempty"`
	Priority    int     `json:"priority"`
}

type NodeProfile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	IsSystem    bool            `json:"is_system"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateInput struct {
	Name        string
	Description *string
	Config      json.RawMessage
}

type UpdateInput struct {
	Name        *string
	Description *string
	Config      json.RawMessage
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidProfile
	}
	return nil
}

func ParseConfig(raw json.RawMessage) (*ProfileConfig, error) {
	if len(raw) == 0 {
		return &ProfileConfig{}, nil
	}
	var cfg ProfileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, ErrInvalidProfile
	}
	return &cfg, nil
}
