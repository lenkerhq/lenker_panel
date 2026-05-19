package routing

import (
	"context"
	"testing"
	"time"
)

// memRepo is an in-memory repository for testing.
type memRepo struct {
	rules  []*Rule
	nextID int
}

func newMemRepo() *memRepo { return &memRepo{} }

func (m *memRepo) List(_ context.Context, nodeID *string) ([]*Rule, error) {
	var result []*Rule
	for _, r := range m.rules {
		if nodeID == nil {
			if r.NodeID == nil {
				result = append(result, r)
			}
		} else {
			if r.NodeID == nil || *r.NodeID == *nodeID {
				result = append(result, r)
			}
		}
	}
	return result, nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*Rule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Create(_ context.Context, input CreateInput) (*Rule, error) {
	m.nextID++
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	rule := &Rule{
		ID:          idStr(m.nextID),
		NodeID:      input.NodeID,
		RuleType:    input.RuleType,
		Target:      input.Target,
		Action:      input.Action,
		OutboundTag: input.OutboundTag,
		Priority:    input.Priority,
		Enabled:     enabled,
		Description: input.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.rules = append(m.rules, rule)
	return rule, nil
}

func (m *memRepo) Update(_ context.Context, id string, input UpdateInput) (*Rule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			if input.RuleType != nil {
				r.RuleType = *input.RuleType
			}
			if input.Target != nil {
				r.Target = *input.Target
			}
			if input.Action != nil {
				r.Action = *input.Action
			}
			if input.OutboundTag != nil {
				r.OutboundTag = input.OutboundTag
			}
			if input.Priority != nil {
				r.Priority = *input.Priority
			}
			if input.Enabled != nil {
				r.Enabled = *input.Enabled
			}
			if input.Description != nil {
				r.Description = input.Description
			}
			r.UpdatedAt = time.Now()
			return r, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	for i, r := range m.rules {
		if r.ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memRepo) Reorder(_ context.Context, entries []ReorderEntry) error {
	for _, e := range entries {
		for _, r := range m.rules {
			if r.ID == e.ID {
				r.Priority = e.Priority
			}
		}
	}
	return nil
}

func idStr(n int) string {
	return "rule-" + string(rune('0'+n))
}

func TestServiceCreateValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateInput{RuleType: "invalid", Target: "example.com", Action: "block"})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule, got %v", err)
	}

	_, err = svc.Create(ctx, CreateInput{RuleType: "domain", Target: "", Action: "block"})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for empty target, got %v", err)
	}

	_, err = svc.Create(ctx, CreateInput{RuleType: "domain", Target: "example.com", Action: "invalid"})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for invalid action, got %v", err)
	}
}

func TestServiceCreateSuccess(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	rule, err := svc.Create(ctx, CreateInput{RuleType: "domain", Target: "example.com", Action: "block", Priority: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.RuleType != "domain" || rule.Target != "example.com" || rule.Action != "block" || rule.Priority != 10 {
		t.Fatalf("unexpected rule: %+v", rule)
	}
	if rule.NodeID != nil {
		t.Fatalf("expected global rule (nil node_id)")
	}
}

func TestServiceListGlobalAndNode(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()
	nodeID := "node-1"

	svc.Create(ctx, CreateInput{RuleType: "geosite", Target: "category-ads", Action: "block", Priority: 10})
	svc.Create(ctx, CreateInput{NodeID: &nodeID, RuleType: "domain", Target: "blocked.com", Action: "block", Priority: 20})

	globals, _ := svc.ListGlobal(ctx)
	if len(globals) != 1 {
		t.Fatalf("expected 1 global rule, got %d", len(globals))
	}

	nodeRules, _ := svc.ListForNode(ctx, nodeID)
	if len(nodeRules) != 2 {
		t.Fatalf("expected 2 rules for node (global + node), got %d", len(nodeRules))
	}
}

func TestServiceEffectiveRulesFiltersDisabled(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()
	nodeID := "node-1"
	disabled := false

	svc.Create(ctx, CreateInput{RuleType: "geosite", Target: "category-ads", Action: "block", Priority: 10})
	svc.Create(ctx, CreateInput{NodeID: &nodeID, RuleType: "domain", Target: "blocked.com", Action: "block", Priority: 20, Enabled: &disabled})

	effective, _ := svc.EffectiveRules(ctx, nodeID)
	if len(effective) != 1 {
		t.Fatalf("expected 1 effective rule, got %d", len(effective))
	}
}

func TestServiceReorderValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	err := svc.Reorder(ctx, nil)
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for empty entries")
	}

	err = svc.Reorder(ctx, []ReorderEntry{{ID: "", Priority: 1}})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for empty ID")
	}

	err = svc.Reorder(ctx, []ReorderEntry{{ID: "rule-1", Priority: 0}})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for zero priority")
	}
}

func TestServiceUpdateValidation(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo)
	ctx := context.Background()

	rule, _ := svc.Create(ctx, CreateInput{RuleType: "domain", Target: "example.com", Action: "block", Priority: 10})

	badType := "invalid"
	_, err := svc.Update(ctx, rule.ID, UpdateInput{RuleType: &badType})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for invalid rule_type update")
	}

	badAction := "invalid"
	_, err = svc.Update(ctx, rule.ID, UpdateInput{Action: &badAction})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for invalid action update")
	}

	emptyTarget := ""
	_, err = svc.Update(ctx, rule.ID, UpdateInput{Target: &emptyTarget})
	if err != ErrInvalidRule {
		t.Fatalf("expected ErrInvalidRule for empty target update")
	}
}
