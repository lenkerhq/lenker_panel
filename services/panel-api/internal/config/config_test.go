package config

import (
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/configrender"
)

func TestLoadRealityConfigFromEnv(t *testing.T) {
	t.Setenv("LENKER_VLESS_PORT", "8443")
	t.Setenv("LENKER_REALITY_SNI", "example.com")
	t.Setenv("LENKER_REALITY_DEST", "example.com:443")
	t.Setenv("LENKER_REALITY_SHORT_ID", "abcd1234")
	t.Setenv("LENKER_REALITY_PRIVATE_KEY", "private-key")
	t.Setenv("LENKER_REALITY_PUBLIC_KEY", "public-key")
	t.Setenv("LENKER_REALITY_FINGERPRINT", "firefox")
	t.Setenv("LENKER_REALITY_SPIDER_X", "/spider")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config: %v", err)
	}

	expected := configrender.RealityConfig{
		VLESSPort:   8443,
		SNI:         "example.com",
		Dest:        "example.com:443",
		ShortID:     "abcd1234",
		PrivateKey:  "private-key",
		PublicKey:   "public-key",
		Fingerprint: "firefox",
		SpiderX:     "/spider",
	}
	if cfg.Reality != expected {
		t.Fatalf("unexpected reality config: %#v", cfg.Reality)
	}
}

func TestLoadRejectsInvalidVLESSPort(t *testing.T) {
	t.Setenv("LENKER_VLESS_PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatalf("expected invalid port error")
	}
}
