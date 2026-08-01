package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("KINOPS_LISTEN_ADDRESS", "")
	t.Setenv("KINOPS_DATABASE_PATH", "")
	t.Setenv("KINOPS_TIMEZONE", "")
	t.Setenv("KINOPS_ADMIN_USERNAME", "")
	t.Setenv("KINOPS_ADMIN_PASSWORD_HASH", "")
	t.Setenv("KINOPS_ADMIN_COOKIE_SECURE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}
	if cfg.ListenAddress != ":8081" {
		t.Errorf(
			"ListenAddress = %q want %q",
			cfg.ListenAddress,
			":8081",
		)
	}
	if cfg.DatabasePath != "./data/kinops.db" {
		t.Errorf(
			"Database Path = %q want %q",
			cfg.DatabasePath,
			"./data/kinops.db",
		)
	}
	if cfg.TimeZone != "America/New_York" {
		t.Errorf(
			"TimeZone = %q want %q",
			cfg.TimeZone,
			"America/New_York",
		)
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("KINOPS_LISTEN_ADDRESS", ":9090")
	t.Setenv("KINOPS_DATABASE_PATH", "/tmp/test.db")
	t.Setenv("KINOPS_TIMEZONE", "UTC")
	t.Setenv("KINOPS_ADMIN_USERNAME", "admin")
	t.Setenv("KINOPS_ADMIN_PASSWORD_HASH", "pbkdf2-sha256$600000$c2FsdHNhbHRzYWx0c2FsdA$MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA")
	t.Setenv("KINOPS_ADMIN_COOKIE_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.ListenAddress != ":9090" {
		t.Errorf(
			"ListenAddress = %q, want %q",
			cfg.ListenAddress,
			":9090",
		)
	}

	if cfg.DatabasePath != "/tmp/test.db" {
		t.Errorf(
			"DatabasePath = %q, want %q",
			cfg.DatabasePath,
			"/tmp/test.db",
		)
	}

	if cfg.TimeZone != "UTC" {
		t.Errorf(
			"TimeZone = %q, want %q",
			cfg.TimeZone,
			"UTC",
		)
	}
	if !cfg.AdminEnabled() || cfg.AdminUsername != "admin" || !cfg.AdminCookieSecure {
		t.Errorf("admin config = %#v", cfg)
	}
}

func TestLoadRejectsInvalidTimeZone(t *testing.T) {
	t.Setenv("KINOPS_TIMEZONE", "Definitely/Not-A-Timezone")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error, want an error")
	}
}

func TestLoadRejectsPartialOrInvalidAdminConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		username string
		hash     string
		secure   string
	}{
		{name: "username only", username: "admin"},
		{name: "hash only", hash: "hash"},
		{name: "invalid secure", secure: "sometimes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KINOPS_ADMIN_USERNAME", tt.username)
			t.Setenv("KINOPS_ADMIN_PASSWORD_HASH", tt.hash)
			t.Setenv("KINOPS_ADMIN_COOKIE_SECURE", tt.secure)
			if _, err := Load(); err == nil {
				t.Fatal("Load() returned nil error")
			}
		})
	}
}

func TestLoadTimeZones(t *testing.T) {
	tests := []struct {
		name     string
		timeZone string
		wantErr  bool
	}{
		{
			name:     "UTC",
			timeZone: "UTC",
			wantErr:  false,
		},
		{
			name:     "New York",
			timeZone: "America/New_York",
			wantErr:  false,
		},
		{
			name:     "invalid",
			timeZone: "Not/A-Zone",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KINOPS_TIMEZONE", tt.timeZone)

			_, err := Load()

			if tt.wantErr && err == nil {
				t.Fatal("Load() returned nil error, want an error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
		})
	}

}
