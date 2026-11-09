package ai

import "testing"

const validMatch = `{"match_score":80,"summary":"Good","strengths":["Go"],"gaps":[],"score_breakdown":{"technical_skills":{"score":80,"reason":"Relevant","matched":["Go"],"missing":[]},"experience":{"score":80,"reason":"Relevant"},"role_alignment":{"score":80,"reason":"Relevant"},"location":{"score":null,"reason":"UNKNOWN"},"salary":{"score":null,"reason":"UNKNOWN"},"eligibility":{"score":null,"reason":"UNKNOWN"}},"explanation":{"experience_analysis":"Relevant","role_alignment":"Relevant","location_analysis":"UNKNOWN","salary_analysis":"UNKNOWN","eligibility_analysis":"UNKNOWN"},"recommendation":"RECOMMENDED"}`

func TestValidateMatchingResponse(t *testing.T) {
	if _, raw, err := ValidateMatchingResponse(validMatch); err != nil || len(raw) == 0 {
		t.Fatalf("valid response rejected: %v", err)
	}
}
func TestValidateMatchingResponseRejectsInvalidScoreAndShape(t *testing.T) {
	for _, input := range []string{`{"match_score":101}`, validMatch + ` {}`, `{"match_score":80,"summary":"x","strengths":[],"gaps":[],"score_breakdown":{},"explanation":{},"recommendation":"SURE"}`} {
		if _, _, err := ValidateMatchingResponse(input); err == nil {
			t.Fatalf("accepted invalid response: %s", input)
		}
	}
}

func TestValidateMatchingResponseRejectsIncompleteTypedFields(t *testing.T) {
	for _, input := range []string{
		`{"match_score":80,"summary":"x","strengths":[],"gaps":[],"score_breakdown":{"technical_skills":{"score":80,"reason":"Relevant"},"experience":{"score":80,"reason":"Relevant"},"role_alignment":{"score":80,"reason":"Relevant"},"location":{"score":null,"reason":"UNKNOWN"},"salary":{"score":null,"reason":"UNKNOWN"},"eligibility":{"score":null,"reason":"UNKNOWN"}},"explanation":{"experience_analysis":"Relevant","role_alignment":"Relevant","location_analysis":"UNKNOWN","salary_analysis":"UNKNOWN","eligibility_analysis":null},"recommendation":"RECOMMENDED"}`,
	} {
		if _, _, err := ValidateMatchingResponse(input); err == nil {
			t.Fatalf("accepted incomplete response: %s", input)
		}
	}
}
