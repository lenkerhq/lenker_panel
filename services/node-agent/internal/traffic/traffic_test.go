package traffic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func TestNoopCollector(t *testing.T) {
	c := NoopCollector{}
	records, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestParseXrayStats(t *testing.T) {
	input := `stat: { name: "user>>>sub1@dev1>>>traffic>>>uplink" value: 1000 }
stat: { name: "user>>>sub1@dev1>>>traffic>>>downlink" value: 2000 }
stat: { name: "user>>>sub2>>>traffic>>>uplink" value: 500 }
stat: { name: "user>>>sub2>>>traffic>>>downlink" value: 1500 }
stat: { name: "user>>>sub1@dev1>>>traffic>>>uplink" value: 100 }
`
	records, err := parseXrayStats(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].SubscriptionID < records[j].SubscriptionID })

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	r1 := records[0]
	if r1.SubscriptionID != "sub1" || r1.DeviceID != "dev1" {
		t.Errorf("record 0: got sub=%q device=%q", r1.SubscriptionID, r1.DeviceID)
	}
	if r1.BytesUp != 1100 || r1.BytesDown != 2000 {
		t.Errorf("record 0: got up=%d down=%d", r1.BytesUp, r1.BytesDown)
	}

	r2 := records[1]
	if r2.SubscriptionID != "sub2" || r2.DeviceID != "" {
		t.Errorf("record 1: got sub=%q device=%q", r2.SubscriptionID, r2.DeviceID)
	}
	if r2.BytesUp != 500 || r2.BytesDown != 1500 {
		t.Errorf("record 1: got up=%d down=%d", r2.BytesUp, r2.BytesDown)
	}
}

func TestParseXrayStatsEmpty(t *testing.T) {
	records, err := parseXrayStats("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestReporterSuccess(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/traffic/report" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}

		var records []Record
		if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}

		body, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"node_id":     "node-1",
				"accepted":    1,
				"bytes_up":    records[0].BytesUp,
				"bytes_down":  records[0].BytesDown,
				"bytes_total": records[0].BytesUp + records[0].BytesDown,
			},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}

	reporter := &Reporter{PanelURL: "http://panel.example.com", NodeToken: "test-token", HTTPClient: client}
	result, err := reporter.Report(context.Background(), []Record{
		{SubscriptionID: "sub-1", BytesUp: 100, BytesDown: 200},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Accepted != 1 {
		t.Errorf("expected accepted=1, got %d", result.Accepted)
	}
	if result.BytesTotal != 300 {
		t.Errorf("expected bytes_total=300, got %d", result.BytesTotal)
	}
}

func TestReporterUnauthorized(t *testing.T) {
	reporter := &Reporter{
		PanelURL:  "http://panel.example.com",
		NodeToken: "bad-token",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
			}, nil
		})},
	}
	_, err := reporter.Report(context.Background(), []Record{
		{SubscriptionID: "sub-1", BytesUp: 100, BytesDown: 200},
	})
	if err != ErrReportAuth {
		t.Fatalf("expected ErrReportAuth, got %v", err)
	}
}

func TestReporterEmptyRecords(t *testing.T) {
	reporter := &Reporter{PanelURL: "http://localhost", NodeToken: "token"}
	result, err := reporter.Report(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty records")
	}
}

func TestParseIdentity(t *testing.T) {
	tests := []struct {
		input      string
		wantSub    string
		wantDevice string
	}{
		{"sub1@dev1", "sub1", "dev1"},
		{"sub1", "sub1", ""},
		{"sub@dev@extra", "sub", "dev@extra"},
	}
	for _, tt := range tests {
		sub, device := parseIdentity(tt.input)
		if sub != tt.wantSub || device != tt.wantDevice {
			t.Errorf("parseIdentity(%q) = (%q, %q), want (%q, %q)", tt.input, sub, device, tt.wantSub, tt.wantDevice)
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
