package traffic

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// XrayCollector collects traffic stats from Xray's stats API via the xray binary.
// It parses `xray api statsquery` output lines like:
//
//	user>>>sub_id@device_id>>>traffic>>>uplink  value: 12345
//	user>>>sub_id@device_id>>>traffic>>>downlink  value: 67890
type XrayCollector struct {
	XrayBin    string
	APIAddress string // e.g. "127.0.0.1:10085"
}

func (c *XrayCollector) Collect(ctx context.Context) ([]Record, error) {
	if c.XrayBin == "" || c.APIAddress == "" {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, c.XrayBin, "api", "statsquery", "-s", c.APIAddress, "-pattern", "user>>>", "-reset")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xray statsquery: %w", err)
	}

	return parseXrayStats(string(out))
}

func parseXrayStats(output string) ([]Record, error) {
	type key struct {
		sub    string
		device string
	}
	accum := map[key]*Record{}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, "user>>>") {
			continue
		}
		// Format: stat: { name: "user>>>SUB@DEV>>>traffic>>>uplink" value: 12345 }
		// Or simpler statsquery output: user>>>SUB@DEV>>>traffic>>>uplink  value: 12345
		name, value := parseStatLine(line)
		if name == "" {
			continue
		}

		parts := strings.Split(name, ">>>")
		if len(parts) < 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}

		identity := parts[1]
		direction := parts[3]

		sub, device := parseIdentity(identity)
		if sub == "" {
			continue
		}

		k := key{sub: sub, device: device}
		rec, ok := accum[k]
		if !ok {
			rec = &Record{SubscriptionID: sub, DeviceID: device}
			accum[k] = rec
		}

		switch direction {
		case "uplink":
			rec.BytesUp += value
		case "downlink":
			rec.BytesDown += value
		}
	}

	records := make([]Record, 0, len(accum))
	for _, rec := range accum {
		if rec.BytesUp > 0 || rec.BytesDown > 0 {
			records = append(records, *rec)
		}
	}
	return records, nil
}

func parseIdentity(identity string) (sub, device string) {
	at := strings.IndexByte(identity, '@')
	if at < 0 {
		return identity, ""
	}
	return identity[:at], identity[at+1:]
}

func parseStatLine(line string) (string, int64) {
	// Handle JSON-like format: name: "..." value: N
	nameStart := strings.Index(line, "name: \"")
	if nameStart >= 0 {
		nameStart += len("name: \"")
		nameEnd := strings.Index(line[nameStart:], "\"")
		if nameEnd < 0 {
			return "", 0
		}
		name := line[nameStart : nameStart+nameEnd]
		valIdx := strings.Index(line, "value: ")
		if valIdx < 0 {
			return name, 0
		}
		valStr := strings.TrimSpace(line[valIdx+len("value: "):])
		valStr = strings.TrimRight(valStr, " }")
		var v int64
		fmt.Sscanf(valStr, "%d", &v)
		return name, v
	}

	// Handle plain format: user>>>...>>>traffic>>>uplink  value: N
	valIdx := strings.LastIndex(line, "value:")
	if valIdx < 0 {
		return "", 0
	}
	name := strings.TrimSpace(line[:valIdx])
	valStr := strings.TrimSpace(line[valIdx+len("value:"):])
	var v int64
	fmt.Sscanf(valStr, "%d", &v)
	return name, v
}
