package configrender

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidXrayConfig = errors.New("invalid xray config")

type ValidationError struct {
	Reason string
}

func (e ValidationError) Error() string {
	if e.Reason == "" {
		return ErrInvalidXrayConfig.Error()
	}
	return ErrInvalidXrayConfig.Error() + ": " + e.Reason
}

func (e ValidationError) Unwrap() error {
	return ErrInvalidXrayConfig
}

func ValidateVLESSRealityPayload(payload map[string]any) error {
	if payload == nil {
		return invalidXrayConfig("missing_payload")
	}
	if value, ok := payload["schema_version"].(string); !ok || value != SchemaVersion {
		return invalidXrayConfig("invalid_schema_version")
	}
	if value, ok := payload["generated_by"].(string); !ok || value != GeneratedBy {
		return invalidXrayConfig("invalid_generated_by")
	}
	if value, ok := payload["protocol"].(string); !ok || value != ProtocolVLESS {
		return invalidXrayConfig("invalid_protocol")
	}
	if value, ok := payload["core_type"].(string); !ok || value != CoreTypeXray {
		return invalidXrayConfig("invalid_core_type")
	}
	if value, ok := payload["config_kind"].(string); !ok || value != ConfigKind {
		return invalidXrayConfig("invalid_config_kind")
	}
	if value, ok := payload["operation_kind"].(string); !ok || (value != OperationDeploy && value != OperationRollback) {
		return invalidXrayConfig("invalid_operation_kind")
	}
	config, ok := payload["config"].(map[string]any)
	if !ok {
		return invalidXrayConfig("missing_config")
	}
	return ValidateVLESSRealityConfig(config)
}

func ValidateVLESSRealityConfig(config map[string]any) error {
	if config == nil {
		return invalidXrayConfig("missing_config")
	}
	if _, ok := config["log"].(map[string]any); !ok {
		return invalidXrayConfig("missing_log")
	}
	if _, ok := config["policy"].(map[string]any); !ok {
		return invalidXrayConfig("missing_policy")
	}
	if _, ok := config["stats"].(map[string]any); !ok {
		return invalidXrayConfig("missing_stats")
	}

	inbounds, ok := config["inbounds"].([]any)
	if !ok {
		return invalidXrayConfig("missing_inbounds")
	}
	if len(inbounds) != 1 {
		return invalidXrayConfig("invalid_inbound_count")
	}
	inbound, ok := inbounds[0].(map[string]any)
	if !ok {
		return invalidXrayConfig("invalid_inbound")
	}
	inboundTag, err := validateInbound(inbound)
	if err != nil {
		return err
	}

	outbounds, ok := config["outbounds"].([]any)
	if !ok || len(outbounds) == 0 {
		return invalidXrayConfig("missing_outbounds")
	}
	outboundTags, err := validateOutbounds(outbounds)
	if err != nil {
		return err
	}

	routing, ok := config["routing"].(map[string]any)
	if !ok {
		return invalidXrayConfig("missing_routing")
	}
	return validateRouting(routing, map[string]bool{inboundTag: true}, outboundTags)
}

func validateInbound(inbound map[string]any) (string, error) {
	tag, ok := stringField(inbound, "tag")
	if !ok {
		return "", invalidXrayConfig("missing_inbound_tag")
	}
	if protocol, ok := stringField(inbound, "protocol"); !ok || protocol != "vless" {
		return "", invalidXrayConfig("invalid_inbound_protocol")
	}
	if port, ok := numberAsInt(inbound["port"]); !ok || port <= 0 {
		return "", invalidXrayConfig("invalid_inbound_port")
	}
	settings, ok := inbound["settings"].(map[string]any)
	if !ok {
		return "", invalidXrayConfig("missing_inbound_settings")
	}
	if decryption, ok := stringField(settings, "decryption"); !ok || decryption != "none" {
		return "", invalidXrayConfig("invalid_vless_decryption")
	}
	clients, ok := settings["clients"].([]any)
	if !ok {
		return "", invalidXrayConfig("missing_vless_clients")
	}
	for index, rawClient := range clients {
		client, ok := rawClient.(map[string]any)
		if !ok {
			return "", invalidXrayConfig("invalid_vless_client")
		}
		if err := validateClient(client); err != nil {
			return "", fmt.Errorf("%w: client_%d", err, index)
		}
	}
	streamSettings, ok := inbound["streamSettings"].(map[string]any)
	if !ok {
		return "", invalidXrayConfig("missing_stream_settings")
	}
	if network, ok := stringField(streamSettings, "network"); !ok || network != "tcp" {
		return "", invalidXrayConfig("invalid_stream_network")
	}
	if security, ok := stringField(streamSettings, "security"); !ok || security != "reality" {
		return "", invalidXrayConfig("invalid_stream_security")
	}
	realitySettings, ok := streamSettings["realitySettings"].(map[string]any)
	if !ok {
		return "", invalidXrayConfig("missing_reality_settings")
	}
	if err := validateReality(realitySettings); err != nil {
		return "", err
	}
	return tag, nil
}

