package devices

import (
	"context"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

// --- mocks ---

type mockRepo struct {
	devices map[string]*Device // keyed by id
}

func newMockRepo() *mockRepo {
	return &mockRepo{devices: make(map[string]*Device)}
}

func (m *mockRepo) Create(_ context.Context, d Device) (*Device, error) {
	d.ID = "dev-" + d.DeviceFingerprint
	d.IsActive = true
	d.FirstSeenAt = time.Now()
	d.LastSeenAt = time.Now()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.devices[d.ID] = &d
	return &d, nil
}

func (m *mockRepo) Update(_ context.Context, id string, d Device) (*Device, error) {
	existing, ok := m.devices[id]
	if !ok {
		return nil, ErrNotFound
	}
	if d.DeviceName != nil {
		existing.DeviceName = d.DeviceName
	}
	if d.Platform != nil {
		existing.Platform = d.Platform
	}
	if d.AppVersion != nil {
		existing.AppVersion = d.AppVersion
	}
	existing.UpdatedAt = time.Now()
	return existing, nil
}

func (m *mockRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.devices[id]; !ok {
		return ErrNotFound
	}
	delete(m.devices, id)
	return nil
}

func (m *mockRepo) FindByID(_ context.Context, id string) (*Device, error) {
	d, ok := m.devices[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (m *mockRepo) FindByFingerprint(_ context.Context, subID, fp string) (*Device, error) {
	for _, d := range m.devices {
		if d.SubscriptionID == subID && d.DeviceFingerprint == fp {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockRepo) ListBySubscription(_ context.Context, subID string) ([]*Device, error) {
	var result []*Device
	for _, d := range m.devices {
		if d.SubscriptionID == subID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockRepo) CountActiveBySubscription(_ context.Context, subID string) (int, error) {
	count := 0
	for _, d := range m.devices {
		if d.SubscriptionID == subID && d.IsActive {
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) MarkInactive(_ context.Context, id string) error {
	d, ok := m.devices[id]
	if !ok {
		return ErrNotFound
	}
	d.IsActive = false
	return nil
}

func (m *mockRepo) UpdateLastSeen(_ context.Context, id string, ip string) error {
	d, ok := m.devices[id]
	if !ok {
		return ErrNotFound
	}
	d.LastSeenAt = time.Now()
	if ip != "" {
		d.LastIP = &ip
	}
	return nil
}

type mockSubGetter struct {
	subs map[string]storage.Subscription
}

func (m *mockSubGetter) FindByID(_ context.Context, id string) (storage.Subscription, error) {
	s, ok := m.subs[id]
	if !ok {
		return storage.Subscription{}, storage.ErrNotFound
	}
	return s, nil
}

// --- tests ---

func TestRegisterDevice_NewDevice(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 3},
	}}
	svc := NewService(repo, subs)

	dev, err := svc.RegisterDevice(context.Background(), "sub-1", "fp-abc", "iPhone", "ios", "1.0", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.SubscriptionID != "sub-1" {
		t.Errorf("expected sub-1, got %s", dev.SubscriptionID)
	}
	if dev.DeviceFingerprint != "fp-abc" {
		t.Errorf("expected fp-abc, got %s", dev.DeviceFingerprint)
	}
}

func TestRegisterDevice_ExistingDevice(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 3},
	}}
	svc := NewService(repo, subs)

	// Register first time.
	_, err := svc.RegisterDevice(context.Background(), "sub-1", "fp-abc", "iPhone", "ios", "1.0", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Register again — should update, not create new.
	dev, err := svc.RegisterDevice(context.Background(), "sub-1", "fp-abc", "iPhone 15", "ios", "2.0", "5.6.7.8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.DeviceName == nil || *dev.DeviceName != "iPhone 15" {
		t.Errorf("expected updated name")
	}

	// Should still be 1 device.
	count, _ := repo.CountActiveBySubscription(context.Background(), "sub-1")
	if count != 1 {
		t.Errorf("expected 1 device, got %d", count)
	}
}

func TestRegisterDevice_LimitExceeded(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 2},
	}}
	svc := NewService(repo, subs)

	_, _ = svc.RegisterDevice(context.Background(), "sub-1", "fp-1", "Dev1", "ios", "1.0", "")
	_, _ = svc.RegisterDevice(context.Background(), "sub-1", "fp-2", "Dev2", "android", "1.0", "")

	// Third device should fail.
	_, err := svc.RegisterDevice(context.Background(), "sub-1", "fp-3", "Dev3", "windows", "1.0", "")
	if err != ErrDeviceLimitExceeded {
		t.Fatalf("expected ErrDeviceLimitExceeded, got %v", err)
	}
}

func TestEnforceDeviceLimit_UnderLimit(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 5},
	}}
	svc := NewService(repo, subs)

	if err := svc.EnforceDeviceLimit(context.Background(), "sub-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnforceDeviceLimit_AtLimit(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 1},
	}}
	svc := NewService(repo, subs)

	_, _ = svc.RegisterDevice(context.Background(), "sub-1", "fp-1", "Dev1", "ios", "1.0", "")

	err := svc.EnforceDeviceLimit(context.Background(), "sub-1")
	if err != ErrDeviceLimitExceeded {
		t.Fatalf("expected ErrDeviceLimitExceeded, got %v", err)
	}
}

func TestEnforceDeviceLimit_SubscriptionNotFound(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{}}
	svc := NewService(repo, subs)

	err := svc.EnforceDeviceLimit(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent subscription")
	}
}

func TestValidPlatform(t *testing.T) {
	for _, p := range []string{"ios", "android", "windows", "macos", "linux"} {
		if !ValidPlatform(p) {
			t.Errorf("expected %s to be valid", p)
		}
	}
	if ValidPlatform("freebsd") {
		t.Error("expected freebsd to be invalid")
	}
}
