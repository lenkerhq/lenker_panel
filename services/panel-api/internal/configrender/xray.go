package configrender

import (
	"encoding/json"
	"fmt"
	"sort"
)

const (
	SchemaVersion = "config-bundle.v1alpha1"
	GeneratedBy   = "panel-api"
	ProtocolVLESS = "vless-reality-xtls-vision"
	CoreTypeXray  = "xray"
	ConfigKind    = "xray-config-compatible-skeleton"

	OperationDeploy   = "deploy"
	OperationRollback = "rollback"

	DefaultVLESSPort      = 443
	DefaultVLESSInbound   = "vless-reality-in"
	DefaultVLESSOutbound  = "direct"
	DefaultVLESSFlow      = "xtls-rprx-vision"
	DefaultRealitySNI     = "www.cloudflare.com"
	DefaultRealityDest    = "www.cloudflare.com:443"
	DefaultRealityShortID = "lenker00"
	DefaultRealityPrivate = "lenker-placeholder-reality-private-key"
	DefaultRealityPublic  = "lenker-placeholder-reality-public-key"
	DefaultFingerprint    = "chrome"
	DefaultSpiderX        = "/"
)

type RealityConfig struct {
	VLESSPort   int
	SNI         string
	Dest        string
	ShortID     string
	PrivateKey  string
	PublicKey   string
	Fingerprint string
	SpiderX     string
}

func DefaultRealityConfig() RealityConfig {
	return RealityConfig{
		VLESSPort:   DefaultVLESSPort,
		SNI:         DefaultRealitySNI,
		Dest:        DefaultRealityDest,
		ShortID:     DefaultRealityShortID,
		PrivateKey:  DefaultRealityPrivate,
		PublicKey:   DefaultRealityPublic,
		Fingerprint: DefaultFingerprint,
		SpiderX:     DefaultSpiderX,
	}
}

func (cfg RealityConfig) WithDefaults() RealityConfig {
	defaults := DefaultRealityConfig()
	if cfg.VLESSPort <= 0 {
		cfg.VLESSPort = defaults.VLESSPort
	}
	if cfg.SNI == "" {
		cfg.SNI = defaults.SNI
	}
	if cfg.Dest == "" {
		cfg.Dest = defaults.Dest
	}
	if cfg.ShortID == "" {
		cfg.ShortID = defaults.ShortID
	}
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = defaults.PrivateKey
	}
	if cfg.PublicKey == "" {
		cfg.PublicKey = defaults.PublicKey
	}
	if cfg.Fingerprint == "" {
		cfg.Fingerprint = defaults.Fingerprint
	}
	if cfg.SpiderX == "" {
		cfg.SpiderX = defaults.SpiderX
	}
	return cfg
}

type RenderInput struct {
	NodeID                 string
	RevisionNumber         int
	Hostname               string
	Region                 string
	CountryCode            string
	SubscriptionInputs     []SubscriptionInput
	RoutingRules           []RoutingRuleInput
	GlobalSettings         *GlobalSettingsInput
	WarpCredentials        *WarpInput
	RollbackTargetRevision int
}

// GlobalSettingsInput holds resolved global settings for config rendering.
type GlobalSettingsInput struct {
	LogLevel      string
	Sniffing      bool
	DNSServers    []string
}

// WarpInput holds WARP credentials for rendering a wireguard outbound.
type WarpInput struct {
	PrivateKey string
	PublicKey  string
	Address   string
	Endpoint  string
}

// RoutingRuleInput represents a routing rule to be rendered into the Xray config.
type RoutingRuleInput struct {
	RuleType string // geosite, geoip, domain, ip, port, protocol
	Target   string
	Action   string // block, proxy, direct, warp
}

type SubscriptionInput struct {
	SubscriptionID     string
	UserID             string
	PlanID             string
	UserStatus         string
	SubscriptionStatus string
	PreferredRegion    string
	PlanName           string
	DeviceLimit        int
	TrafficLimitBytes  *int64
	StartsAt           string
	ExpiresAt          string
}

type AccessEntry struct {
	SubscriptionID    string
	UserID            string
	PlanID            string
	VLESSClientID     string
	Email             string
	Flow              string
	DeviceLimit       int
	TrafficLimitBytes *int64
	ExpiresAt         string
}

type RollbackInput struct {
	RevisionNumber         int
	RollbackTargetRevision int
	SourceRevisionID       string
	SourceRevisionNumber   int
}

func RenderVLESSRealityPayload(input RenderInput) map[string]any {
	return RenderVLESSRealityPayloadWithReality(input, DefaultRealityConfig())
}

