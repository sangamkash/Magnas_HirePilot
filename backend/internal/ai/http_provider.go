package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProviderError normalizes transport and upstream failures without including
// request content or credentials.
type ProviderError struct {
	Provider   string
	StatusCode int
	Retryable  bool
	Cause      error
}

func (e *ProviderError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s AI provider returned HTTP %d", e.Provider, e.StatusCode)
	}
	return fmt.Sprintf("%s AI provider request failed: %v", e.Provider, e.Cause)
}
func (e *ProviderError) Unwrap() error { return e.Cause }

type httpProvider struct {
	name, baseURL, apiKey, model string
	client                       *http.Client
	maxRetries                   int
	retryInitial, retryMax       time.Duration
}

func (p *httpProvider) GetName() string { return p.name }

func (p *httpProvider) Generate(ctx context.Context, req AIRequest) (*AIResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, errors.New("AI model is required")
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	endpoint, body, err := p.request(req, model)
	if err != nil {
		return nil, err
	}
	var data []byte
	delay := p.retryInitial
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return nil, &ProviderError{Provider: p.name, Retryable: false, Cause: ctx.Err()}
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		switch p.name {
		case "openai":
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		case "anthropic":
			httpReq.Header.Set("x-api-key", p.apiKey)
			httpReq.Header.Set("anthropic-version", "2023-06-01")
		}
		if p.name == "gemini" {
			httpReq.Header.Set("x-goog-api-key", p.apiKey)
		}
		resp, requestErr := p.client.Do(httpReq)
		retryable := false
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, &ProviderError{Provider: p.name, Retryable: false, Cause: ctx.Err()}
			}
			err = &ProviderError{Provider: p.name, Retryable: true, Cause: requestErr}
			retryable = true
		} else {
			data, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			_ = resp.Body.Close()
			if err != nil {
				err = &ProviderError{Provider: p.name, Retryable: true, Cause: err}
				retryable = true
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
				err = &ProviderError{Provider: p.name, StatusCode: resp.StatusCode, Retryable: retryable}
			} else {
				break
			}
		}
		if !retryable || attempt >= p.maxRetries {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, &ProviderError{Provider: p.name, Retryable: false, Cause: ctx.Err()}
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, &ProviderError{Provider: p.name, Retryable: true, Cause: ctx.Err()}
			case <-timer.C:
			}
		}
		if delay == 0 {
			delay = 100 * time.Millisecond
		} else {
			delay *= 2
		}
		if p.retryMax > 0 && delay > p.retryMax {
			delay = p.retryMax
		}
	}
	out, err := p.parse(data)
	if err != nil {
		return nil, &ProviderError{Provider: p.name, Cause: errors.New("invalid provider response: " + err.Error())}
	}
	out.Provider = p.name
	if out.Model == "" {
		out.Model = model
	}
	out.Duration = time.Since(started)
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	return out, nil
}

func (p *httpProvider) request(req AIRequest, model string) (string, []byte, error) {
	switch p.name {
	case "ollama":
		payload := map[string]any{"model": model, "stream": false, "messages": []map[string]string{{"role": "system", "content": req.SystemPrompt}, {"role": "user", "content": req.UserPrompt}}, "options": map[string]any{"temperature": req.Temperature, "num_predict": req.MaxOutputTokens}}
		if req.StructuredJSON {
			payload["format"] = "json"
		}
		b, e := json.Marshal(payload)
		return "/api/chat", b, e
	case "openai":
		payload := map[string]any{"model": model, "messages": []map[string]string{{"role": "system", "content": req.SystemPrompt}, {"role": "user", "content": req.UserPrompt}}, "temperature": req.Temperature, "max_tokens": req.MaxOutputTokens}
		if req.StructuredJSON {
			payload["response_format"] = map[string]string{"type": "json_object"}
		}
		b, e := json.Marshal(payload)
		return "/chat/completions", b, e
	case "gemini":
		payload := map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": req.SystemPrompt}}}, "contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": req.UserPrompt}}}}, "generationConfig": map[string]any{"temperature": req.Temperature, "maxOutputTokens": req.MaxOutputTokens}}
		if req.StructuredJSON {
			payload["generationConfig"].(map[string]any)["responseMimeType"] = "application/json"
		}
		b, e := json.Marshal(payload)
		return "/models/" + model + ":generateContent", b, e
	case "anthropic":
		payload := map[string]any{"model": model, "system": req.SystemPrompt, "messages": []map[string]string{{"role": "user", "content": req.UserPrompt}}, "temperature": req.Temperature, "max_tokens": req.MaxOutputTokens}
		b, e := json.Marshal(payload)
		return "/messages", b, e
	default:
		return "", nil, fmt.Errorf("%w %q", ErrUnknownProvider, p.name)
	}
}

func (p *httpProvider) parse(data []byte) (*AIResponse, error) {
	var r AIResponse
	switch p.name {
	case "ollama":
		var v struct {
			Model   string `json:"model"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Prompt int64  `json:"prompt_eval_count"`
			Eval   int64  `json:"eval_count"`
			Done   string `json:"done_reason"`
		}
		if e := json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		r = AIResponse{Content: v.Message.Content, Model: v.Model, InputTokens: v.Prompt, OutputTokens: v.Eval, FinishReason: v.Done}
	case "openai":
		var v struct {
			Model   string `json:"model"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				Prompt     int64 `json:"prompt_tokens"`
				Completion int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if e := json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		if len(v.Choices) == 0 {
			return nil, errors.New("missing choices")
		}
		r = AIResponse{Content: v.Choices[0].Message.Content, Model: v.Model, InputTokens: v.Usage.Prompt, OutputTokens: v.Usage.Completion, FinishReason: v.Choices[0].Finish}
	case "gemini":
		var v struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				Finish string `json:"finishReason"`
			} `json:"candidates"`
			Usage struct {
				Prompt int64 `json:"promptTokenCount"`
				Output int64 `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if e := json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		if len(v.Candidates) == 0 {
			return nil, errors.New("missing candidates")
		}
		var parts []string
		for _, part := range v.Candidates[0].Content.Parts {
			parts = append(parts, part.Text)
		}
		r = AIResponse{Content: strings.Join(parts, ""), InputTokens: v.Usage.Prompt, OutputTokens: v.Usage.Output, FinishReason: v.Candidates[0].Finish}
	case "anthropic":
		var v struct {
			Model   string `json:"model"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				Input  int64 `json:"input_tokens"`
				Output int64 `json:"output_tokens"`
			} `json:"usage"`
			Stop string `json:"stop_reason"`
		}
		if e := json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		var parts []string
		for _, part := range v.Content {
			if part.Type == "text" {
				parts = append(parts, part.Text)
			}
		}
		r = AIResponse{Content: strings.Join(parts, ""), Model: v.Model, InputTokens: v.Usage.Input, OutputTokens: v.Usage.Output, FinishReason: v.Stop}
	}
	if strings.TrimSpace(r.Content) == "" {
		return nil, errors.New("empty generated content")
	}
	return &r, nil
}

func (p *httpProvider) HealthCheck(ctx context.Context) error {
	path := "/api/tags"
	if p.name != "ollama" {
		path = "/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return err
	}
	if p.name == "openai" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	if p.name == "gemini" {
		req.Header.Set("x-goog-api-key", p.apiKey)
	}
	if p.name == "anthropic" {
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return &ProviderError{Provider: p.name, Retryable: true, Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ProviderError{Provider: p.name, StatusCode: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	return nil
}
