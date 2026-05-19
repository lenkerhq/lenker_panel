package routing

import (
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("routing rule not found")
	ErrInvalidRule = errors.New("invalid routing rule")
)

var ValidRuleTypes = map[string]bool{
	"geosite":  true,
	"geoip":    true,
	"domain":   true,
	"ip":       true,
	"port":     true,
	"protocol": true,
}

var ValidActions = map[string]bool{
	"block":  true,
	"proxy":  true,
	"direct": true,
	"warp":   true,
}

type Rule struct {
	ID          string    `json:"id"`
	NodeID      *string   `json:"node_id"`
	RuleType    string    `json:"rule_type"`
	Target      string    `json:"target"`
	Action      string    `json:"action"`
	OutboundTag *string   `json:"outbound_tag"`
	Priority    int       `json:"priority"`
	Enabled     bool      `json:"enabled"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Scope returns "global" or "node" based on whether NodeID is set.
func (r Rule) Scope() string {
	if r.NodeID == nil {
		return "global"
	}
	return "node"
}

type CreateInput struct {
	NodeID      *string
	RuleType    string
	Target      string
	Action      string
	OutboundTag *string
	Priority    int
	Enabled     *bool
	Description *string
}

type UpdateInput struct {
	RuleType    *string
	Target      *string
	Action      *string
	OutboundTag *string
	Priority    *int
	Enabled     *bool
	Description *string
}

type ReorderEntry struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
}
