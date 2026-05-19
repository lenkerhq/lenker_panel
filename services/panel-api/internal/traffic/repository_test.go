package traffic

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestRepositoryCreateLogScansDerivedFields(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	db := &fakeQueryer{
		rows: []scanner{row("log-1", "sub-1", "dev-1", "node-1", int64(10), int64(20), now, now)},
	}
	repo := newRepositoryWithQueryer(db)
	deviceID := "dev-1"

	log, err := repo.CreateLog(context.Background(), TrafficLog{
		SubscriptionID: "sub-1",
		DeviceID:       &deviceID,
		NodeID:         "node-1",
		BytesUp:        10,
		BytesDown:      20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if log.BytesTotal != 30 || log.DeviceID == nil || *log.DeviceID != "dev-1" {
		t.Fatalf("unexpected scanned log: %#v", log)
	}
	if db.lastArgs[1] != "dev-1" {
		t.Fatalf("expected device id argument, got %#v", db.lastArgs[1])
	}
}

func TestRepositoryGetQuotaNotFound(t *testing.T) {
	db := &fakeQueryer{rows: []scanner{rowErr(sql.ErrNoRows)}}
	repo := newRepositoryWithQueryer(db)

	_, err := repo.GetQuota(context.Background(), "sub-404")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositoryGetQuotaScansNullableFields(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	resetAt := now.Add(24 * time.Hour)
	db := &fakeQueryer{
		rows: []scanner{row("quota-1", "sub-1", nil, int64(90), resetAt, now, now)},
	}
	repo := newRepositoryWithQueryer(db)

	quota, err := repo.GetQuota(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quota.BytesLimit != nil || quota.BytesRemaining != nil || quota.Exceeded {
		t.Fatalf("expected unlimited quota, got %#v", quota)
	}
	if quota.ResetAt == nil || !quota.ResetAt.Equal(resetAt) {
		t.Fatalf("expected reset_at to scan, got %#v", quota.ResetAt)
	}
}

func TestRepositorySetQuotaSyncsSubscription(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	limit := int64(100)
	db := &fakeQueryer{
		rows:       []scanner{row("quota-1", "sub-1", limit, int64(90), nil, now, now)},
		execResult: fakeResult(1),
	}
	repo := newRepositoryWithQueryer(db)

	quota, err := repo.SetQuota(context.Background(), SetQuotaInput{SubscriptionID: "sub-1", BytesLimit: &limit}, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quota.BytesRemaining == nil || *quota.BytesRemaining != 10 || quota.Exceeded {
		t.Fatalf("unexpected quota derived fields: %#v", quota)
	}
	if db.execCalls != 1 {
		t.Fatalf("expected subscription sync exec")
	}
	if db.lastExecArgs[2] != int64(90) {
		t.Fatalf("expected synced bytes_used, got %#v", db.lastExecArgs[2])
	}
}

func TestRepositorySetQuotaPropagatesSyncError(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	db := &fakeQueryer{
		rows:       []scanner{row("quota-1", "sub-404", nil, int64(0), nil, now, now)},
		execResult: fakeResult(0),
	}
	repo := newRepositoryWithQueryer(db)

	_, err := repo.SetQuota(context.Background(), SetQuotaInput{SubscriptionID: "sub-404"}, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected sync not found, got %v", err)
	}
}

func TestRepositoryIncrementQuotaReturnsExceeded(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	limit := int64(100)
	db := &fakeQueryer{
		rows:       []scanner{row("quota-1", "sub-1", limit, int64(120), nil, now, now)},
		execResult: fakeResult(1),
	}
	repo := newRepositoryWithQueryer(db)

	quota, err := repo.IncrementQuota(context.Background(), "sub-1", 40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !quota.Exceeded || quota.BytesRemaining == nil || *quota.BytesRemaining != 0 {
		t.Fatalf("expected exceeded quota, got %#v", quota)
	}
	if db.execCalls != 1 || db.lastExecArgs[1] != int64(40) {
		t.Fatalf("expected subscription usage increment, got calls=%d args=%#v", db.execCalls, db.lastExecArgs)
	}
}

func TestRepositoryUpdateQuotaSyncsSubscription(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	limit := int64(500)
	db := &fakeQueryer{
		rows:       []scanner{row("quota-1", "sub-1", limit, int64(250), nil, now, now)},
		execResult: fakeResult(1),
	}
	repo := newRepositoryWithQueryer(db)

	if err := repo.UpdateQuota(context.Background(), "sub-1", 250); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.execCalls != 1 || db.lastExecArgs[2] != int64(250) {
		t.Fatalf("expected subscription sync, got args=%#v", db.lastExecArgs)
	}
}

func TestRepositoryResetQuotaSuccess(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	limit := int64(500)
	db := &fakeQueryer{
		rows:       []scanner{row("quota-1", "sub-1", limit, int64(0), nil, now, now)},
		execResult: fakeResult(1),
	}
	repo := newRepositoryWithQueryer(db)

	quota, err := repo.ResetQuota(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quota.BytesUsed != 0 || quota.ResetAt != nil {
		t.Fatalf("expected reset quota, got %#v", quota)
	}
	if db.execCalls != 1 {
		t.Fatalf("expected subscription sync")
	}
}

func TestRepositoryResetQuotaReturnsNotFound(t *testing.T) {
	db := &fakeQueryer{rows: []scanner{rowErr(sql.ErrNoRows)}}
	repo := newRepositoryWithQueryer(db)

	_, err := repo.ResetQuota(context.Background(), "sub-404")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositoryUsageIncludesPeriodAndTotals(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	db := &fakeQueryer{rows: []scanner{row(int64(50), int64(70))}}
	repo := newRepositoryWithQueryer(db)

	usage, err := repo.GetUsageByNode(context.Background(), "node-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.ResourceType != "node" || usage.ResourceID != "node-1" || usage.BytesTotal != 120 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if usage.From == nil || !usage.From.Equal(from) || usage.To == nil || !usage.To.Equal(to) {
		t.Fatalf("expected period in usage: %#v", usage)
	}
}

func TestRepositoryUsageWrappers(t *testing.T) {
	db := &fakeQueryer{rows: []scanner{
		row(int64(1), int64(2)),
		row(int64(3), int64(4)),
	}}
	repo := newRepositoryWithQueryer(db)

	subscriptionUsage, err := repo.GetUsageBySubscription(context.Background(), "sub-1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected subscription usage error: %v", err)
	}
	if subscriptionUsage.ResourceType != "subscription" || subscriptionUsage.BytesTotal != 3 || subscriptionUsage.From != nil || subscriptionUsage.To != nil {
		t.Fatalf("unexpected subscription usage: %#v", subscriptionUsage)
	}

	deviceUsage, err := repo.GetUsageByDevice(context.Background(), "dev-1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected device usage error: %v", err)
	}
	if deviceUsage.ResourceType != "device" || deviceUsage.BytesTotal != 7 {
		t.Fatalf("unexpected device usage: %#v", deviceUsage)
	}
}

func TestRepositorySyncSubscriptionMissingReturnsNotFound(t *testing.T) {
	db := &fakeQueryer{execResult: fakeResult(0)}
	repo := newRepositoryWithQueryer(db)

	err := repo.syncSubscriptionQuota(context.Background(), "sub-404", nil, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type fakeQueryer struct {
	rows         []scanner
	lastArgs     []any
	lastExecArgs []any
	execCalls    int
	execResult   sql.Result
	execErr      error
}

func (f *fakeQueryer) queryRow(_ context.Context, _ string, args ...any) scanner {
	f.lastArgs = args
	if len(f.rows) == 0 {
		return rowErr(sql.ErrNoRows)
	}
	row := f.rows[0]
	f.rows = f.rows[1:]
	return row
}

func (f *fakeQueryer) exec(_ context.Context, _ string, args ...any) (sql.Result, error) {
	f.execCalls++
	f.lastExecArgs = args
	if f.execErr != nil {
		return nil, f.execErr
	}
	if f.execResult != nil {
		return f.execResult, nil
	}
	return fakeResult(1), nil
}

type fakeRow struct {
	values []any
	err    error
}

func row(values ...any) fakeRow {
	return fakeRow{values: values}
}

func rowErr(err error) fakeRow {
	return fakeRow{err: err}
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		assignScanValue(dest[i], r.values[i])
	}
	return nil
}

func assignScanValue(dest any, value any) {
	switch target := dest.(type) {
	case *string:
		*target = value.(string)
	case *int64:
		*target = value.(int64)
	case *time.Time:
		*target = value.(time.Time)
	case *sql.NullString:
		if value == nil {
			*target = sql.NullString{}
			return
		}
		*target = sql.NullString{String: value.(string), Valid: true}
	case *sql.NullInt64:
		if value == nil {
			*target = sql.NullInt64{}
			return
		}
		*target = sql.NullInt64{Int64: value.(int64), Valid: true}
	case *sql.NullTime:
		if value == nil {
			*target = sql.NullTime{}
			return
		}
		*target = sql.NullTime{Time: value.(time.Time), Valid: true}
	default:
		panic("unsupported scan destination")
	}
}

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return int64(r), nil }

type fakeResult int64
