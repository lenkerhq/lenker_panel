package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	"github.com/lenker/lenker/services/panel-api/internal/traffic"
)

func TestSubscriptionTrafficSuccess(t *testing.T) {
	service := &fakeTrafficService{}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	from := "2026-05-01T00:00:00Z"
	to := "2026-05-19T00:00:00Z"
	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/traffic?from="+from+"&to="+to, nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.SubscriptionTraffic(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if service.subscriptionID != "sub-1" || service.from.IsZero() || service.to.IsZero() {
		t.Fatalf("expected subscription usage period to reach service")
	}
	if !strings.Contains(response.Body.String(), `"bytes_total":30`) {
		t.Fatalf("expected usage response, got %s", response.Body.String())
	}
}

func TestSubscriptionTrafficInvalidPeriod(t *testing.T) {
	handler := NewTrafficHandler(nil, &fakeTrafficService{}, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/traffic?from=not-a-date", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.SubscriptionTraffic(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestDeviceAndNodeTrafficSuccess(t *testing.T) {
	service := &fakeTrafficService{}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)

	deviceRequest := httptest.NewRequest(http.MethodGet, "/api/v1/devices/dev-1/traffic", nil)
	deviceRequest.SetPathValue("id", "dev-1")
	deviceResponse := httptest.NewRecorder()
	handler.DeviceTraffic(deviceResponse, deviceRequest)
	if deviceResponse.Code != http.StatusOK || service.deviceID != "dev-1" {
		t.Fatalf("expected device usage success, code=%d device=%q", deviceResponse.Code, service.deviceID)
	}

	nodeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/traffic", nil)
	nodeRequest.SetPathValue("id", "node-1")
	nodeResponse := httptest.NewRecorder()
	handler.NodeTraffic(nodeResponse, nodeRequest)
	if nodeResponse.Code != http.StatusOK || service.nodeID != "node-1" {
		t.Fatalf("expected node usage success, code=%d node=%q", nodeResponse.Code, service.nodeID)
	}
}

func TestGetQuotaSuccess(t *testing.T) {
	limit := int64(1000)
	service := &fakeTrafficService{quota: quotaResponse("sub-1", &limit, 250)}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/quota", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.GetQuota(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"bytes_remaining":750`) {
		t.Fatalf("expected quota response, got %s", response.Body.String())
	}
}

