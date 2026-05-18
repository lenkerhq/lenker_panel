package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/configrender"
)

type Config struct {
	AppEnv          string
	HTTPAddr        string
	DatabaseURL     string
	DatabasePing    bool
	LogLevel        string
	ShutdownTimeout time.Duration
	Reality         configrender.RealityConfig
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:          getenv("LENKER_APP_ENV", "development"),
		HTTPAddr:        getenv("LENKER_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("LENKER_DATABASE_URL"),
		DatabasePing:    getenv("LENKER_DATABASE_PING", "false") == "true",
		LogLevel:        getenv("LENKER_LOG_LEVEL", "info"),
		ShutdownTimeout: 10 * time.Second,
		Reality: configrender.RealityConfig{
			SNI:         getenv("LENKER_REALITY_SNI", configrender.DefaultRealitySNI),
			Dest:        getenv("LENKER_REALITY_DEST", configrender.DefaultRealityDest),
			ShortID:     getenv("LENKER_REALITY_SHORT_ID", configrender.DefaultRealityShortID),
			PrivateKey:  getenv("LENKER_REALITY_PRIVATE_KEY", configrender.DefaultRealityPrivate),
			PublicKey:   getenv("LENKER_REALITY_PUBLIC_KEY", configrender.DefaultRealityPublic),
			Fingerprint: getenv("LENKER_REALITY_FINGERPRINT", configrender.DefaultFingerprint),
			SpiderX:     getenv("LENKER_REALITY_SPIDER_X", configrender.DefaultSpiderX),
		},
	}

	if raw := os.Getenv("LENKER_SHUTDOWN_TIMEOUT_SECONDS"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return Config{}, errors.New("LENKER_SHUTDOWN_TIMEOUT_SECONDS must be a positive integer")
		}
		cfg.ShutdownTimeout = time.Duration(seconds) * time.Second
	}
	if raw := os.Getenv("LENKER_VLESS_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 || port > 65535 {
			return Config{}, errors.New("LENKER_VLESS_PORT must be an integer between 1 and 65535")
		}
		cfg.Reality.VLESSPort = port
	}
	cfg.Reality = cfg.Reality.WithDefaults()

	return cfg, nil
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
