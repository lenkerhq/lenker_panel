package configrender

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/configbundle"
)

func TestRenderVLESSRealityPayloadDeterministic(t *testing.T) {
	input := RenderInput{
		NodeID:                 "node-1",
		RevisionNumber:         7,
		Hostname:               "node-1.example.com",
		Region:                 "eu",
		CountryCode:            "FI",
		RollbackTargetRevision: 6,
	}

	first := RenderVLESSRealityPayload(input)
	second := RenderVLESSRealityPayload(input)

	firstJSON := mustJSON(t, first)
	secondJSON := mustJSON(t, second)
	if firstJSON != secondJSON {
		t.Fatalf("expected deterministic render:\n%s\n---\n%s", firstJSON, secondJSON)
	}

	expected := `{"access_entries":[],"config":{"inbounds":[{"listen":"0.0.0.0","port":443,"protocol":"vless","settings":{"clients":[],"decryption":"none","fallbacks":[]},"sniffing":{"destOverride":["http","tls","quic"],"enabled":true},"streamSettings":{"network":"tcp","realitySettings":{"dest":"www.cloudflare.com:443","maxClientVer":"","maxTimeDiff":0,"minClientVer":"","privateKey":"lenker-placeholder-reality-private-key","serverNames":["www.cloudflare.com"],"shortIds":["lenker00"],"show":false,"xver":0},"security":"reality"},"tag":"vless-reality-in"}],"log":{"loglevel":"warning"},"outbounds":[{"protocol":"freedom","tag":"direct"}],"policy":{"levels":{"0":{"connIdle":300,"downlinkOnly":5,"handshake":4,"statsUserDownlink":true,"statsUserUplink":true,"uplinkOnly":2}},"system":{"statsInboundDownlink":true,"statsInboundUplink":true,"statsOutboundDownlink":true,"statsOutboundUplink":true}},"routing":{"domainStrategy":"AsIs","rules":[{"inboundTag":["vless-reality-in"],"outboundTag":"direct","type":"field"}]},"stats":{}},"config_kind":"xray-config-compatible-skeleton","config_text":"lenker xray vless reality skeleton node=node-1 revision=7 protocol=vless-reality-xtls-vision subscriptions=0","core_type":"xray","generated_by":"panel-api","node":{"country_code":"FI","hostname":"node-1.example.com","id":"node-1","region":"eu"},"operation_kind":"deploy","protocol":"vless-reality-xtls-vision","revision_number":7,"rollback_target_revision":6,"schema_version":"config-bundle.v1alpha1","subscription_inputs":[],"transport":{"network":"tcp","security":"reality","xtls":"vision"}}`
	if firstJSON != expected {
		t.Fatalf("unexpected render:\n%s", firstJSON)
	}
}

func TestRenderVLESSRealityPayloadHashStable(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{NodeID: "node-1", RevisionNumber: 7})
	firstHash, err := configbundle.HashPayload(payload)
	if err != nil {
		t.Fatalf("expected hash: %v", err)
	}
	secondHash, err := configbundle.HashPayload(payload)
	if err != nil {
		t.Fatalf("expected hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("expected stable hash")
	}
}

func TestRenderVLESSRealityPayloadPassesValidation(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		SubscriptionInputs: []SubscriptionInput{
			{SubscriptionID: "sub-a", UserID: "user-a", PlanID: "plan-a", UserStatus: "active", SubscriptionStatus: "active", DeviceLimit: 2, StartsAt: "2026-05-01T00:00:00Z", ExpiresAt: "2026-06-01T00:00:00Z"},
		},
	})

	if err := ValidateVLESSRealityPayload(payload); err != nil {
		t.Fatalf("expected rendered payload to pass validation: %v", err)
	}
}

func TestRenderVLESSRealityPayloadWithRealityOverridesEndpoint(t *testing.T) {
	payload := RenderVLESSRealityPayloadWithReality(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
	}, RealityConfig{
		VLESSPort:  8443,
		SNI:        "example.com",
		Dest:       "example.com:443",
		ShortID:    "abcd1234",
		PrivateKey: "private-key",
	})

	config := payload["config"].(map[string]any)
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	if inbound["port"] != 8443 {
		t.Fatalf("expected custom port, got %#v", inbound["port"])
	}
	streamSettings := inbound["streamSettings"].(map[string]any)
	realitySettings := streamSettings["realitySettings"].(map[string]any)
	if realitySettings["dest"] != "example.com:443" || realitySettings["privateKey"] != "private-key" {
		t.Fatalf("unexpected reality settings: %#v", realitySettings)
	}
	serverNames := realitySettings["serverNames"].([]any)
	shortIDs := realitySettings["shortIds"].([]any)
	if len(serverNames) != 1 || serverNames[0] != "example.com" || len(shortIDs) != 1 || shortIDs[0] != "abcd1234" {
		t.Fatalf("unexpected reality identity fields: %#v", realitySettings)
	}
}

