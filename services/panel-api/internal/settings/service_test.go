package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type memRepo struct {
	data map[string]*Setting
}

func newMemRepo() *memRepo { return &memRepo{data: make(map[string]*Setting)} }

func (m *memRepo) List(_ context.Context) ([]*Setting, error) {
	var result []*Setting
	for _, s := range m.data {
		result = append(result, s)
	}
	return result, nil
}

func (m *memRepo) Get(_ context.Context, key string) (*Setting, error) {
	s, ok := m.data[key]
	if !ok {
		return nil, ErrUnknownKey
	}
	return s, nil
}

func (m *memRepo) Set(_ context.Context, key string, value json.RawMessage, adminID string) (*Setting, error) {
	s := &Setting{Key: key, Value: value, UpdatedBy: &adminID, UpdatedAt: time.Now()}
	m.data[key] = s
	return s, nil
}

func TestServiceListAllReturnsDefaults(t *testing.T) {
	svc := NewService(newMemRepo())
	all, err := svc.ListAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != len(SupportedKeys) {
		t.Fatalf("expected %d settings, got %d", len(SupportedKeys), len(all))
	}
	for i, s := range all {
		if s.Key != SupportedKeys[i] {
			t.Errorf("expected key %q at index %d, got %q", SupportedKeys[i], i, s.Key)
		}
	}
}

func TestServiceUpdateValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	_, err := svc.Update(ctx, "unknown_key", json.RawMessage(`"x"`), "admin-1")
	if err != ErrUnknownKey {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}

	_, err = svc.Update(ctx, KeyDefaultLogLevel, json.RawMessage(`"invalid_level"`), "admin-1")
	if err != ErrInvalidValue {
		t.Fatalf("expected ErrInvalidValue for bad log level, got %v", err)
	}

	_, err = svc.Update(ctx, KeyEnableWarpOutbound, json.RawMessage(`"not_bool"`), "admin-1")
	if err != ErrInvalidValue {
		t.Fatalf("expected ErrInvalidValue for non-bool, got %v", err)
	}

	_, err = svc.Update(ctx, KeyDefaultDNSServers, json.RawMessage(`"not_array"`), "admin-1")
	if err != ErrInvalidValue {
		t.Fatalf("expected ErrInvalidValue for non-array dns, got %v", err)
	}
}

func TestServiceUpdateSuccess(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	s, err := svc.Update(ctx, KeyDefaultLogLevel, json.RawMessage(`"debug"`), "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Key != KeyDefaultLogLevel || string(s.Value) != `"debug"` {
		t.Fatalf("unexpected setting: %+v", s)
	}
}

func TestResolveDefaults(t *testing.T) {
	r := Resolve(nil)
	if r.DefaultLogLevel != "warning" || r.DefaultSniffing != true || r.EnableWarpOutbound != false {
		t.Fatalf("unexpected defaults: %+v", r)
	}
	if len(r.DefaultDNSServers) != 2 || r.DefaultDNSServers[0] != "1.1.1.1" {
		t.Fatalf("unexpected dns defaults: %v", r.DefaultDNSServers)
	}
}

func TestResolveOverrides(t *testing.T) {
	settings := []*Setting{
		{Key: KeyDefaultLogLevel, Value: json.RawMessage(`"debug"`)},
		{Key: KeyDefaultSniffing, Value: json.RawMessage(`false`)},
		{Key: KeyDefaultDNSServers, Value: json.RawMessage(`["9.9.9.9"]`)},
	}
	r := Resolve(settings)
	if r.DefaultLogLevel != "debug" {
		t.Fatalf("expected debug, got %s", r.DefaultLogLevel)
	}
	if r.DefaultSniffing != false {
		t.Fatalf("expected sniffing=false")
	}
	if len(r.DefaultDNSServers) != 1 || r.DefaultDNSServers[0] != "9.9.9.9" {
		t.Fatalf("unexpected dns: %v", r.DefaultDNSServers)
	}
}
