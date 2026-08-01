package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddress     string
	DatabasePath      string
	TimeZone          string
	Location          *time.Location
	AdminUsername     string
	AdminPasswordHash string
	AdminCookieSecure bool
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress:     envOrDefault("KINOPS_LISTEN_ADDRESS", ":8081"),
		DatabasePath:      envOrDefault("KINOPS_DATABASE_PATH", "./data/kinops.db"),
		TimeZone:          envOrDefault("KINOPS_TIMEZONE", "America/New_York"),
		AdminUsername:     os.Getenv("KINOPS_ADMIN_USERNAME"),
		AdminPasswordHash: os.Getenv("KINOPS_ADMIN_PASSWORD_HASH"),
	}
	if (cfg.AdminUsername == "") != (cfg.AdminPasswordHash == "") {
		return Config{}, fmt.Errorf("KINOPS_ADMIN_USERNAME and KINOPS_ADMIN_PASSWORD_HASH must be configured together")
	}
	secure, err := envBool("KINOPS_ADMIN_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	cfg.AdminCookieSecure = secure

	location, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return Config{}, fmt.Errorf(
			"load timezone %q: %w",
			cfg.TimeZone,
			err,
		)
	}

	cfg.Location = location

	return cfg, nil
}

func (c Config) AdminEnabled() bool {
	return c.AdminUsername != "" && c.AdminPasswordHash != ""
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