func RenderVLESSRealityPayloadWithReality(input RenderInput, reality RealityConfig) map[string]any {
	reality = reality.WithDefaults()
	inboundTag := DefaultVLESSInbound
	outboundTag := DefaultVLESSOutbound
	subscriptionInputs := sortedSubscriptionInputs(input.SubscriptionInputs)
	accessEntries := renderAccessEntries(subscriptionInputs)
	subscriptionSummary := renderSubscriptionSummary(subscriptionInputs)

	// Resolve global settings with defaults.
	logLevel := "warning"
	sniffingEnabled := true
	var dnsServers []string
	if input.GlobalSettings != nil {
		if input.GlobalSettings.LogLevel != "" {
			logLevel = input.GlobalSettings.LogLevel
		}
		sniffingEnabled = input.GlobalSettings.Sniffing
		dnsServers = input.GlobalSettings.DNSServers
	}

	outbounds := []any{
		map[string]any{
			"tag":      outboundTag,
			"protocol": "freedom",
		},
	}

	// Determine if we need a blackhole outbound for block rules.
	needsBlock := false
	for _, rule := range input.RoutingRules {
		if rule.Action == "block" {
			needsBlock = true
			break
		}
	}
	if needsBlock {
		outbounds = append(outbounds, map[string]any{
			"tag":      "block",
			"protocol": "blackhole",
		})
	}

	// Add WARP (WireGuard) outbound if credentials are provided.
	if input.WarpCredentials != nil {
		outbounds = append(outbounds, map[string]any{
			"tag":      "warp",
			"protocol": "wireguard",
			"settings": map[string]any{
				"secretKey": input.WarpCredentials.PrivateKey,
				"address":   []any{input.WarpCredentials.Address},
				"peers": []any{
					map[string]any{
						"publicKey": input.WarpCredentials.PublicKey,
						"endpoint":  input.WarpCredentials.Endpoint,
					},
				},
			},
		})
	}

	// Build custom routing rules before the default catch-all.
	customRules := renderCustomRoutingRules(input.RoutingRules, inboundTag)

	// Default catch-all rule.
	defaultRule := map[string]any{
		"type":        "field",
		"inboundTag":  []any{inboundTag},
		"outboundTag": outboundTag,
	}

	allRules := make([]any, 0, len(customRules)+1)
	allRules = append(allRules, customRules...)
	allRules = append(allRules, defaultRule)

	configMap := map[string]any{
		"log": map[string]any{
			"loglevel": logLevel,
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"handshake":         4,
					"connIdle":          300,
					"uplinkOnly":        2,
					"downlinkOnly":      5,
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds": []any{
			map[string]any{
				"tag":      inboundTag,
				"listen":   "0.0.0.0",
				"port":     reality.VLESSPort,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    renderClients(accessEntries),
					"decryption": "none",
					"fallbacks":  []any{},
				},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show":         false,
						"dest":         reality.Dest,
						"xver":         0,
						"serverNames":  []any{reality.SNI},
						"privateKey":   reality.PrivateKey,
						"shortIds":     []any{reality.ShortID},
						"minClientVer": "",
						"maxClientVer": "",
						"maxTimeDiff":  0,
					},
				},
				"sniffing": map[string]any{
					"enabled":      sniffingEnabled,
					"destOverride": []any{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules":          allRules,
		},
	}

	if len(dnsServers) > 0 {
		servers := make([]any, len(dnsServers))
		for i, s := range dnsServers {
			servers[i] = s
		}
		configMap["dns"] = map[string]any{"servers": servers}
	}

	return map[string]any{
		"schema_version":           SchemaVersion,
		"generated_by":             GeneratedBy,
		"protocol":                 ProtocolVLESS,
		"revision_number":          input.RevisionNumber,
		"rollback_target_revision": input.RollbackTargetRevision,
		"operation_kind":           OperationDeploy,
		"node": map[string]any{
			"id":           input.NodeID,
			"hostname":     input.Hostname,
			"region":       input.Region,
			"country_code": input.CountryCode,
		},
		"core_type": CoreTypeXray,
		"transport": map[string]any{
			"network":  "tcp",
			"security": "reality",
			"xtls":     "vision",
		},
		"config_kind":        ConfigKind,
		"config":             configMap,
		"subscription_inputs": subscriptionSummary,
		"access_entries":      accessEntries,
		"config_text": fmt.Sprintf(
			"lenker xray vless reality skeleton node=%s revision=%d protocol=%s subscriptions=%d",
			input.NodeID,
			input.RevisionNumber,
			ProtocolVLESS,
			len(subscriptionInputs),
		),
	}
}

