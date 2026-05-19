package traffic

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Service struct {
	repo         Repository
	nodeResolver NodeResolver
	now          func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) WithNodeResolver(resolver NodeResolver) *Service {
	if resolver != nil {
		s.nodeResolver = resolver
	}
	return s
}

func (s *Service) RecordTraffic(ctx context.Context, subscriptionID string, deviceID *string, nodeID string, bytesUp, bytesDown int64) error {
	if strings.TrimSpace(subscriptionID) == "" || strings.TrimSpace(nodeID) == "" {
		return ErrInvalidInput
	}
	if bytesUp < 0 || bytesDown < 0 {
		return ErrInvalidInput
	}

	if _, err := s.repo.CreateLog(ctx, TrafficLog{
		SubscriptionID: strings.TrimSpace(subscriptionID),
		DeviceID:       cleanStringPtr(deviceID),
		NodeID:         strings.TrimSpace(nodeID),
		BytesUp:        bytesUp,
		BytesDown:      bytesDown,
		RecordedAt:     s.now().UTC(),
	}); err != nil {
		return err
	}

	_, err := s.repo.IncrementQuota(ctx, strings.TrimSpace(subscriptionID), bytesUp+bytesDown)
	return err
}

func (s *Service) RecordReport(ctx context.Context, nodeToken string, entries []TrafficReportItem) (*TrafficReportResult, error) {
	if s.nodeResolver == nil || strings.TrimSpace(nodeToken) == "" {
		return nil, ErrUnauthorized
	}
	if len(entries) == 0 {
		return nil, ErrInvalidInput
	}

	cleaned := make([]TrafficReportItem, 0, len(entries))
	var bytesUp int64
	var bytesDown int64
	for _, entry := range entries {
		entry.SubscriptionID = strings.TrimSpace(entry.SubscriptionID)
		entry.DeviceID = cleanStringPtr(entry.DeviceID)
		if entry.SubscriptionID == "" || entry.BytesUp < 0 || entry.BytesDown < 0 {
			return nil, ErrInvalidInput
		}
		bytesUp += entry.BytesUp
		bytesDown += entry.BytesDown
		cleaned = append(cleaned, entry)
	}

	nodeID, err := s.nodeResolver.NodeIDByToken(ctx, strings.TrimSpace(nodeToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}

	reportedAt := s.now().UTC()
	for _, entry := range cleaned {
		if err := s.RecordTraffic(ctx, entry.SubscriptionID, entry.DeviceID, nodeID, entry.BytesUp, entry.BytesDown); err != nil {
			return nil, err
		}
	}

	return &TrafficReportResult{
		NodeID:     nodeID,
		Accepted:   len(cleaned),
		BytesUp:    bytesUp,
		BytesDown:  bytesDown,
		BytesTotal: bytesUp + bytesDown,
		ReportedAt: reportedAt,
	}, nil
}

func (s *Service) GetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, ErrInvalidInput
	}
	if err := s.ResetQuotaIfNeeded(ctx, subscriptionID); err != nil {
		return nil, err
	}
	return s.repo.GetQuota(ctx, strings.TrimSpace(subscriptionID))
}

func (s *Service) SetQuota(ctx context.Context, input SetQuotaInput) (*TrafficQuota, error) {
	input.SubscriptionID = strings.TrimSpace(input.SubscriptionID)
	if input.SubscriptionID == "" {
		return nil, ErrInvalidInput
	}
	if input.BytesLimit != nil && *input.BytesLimit <= 0 {
		return nil, ErrInvalidInput
	}
	if input.BytesUsed != nil && *input.BytesUsed < 0 {
		return nil, ErrInvalidInput
	}

	bytesUsed := int64(0)
	if input.BytesUsed != nil {
		bytesUsed = *input.BytesUsed
	} else if current, err := s.repo.GetQuota(ctx, input.SubscriptionID); err == nil {
		bytesUsed = current.BytesUsed
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	return s.repo.SetQuota(ctx, input, bytesUsed)
}

func (s *Service) ResetQuota(ctx context.Context, subscriptionID string) (*TrafficQuota, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ResetQuota(ctx, strings.TrimSpace(subscriptionID))
}

func (s *Service) CheckQuota(ctx context.Context, subscriptionID string) (used, limit int64, exceeded bool, err error) {
	quota, err := s.GetQuota(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	used = quota.BytesUsed
	if quota.BytesLimit != nil {
		limit = *quota.BytesLimit
		exceeded = quota.Exceeded
	}
	return used, limit, exceeded, nil
}

func (s *Service) ResetQuotaIfNeeded(ctx context.Context, subscriptionID string) error {
	quota, err := s.repo.GetQuota(ctx, strings.TrimSpace(subscriptionID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if quota.ResetAt != nil && !quota.ResetAt.After(s.now()) {
		_, err := s.repo.ResetQuota(ctx, strings.TrimSpace(subscriptionID))
		return err
	}
	return nil
}

func (s *Service) GetUsageBySubscription(ctx context.Context, subscriptionID string, from, to time.Time) (*TrafficUsage, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetUsageBySubscription(ctx, strings.TrimSpace(subscriptionID), from, to)
}

func (s *Service) GetUsageByDevice(ctx context.Context, deviceID string, from, to time.Time) (*TrafficUsage, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetUsageByDevice(ctx, strings.TrimSpace(deviceID), from, to)
}

func (s *Service) GetUsageByNode(ctx context.Context, nodeID string, from, to time.Time) (*TrafficUsage, error) {
	if strings.TrimSpace(nodeID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetUsageByNode(ctx, strings.TrimSpace(nodeID), from, to)
}

func cleanStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
