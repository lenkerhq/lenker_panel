package devices

import (
	"context"
	"errors"

	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

// SubscriptionGetter provides device_limit lookup for enforcement.
type SubscriptionGetter interface {
	FindByID(ctx context.Context, id string) (storage.Subscription, error)
}

type Service struct {
	repo          Repository
	subscriptions SubscriptionGetter
}

func NewService(repo Repository, subscriptions SubscriptionGetter) *Service {
	return &Service{repo: repo, subscriptions: subscriptions}
}

// RegisterDevice creates a new device or updates an existing one (by fingerprint).
// Returns ErrDeviceLimitExceeded if the subscription's device_limit is reached.
func (s *Service) RegisterDevice(ctx context.Context, subscriptionID, fingerprint, name, platform, appVersion, ip string) (*Device, error) {
	// Check if device already exists for this subscription+fingerprint.
	existing, err := s.repo.FindByFingerprint(ctx, subscriptionID, fingerprint)
	if err == nil {
		// Device exists — update last_seen and metadata.
		if ip != "" {
			_ = s.repo.UpdateLastSeen(ctx, existing.ID, ip)
		}
		// Re-activate if inactive.
		if !existing.IsActive {
			if err := s.enforceLimit(ctx, subscriptionID); err != nil {
				return nil, err
			}
		}
		updated := Device{DeviceName: strPtr(name), Platform: strPtr(platform), AppVersion: strPtr(appVersion)}
		dev, err := s.repo.Update(ctx, existing.ID, updated)
		if err != nil {
			return nil, err
		}
		return dev, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// New device — enforce limit.
	if err := s.enforceLimit(ctx, subscriptionID); err != nil {
		return nil, err
	}

	dev, err := s.repo.Create(ctx, Device{
		SubscriptionID:    subscriptionID,
		DeviceFingerprint: fingerprint,
		DeviceName:        strPtr(name),
		Platform:          strPtr(platform),
		AppVersion:        strPtr(appVersion),
		LastIP:            strPtr(ip),
	})
	if err != nil {
		return nil, err
	}
	return dev, nil
}

// EnforceDeviceLimit checks if the subscription has room for another active device.
func (s *Service) EnforceDeviceLimit(ctx context.Context, subscriptionID string) error {
	return s.enforceLimit(ctx, subscriptionID)
}

func (s *Service) enforceLimit(ctx context.Context, subscriptionID string) error {
	sub, err := s.subscriptions.FindByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	count, err := s.repo.CountActiveBySubscription(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if count >= sub.DeviceLimit {
		return ErrDeviceLimitExceeded
	}
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