func validateClient(client map[string]any) error {
	if value, ok := stringField(client, "id"); !ok || value == "" {
		return invalidXrayConfig("missing_vless_client_id")
	}
	if value, ok := stringField(client, "email"); !ok || value == "" {
		return invalidXrayConfig("missing_vless_client_email")
	}
	if value, ok := stringField(client, "flow"); !ok || value != "xtls-rprx-vision" {
		return invalidXrayConfig("invalid_vless_client_flow")
	}
	if _, ok := numberAsInt(client["level"]); !ok {
		return invalidXrayConfig("missing_vless_client_level")
	}
	return nil
}

func validateReality(settings map[string]any) error {
	if _, ok := settings["show"].(bool); !ok {
		return invalidXrayConfig("missing_reality_show")
	}
	for _, key := range []string{"dest", "privateKey"} {
		if _, ok := stringField(settings, key); !ok {
			return invalidXrayConfig("missing_reality_" + key)
		}
	}
	for _, key := range []string{"minClientVer", "maxClientVer"} {
		if _, ok := settings[key].(string); !ok {
			return invalidXrayConfig("missing_reality_" + key)
		}
	}
	if values, ok := stringArray(settings["serverNames"]); !ok || len(values) == 0 {
		return invalidXrayConfig("missing_reality_server_names")
	}
	if _, ok := stringArray(settings["shortIds"]); !ok {
		return invalidXrayConfig("missing_reality_short_ids")
	}
	if _, ok := numberAsInt(settings["xver"]); !ok {
		return invalidXrayConfig("missing_reality_xver")
	}
	if _, ok := numberAsInt(settings["maxTimeDiff"]); !ok {
		return invalidXrayConfig("missing_reality_max_time_diff")
	}
	return nil
}

func validateOutbounds(outbounds []any) (map[string]bool, error) {
	tags := make(map[string]bool, len(outbounds))
	hasDirectFreedom := false
	for _, rawOutbound := range outbounds {
		outbound, ok := rawOutbound.(map[string]any)
		if !ok {
			return nil, invalidXrayConfig("invalid_outbound")
		}
		tag, ok := stringField(outbound, "tag")
		if !ok {
			return nil, invalidXrayConfig("missing_outbound_tag")
		}
		protocol, ok := stringField(outbound, "protocol")
		if !ok {
			return nil, invalidXrayConfig("missing_outbound_protocol")
		}
		if protocol != "freedom" && protocol != "blackhole" {
			return nil, invalidXrayConfig("unsupported_outbound_protocol")
		}
		tags[tag] = true
		if tag == "direct" && protocol == "freedom" {
			hasDirectFreedom = true
		}
	}
	if !hasDirectFreedom {
		return nil, invalidXrayConfig("missing_direct_freedom_outbound")
	}
	return tags, nil
}

func validateRouting(routing map[string]any, inboundTags map[string]bool, outboundTags map[string]bool) error {
	rules, ok := routing["rules"].([]any)
	if !ok || len(rules) == 0 {
		return invalidXrayConfig("missing_routing_rules")
	}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return invalidXrayConfig("invalid_routing_rule")
		}
		if ruleType, ok := stringField(rule, "type"); !ok || ruleType != "field" {
			return invalidXrayConfig("invalid_routing_rule_type")
		}
		outboundTag, ok := stringField(rule, "outboundTag")
		if !ok || !outboundTags[outboundTag] {
			return invalidXrayConfig("invalid_routing_outbound_reference")
		}
		// inboundTag is optional for custom routing rules (domain/ip/port matching).
		if rawInboundTags, ok := rule["inboundTag"]; ok {
			tags, ok := stringArray(rawInboundTags)
			if !ok {
				return invalidXrayConfig("invalid_routing_inbound_reference")
			}
			for _, inboundTag := range tags {
				if !inboundTags[inboundTag] {
					return invalidXrayConfig("invalid_routing_inbound_reference")
				}
			}
		}
	}
	return nil
}

func invalidXrayConfig(reason string) error {
	return ValidationError{Reason: reason}
}

func stringField(values map[string]any, key string) (string, bool) {
	value, ok := values[key].(string)
	if !ok {
		return "", false
	}
	return value, strings.TrimSpace(value) != ""
}

func stringArray(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func numberAsInt(value any) (int, bool) {
	switch typedValue := value.(type) {
	case int:
		return typedValue, true
	case int64:
		return int(typedValue), true
	case float64:
		if typedValue != float64(int(typedValue)) {
			return 0, false
		}
		return int(typedValue), true
	default:
		return 0, false
	}
}
