package traffic

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordTrafficCreatesLogAndIncrementsQuota(t *testing.T) {
	repo := &mockTrafficRepo{
		quota: quotaFixture(1000, 100),
	}
	service := NewService(repo)
	service.now = func() time.Time { return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC) }
	deviceID := "dev-1"

	err := service.RecordTraffic(context.Background(), " sub-1 ", &deviceID, " node-1 ", 10, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Fatalf("expected one log, got %d", len(repo.logs))
	}
	if repo.logs[0].SubscriptionID != "sub-1" || repo.logs[0].NodeID != "node-1" {
		t.Fatalf("expected trimmed ids in log: %#v", repo.logs[0])
	}
	if repo.incrementDelta != 30 {
		t.Fatalf("expected increment delta 30, got %d", repo.incrementDelta)
	}
}

func TestRecordTrafficRejectsInvalidInput(t *testing.T) {
	repo := &mockTrafficRepo{}
	service := NewService(repo)

	err := service.RecordTraffic(context.Background(), "", nil, "node-1", 1, 1)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for missing subscription id, got %v", err)
	}
	err = service.RecordTraffic(context.Background(), "sub-1", nil, "node-1", -1, 1)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for negative bytes, got %v", err)
	}
	if len(repo.logs) != 0 {
		t.Fatalf("invalid input should not create logs")
	}
}

func TestRecordReportResolvesNodeAndRecordsEntries(t *testing.T) {
	repo := &mockTrafficRepo{quota: quotaFixture(1000, 0)}
	resolver := &mockNodeResolver{nodeID: "node-1"}
	service := NewService(repo).WithNodeResolver(resolver)
	service.now = func() time.Time { return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC) }
	deviceID := " dev-1 "

	result, err := service.RecordReport(context.Background(), " node-token ", []TrafficReportItem{
		{SubscriptionID: " sub-1 ", DeviceID: &deviceID, BytesUp: 10, BytesDown: 20},
		{SubscriptionID: "sub-2", BytesUp: 30, BytesDown: 40},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolver.token != "node-token" {
		t.Fatalf("expected trimmed node token, got %q", resolver.token)
	}
	if result.NodeID != "node-1" || result.Accepted != 2 || result.BytesTotal != 100 {
		t.Fatalf("unexpected report result: %#v", result)
	}
	if len(repo.logs) != 2 {
		t.Fatalf("expected two traffic logs, got %d", len(repo.logs))
	}
	if repo.logs[0].DeviceID == nil || *repo.logs[0].DeviceID != "dev-1" {
		t.Fatalf("expected trimmed device id, got %#v", repo.logs[0].DeviceID)
	}
}

func TestRecordReportRejectsMissingResolverOrToken(t *testing.T) {
	service := NewService(&mockTrafficRepo{})

	_, err := service.RecordReport(context.Background(), "node-token", []TrafficReportItem{{SubscriptionID: "sub-1"}})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized without resolver, got %v", err)
	}
	service.WithNodeResolver(&mockNodeResolver{nodeID: "node-1"})
	_, err = service.RecordReport(context.Background(), "", []TrafficReportItem{{SubscriptionID: "sub-1"}})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized without token, got %v", err)
	}
}

