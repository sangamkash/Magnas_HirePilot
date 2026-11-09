package config

import (
	"testing"
	"time"
)

func TestLoadFromPostgresEnvironment(t *testing.T) {
	for key, value := range map[string]string{
		"DATABASE_URL": "", "PGHOST": "localhost", "PGPORT": "5432", "PGDATABASE": "hirepilot",
		"PGUSER": "local-user", "PGPASSWORD": "p@ss word", "PGSSLMODE": "disable", "HTTP_ADDR": "127.0.0.1:8081",
		"AUTH_SECRET": "01234567890123456789012345678901", "COOKIE_SECURE": "true",
	} {
		t.Setenv(key, value)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8081" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://local-user:p%40ss%20word@localhost:5432/hirepilot?sslmode=disable" {
		t.Fatalf("unexpected URL encoding: %q", cfg.DatabaseURL)
	}
	if cfg.AuthSecret != "01234567890123456789012345678901" || !cfg.CookieSecure {
		t.Fatalf("auth settings not loaded: %+v", cfg)
	}
}

func TestAIConfigurationDefaultsAndProviderSwitch(t *testing.T) {
	for _, key := range []string{"AI_DEFAULT_PROVIDER", "AI_OLLAMA_ENABLED", "AI_OLLAMA_MODEL", "AI_OPENAI_ENABLED", "AI_OPENAI_API_KEY", "AI_OPENAI_MODEL", "AI_GEMINI_ENABLED", "AI_GEMINI_API_KEY", "AI_GEMINI_MODEL", "AI_ANTHROPIC_ENABLED", "AI_ANTHROPIC_API_KEY", "AI_ANTHROPIC_MODEL", "AI_FALLBACK_ENABLED", "AI_FALLBACK_PROVIDER", "AI_REQUEST_TIMEOUT", "AI_MAX_RETRIES", "AI_RETRY_INITIAL_DELAY", "AI_RETRY_MAX_DELAY"} {
		t.Setenv(key, "")
	}
	defaults, err := loadAIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.DefaultProvider != "ollama" || defaults.FallbackEnabled || defaults.Providers["ollama"].Model != "" || defaults.RequestTimeout != 90*time.Second || defaults.MaxRetries != 3 {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
	t.Setenv("AI_DEFAULT_PROVIDER", "openai")
	t.Setenv("AI_OLLAMA_ENABLED", "false")
	t.Setenv("AI_OPENAI_ENABLED", "true")
	t.Setenv("AI_OPENAI_API_KEY", "test-secret")
	t.Setenv("AI_OPENAI_MODEL", "test-model")
	selected, err := loadAIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if selected.DefaultProvider != "openai" || selected.Providers["openai"].Model != "test-model" {
		t.Fatalf("provider selection not configurable: %+v", selected)
	}
	t.Setenv("AI_OLLAMA_ENABLED", "true")
	t.Setenv("AI_OLLAMA_MODEL", "local-test-model")
	t.Setenv("AI_FALLBACK_ENABLED", "true")
	t.Setenv("AI_FALLBACK_PROVIDER", "ollama")
	if _, err = loadAIConfig(); err != nil {
		t.Fatalf("configured local fallback rejected: %v", err)
	}
	t.Setenv("AI_DEFAULT_PROVIDER", "invalid")
	if _, err = loadAIConfig(); err == nil {
		t.Fatal("expected invalid provider rejection")
	}
}

func TestAIConfigurationRejectsInvalidEnabledProviderURL(t *testing.T) {
	t.Setenv("AI_OLLAMA_ENABLED", "true")
	t.Setenv("AI_OLLAMA_BASE_URL", "file:///etc/passwd")
	if _, err := loadAIConfig(); err == nil {
		t.Fatal("expected enabled provider URL validation error")
	}
}

func TestLoadRejectsMissingDatabaseSettings(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGPORT", "PGSSLMODE", "HTTP_ADDR", "AUTH_SECRET", "COOKIE_SECURE"} {
		t.Setenv(key, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database configuration error")
	}
}
