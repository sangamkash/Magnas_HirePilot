package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains process configuration sourced from environment variables.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	MigrationsDir string
	AuthSecret    string
	CookieSecure  bool
	AI            AIConfig
}

// AIConfig holds provider credentials and workflow generation settings. Secrets are
// read only from the process environment and are never included in logs.
type AIConfig struct {
	DefaultProvider        string
	Providers              map[string]AIProviderConfig
	FallbackEnabled        bool
	FallbackProvider       string
	Temperature            float64
	MaxOutputTokens        int
	RequestTimeout         time.Duration
	MaxRetries             int
	RetryInitialDelay      time.Duration
	RetryMaxDelay          time.Duration
	MatchingEnabled        bool
	MatchingMinScore       float64
	MatchingBatchSize      int
	MatchingMaxConcurrency int
	MatchingAsync          bool
}

type AIProviderConfig struct {
	Enabled bool
	BaseURL string
	APIKey  string
	Model   string
}

func Load() (Config, error) {
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	databaseURL, err := postgresURL()
	if err != nil {
		return Config{}, err
	}
	migrationsDir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR"))
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	authSecret := os.Getenv("AUTH_SECRET")
	if len(authSecret) < 32 {
		return Config{}, errors.New("AUTH_SECRET must contain at least 32 characters")
	}
	cookieSecure, err := strconv.ParseBool(envOrDefault("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, errors.New("COOKIE_SECURE must be true or false")
	}
	ai, err := loadAIConfig()
	if err != nil {
		return Config{}, err
	}
	return Config{HTTPAddr: addr, DatabaseURL: databaseURL, MigrationsDir: migrationsDir, AuthSecret: authSecret, CookieSecure: cookieSecure, AI: ai}, nil
}

func loadAIConfig() (AIConfig, error) {
	ollamaEnabled, err := envBoolValue("AI_OLLAMA_ENABLED", true)
	if err != nil {
		return AIConfig{}, err
	}
	openAIEnabled, err := envBoolValue("AI_OPENAI_ENABLED", false)
	if err != nil {
		return AIConfig{}, err
	}
	geminiEnabled, err := envBoolValue("AI_GEMINI_ENABLED", false)
	if err != nil {
		return AIConfig{}, err
	}
	anthropicEnabled, err := envBoolValue("AI_ANTHROPIC_ENABLED", false)
	if err != nil {
		return AIConfig{}, err
	}
	provider := strings.ToLower(envOrDefault("AI_DEFAULT_PROVIDER", "ollama"))
	providers := map[string]AIProviderConfig{
		"ollama":    {Enabled: ollamaEnabled, BaseURL: envOrDefault("AI_OLLAMA_BASE_URL", "http://localhost:11434"), Model: strings.TrimSpace(os.Getenv("AI_OLLAMA_MODEL"))},
		"openai":    {Enabled: openAIEnabled, BaseURL: envOrDefault("AI_OPENAI_BASE_URL", "https://api.openai.com/v1"), APIKey: os.Getenv("AI_OPENAI_API_KEY"), Model: strings.TrimSpace(os.Getenv("AI_OPENAI_MODEL"))},
		"gemini":    {Enabled: geminiEnabled, BaseURL: envOrDefault("AI_GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"), APIKey: os.Getenv("AI_GEMINI_API_KEY"), Model: strings.TrimSpace(os.Getenv("AI_GEMINI_MODEL"))},
		"anthropic": {Enabled: anthropicEnabled, BaseURL: envOrDefault("AI_ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"), APIKey: os.Getenv("AI_ANTHROPIC_API_KEY"), Model: strings.TrimSpace(os.Getenv("AI_ANTHROPIC_MODEL"))},
	}
	for name, providerConfig := range providers {
		if providerConfig.Enabled {
			u, parseErr := url.Parse(strings.TrimSpace(providerConfig.BaseURL))
			if parseErr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return AIConfig{}, fmt.Errorf("AI_%s_BASE_URL must be a valid HTTP(S) URL without user information", strings.ToUpper(name))
			}
		}
	}
	if _, ok := providers[provider]; !ok {
		return AIConfig{}, errors.New("AI_DEFAULT_PROVIDER must be ollama, openai, gemini, or anthropic")
	}
	if !providers[provider].Enabled {
		return AIConfig{}, fmt.Errorf("default AI provider %q is disabled", provider)
	}
	if (provider == "openai" || provider == "gemini" || provider == "anthropic") && strings.TrimSpace(providers[provider].APIKey) == "" {
		return AIConfig{}, fmt.Errorf("AI_%s_API_KEY is required for the default provider", strings.ToUpper(provider))
	}
	temperature, err := envFloat("AI_TEMPERATURE", 0.2)
	if err != nil || temperature < 0 || temperature > 2 {
		return AIConfig{}, errors.New("AI_TEMPERATURE must be between 0 and 2")
	}
	tokens, err := envInt("AI_MAX_OUTPUT_TOKENS", 2048)
	if err != nil || tokens < 1 {
		return AIConfig{}, errors.New("AI_MAX_OUTPUT_TOKENS must be positive")
	}
	timeout, err := envDuration("AI_REQUEST_TIMEOUT", 90*time.Second)
	if err != nil || timeout <= 0 {
		return AIConfig{}, errors.New("AI_REQUEST_TIMEOUT must be a positive duration")
	}
	retries, err := envInt("AI_MAX_RETRIES", 3)
	if err != nil || retries < 0 {
		return AIConfig{}, errors.New("AI_MAX_RETRIES must not be negative")
	}
	initial, err := envDuration("AI_RETRY_INITIAL_DELAY", time.Second)
	if err != nil || initial < 0 {
		return AIConfig{}, errors.New("AI_RETRY_INITIAL_DELAY must not be negative")
	}
	maximum, err := envDuration("AI_RETRY_MAX_DELAY", 10*time.Second)
	if err != nil || maximum < initial {
		return AIConfig{}, errors.New("AI_RETRY_MAX_DELAY must be at least AI_RETRY_INITIAL_DELAY")
	}
	minScore, err := envFloat("MATCHING_MIN_SCORE", 0)
	if err != nil || minScore < 0 || minScore > 100 {
		return AIConfig{}, errors.New("MATCHING_MIN_SCORE must be between 0 and 100")
	}
	batch, err := envInt("MATCHING_BATCH_SIZE", 10)
	if err != nil || batch < 1 {
		return AIConfig{}, errors.New("MATCHING_BATCH_SIZE must be positive")
	}
	concurrency, err := envInt("MATCHING_MAX_CONCURRENCY", 3)
	if err != nil || concurrency < 1 {
		return AIConfig{}, errors.New("MATCHING_MAX_CONCURRENCY must be positive")
	}
	fallbackEnabled, err := envBoolValue("AI_FALLBACK_ENABLED", false)
	if err != nil {
		return AIConfig{}, err
	}
	fallback := strings.ToLower(strings.TrimSpace(os.Getenv("AI_FALLBACK_PROVIDER")))
	if fallbackEnabled {
		if _, ok := providers[fallback]; !ok || fallback == provider {
			return AIConfig{}, errors.New("AI_FALLBACK_PROVIDER must be a different supported provider")
		}
		if !providers[fallback].Enabled {
			return AIConfig{}, errors.New("AI_FALLBACK_PROVIDER must be enabled")
		}
		if strings.TrimSpace(providers[fallback].Model) == "" {
			return AIConfig{}, fmt.Errorf("AI_%s_MODEL is required for fallback provider", strings.ToUpper(fallback))
		}
		if fallback != "ollama" && strings.TrimSpace(providers[fallback].APIKey) == "" {
			return AIConfig{}, fmt.Errorf("AI_%s_API_KEY is required for fallback provider", strings.ToUpper(fallback))
		}
	}
	matchingEnabled, err := envBoolValue("MATCHING_ENABLED", true)
	if err != nil {
		return AIConfig{}, err
	}
	matchingAsync, err := envBoolValue("MATCHING_ASYNC", true)
	if err != nil {
		return AIConfig{}, err
	}
	return AIConfig{DefaultProvider: provider, Providers: providers, FallbackEnabled: fallbackEnabled, FallbackProvider: fallback, Temperature: temperature, MaxOutputTokens: tokens, RequestTimeout: timeout, MaxRetries: retries, RetryInitialDelay: initial, RetryMaxDelay: maximum, MatchingEnabled: matchingEnabled, MatchingMinScore: minScore, MatchingBatchSize: batch, MatchingMaxConcurrency: concurrency, MatchingAsync: matchingAsync}, nil
}

func envBoolValue(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return parsed, nil
}
func envInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
func envFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseFloat(value, 64)
}
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func postgresURL() (string, error) {
	if databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); databaseURL != "" {
		u, err := url.Parse(databaseURL)
		if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" || u.Host == "" || u.Path == "/" {
			return "", errors.New("DATABASE_URL must be a valid PostgreSQL connection URL")
		}
		return databaseURL, nil
	}

	host := strings.TrimSpace(os.Getenv("PGHOST"))
	database := strings.TrimSpace(os.Getenv("PGDATABASE"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	password := os.Getenv("PGPASSWORD")
	if host == "" || database == "" || user == "" {
		return "", errors.New("set DATABASE_URL or PGHOST, PGDATABASE, and PGUSER")
	}
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("PGPORT must be an integer: %w", err)
	}
	sslmode := strings.TrimSpace(os.Getenv("PGSSLMODE"))
	if sslmode == "" {
		sslmode = "prefer"
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(user, password), Host: host + ":" + port, Path: "/" + database}
	query := url.Values{}
	query.Set("sslmode", sslmode)
	u.RawQuery = query.Encode()
	return u.String(), nil
}