func RenderRollbackPayload(target map[string]any, input RollbackInput) (map[string]any, error) {
	payload, err := clonePayload(target)
	if err != nil {
		return nil, err
	}

	payload["revision_number"] = input.RevisionNumber
	payload["rollback_target_revision"] = input.RollbackTargetRevision
	payload["operation_kind"] = OperationRollback
	payload["source_revision_id"] = input.SourceRevisionID
	payload["source_revision_number"] = input.SourceRevisionNumber
	payload["config_kind"] = ConfigKind
	payload["config_text"] = fmt.Sprintf(
		"lenker xray vless reality rollback skeleton revision=%d source_revision=%d",
		input.RevisionNumber,
		input.SourceRevisionNumber,
	)

	return payload, nil
}

func clonePayload(target map[string]any) (map[string]any, error) {
	body, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func sortedSubscriptionInputs(inputs []SubscriptionInput) []SubscriptionInput {
	result := append([]SubscriptionInput(nil), inputs...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].SubscriptionID != result[j].SubscriptionID {
			return result[i].SubscriptionID < result[j].SubscriptionID
		}
		if result[i].UserID != result[j].UserID {
			return result[i].UserID < result[j].UserID
		}
		return result[i].PlanID < result[j].PlanID
	})
	return result
}

func renderSubscriptionSummary(inputs []SubscriptionInput) []any {
	result := make([]any, 0, len(inputs))
	for _, input := range inputs {
		entry := map[string]any{
			"subscription_id":     input.SubscriptionID,
			"user_id":             input.UserID,
			"plan_id":             input.PlanID,
			"user_status":         input.UserStatus,
			"subscription_status": input.SubscriptionStatus,
			"preferred_region":    input.PreferredRegion,
			"plan_name":           input.PlanName,
			"device_limit":        input.DeviceLimit,
			"starts_at":           input.StartsAt,
			"expires_at":          input.ExpiresAt,
		}
		if input.TrafficLimitBytes != nil {
			entry["traffic_limit_bytes"] = *input.TrafficLimitBytes
		} else {
			entry["traffic_limit_bytes"] = nil
		}
		result = append(result, entry)
	}
	return result
}

func renderAccessEntries(inputs []SubscriptionInput) []any {
	result := make([]any, 0, len(inputs))
	for _, input := range inputs {
		access := BuildAccessEntry(input)
		entry := map[string]any{
			"subscription_id": access.SubscriptionID,
			"user_id":         access.UserID,
			"plan_id":         access.PlanID,
			"vless_client_id": access.VLESSClientID,
			"email":           access.Email,
			"flow":            access.Flow,
			"device_limit":    access.DeviceLimit,
			"expires_at":      access.ExpiresAt,
		}
		if access.TrafficLimitBytes != nil {
			entry["traffic_limit_bytes"] = *access.TrafficLimitBytes
		} else {
			entry["traffic_limit_bytes"] = nil
		}
		result = append(result, entry)
	}
	return result
}

func BuildAccessEntry(input SubscriptionInput) AccessEntry {
	return AccessEntry{
		SubscriptionID:    input.SubscriptionID,
		UserID:            input.UserID,
		PlanID:            input.PlanID,
		VLESSClientID:     input.SubscriptionID,
		Email:             fmt.Sprintf("subscription:%s", input.SubscriptionID),
		Flow:              DefaultVLESSFlow,
		DeviceLimit:       input.DeviceLimit,
		TrafficLimitBytes: input.TrafficLimitBytes,
		ExpiresAt:         input.ExpiresAt,
	}
}

func renderClients(accessEntries []any) []any {
	result := make([]any, 0, len(accessEntries))
	for _, raw := range accessEntries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"id":    entry["vless_client_id"],
			"email": entry["email"],
			"flow":  entry["flow"],
			"level": 0,
		})
	}
	return result
}

func renderCustomRoutingRules(rules []RoutingRuleInput, inboundTag string) []any {
	var result []any
	for _, rule := range rules {
		outbound := actionToOutbound(rule.Action)
		xrayRule := map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"inboundTag":  []any{inboundTag},
		}
		switch rule.RuleType {
		case "domain":
			xrayRule["domain"] = []any{rule.Target}
		case "geosite":
			xrayRule["domain"] = []any{"geosite:" + rule.Target}
		case "geoip":
			xrayRule["ip"] = []any{"geoip:" + rule.Target}
		case "ip":
			xrayRule["ip"] = []any{rule.Target}
		case "port":
			xrayRule["port"] = rule.Target
		case "protocol":
			xrayRule["protocol"] = []any{rule.Target}
		default:
			continue
		}
		result = append(result, xrayRule)
	}
	return result
}

func actionToOutbound(action string) string {
	switch action {
	case "block":
		return "block"
	case "direct":
		return "direct"
	case "warp":
		return "warp"
	case "proxy":
		return "direct"
	default:
		return "direct"
	}
}
