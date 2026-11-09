package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/magnas/hirepilot/internal/config"
)

var ErrUnknownProvider = errors.New("unknown AI provider")

// NewProvider selects a provider from configuration. It never changes providers
// implicitly; fallback is an explicit caller-level policy.
func NewProvider(cfg config.AIConfig, client *http.Client) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	primary, err := configuredProvider(name, cfg, client)
	if err != nil {
		return nil, err
	}
	if !cfg.FallbackEnabled {
		return primary, nil
	}
	fallbackName := strings.ToLower(strings.TrimSpace(cfg.FallbackProvider))
	if fallbackName == "" || fallbackName == name {
		return nil, errors.New("AI fallback provider must be set to a different provider")
	}
	fallback, err := configuredProvider(fallbackName, cfg, client)
	if err != nil {
		return nil, fmt.Errorf("configure AI fallback: %w", err)
	}
	return &fallbackProvider{primary: primary, fallback: fallback}, nil
}

func configuredProvider(name string, cfg config.AIConfig, client *http.Client) (Provider, error) {
	pc, ok := cfg.Providers[name]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownProvider, name)
	}
	if !pc.Enabled {
		return nil, fmt.Errorf("AI provider %q is disabled", name)
	}
	if name != "ollama" && strings.TrimSpace(pc.APIKey) == "" {
		return nil, fmt.Errorf("credentials are required for AI provider %q", name)
	}
	if strings.TrimSpace(pc.Model) == "" {
		return nil, fmt.Errorf("model is required for AI provider %q", name)
	}
	u, err := url.Parse(pc.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid base URL for AI provider %q", name)
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}
	return &httpProvider{name: name, baseURL: strings.TrimRight(pc.BaseURL, "/"), apiKey: pc.APIKey, model: pc.Model, client: client, maxRetries: cfg.MaxRetries, retryInitial: cfg.RetryInitialDelay, retryMax: cfg.RetryMaxDelay}, nil
}

type fallbackProvider struct{ primary, fallback Provider }

func (p *fallbackProvider) GetName() string { return p.primary.GetName() }
func (p *fallbackProvider) Generate(ctx context.Context, request AIRequest) (*AIResponse, error) {
	response, err := p.primary.Generate(ctx, request)
	if err == nil {
		return response, nil
	}
	return p.fallback.Generate(ctx, request)
}
func (p *fallbackProvider) HealthCheck(ctx context.Context) error { return p.primary.HealthCheck(ctx) }
