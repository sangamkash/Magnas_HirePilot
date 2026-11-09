package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type MatchDimension struct {
	Score   *float64 `json:"score"`
	Reason  string   `json:"reason"`
	Matched []string `json:"matched,omitempty"`
	Missing []string `json:"missing,omitempty"`
}
type MatchingResponse struct {
	MatchScore     float64                   `json:"match_score"`
	Summary        string                    `json:"summary"`
	Strengths      []string                  `json:"strengths"`
	Gaps           []string                  `json:"gaps"`
	ScoreBreakdown map[string]MatchDimension `json:"score_breakdown"`
	Explanation    map[string]any            `json:"explanation"`
	Recommendation string                    `json:"recommendation"`
}

var recommendations = map[string]bool{"HIGHLY_RECOMMENDED": true, "RECOMMENDED": true, "CONSIDER": true, "NOT_RECOMMENDED": true, "UNKNOWN": true}

// ValidateMatchingResponse strictly decodes and checks the structured response
// before callers persist it. It returns the validated original JSON object too.
func ValidateMatchingResponse(content string) (MatchingResponse, json.RawMessage, error) {
	var result MatchingResponse
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &fields); err != nil {
		return result, nil, fmt.Errorf("decode match response: %w", err)
	}
	for _, key := range []string{"match_score", "summary", "strengths", "gaps", "score_breakdown", "explanation", "recommendation"} {
		if _, ok := fields[key]; !ok {
			return result, nil, fmt.Errorf("match response is missing %s", key)
		}
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, nil, fmt.Errorf("decode match response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return result, nil, errors.New("match response must contain exactly one JSON value")
	}
	if math.IsNaN(result.MatchScore) || math.IsInf(result.MatchScore, 0) || result.MatchScore < 0 || result.MatchScore > 100 {
		return result, nil, errors.New("match_score must be between 0 and 100")
	}
	if strings.TrimSpace(result.Summary) == "" || result.Strengths == nil || result.Gaps == nil || result.ScoreBreakdown == nil || result.Explanation == nil {
		return result, nil, errors.New("match response is missing required fields")
	}
	if !recommendations[result.Recommendation] {
		return result, nil, errors.New("recommendation is not supported")
	}
	for _, dimension := range []string{"technical_skills", "experience", "role_alignment", "location", "salary", "eligibility"} {
		d, ok := result.ScoreBreakdown[dimension]
		if !ok {
			return result, nil, fmt.Errorf("score_breakdown is missing %s", dimension)
		}
		if d.Score != nil && (math.IsNaN(*d.Score) || math.IsInf(*d.Score, 0) || *d.Score < 0 || *d.Score > 100) {
			return result, nil, fmt.Errorf("%s score must be between 0 and 100", dimension)
		}
		if strings.TrimSpace(d.Reason) == "" {
			return result, nil, fmt.Errorf("%s reason is required (use UNKNOWN when unavailable)", dimension)
		}
	}
	for _, key := range []string{"experience_analysis", "role_alignment", "location_analysis", "salary_analysis", "eligibility_analysis"} {
		value, ok := result.Explanation[key]
		if !ok {
			return result, nil, fmt.Errorf("explanation is missing %s", key)
		}
		if text, valid := value.(string); !valid || strings.TrimSpace(text) == "" {
			return result, nil, fmt.Errorf("explanation %s must be a non-empty string", key)
		}
	}
	technical := result.ScoreBreakdown["technical_skills"]
	if technical.Matched == nil || technical.Missing == nil {
		return result, nil, errors.New("technical_skills matched and missing must be string arrays (use empty arrays when none)")
	}
	raw := json.RawMessage(bytes.TrimSpace([]byte(content)))
	return result, raw, nil
}
