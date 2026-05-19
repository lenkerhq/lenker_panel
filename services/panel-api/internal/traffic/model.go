package traffic

import (
	"errors"
	"time"
)

type TrafficLog struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	DeviceID       *string   `json:"device_id"`
	NodeID         string    `json:"node_id"`
	BytesUp        int64     `json:"bytes_up"`
	BytesDown      int64     `json:"bytes_down"`
	BytesTotal     int64     `json:"bytes_total"`
	RecordedAt     time.Time `json:"recorded_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type TrafficQuota struct {
	ID             string     `json:"id"`
	SubscriptionID string     `json:"subscription_id"`
	BytesLimit     *int64     `json:"bytes_limit"`
	BytesUsed      int64      `json:"bytes_used"`
	BytesRemaining *int64     `json:"bytes_remaining"`
	Exceeded       bool       `json:"exceeded"`
	ResetAt        *time.Time `json:"reset_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type TrafficUsage struct {
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	BytesUp      int64      `json:"bytes_up"`
	BytesDown    int64      `json:"bytes_down"`
	BytesTotal   int64      `json:"bytes_total"`
	From         *time.Time `json:"from"`
	To           *time.Time `json:"to"`
}

type TrafficReportItem struct {
	SubscriptionID string  `json:"subscription_id"`
	DeviceID       *string `json:"device_id"`
	BytesUp        int64   `json:"bytes_up"`
	BytesDown      int64   `json:"bytes_down"`
}

type TrafficReportResult struct {
	NodeID     string    `json:"node_id"`
	Accepted   int       `json:"accepted"`
	BytesUp    int64     `json:"bytes_up"`
	BytesDown  int64     `json:"bytes_down"`
	BytesTotal int64     `json:"bytes_total"`
	ReportedAt time.Time `json:"reported_at"`
}

type SetQuotaInput struct {
	SubscriptionID string
	BytesLimit     *int64
	BytesUsed      *int64
	ResetAt        *time.Time
}

var (
	ErrNotFound      = errors.New("traffic resource not found")
	ErrInvalidInput  = errors.New("invalid traffic input")
	ErrUnauthorized  = errors.New("traffic report unauthorized")
	ErrQuotaExceeded = errors.New("traffic quota exceeded")
)

func (q TrafficQuota) WithDerivedFields() TrafficQuota {
	q.Exceeded = false
	q.BytesRemaining = nil
	if q.BytesLimit == nil {
		return q
	}
	remaining := *q.BytesLimit - q.BytesUsed
	if remaining < 0 {
		remaining = 0
	}
	q.BytesRemaining = &remaining
	q.Exceeded = q.BytesUsed >= *q.BytesLimit
	return q
}

func (u TrafficUsage) WithDerivedFields() TrafficUsage {
	u.BytesTotal = u.BytesUp + u.BytesDown
	return u
}

func (l TrafficLog) WithDerivedFields() TrafficLog {
	l.BytesTotal = l.BytesUp + l.BytesDown
	return l
}
