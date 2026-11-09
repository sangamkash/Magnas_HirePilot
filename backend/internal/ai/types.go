package ai

import (
	"context"
	"time"
)

type AIRequest struct {
	SystemPrompt    string
	UserPrompt      string
	Model           string
	Temperature     float64
	MaxOutputTokens int
	StructuredJSON  bool
	Timeout         time.Duration
}

type AIResponse struct {
	Content       string        `json:"content"`
	Provider      string        `json:"provider"`
	Model         string        `json:"model"`
	InputTokens   int64         `json:"input_tokens"`
	OutputTokens  int64         `json:"output_tokens"`
	TotalTokens   int64         `json:"total_tokens"`
	EstimatedCost *float64      `json:"estimated_cost,omitempty"`
	Duration      time.Duration `json:"duration"`
	FinishReason  string        `json:"finish_reason"`
}

type Provider interface {
	Generate(ctx context.Context, request AIRequest) (*AIResponse, error)
	GetName() string
	HealthCheck(ctx context.Context) error
}
