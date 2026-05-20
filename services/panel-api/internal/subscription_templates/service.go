package subscription_templates

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

type Service struct {
	repo          Repository
	subscriptions storage.SubscriptionsRepository
}

func NewService(repo Repository, subscriptions storage.SubscriptionsRepository) *Service {
	return &Service{repo: repo, subscriptions: subscriptions}
}

func (s *Service) List(ctx context.Context) ([]*Template, error) {
	return s.repo.List(ctx)
}

func (s *Service) FindByID(ctx context.Context, id string) (*Template, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*Template, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, ErrInvalidInput
	}
	if len(input.Config) == 0 {
		return nil, ErrInvalidInput
	}
	var cfg TemplateConfig
	if err := json.Unmarshal(input.Config, &cfg); err != nil {
		return nil, ErrInvalidInput
	}
	if cfg.DurationDays <= 0 || cfg.DeviceLimit <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repo.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*Template, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return nil, ErrInvalidInput
	}
	if len(input.Config) > 0 && string(input.Config) != "null" {
		var cfg TemplateConfig
		if err := json.Unmarshal(input.Config, &cfg); err != nil {
			return nil, ErrInvalidInput
		}
		if cfg.DurationDays <= 0 || cfg.DeviceLimit <= 0 {
			return nil, ErrInvalidInput
		}
	} else {
		input.Config = nil
	}
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.IsSystem {
		return nil, ErrSystemTemplate
	}
	return s.repo.Update(ctx, id, input)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if t.IsSystem {
		return ErrSystemTemplate
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) CreateFromTemplate(ctx context.Context, templateID, userID string, preferredRegion *string) (storage.Subscription, error) {
	t, err := s.repo.FindByID(ctx, templateID)
	if err != nil {
		return storage.Subscription{}, err
	}

	if t.PlanID != nil && *t.PlanID != "" {
		return s.subscriptions.Create(ctx, storage.CreateSubscriptionInput{
			UserID:          userID,
			PlanID:          *t.PlanID,
			PreferredRegion: preferredRegion,
		})
	}

	return storage.Subscription{}, ErrInvalidInput
}

// ResolvePlanID implements subscriptions.TemplateResolver.
func (s *Service) ResolvePlanID(ctx context.Context, templateID string) (string, error) {
	t, err := s.repo.FindByID(ctx, templateID)
	if err != nil {
		return "", err
	}
	if t.PlanID == nil || *t.PlanID == "" {
		return "", ErrInvalidInput
	}
	return *t.PlanID, nil
}
