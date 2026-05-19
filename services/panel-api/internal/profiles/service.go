package profiles

import (
	"context"
	"strings"

	"github.com/lenker/lenker/services/panel-api/internal/routing"
)

type Service struct {
	repo       Repository
	routingSvc *routing.Service
}

func NewService(repo Repository, routingSvc *routing.Service) *Service {
	return &Service{repo: repo, routingSvc: routingSvc}
}

func (s *Service) List(ctx context.Context) ([]*NodeProfile, error) {
	return s.repo.List(ctx)
}

func (s *Service) FindByID(ctx context.Context, id string) (*NodeProfile, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*NodeProfile, error) {
	if err := ValidateName(input.Name); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*NodeProfile, error) {
	if input.Name != nil {
		if err := ValidateName(*input.Name); err != nil {
			return nil, err
		}
	}
	return s.repo.Update(ctx, id, input)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	profile, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if profile.IsSystem {
		return ErrSystemProfile
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) ApplyToNode(ctx context.Context, profileID, nodeID string) error {
	profile, err := s.repo.FindByID(ctx, profileID)
	if err != nil {
		return err
	}

	cfg, err := ParseConfig(profile.Config)
	if err != nil {
		return err
	}

	for _, rule := range cfg.RoutingRules {
		ruleType := strings.TrimSpace(rule.RuleType)
		target := strings.TrimSpace(rule.Target)
		action := strings.TrimSpace(rule.Action)
		if ruleType == "" || target == "" || action == "" {
			continue
		}
		priority := rule.Priority
		if priority <= 0 {
			priority = 100
		}
		_, err := s.routingSvc.Create(ctx, routing.CreateInput{
			NodeID:      &nodeID,
			RuleType:    ruleType,
			Target:      target,
			Action:      action,
			OutboundTag: rule.OutboundTag,
			Priority:    priority,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
