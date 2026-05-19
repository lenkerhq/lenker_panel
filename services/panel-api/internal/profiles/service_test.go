package profiles

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/routing"
)

type memRepo struct {
	profiles []*NodeProfile
	nextID   int
}

func newMemRepo() *memRepo { return &memRepo{} }

func (m *memRepo) List(_ context.Context) ([]*NodeProfile, error) {
	return m.profiles, nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*NodeProfile, error) {
	for _, p := range m.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Create(_ context.Context, input CreateInput) (*NodeProfile, error) {
	m.nextID++
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	p := &NodeProfile{
		ID:          idStr(m.nextID),
		Name:        input.Name,
		Description: input.Description,
		IsSystem:    false,
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.profiles = append(m.profiles, p)
	return p, nil
}

func (m *memRepo) Update(_ context.Context, id string, input UpdateInput) (*NodeProfile, error) {
	for _, p := range m.profiles {
		if p.ID == id {
			if input.Name != nil {
				p.Name = *input.Name
			}
			if input.Description != nil {
				p.Description = input.Description
			}
			if len(input.Config) > 0 {
				p.Config = input.Config
			}
			p.UpdatedAt = time.Now()
			return p, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	for i, p := range m.profiles {
		if p.ID == id {
			m.profiles = append(m.profiles[:i], m.profiles[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memRepo) addSystem(name string, config string) *NodeProfile {
	m.nextID++
	p := &NodeProfile{
		ID:       idStr(m.nextID),
		Name:     name,
		IsSystem: true,
		Config:   json.RawMessage(config),
	}
	m.profiles = append(m.profiles, p)
	return p
}

func idStr(n int) string { return "profile-" + string(rune('0'+n)) }

func TestServiceCreateValidation(t *testing.T) {
	repo := newMemRepo()
	routingRepo := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingRepo)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateInput{Name: ""})
	if err != ErrInvalidProfile {
		t.Fatalf("expected ErrInvalidProfile for empty name, got %v", err)
	}

	_, err = svc.Create(ctx, CreateInput{Name: "   "})
	if err != ErrInvalidProfile {
		t.Fatalf("expected ErrInvalidProfile for whitespace name, got %v", err)
	}
}

func TestServiceCreateSuccess(t *testing.T) {
	repo := newMemRepo()
	routingRepo := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingRepo)
	ctx := context.Background()

	p, err := svc.Create(ctx, CreateInput{Name: "test-profile", Config: json.RawMessage(`{"routing_rules":[]}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "test-profile" || p.IsSystem {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestServiceDeleteSystemProfile(t *testing.T) {
	repo := newMemRepo()
	sys := repo.addSystem("default-vless-reality", `{"routing_rules":[]}`)
	routingRepo := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingRepo)
	ctx := context.Background()

	err := svc.Delete(ctx, sys.ID)
	if err != ErrSystemProfile {
		t.Fatalf("expected ErrSystemProfile, got %v", err)
	}
}

func TestServiceDeleteUserProfile(t *testing.T) {
	repo := newMemRepo()
	routingRepo := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingRepo)
	ctx := context.Background()

	p, _ := svc.Create(ctx, CreateInput{Name: "custom"})
	err := svc.Delete(ctx, p.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.FindByID(ctx, p.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestServiceApplyToNode(t *testing.T) {
	repo := newMemRepo()
	routingMem := newMemRoutingRepo()
	routingSvc := routing.NewService(routingMem)
	svc := NewService(repo, routingSvc)
	ctx := context.Background()

	cfg := `{"routing_rules":[{"rule_type":"geosite","target":"category-ads","action":"block","priority":10},{"rule_type":"geoip","target":"cn","action":"direct","priority":20}]}`
	p, _ := svc.Create(ctx, CreateInput{Name: "block-ads", Config: json.RawMessage(cfg)})

	nodeID := "node-123"
	err := svc.ApplyToNode(ctx, p.ID, nodeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules, _ := routingSvc.ListForNode(ctx, nodeID)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules applied, got %d", len(rules))
	}
	if rules[0].RuleType != "geosite" || rules[0].Target != "category-ads" {
		t.Errorf("unexpected first rule: %+v", rules[0])
	}
	if rules[1].RuleType != "geoip" || rules[1].Target != "cn" {
		t.Errorf("unexpected second rule: %+v", rules[1])
	}
}

func TestServiceApplyNotFound(t *testing.T) {
	repo := newMemRepo()
	routingSvc := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingSvc)
	ctx := context.Background()

	err := svc.ApplyToNode(ctx, "nonexistent", "node-1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// memRoutingRepo implements routing.Repository for testing.
type memRoutingRepo struct {
	rules  []*routing.Rule
	nextID int
}

func newMemRoutingRepo() *memRoutingRepo { return &memRoutingRepo{} }

func (m *memRoutingRepo) List(_ context.Context, nodeID *string) ([]*routing.Rule, error) {
	var result []*routing.Rule
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

func (m *memRoutingRepo) FindByID(_ context.Context, id string) (*routing.Rule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, routing.ErrNotFound
}

func (m *memRoutingRepo) Create(_ context.Context, input routing.CreateInput) (*routing.Rule, error) {
	m.nextID++
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	rule := &routing.Rule{
		ID:          "rule-" + string(rune('0'+m.nextID)),
		NodeID:      input.NodeID,
		RuleType:    input.RuleType,
		Target:      input.Target,
		Action:      input.Action,
		OutboundTag: input.OutboundTag,
		Priority:    input.Priority,
		Enabled:     enabled,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.rules = append(m.rules, rule)
	return rule, nil
}

func (m *memRoutingRepo) Update(_ context.Context, id string, input routing.UpdateInput) (*routing.Rule, error) {
	return nil, routing.ErrNotFound
}

func (m *memRoutingRepo) Delete(_ context.Context, id string) error {
	return routing.ErrNotFound
}

func (m *memRoutingRepo) Reorder(_ context.Context, entries []routing.ReorderEntry) error {
	return nil
}
