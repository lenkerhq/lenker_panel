package settings

import (
	"context"
	"encoding/json"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListAll returns all supported keys with their current values (or defaults if not in DB).
func (s *Service) ListAll(ctx context.Context) ([]*Setting, error) {
	stored, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]*Setting, len(stored))
	for _, st := range stored {
		byKey[st.Key] = st
	}

	defaults := Defaults()
	result := make([]*Setting, 0, len(SupportedKeys))
	for _, key := range SupportedKeys {
		if st, ok := byKey[key]; ok {
			result = append(result, st)
		} else {
			result = append(result, &Setting{Key: key, Value: defaults[key]})
		}
	}
	return result, nil
}

// Update validates and persists a setting value.
func (s *Service) Update(ctx context.Context, key string, value json.RawMessage, adminID string) (*Setting, error) {
	if !IsSupported(key) {
		return nil, ErrUnknownKey
	}
	if err := validateValue(key, value); err != nil {
		return nil, err
	}
	return s.repo.Set(ctx, key, value, adminID)
}

// GetResolved returns typed settings for config rendering.
func (s *Service) GetResolved(ctx context.Context) (Resolved, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return DefaultResolved(), err
	}
	return Resolve(all), nil
}

func validateValue(key string, value json.RawMessage) error {
	if !json.Valid(value) {
		return ErrInvalidValue
	}
	switch key {
	case KeyDefaultRoutingPreset:
		var v string
		if json.Unmarshal(value, &v) != nil || v == "" {
			return ErrInvalidValue
		}
	case KeyEnableWarpOutbound:
		var v bool
		if json.Unmarshal(value, &v) != nil {
			return ErrInvalidValue
		}
	case KeyDefaultSniffing:
		var v bool
		if json.Unmarshal(value, &v) != nil {
			return ErrInvalidValue
		}
	case KeyDefaultFragment:
		// null or object
		if string(value) != "null" {
			var v map[string]any
			if json.Unmarshal(value, &v) != nil {
				return ErrInvalidValue
			}
		}
	case KeyDefaultLogLevel:
		var v string
		if json.Unmarshal(value, &v) != nil {
			return ErrInvalidValue
		}
		valid := map[string]bool{"debug": true, "info": true, "warning": true, "error": true, "none": true}
		if !valid[v] {
			return ErrInvalidValue
		}
	case KeyDefaultDNSServers:
		var v []string
		if json.Unmarshal(value, &v) != nil {
			return ErrInvalidValue
		}
	}
	return nil
}
