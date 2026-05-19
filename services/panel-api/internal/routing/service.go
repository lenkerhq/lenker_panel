package routing

import (
	"context"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListGlobal(ctx context.Context) ([]*Rule, error) {
	return s.repo.List(ctx, nil)
}

func (s *Service) ListForNode(ctx context.Context, nodeID string) ([]*Rule, error) {
	return s.repo.List(ctx, &nodeID)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*Rule, error) {
	if err := validateInput(input.RuleType, input.Target, input.Action); err != nil {
		return nil, err
	}
	if input.Priority <= 0 {
		input.Priority = 100
	}
	return s.repo.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*Rule, error) {
	if input.RuleType != nil && !ValidRuleTypes[*input.RuleType] {
		return nil, ErrInvalidRule
	}
	if input.Action != nil && !ValidActions[*input.Action] {
		return nil, ErrInvalidRule
	}
	if input.Target != nil && strings.TrimSpace(*input.Target) == "" {
		return nil, ErrInvalidRule
	}
	return s.repo.Update(ctx, id, input)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) Reorder(ctx context.Context, entries []ReorderEntry) error {
	if len(entries) == 0 {
		return ErrInvalidRule
	}
	for _, e := range entries {
		if strings.TrimSpace(e.ID) == "" || e.Priority <= 0 {
			return ErrInvalidRule
		}
	}
	return s.repo.Reorder(ctx, entries)
}

// EffectiveRules returns enabled rules for a node (globals + node-specific), sorted by priority.
func (s *Service) EffectiveRules(ctx context.Context, nodeID string) ([]*Rule, error) {
	rules, err := s.repo.List(ctx, &nodeID)
	if err != nil {
		return nil, err
	}
	var enabled []*Rule
	for _, r := range rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled, nil
}

func validateInput(ruleType, target, action string) error {
	if !ValidRuleTypes[ruleType] {
		return ErrInvalidRule
	}
	if strings.TrimSpace(target) == "" {
		return ErrInvalidRule
	}
	if !ValidActions[action] {
		return ErrInvalidRule
	}
	return nil
}
