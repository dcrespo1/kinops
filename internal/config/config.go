package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	MealieBaseURL     string
	MealiePublicURL   string
	MealieAPIToken    string
	MealieDefaultList string
	MealieTimeout     time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress:     envOrDefault("KINOPS_LISTEN_ADDRESS", ":8081"),
		DatabasePath:      envOrDefault("KINOPS_DATABASE_PATH", "./data/kinops.db"),
		TimeZone:          envOrDefault("KINOPS_TIMEZONE", "America/New_York"),
		AdminUsername:     os.Getenv("KINOPS_ADMIN_USERNAME"),
		AdminPasswordHash: os.Getenv("KINOPS_ADMIN_PASSWORD_HASH"),
		MealieBaseURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("MEALIE_BASE_URL")), "/"),
		MealiePublicURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("MEALIE_PUBLIC_URL")), "/"),
		MealieAPIToken:    strings.TrimSpace(os.Getenv("MEALIE_API_TOKEN")),
		MealieDefaultList: strings.TrimSpace(os.Getenv("MEALIE_DEFAULT_SHOPPING_LIST_ID")),
	}
	if (cfg.AdminUsername == "") != (cfg.AdminPasswordHash == "") {
		return Config{}, fmt.Errorf("KINOPS_ADMIN_USERNAME and KINOPS_ADMIN_PASSWORD_HASH must be configured together")
	}
	secure, err := envBool("KINOPS_ADMIN_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	cfg.AdminCookieSecure = secure
	if (cfg.MealieBaseURL == "") != (cfg.MealieAPIToken == "") {
		return Config{}, fmt.Errorf("MEALIE_BASE_URL and MEALIE_API_TOKEN must be configured together")
	}
	if cfg.MealieBaseURL != "" {
		if err := validateHTTPURL("MEALIE_BASE_URL", cfg.MealieBaseURL); err != nil {
			return Config{}, err
		}
		if cfg.MealiePublicURL == "" {
			cfg.MealiePublicURL = cfg.MealieBaseURL
		}
		if err := validateHTTPURL("MEALIE_PUBLIC_URL", cfg.MealiePublicURL); err != nil {
			return Config{}, err
		}
	}
	timeout, err := time.ParseDuration(envOrDefault("MEALIE_REQUEST_TIMEOUT", "5s"))
	if err != nil || timeout <= 0 || timeout > time.Minute {
		return Config{}, fmt.Errorf("MEALIE_REQUEST_TIMEOUT must be a duration greater than zero and no more than 1m")
	}
	cfg.MealieTimeout = timeout

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

func (c Config) MealieEnabled() bool {
	return c.MealieBaseURL != "" && c.MealieAPIToken != ""
}

func validateHTTPURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain credentials, a query, or a fragment", name)
	}
	return nil
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