func TestValidateVLESSRealityPayloadRejectsBrokenRoutingReference(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{NodeID: "node-1", RevisionNumber: 7})
	config := payload["config"].(map[string]any)
	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	rule := rules[0].(map[string]any)
	rule["outboundTag"] = "missing"

	err := ValidateVLESSRealityPayload(payload)
	if !errors.Is(err, ErrInvalidXrayConfig) {
		t.Fatalf("expected invalid xray config, got %v", err)
	}
	var validationErr ValidationError
	if !errors.As(err, &validationErr) || validationErr.Reason != "invalid_routing_outbound_reference" {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRenderVLESSRealityPayloadOrdersSubscriptionInputs(t *testing.T) {
	limit := int64(1024)
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		SubscriptionInputs: []SubscriptionInput{
			{SubscriptionID: "sub-b", UserID: "user-b", PlanID: "plan-b", UserStatus: "active", SubscriptionStatus: "active", PreferredRegion: "eu", PlanName: "Pro", DeviceLimit: 2, TrafficLimitBytes: &limit, StartsAt: "2026-05-01T00:00:00Z", ExpiresAt: "2026-06-01T00:00:00Z"},
			{SubscriptionID: "sub-a", UserID: "user-a", PlanID: "plan-a", UserStatus: "active", SubscriptionStatus: "active", PreferredRegion: "", PlanName: "Basic", DeviceLimit: 1, StartsAt: "2026-05-01T00:00:00Z", ExpiresAt: "2026-06-01T00:00:00Z"},
		},
	})

	subscriptions := payload["subscription_inputs"].([]any)
	first := subscriptions[0].(map[string]any)
	if first["subscription_id"] != "sub-a" {
		t.Fatalf("expected sorted subscription inputs, got %#v", subscriptions)
	}
	accessEntries := payload["access_entries"].([]any)
	firstAccess := accessEntries[0].(map[string]any)
	if firstAccess["vless_client_id"] != "sub-a" {
		t.Fatalf("expected sorted access entries, got %#v", accessEntries)
	}
	config := payload["config"].(map[string]any)
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	settings := inbound["settings"].(map[string]any)
	clients := settings["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("expected two rendered clients, got %#v", clients)
	}
	firstClient := clients[0].(map[string]any)
	if firstClient["id"] != "sub-a" || firstClient["flow"] != "xtls-rprx-vision" || firstClient["level"] != 0 {
		t.Fatalf("unexpected first client: %#v", firstClient)
	}
}

func TestRenderRollbackPayloadPreservesTargetConfig(t *testing.T) {
	target := RenderVLESSRealityPayload(RenderInput{NodeID: "node-1", RevisionNumber: 3, RollbackTargetRevision: 2})
	rollback, err := RenderRollbackPayload(target, RollbackInput{
		RevisionNumber:         5,
		RollbackTargetRevision: 4,
		SourceRevisionID:       "revision-3",
		SourceRevisionNumber:   3,
	})
	if err != nil {
		t.Fatalf("expected rollback payload: %v", err)
	}

	if rollback["operation_kind"] != OperationRollback {
		t.Fatalf("expected rollback operation kind, got %#v", rollback["operation_kind"])
	}
	if rollback["revision_number"] != 5 || rollback["rollback_target_revision"] != 4 {
		t.Fatalf("unexpected rollback revision metadata: %#v", rollback)
	}
	if rollback["source_revision_id"] != "revision-3" || rollback["source_revision_number"] != 3 {
		t.Fatalf("unexpected source revision metadata: %#v", rollback)
	}
	if mustJSON(t, rollback["config"]) != mustJSON(t, target["config"]) {
		t.Fatalf("rollback config must preserve target config")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("expected json: %v", err)
	}
	return string(body)
}

func TestRenderVLESSRealityPayloadWithRoutingRules(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		RoutingRules: []RoutingRuleInput{
			{RuleType: "geosite", Target: "category-ads", Action: "block"},
			{RuleType: "domain", Target: "blocked.com", Action: "block"},
			{RuleType: "geoip", Target: "cn", Action: "direct"},
		},
	})

	if err := ValidateVLESSRealityPayload(payload); err != nil {
		t.Fatalf("expected valid payload with routing rules: %v", err)
	}

	config := payload["config"].(map[string]any)

	// Should have 2 outbounds: direct + block
	outbounds := config["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(outbounds))
	}
	blockOutbound := outbounds[1].(map[string]any)
	if blockOutbound["tag"] != "block" || blockOutbound["protocol"] != "blackhole" {
		t.Fatalf("expected block/blackhole outbound, got %v", blockOutbound)
	}

	// Should have 4 routing rules: 3 custom + 1 default catch-all
	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 4 {
		t.Fatalf("expected 4 routing rules, got %d", len(rules))
	}

	// First rule should be geosite block
	firstRule := rules[0].(map[string]any)
	if firstRule["outboundTag"] != "block" {
		t.Fatalf("expected first rule outbound=block, got %v", firstRule["outboundTag"])
	}
	domains := firstRule["domain"].([]any)
	if len(domains) != 1 || domains[0] != "geosite:category-ads" {
		t.Fatalf("unexpected domain in first rule: %v", domains)
	}

	// Last rule should be the default catch-all
	lastRule := rules[3].(map[string]any)
	if lastRule["outboundTag"] != "direct" {
		t.Fatalf("expected last rule to be default catch-all with outbound=direct")
	}
}

