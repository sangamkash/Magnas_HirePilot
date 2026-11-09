package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magnas/hirepilot/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestFactoryBuildsAllConfiguredProviders(t *testing.T) {
	for _, name := range []string{"ollama", "openai", "gemini", "anthropic"} {
		t.Run(name, func(t *testing.T) {
			cfg := config.AIConfig{DefaultProvider: name, RequestTimeout: time.Second, Providers: map[string]config.AIProviderConfig{name: {Enabled: true, BaseURL: "http://localhost", Model: "test-model", APIKey: "test-key"}}}
			p, err := NewProvider(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			if p.GetName() != name {
				t.Fatalf("provider=%s", p.GetName())
			}
		})
	}
}

func TestFactoryRejectsMissingCredentialAndModel(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  config.AIConfig
	}{{"key", config.AIConfig{DefaultProvider: "openai", Providers: map[string]config.AIProviderConfig{"openai": {Enabled: true, BaseURL: "http://localhost", Model: "m"}}}}, {"model", config.AIConfig{DefaultProvider: "ollama", Providers: map[string]config.AIProviderConfig{"ollama": {Enabled: true, BaseURL: "http://localhost"}}}}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewProvider(tc.cfg, nil); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestFactoryRejectsUnsafeOrAmbiguousBaseURL(t *testing.T) {
	for _, baseURL := range []string{"https://user:pass@example.test/v1", "https://example.test/v1?key=secret", "https://example.test/v1#fragment"} {
		cfg := config.AIConfig{DefaultProvider: "openai", Providers: map[string]config.AIProviderConfig{"openai": {Enabled: true, BaseURL: baseURL, APIKey: "test-key", Model: "m"}}}
		if _, err := NewProvider(cfg, nil); err == nil {
			t.Errorf("accepted base URL %q", baseURL)
		}
	}
}

func TestProviderCallsConfiguredLocalEndpointAndParsesUsage(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["model"] != "chosen-model" {
			t.Errorf("model=%v", req["model"])
		}
		return response(http.StatusOK, `{"model":"chosen-model","message":{"content":"{\"ok\":true}"},"prompt_eval_count":3,"eval_count":4,"done_reason":"stop"}`), nil
	})}
	cfg := config.AIConfig{DefaultProvider: "ollama", RequestTimeout: time.Second, Providers: map[string]config.AIProviderConfig{"ollama": {Enabled: true, BaseURL: "http://provider.test", Model: "configured"}}}
	p, err := NewProvider(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Generate(context.Background(), AIRequest{UserPrompt: "hello", Model: "chosen-model", StructuredJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalTokens != 7 || response.InputTokens != 3 || response.OutputTokens != 4 || response.Content != `{"ok":true}` {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestHostedProviderAdaptersParseResponsesAndAuthenticate(t *testing.T) {
	cases := []struct {
		name, path, body, content string
		tokens                    int64
	}{
		{"openai", "/v1/chat/completions", `{"model":"m","choices":[{"message":{"content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`, "answer", 5},
		{"gemini", "/v1/models/m:generateContent", `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`, "answer", 5},
		{"anthropic", "/v1/messages", `{"model":"m","content":[{"type":"text","text":"answer"}],"usage":{"input_tokens":2,"output_tokens":3},"stop_reason":"end_turn"}`, "answer", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%s", r.URL.Path)
				}
				if tc.name == "openai" && r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing bearer auth")
				}
				if tc.name == "gemini" && (r.URL.Query().Has("key") || r.Header.Get("x-goog-api-key") != "secret") {
					t.Error("missing API key")
				}
				if tc.name == "anthropic" && (r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") == "") {
					t.Error("missing Anthropic headers")
				}
				return response(http.StatusOK, tc.body), nil
			})}
			p, err := NewProvider(config.AIConfig{DefaultProvider: tc.name, Providers: map[string]config.AIProviderConfig{tc.name: {Enabled: true, BaseURL: "http://provider.test/v1", APIKey: "secret", Model: "m"}}}, client)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Generate(context.Background(), AIRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Content != tc.content || got.TotalTokens != tc.tokens {
				t.Fatalf("unexpected response: %+v", got)
			}
		})
	}
}

func TestProviderDoesNotRetryCallerCancellation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, r.Context().Err()
	})}
	p := &httpProvider{name: "ollama", baseURL: "http://provider.test", model: "m", client: client, maxRetries: 5, retryInitial: time.Millisecond, retryMax: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Generate(ctx, AIRequest{Timeout: time.Second})
	if err == nil || calls > 1 {
		t.Fatalf("canceled request retried: calls=%d err=%v", calls, err)
	}
}

func TestExplicitFallbackUsesConfiguredFallbackOnly(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/chat/completions" {
			return response(http.StatusServiceUnavailable, `{}`), nil
		}
		return response(http.StatusOK, `{"model":"local","message":{"content":"answer"},"prompt_eval_count":1,"eval_count":1}`), nil
	})}
	cfg := config.AIConfig{DefaultProvider: "openai", FallbackEnabled: true, FallbackProvider: "ollama", Providers: map[string]config.AIProviderConfig{"openai": {Enabled: true, BaseURL: "http://cloud.test/v1", APIKey: "secret", Model: "cloud"}, "ollama": {Enabled: true, BaseURL: "http://local.test", Model: "local"}}}
	p, err := NewProvider(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Generate(context.Background(), AIRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "ollama" || result.Content != "answer" {
		t.Fatalf("fallback response not attributed: %+v", result)
	}
}

func TestProviderNormalizesUnavailableAndTimeout(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	p := &httpProvider{name: "ollama", baseURL: "http://provider.test", model: "m", client: client}
	_, err := p.Generate(context.Background(), AIRequest{Timeout: 5 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout")
	}
	p.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return response(http.StatusTooManyRequests, `{}`), nil })}
	_, err = p.Generate(context.Background(), AIRequest{Timeout: time.Second})
	if err == nil {
		t.Fatal("expected rate limit")
	}
	pe, ok := err.(*ProviderError)
	if !ok || !pe.Retryable || pe.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected error: %#v", err)
	}
}
