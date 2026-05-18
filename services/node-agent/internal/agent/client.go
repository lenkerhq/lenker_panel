package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrPanelURLRequired        = errors.New("panel url is required")
	ErrNodeTokenRequired       = errors.New("node token is required")
	ErrPendingRevisionAuth     = errors.New("pending config revision auth failed")
	ErrHeartbeatAuth           = errors.New("node heartbeat auth failed")
	ErrUnexpectedPanelResponse = errors.New("unexpected panel response")
)

type PendingConfigRevisionClient interface {
	FetchPendingConfigRevision(ctx context.Context, nodeID string, nodeToken string) (ConfigRevision, bool, error)
	ReportConfigRevision(ctx context.Context, nodeID string, nodeToken string, revisionID string, report ConfigRevisionReport) error
}

type HeartbeatClient interface {
	SendHeartbeat(ctx context.Context, nodeID string, nodeToken string, payload HeartbeatPayload) error
}

type PanelClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c PanelClient) FetchPendingConfigRevision(ctx context.Context, nodeID string, nodeToken string) (ConfigRevision, bool, error) {
	if strings.TrimSpace(nodeID) == "" {
		return ConfigRevision{}, false, ErrNodeIDRequired
	}
	if strings.TrimSpace(nodeToken) == "" {
		return ConfigRevision{}, false, ErrNodeTokenRequired
	}

	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return ConfigRevision{}, false, ErrPanelURLRequired
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/nodes/%s/config-revisions/pending", baseURL, url.PathEscape(nodeID)),
		nil,
	)
	if err != nil {
		return ConfigRevision{}, false, err
	}
	request.Header.Set("Authorization", "Bearer "+nodeToken)

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return ConfigRevision{}, false, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ConfigRevision{}, false, nil
	case http.StatusUnauthorized:
		return ConfigRevision{}, false, ErrPendingRevisionAuth
	default:
		return ConfigRevision{}, false, fmt.Errorf("%w: status %d", ErrUnexpectedPanelResponse, response.StatusCode)
	}

	var envelope struct {
		Data *ConfigRevision `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return ConfigRevision{}, false, fmt.Errorf("%w: %v", ErrUnexpectedPanelResponse, err)
	}
	if envelope.Data == nil {
		return ConfigRevision{}, false, ErrUnexpectedPanelResponse
	}

	return *envelope.Data, true, nil
}

func (c PanelClient) ReportConfigRevision(ctx context.Context, nodeID string, nodeToken string, revisionID string, report ConfigRevisionReport) error {
	if strings.TrimSpace(nodeID) == "" {
		return ErrNodeIDRequired
	}
	if strings.TrimSpace(nodeToken) == "" {
		return ErrNodeTokenRequired
	}
	if strings.TrimSpace(revisionID) == "" {
		return ErrInvalidConfigRevision
	}

	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return ErrPanelURLRequired
	}

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf(
			"%s/api/v1/nodes/%s/config-revisions/%s/report",
			baseURL,
			url.PathEscape(nodeID),
			url.PathEscape(revisionID),
		),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+nodeToken)
	request.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrPendingRevisionAuth
	case http.StatusNotFound:
		return ErrInvalidConfigRevision
	default:
		return fmt.Errorf("%w: status %d", ErrUnexpectedPanelResponse, response.StatusCode)
	}
}

func (c PanelClient) SendHeartbeat(ctx context.Context, nodeID string, nodeToken string, payload HeartbeatPayload) error {
	if strings.TrimSpace(nodeID) == "" {
		return ErrNodeIDRequired
	}
	if strings.TrimSpace(nodeToken) == "" {
		return ErrNodeTokenRequired
	}

	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return ErrPanelURLRequired
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/nodes/%s/heartbeat", baseURL, url.PathEscape(nodeID)),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+nodeToken)
	request.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrHeartbeatAuth
	case http.StatusNotFound:
		return ErrInvalidConfigRevision
	default:
		return fmt.Errorf("%w: status %d", ErrUnexpectedPanelResponse, response.StatusCode)
	}
}
