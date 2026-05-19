package settings

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrUnknownKey = errors.New("unknown settings key")
var ErrInvalidValue = errors.New("invalid settings value")

// Supported setting keys.
const (
	KeyDefaultRoutingPreset = "default_routing_preset"
	KeyEnableWarpOutbound   = "enable_warp_outbound"
	KeyDefaultSniffing      = "default_sniffing"
	KeyDefaultFragment      = "default_fragment"
	KeyDefaultLogLevel      = "default_log_level"
	KeyDefaultDNSServers    = "default_dns_servers"
)

var SupportedKeys = []string{
	KeyDefaultRoutingPreset,
	KeyEnableWarpOutbound,
	KeyDefaultSniffing,
	KeyDefaultFragment,
	KeyDefaultLogLevel,
	KeyDefaultDNSServers,
}

var supportedKeySet = func() map[string]bool {
	m := make(map[string]bool, len(SupportedKeys))
	for _, k := range SupportedKeys {
		m[k] = true
	}
	return m
}()

func IsSupported(key string) bool { return supportedKeySet[key] }

// Setting represents a single global setting row.
type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description *string         `json:"description"`
	UpdatedBy   *string         `json:"updated_by"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Defaults returns the built-in default value for a key.
func Defaults() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		KeyDefaultRoutingPreset: json.RawMessage(`"standard"`),
		KeyEnableWarpOutbound:   json.RawMessage(`false`),
		KeyDefaultSniffing:      json.RawMessage(`true`),
		KeyDefaultFragment:      json.RawMessage(`null`),
		KeyDefaultLogLevel:      json.RawMessage(`"warning"`),
		KeyDefaultDNSServers:    json.RawMessage(`["1.1.1.1","8.8.8.8"]`),
	}
}

// Resolved holds typed settings for use in config rendering.
type Resolved struct {
	DefaultRoutingPreset string
	EnableWarpOutbound   bool
	DefaultSniffing      bool
	DefaultFragment      json.RawMessage
	DefaultLogLevel      string
	DefaultDNSServers    []string
}

// DefaultResolved returns built-in defaults as typed struct.
func DefaultResolved() Resolved {
	return Resolved{
		DefaultRoutingPreset: "standard",
		EnableWarpOutbound:   false,
		DefaultSniffing:      true,
		DefaultFragment:      nil,
		DefaultLogLevel:      "warning",
		DefaultDNSServers:    []string{"1.1.1.1", "8.8.8.8"},
	}
}

// Resolve converts a list of settings into a typed Resolved struct, filling defaults for missing keys.
func Resolve(settings []*Setting) Resolved {
	r := DefaultResolved()
	for _, s := range settings {
		switch s.Key {
		case KeyDefaultRoutingPreset:
			var v string
			if json.Unmarshal(s.Value, &v) == nil && v != "" {
				r.DefaultRoutingPreset = v
			}
		case KeyEnableWarpOutbound:
			var v bool
			if json.Unmarshal(s.Value, &v) == nil {
				r.EnableWarpOutbound = v
			}
		case KeyDefaultSniffing:
			var v bool
			if json.Unmarshal(s.Value, &v) == nil {
				r.DefaultSniffing = v
			}
		case KeyDefaultFragment:
			r.DefaultFragment = s.Value
		case KeyDefaultLogLevel:
			var v string
			if json.Unmarshal(s.Value, &v) == nil && v != "" {
				r.DefaultLogLevel = v
			}
		case KeyDefaultDNSServers:
			var v []string
			if json.Unmarshal(s.Value, &v) == nil {
				r.DefaultDNSServers = v
			}
		}
	}
	return r
}
