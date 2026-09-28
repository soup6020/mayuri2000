// Package config loads bot configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type Config struct {
	Token        string
	Prefix       string
	DatabasePath string
	DevGuildIDs  []snowflake.ID
	TenorAPIKey  string
	GeniusToken  string
	MagickPath   string
	LogLevel     slog.Level
	RSSInterval  time.Duration
}

// Load reads configuration from environment variables. Secrets may also be
// provided through a file by setting the variable name suffixed with _FILE
// (e.g. DISCORD_TOKEN_FILE), which works well with podman secrets.
func Load() (*Config, error) {
	token, err := secret("DISCORD_TOKEN")
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("DISCORD_TOKEN (or DISCORD_TOKEN_FILE) must be set")
	}
	tenor, err := secret("TENOR_API_KEY")
	if err != nil {
		return nil, err
	}
	genius, err := secret("GENIUS_TOKEN")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Token:        token,
		Prefix:       env("BOT_PREFIX", "%"),
		DatabasePath: env("DATABASE_PATH", "mayuri.db"),
		TenorAPIKey:  tenor,
		GeniusToken:  genius,
		MagickPath:   env("MAGICK_PATH", "magick"),
	}

	for _, raw := range strings.FieldsFunc(os.Getenv("DEV_GUILD_IDS"), func(r rune) bool { return r == ',' || r == ' ' }) {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid DEV_GUILD_IDS entry %q: %w", raw, err)
		}
		cfg.DevGuildIDs = append(cfg.DevGuildIDs, id)
	}

	cfg.RSSInterval, err = time.ParseDuration(env("RSS_INTERVAL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("invalid RSS_INTERVAL: %w", err)
	}
	if cfg.RSSInterval < time.Minute {
		return nil, errors.New("RSS_INTERVAL must be at least 1m")
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(env("LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}
	return cfg, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func secret(key string) (string, error) {
	if path := os.Getenv(key + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(os.Getenv(key)), nil
}