func TestRecordReportRejectsInvalidEntriesBeforeWriting(t *testing.T) {
	repo := &mockTrafficRepo{}
	service := NewService(repo).WithNodeResolver(&mockNodeResolver{nodeID: "node-1"})

	_, err := service.RecordReport(context.Background(), "node-token", []TrafficReportItem{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for empty report, got %v", err)
	}
	_, err = service.RecordReport(context.Background(), "node-token", []TrafficReportItem{{SubscriptionID: "", BytesUp: 1}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for missing subscription, got %v", err)
	}
	_, err = service.RecordReport(context.Background(), "node-token", []TrafficReportItem{{SubscriptionID: "sub-1", BytesDown: -1}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for negative bytes, got %v", err)
	}
	if len(repo.logs) != 0 {
		t.Fatalf("invalid report should not write logs")
	}
}

func TestRecordReportInvalidToken(t *testing.T) {
	service := NewService(&mockTrafficRepo{}).WithNodeResolver(&mockNodeResolver{err: ErrNotFound})

	_, err := service.RecordReport(context.Background(), "bad-token", []TrafficReportItem{{SubscriptionID: "sub-1"}})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized for missing node, got %v", err)
	}
}

func TestSetQuotaPreservesExistingBytesUsedWhenOmitted(t *testing.T) {
	repo := &mockTrafficRepo{quota: quotaFixture(1000, 345)}
	service := NewService(repo)
	limit := int64(2048)

	quota, err := service.SetQuota(context.Background(), SetQuotaInput{
		SubscriptionID: "sub-1",
		BytesLimit:     &limit,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.setBytesUsed != 345 {
		t.Fatalf("expected existing bytes_used to be preserved, got %d", repo.setBytesUsed)
	}
	if quota.BytesLimit == nil || *quota.BytesLimit != limit {
		t.Fatalf("expected updated limit, got %#v", quota.BytesLimit)
	}
}

func TestSetQuotaUsesExplicitBytesUsed(t *testing.T) {
	repo := &mockTrafficRepo{quota: quotaFixture(1000, 345)}
	service := NewService(repo)
	limit := int64(2048)
	used := int64(128)

	_, err := service.SetQuota(context.Background(), SetQuotaInput{
		SubscriptionID: "sub-1",
		BytesLimit:     &limit,
		BytesUsed:      &used,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.setBytesUsed != used {
		t.Fatalf("expected explicit bytes_used %d, got %d", used, repo.setBytesUsed)
	}
}

func TestSetQuotaRejectsInvalidLimitAndUsage(t *testing.T) {
	service := NewService(&mockTrafficRepo{})
	limit := int64(0)
	used := int64(-1)

	if _, err := service.SetQuota(context.Background(), SetQuotaInput{SubscriptionID: "sub-1", BytesLimit: &limit}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid limit error, got %v", err)
	}
	if _, err := service.SetQuota(context.Background(), SetQuotaInput{SubscriptionID: "sub-1", BytesUsed: &used}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid usage error, got %v", err)
	}
}

func TestCheckQuotaExceeded(t *testing.T) {
	repo := &mockTrafficRepo{quota: quotaFixture(100, 120)}
	service := NewService(repo)

	used, limit, exceeded, err := service.CheckQuota(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != 120 || limit != 100 || !exceeded {
		t.Fatalf("unexpected quota status: used=%d limit=%d exceeded=%v", used, limit, exceeded)
	}
}

func TestCheckQuotaMissingMeansUnlimited(t *testing.T) {
	repo := &mockTrafficRepo{getQuotaErr: ErrNotFound}
	service := NewService(repo)

	used, limit, exceeded, err := service.CheckQuota(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != 0 || limit != 0 || exceeded {
		t.Fatalf("expected unlimited missing quota, got used=%d limit=%d exceeded=%v", used, limit, exceeded)
	}
}

func TestResetQuotaIfNeededResetsPastQuota(t *testing.T) {
	resetAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	repo := &mockTrafficRepo{quota: quotaFixture(1000, 900)}
	repo.quota.ResetAt = &resetAt
	service := NewService(repo)
	service.now = func() time.Time { return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC) }

	if err := service.ResetQuotaIfNeeded(context.Background(), "sub-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.resetCalled {
		t.Fatalf("expected quota reset")
	}
}

func TestResetQuotaIfNeededSkipsFutureQuota(t *testing.T) {
	resetAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	repo := &mockTrafficRepo{quota: quotaFixture(1000, 900)}
	repo.quota.ResetAt = &resetAt
	service := NewService(repo)
	service.now = func() time.Time { return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC) }

	if err := service.ResetQuotaIfNeeded(context.Background(), "sub-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.resetCalled {
		t.Fatalf("future quota reset_at should not reset")
	}
}

func TestUsageRejectsMissingIDs(t *testing.T) {
	service := NewService(&mockTrafficRepo{})
	if _, err := service.GetUsageBySubscription(context.Background(), "", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid subscription id, got %v", err)
	}
	if _, err := service.GetUsageByDevice(context.Background(), "", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid device id, got %v", err)
	}
	if _, err := service.GetUsageByNode(context.Background(), "", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid node id, got %v", err)
	}
}

type mockTrafficRepo struct {
	logs           []TrafficLog
	quota          *TrafficQuota
	getQuotaErr    error
	incrementDelta int64
	setBytesUsed   int64
	resetCalled    bool
}

type mockNodeResolver struct {
	nodeID string
	token  string
	err    error
}

func (m *mockNodeResolver) NodeIDByToken(_ context.Context, nodeToken string) (string, error) {
	m.token = nodeToken
	if m.err != nil {
		return "", m.err
	}
	return m.nodeID, nil
}

func (m *mockTrafficRepo) CreateLog(_ context.Context, log TrafficLog) (*TrafficLog, error) {
	m.logs = append(m.logs, log)
	log.ID = "traffic-log-1"
	return &log, nil
}

func (m *mockTrafficRepo) GetQuota(_ context.Context, subscriptionID string) (*TrafficQuota, error) {
	if m.getQuotaErr != nil {
		return nil, m.getQuotaErr
	}
	if m.quota == nil {
		return nil, ErrNotFound
	}
	quota := *m.quota
	quota.SubscriptionID = subscriptionID
	quota = quota.WithDerivedFields()
	return &quota, nil
}

func (m *mockTrafficRepo) SetQuota(_ context.Context, input SetQuotaInput, bytesUsed int64) (*TrafficQuota, error) {
	m.setBytesUsed = bytesUsed
	quota := TrafficQuota{ID: "quota-1", SubscriptionID: input.SubscriptionID, BytesLimit: input.BytesLimit, BytesUsed: bytesUsed}
	quota = quota.WithDerivedFields()
	m.quota = &quota
	return &quota, nil
}

func (m *mockTrafficRepo) UpdateQuota(_ context.Context, _ string, bytesUsed int64) error {
	if m.quota == nil {
		return ErrNotFound
	}
	m.quota.BytesUsed = bytesUsed
	return nil
}

func (m *mockTrafficRepo) IncrementQuota(_ context.Context, _ string, bytesDelta int64) (*TrafficQuota, error) {
	m.incrementDelta = bytesDelta
	if m.quota == nil {
		m.quota = quotaFixture(0, bytesDelta)
		return m.quota, nil
	}
	m.quota.BytesUsed += bytesDelta
	quota := m.quota.WithDerivedFields()
	return &quota, nil
}

func (m *mockTrafficRepo) ResetQuota(_ context.Context, subscriptionID string) (*TrafficQuota, error) {
	m.resetCalled = true
	if m.quota == nil {
		return nil, ErrNotFound
	}
	m.quota.SubscriptionID = subscriptionID
	m.quota.BytesUsed = 0
	m.quota.ResetAt = nil
	quota := m.quota.WithDerivedFields()
	return &quota, nil
}

func (m *mockTrafficRepo) GetUsageBySubscription(_ context.Context, subscriptionID string, from, to time.Time) (*TrafficUsage, error) {
	usage := TrafficUsage{ResourceType: "subscription", ResourceID: subscriptionID, BytesUp: 1, BytesDown: 2, From: timePtrIfSet(from), To: timePtrIfSet(to)}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (m *mockTrafficRepo) GetUsageByDevice(_ context.Context, deviceID string, from, to time.Time) (*TrafficUsage, error) {
	usage := TrafficUsage{ResourceType: "device", ResourceID: deviceID, BytesUp: 3, BytesDown: 4, From: timePtrIfSet(from), To: timePtrIfSet(to)}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (m *mockTrafficRepo) GetUsageByNode(_ context.Context, nodeID string, from, to time.Time) (*TrafficUsage, error) {
	usage := TrafficUsage{ResourceType: "node", ResourceID: nodeID, BytesUp: 5, BytesDown: 6, From: timePtrIfSet(from), To: timePtrIfSet(to)}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func quotaFixture(limit int64, used int64) *TrafficQuota {
	var limitPtr *int64
	if limit > 0 {
		limitPtr = &limit
	}
	quota := TrafficQuota{ID: "quota-1", SubscriptionID: "sub-1", BytesLimit: limitPtr, BytesUsed: used}
	quota = quota.WithDerivedFields()
	return &quota
}
