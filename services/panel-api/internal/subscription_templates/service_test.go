package subscription_templates

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

type memRepo struct {
	templates []*Template
	nextID    int
}

func newMemRepo() *memRepo { return &memRepo{} }

func (m *memRepo) List(_ context.Context) ([]*Template, error) {
	return m.templates, nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*Template, error) {
	for _, t := range m.templates {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Create(_ context.Context, input CreateInput) (*Template, error) {
	m.nextID++
	cfg := input.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	t := &Template{
		ID:          idStr(m.nextID),
		Name:        input.Name,
		Description: input.Description,
		PlanID:      input.PlanID,
		Config:      cfg,
		IsSystem:    false,
		CreatedAt:   time.Now(),
	}
	m.templates = append(m.templates, t)
	return t, nil
}

func (m *memRepo) Update(_ context.Context, id string, input UpdateInput) (*Template, error) {
	for _, t := range m.templates {
		if t.ID == id {
			if t.IsSystem {
				return nil, ErrNotFound
			}
			if input.Name != nil {
				t.Name = *input.Name
			}
			if input.Description != nil {
				t.Description = input.Description
			}
			if input.PlanID != nil {
				t.PlanID = input.PlanID
			}
			if len(input.Config) > 0 {
				t.Config = input.Config
			}
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	for i, t := range m.templates {
		if t.ID == id {
			m.templates = append(m.templates[:i], m.templates[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memRepo) addSystem(name string, planID *string, config string) *Template {
	m.nextID++
	t := &Template{
		ID:       idStr(m.nextID),
		Name:     name,
		PlanID:   planID,
		IsSystem: true,
		Config:   json.RawMessage(config),
	}
	m.templates = append(m.templates, t)
	return t
}

func idStr(n int) string { return "tmpl-" + string(rune('0'+n)) }

type memSubsRepo struct {
	created []storage.Subscription
}

func (m *memSubsRepo) Create(_ context.Context, input storage.CreateSubscriptionInput) (storage.Subscription, error) {
	sub := storage.Subscription{
		ID:     "sub-1",
		UserID: input.UserID,
		PlanID: input.PlanID,
		Status: "active",
	}
	m.created = append(m.created, sub)
	return sub, nil
}

func (m *memSubsRepo) List(_ context.Context) ([]storage.Subscription, error) { return nil, nil }
func (m *memSubsRepo) FindByID(_ context.Context, _ string) (storage.Subscription, error) {
	return storage.Subscription{}, nil
}
func (m *memSubsRepo) Access(_ context.Context, _ string) (storage.SubscriptionAccess, error) {
	return storage.SubscriptionAccess{}, nil
}
func (m *memSubsRepo) AccessTokenStatus(_ context.Context, _ string) (storage.SubscriptionAccessTokenStatus, error) {
	return storage.SubscriptionAccessTokenStatus{}, nil
}
func (m *memSubsRepo) CreateAccessToken(_ context.Context, _ string) (storage.SubscriptionAccessToken, error) {
	return storage.SubscriptionAccessToken{}, nil
}
func (m *memSubsRepo) RotateAccessToken(_ context.Context, _ string) (storage.SubscriptionAccessToken, error) {
	return storage.SubscriptionAccessToken{}, nil
}
func (m *memSubsRepo) RevokeAccessToken(_ context.Context, _ string) (storage.SubscriptionAccessTokenStatus, error) {
	return storage.SubscriptionAccessTokenStatus{}, nil
}
func (m *memSubsRepo) AccessByToken(_ context.Context, _ string) (storage.SubscriptionAccess, error) {
	return storage.SubscriptionAccess{}, nil
}
func (m *memSubsRepo) CreateHandoffInvite(_ context.Context, _ string) (storage.SubscriptionHandoffInvite, error) {
	return storage.SubscriptionHandoffInvite{}, nil
}
func (m *memSubsRepo) HandoffInviteStatus(_ context.Context, _ string) (storage.SubscriptionHandoffInviteStatus, error) {
	return storage.SubscriptionHandoffInviteStatus{}, nil
}
func (m *memSubsRepo) RevokeHandoffInvite(_ context.Context, _ string) (storage.SubscriptionHandoffInviteStatus, error) {
	return storage.SubscriptionHandoffInviteStatus{}, nil
}
func (m *memSubsRepo) ClaimHandoffInvite(_ context.Context, _ string) (storage.SubscriptionHandoffClaim, error) {
	return storage.SubscriptionHandoffClaim{}, nil
}
func (m *memSubsRepo) Update(_ context.Context, _ string, _ storage.UpdateSubscriptionInput) (storage.Subscription, error) {
	return storage.Subscription{}, nil
}
func (m *memSubsRepo) Renew(_ context.Context, _ string, _ int) (storage.Subscription, error) {
	return storage.Subscription{}, nil
}
func (m *memSubsRepo) SubscriptionIDByToken(_ context.Context, _ string) (string, error) {
	return "", nil
}

func TestServiceCreateValidation(t *testing.T) {
	svc := NewService(newMemRepo(), &memSubsRepo{})
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateInput{Name: ""})
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty name, got %v", err)
	}

	_, err = svc.Create(ctx, CreateInput{Name: "test", Config: json.RawMessage(`{}`)})
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for zero duration, got %v", err)
	}

	_, err = svc.Create(ctx, CreateInput{Name: "test", Config: json.RawMessage(`{"duration_days":7,"device_limit":0}`)})
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for zero device_limit, got %v", err)
	}
}

func TestServiceCreateSuccess(t *testing.T) {
	svc := NewService(newMemRepo(), &memSubsRepo{})
	ctx := context.Background()

	tmpl, err := svc.Create(ctx, CreateInput{
		Name:   "test-template",
		Config: json.RawMessage(`{"duration_days":30,"traffic_limit_bytes":107374182400,"device_limit":3}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.Name != "test-template" || tmpl.IsSystem {
		t.Fatalf("unexpected template: %+v", tmpl)
	}
}

func TestServiceDeleteSystemTemplate(t *testing.T) {
	repo := newMemRepo()
	repo.addSystem("trial-7-days", nil, `{"duration_days":7,"traffic_limit_bytes":10737418240,"device_limit":1}`)
	svc := NewService(repo, &memSubsRepo{})
	ctx := context.Background()

	err := svc.Delete(ctx, idStr(1))
	if err != ErrSystemTemplate {
		t.Fatalf("expected ErrSystemTemplate, got %v", err)
	}
}

func TestServiceUpdateSystemTemplate(t *testing.T) {
	repo := newMemRepo()
	repo.addSystem("trial-7-days", nil, `{"duration_days":7,"traffic_limit_bytes":10737418240,"device_limit":1}`)
	svc := NewService(repo, &memSubsRepo{})
	ctx := context.Background()

	name := "new-name"
	_, err := svc.Update(ctx, idStr(1), UpdateInput{Name: &name})
	if err != ErrSystemTemplate {
		t.Fatalf("expected ErrSystemTemplate, got %v", err)
	}
}

func TestServiceDeleteUserTemplate(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, &memSubsRepo{})
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateInput{
		Name:   "custom",
		Config: json.RawMessage(`{"duration_days":30,"device_limit":2}`),
	})
	err := svc.Delete(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.FindByID(ctx, tmpl.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestServiceCreateFromTemplate(t *testing.T) {
	repo := newMemRepo()
	subsRepo := &memSubsRepo{}
	planID := "plan-123"
	repo.addSystem("monthly-basic", &planID, `{"duration_days":30,"traffic_limit_bytes":107374182400,"device_limit":3}`)
	svc := NewService(repo, subsRepo)
	ctx := context.Background()

	sub, err := svc.CreateFromTemplate(ctx, idStr(1), "user-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.PlanID != "plan-123" || sub.UserID != "user-1" {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
}

func TestServiceCreateFromTemplateNoPlan(t *testing.T) {
	repo := newMemRepo()
	repo.addSystem("no-plan", nil, `{"duration_days":7,"device_limit":1}`)
	svc := NewService(repo, &memSubsRepo{})
	ctx := context.Background()

	_, err := svc.CreateFromTemplate(ctx, idStr(1), "user-1", nil)
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestServiceResolvePlanID(t *testing.T) {
	repo := newMemRepo()
	planID := "plan-456"
	repo.addSystem("yearly-basic", &planID, `{"duration_days":365,"device_limit":3}`)
	svc := NewService(repo, &memSubsRepo{})
	ctx := context.Background()

	resolved, err := svc.ResolvePlanID(ctx, idStr(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "plan-456" {
		t.Fatalf("expected plan-456, got %s", resolved)
	}
}

func TestServiceResolvePlanIDNotFound(t *testing.T) {
	svc := NewService(newMemRepo(), &memSubsRepo{})
	ctx := context.Background()

	_, err := svc.ResolvePlanID(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