func TestGetQuotaNotFound(t *testing.T) {
	service := &fakeTrafficService{err: traffic.ErrNotFound}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-404/quota", nil)
	request.SetPathValue("id", "sub-404")
	response := httptest.NewRecorder()

	handler.GetQuota(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestSetQuotaSuccessAudits(t *testing.T) {
	limit := int64(1000)
	recorder := &trafficAuditRecorder{}
	service := &fakeTrafficService{quota: quotaResponse("sub-1", &limit, 100)}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly).WithAudit(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/quota", strings.NewReader(`{"bytes_limit":1000,"bytes_used":100}`))
	request.SetPathValue("id", "sub-1")
	request = request.WithContext(auth.WithAdmin(request.Context(), trafficTestAdmin()))
	response := httptest.NewRecorder()

	handler.SetQuota(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if service.setInput.SubscriptionID != "sub-1" || service.setInput.BytesLimit == nil || *service.setInput.BytesLimit != 1000 {
		t.Fatalf("expected quota input to reach service: %#v", service.setInput)
	}
	assertTrafficAudit(t, recorder.events, audit.ActionTrafficQuotaSet, audit.OutcomeSuccess)
}

func TestSetQuotaValidationError(t *testing.T) {
	recorder := &trafficAuditRecorder{}
	handler := NewTrafficHandler(nil, &fakeTrafficService{}, trafficTestAdminOnly).WithAudit(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/quota", strings.NewReader(`{"bytes_limit":0}`))
	request.SetPathValue("id", "sub-1")
	request = request.WithContext(auth.WithAdmin(request.Context(), trafficTestAdmin()))
	response := httptest.NewRecorder()

	handler.SetQuota(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("validation error should not write audit event")
	}
}

func TestSetQuotaFailureAudits(t *testing.T) {
	recorder := &trafficAuditRecorder{}
	service := &fakeTrafficService{err: traffic.ErrNotFound}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly).WithAudit(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-404/quota", strings.NewReader(`{"bytes_limit":100}`))
	request.SetPathValue("id", "sub-404")
	request = request.WithContext(auth.WithAdmin(request.Context(), trafficTestAdmin()))
	response := httptest.NewRecorder()

	handler.SetQuota(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
	assertTrafficAudit(t, recorder.events, audit.ActionTrafficQuotaSet, audit.OutcomeFailure)
}

func TestResetQuotaSuccessAudits(t *testing.T) {
	recorder := &trafficAuditRecorder{}
	service := &fakeTrafficService{quota: quotaResponse("sub-1", nil, 0)}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly).WithAudit(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/quota/reset", nil)
	request.SetPathValue("id", "sub-1")
	request = request.WithContext(auth.WithAdmin(request.Context(), trafficTestAdmin()))
	response := httptest.NewRecorder()

	handler.ResetQuota(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if service.resetID != "sub-1" {
		t.Fatalf("expected reset id, got %q", service.resetID)
	}
	assertTrafficAudit(t, recorder.events, audit.ActionTrafficQuotaReset, audit.OutcomeSuccess)
}

func TestResetQuotaStorageError(t *testing.T) {
	recorder := &trafficAuditRecorder{}
	service := &fakeTrafficService{err: errors.New("database down")}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly).WithAudit(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/quota/reset", nil)
	request.SetPathValue("id", "sub-1")
	request = request.WithContext(auth.WithAdmin(request.Context(), trafficTestAdmin()))
	response := httptest.NewRecorder()

	handler.ResetQuota(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
	assertTrafficAudit(t, recorder.events, audit.ActionTrafficQuotaReset, audit.OutcomeFailure)
}

func TestReportTrafficSuccess(t *testing.T) {
	service := &fakeTrafficService{}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/traffic/report", strings.NewReader(`[
		{"subscription_id":"sub-1","device_id":"dev-1","bytes_up":10,"bytes_down":20},
		{"subscription_id":"sub-2","bytes_up":30,"bytes_down":40}
	]`))
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.ReportTraffic(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if service.reportToken != "node-token" || len(service.reportEntries) != 2 {
		t.Fatalf("expected report to reach service, token=%q entries=%#v", service.reportToken, service.reportEntries)
	}
	if !strings.Contains(response.Body.String(), `"accepted":2`) || !strings.Contains(response.Body.String(), `"bytes_total":100`) {
		t.Fatalf("expected report response, got %s", response.Body.String())
	}
}

func TestReportTrafficRequiresBearerToken(t *testing.T) {
	service := &fakeTrafficService{}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/traffic/report", strings.NewReader(`[]`))
	response := httptest.NewRecorder()

	handler.ReportTraffic(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
	if service.reportToken != "" {
		t.Fatalf("report service should not be called without token")
	}
}

func TestReportTrafficRejectsInvalidJSON(t *testing.T) {
	handler := NewTrafficHandler(nil, &fakeTrafficService{}, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/traffic/report", strings.NewReader(`{`))
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.ReportTraffic(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestReportTrafficInvalidNodeToken(t *testing.T) {
	service := &fakeTrafficService{err: traffic.ErrUnauthorized}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/traffic/report", strings.NewReader(`[{"subscription_id":"sub-1"}]`))
	request.Header.Set("Authorization", "Bearer bad-token")
	response := httptest.NewRecorder()

	handler.ReportTraffic(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRegisterTrafficRoutes(t *testing.T) {
	service := &fakeTrafficService{quota: quotaResponse("sub-1", nil, 0)}
	handler := NewTrafficHandler(nil, service, trafficTestAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/quota", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected route to be registered, got %d", response.Code)
	}
}

type fakeTrafficService struct {
	err            error
	quota          *traffic.TrafficQuota
	subscriptionID string
	deviceID       string
	nodeID         string
	from           time.Time
	to             time.Time
	setInput       traffic.SetQuotaInput
	resetID        string
	reportToken    string
	reportEntries  []traffic.TrafficReportItem
}

func (s *fakeTrafficService) GetUsageBySubscription(_ context.Context, subscriptionID string, from, to time.Time) (*traffic.TrafficUsage, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.subscriptionID = subscriptionID
	s.from = from
	s.to = to
	usage := traffic.TrafficUsage{ResourceType: "subscription", ResourceID: subscriptionID, BytesUp: 10, BytesDown: 20}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (s *fakeTrafficService) GetUsageByDevice(_ context.Context, deviceID string, from, to time.Time) (*traffic.TrafficUsage, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.deviceID = deviceID
	usage := traffic.TrafficUsage{ResourceType: "device", ResourceID: deviceID, BytesUp: 1, BytesDown: 2}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (s *fakeTrafficService) GetUsageByNode(_ context.Context, nodeID string, from, to time.Time) (*traffic.TrafficUsage, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.nodeID = nodeID
	usage := traffic.TrafficUsage{ResourceType: "node", ResourceID: nodeID, BytesUp: 3, BytesDown: 4}
	usage = usage.WithDerivedFields()
	return &usage, nil
}

func (s *fakeTrafficService) GetQuota(_ context.Context, subscriptionID string) (*traffic.TrafficQuota, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.quota != nil {
		return s.quota, nil
	}
	return quotaResponse(subscriptionID, nil, 0), nil
}

func (s *fakeTrafficService) SetQuota(_ context.Context, input traffic.SetQuotaInput) (*traffic.TrafficQuota, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.setInput = input
	if s.quota != nil {
		return s.quota, nil
	}
	return quotaResponse(input.SubscriptionID, input.BytesLimit, 0), nil
}

func (s *fakeTrafficService) ResetQuota(_ context.Context, subscriptionID string) (*traffic.TrafficQuota, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.resetID = subscriptionID
	return quotaResponse(subscriptionID, nil, 0), nil
}

func (s *fakeTrafficService) RecordReport(_ context.Context, nodeToken string, entries []traffic.TrafficReportItem) (*traffic.TrafficReportResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.reportToken = nodeToken
	s.reportEntries = entries
	var bytesUp int64
	var bytesDown int64
	for _, entry := range entries {
		bytesUp += entry.BytesUp
		bytesDown += entry.BytesDown
	}
	return &traffic.TrafficReportResult{
		NodeID:     "node-1",
		Accepted:   len(entries),
		BytesUp:    bytesUp,
		BytesDown:  bytesDown,
		BytesTotal: bytesUp + bytesDown,
		ReportedAt: time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC),
	}, nil
}

type trafficAuditRecorder struct {
	events []audit.Event
}

func (r *trafficAuditRecorder) Record(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func trafficTestAdminOnly(next http.Handler) http.Handler {
	return next
}

func trafficTestAdmin() admins.Admin {
	return admins.Admin{ID: "admin-1", Email: "owner@example.com", Status: "active"}
}

func quotaResponse(subscriptionID string, limit *int64, used int64) *traffic.TrafficQuota {
	quota := traffic.TrafficQuota{ID: "quota-1", SubscriptionID: subscriptionID, BytesLimit: limit, BytesUsed: used}
	quota = quota.WithDerivedFields()
	return &quota
}

func assertTrafficAudit(t *testing.T, events []audit.Event, action string, outcome string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("expected audit event")
	}
	event := events[len(events)-1]
	if event.Action != action || event.Outcome != outcome {
		t.Fatalf("unexpected audit event: %#v", event)
	}
	if event.ResourceType != "traffic_quota" {
		t.Fatalf("unexpected resource type: %q", event.ResourceType)
	}
}
