package traffic

import "context"

// Record represents a single traffic measurement for a subscription.
type Record struct {
	SubscriptionID string `json:"subscription_id"`
	DeviceID       string `json:"device_id,omitempty"`
	BytesUp        int64  `json:"bytes_up"`
	BytesDown      int64  `json:"bytes_down"`
}

// Collector gathers traffic records from the underlying VPN runtime.
type Collector interface {
	Collect(ctx context.Context) ([]Record, error)
}

// NoopCollector returns no records. Used when no runtime is configured.
type NoopCollector struct{}

func (NoopCollector) Collect(context.Context) ([]Record, error) { return nil, nil }