func TestRenderVLESSRealityPayloadNoRoutingRulesStaysDefault(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
	})

	config := payload["config"].(map[string]any)
	outbounds := config["outbounds"].([]any)
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound when no routing rules, got %d", len(outbounds))
	}

	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected 1 routing rule (default), got %d", len(rules))
	}
}

func TestRenderVLESSRealityPayloadWithGlobalSettings(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		GlobalSettings: &GlobalSettingsInput{
			LogLevel:   "debug",
			Sniffing:   false,
			DNSServers: []string{"9.9.9.9"},
		},
	})

	if err := ValidateVLESSRealityPayload(payload); err != nil {
		t.Fatalf("expected valid payload with global settings: %v", err)
	}

	config := payload["config"].(map[string]any)

	// Log level should be debug
	logSection := config["log"].(map[string]any)
	if logSection["loglevel"] != "debug" {
		t.Fatalf("expected loglevel=debug, got %v", logSection["loglevel"])
	}

	// Sniffing should be false
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	sniffing := inbound["sniffing"].(map[string]any)
	if sniffing["enabled"] != false {
		t.Fatalf("expected sniffing.enabled=false, got %v", sniffing["enabled"])
	}

	// DNS should be present
	dns, ok := config["dns"].(map[string]any)
	if !ok {
		t.Fatalf("expected dns section in config")
	}
	servers := dns["servers"].([]any)
	if len(servers) != 1 || servers[0] != "9.9.9.9" {
		t.Fatalf("unexpected dns servers: %v", servers)
	}
}

func TestRenderVLESSRealityPayloadNilGlobalSettingsUsesDefaults(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
	})

	config := payload["config"].(map[string]any)
	logSection := config["log"].(map[string]any)
	if logSection["loglevel"] != "warning" {
		t.Fatalf("expected default loglevel=warning, got %v", logSection["loglevel"])
	}

	inbound := config["inbounds"].([]any)[0].(map[string]any)
	sniffing := inbound["sniffing"].(map[string]any)
	if sniffing["enabled"] != true {
		t.Fatalf("expected default sniffing.enabled=true")
	}

	// No DNS section when no servers configured
	if _, ok := config["dns"]; ok {
		t.Fatalf("expected no dns section with nil GlobalSettings")
	}
}

func TestRenderVLESSRealityPayloadWithWarp(t *testing.T) {
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		WarpCredentials: &WarpInput{
			PrivateKey: "warp-private-key",
			PublicKey:  "warp-public-key",
			Address:    "172.16.0.2/32",
			Endpoint:   "engage.cloudflareclient.com:2408",
		},
		RoutingRules: []RoutingRuleInput{
			{RuleType: "geoip", Target: "ru", Action: "warp"},
		},
	})

	if err := ValidateVLESSRealityPayload(payload); err != nil {
		t.Fatalf("expected valid payload with warp: %v", err)
	}

	config := payload["config"].(map[string]any)
	outbounds := config["outbounds"].([]any)

	// Should have direct + warp
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 outbounds (direct + warp), got %d", len(outbounds))
	}

	warpOutbound := outbounds[1].(map[string]any)
	if warpOutbound["tag"] != "warp" || warpOutbound["protocol"] != "wireguard" {
		t.Fatalf("expected warp/wireguard outbound, got %v", warpOutbound)
	}

	settings := warpOutbound["settings"].(map[string]any)
	if settings["secretKey"] != "warp-private-key" {
		t.Fatalf("unexpected secretKey: %v", settings["secretKey"])
	}

	// Routing rule should reference warp outbound
	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 2 { // custom + default
		t.Fatalf("expected 2 routing rules, got %d", len(rules))
	}
	firstRule := rules[0].(map[string]any)
	if firstRule["outboundTag"] != "warp" {
		t.Fatalf("expected first rule outbound=warp, got %v", firstRule["outboundTag"])
	}
}

func TestRenderVLESSRealityPayloadWarpWithoutCredentials(t *testing.T) {
	// Routing rule with action "warp" but no WarpCredentials — outbound "warp" won't exist
	// but the rule still renders (validator won't catch this at render time, it's a config issue)
	payload := RenderVLESSRealityPayload(RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 7,
		RoutingRules: []RoutingRuleInput{
			{RuleType: "geoip", Target: "ru", Action: "warp"},
		},
	})

	config := payload["config"].(map[string]any)
	outbounds := config["outbounds"].([]any)
	// Only direct outbound, no warp
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound without warp credentials, got %d", len(outbounds))
	}
}
