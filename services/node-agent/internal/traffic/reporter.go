package traffic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrReportAuth     = errors.New("traffic report auth failed")
	ErrReportRejected = errors.New("traffic report rejected")
)

// Reporter sends collected traffic records to the panel API.
type Reporter struct {
	PanelURL   string
	NodeToken  string
	HTTPClient *http.Client
}

// ReportResult is the panel API response for a traffic report.
type ReportResult struct {
	NodeID     string `json:"node_id"`
	Accepted   int    `json:"accepted"`
	BytesUp    int64  `json:"bytes_up"`
	BytesDown  int64  `json:"bytes_down"`
	BytesTotal int64  `json:"bytes_total"`
}

func (r *Reporter) Report(ctx context.Context, records []Record) (*ReportResult, error) {
	if len(records) == 0 {
		return nil, nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(r.PanelURL), "/")
	if baseURL == "" {
		return nil, errors.New("panel url is required")
	}
	if strings.TrimSpace(r.NodeToken) == "" {
		return nil, ErrReportAuth
	}

	body, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/traffic/report", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.NodeToken)
	req.Header.Set("Content-Type", "application/json")

	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var envelope struct {
			Data *ReportResult `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			return nil, fmt.Errorf("decode traffic report response: %w", err)
		}
		return envelope.Data, nil
	case http.StatusUnauthorized:
		return nil, ErrReportAuth
	default:
		return nil, fmt.Errorf("%w: status %d", ErrReportRejected, resp.StatusCode)
	}
}
