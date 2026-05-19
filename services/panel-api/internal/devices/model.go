package devices

import (
	"errors"
	"time"
)

type Device struct {
	ID                string    `json:"id"`
	SubscriptionID    string    `json:"subscription_id"`
	DeviceFingerprint string    `json:"device_fingerprint"`
	DeviceName        *string   `json:"device_name"`
	Platform          *string   `json:"platform"`
	AppVersion        *string   `json:"app_version"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	LastIP            *string   `json:"last_ip"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

var validPlatforms = map[string]bool{
	"ios":     true,
	"android": true,
	"windows": true,
	"macos":   true,
	"linux":   true,
}

func ValidPlatform(p string) bool {
	return validPlatforms[p]
}

var (
	ErrDeviceLimitExceeded = errors.New("device_limit_exceeded")
	ErrNotFound            = errors.New("device not found")
)
