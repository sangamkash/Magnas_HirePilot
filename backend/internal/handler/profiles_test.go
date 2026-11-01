package handler

import (
	"encoding/json"
	"testing"
)

func TestDecodeSearchIntegerAcceptsJSONNumbers(t *testing.T) {
	for _, raw := range []string{`2`, `0`, `"3"`, `" 4 "`} {
		got, err := decodeSearchInteger(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		if got < 0 {
			t.Fatalf("unexpected value %d", got)
		}
	}
	for _, raw := range []string{`1.5`, `-1`, `"bad"`, `true`, `"1.5"`, `"-2"`} {
		if _, err := decodeSearchInteger(json.RawMessage(raw)); err == nil {
			t.Errorf("expected %s to be rejected", raw)
		}
	}
}

func TestValidProfileRejectsMalformedNestedEntries(t *testing.T) {
	tests := []struct{ name, data string }{
		{"experience without title", `{"personal":{"name":"Person"},"experience":[{"company":"Example"}]}`},
		{"education without institution", `{"personal":{"name":"Person"},"education":[{"degree":"BSc"}]}`},
		{"search decimal duration", `{"personal":{"name":"Person"},"searches":[{"hours":1.5}]}`},
		{"wrong nested type", `{"personal":{"name":"Person"},"awards":["not an object"]}`},
		{"partial entry", `{"personal":{"name":"Person"},"experience":[{"company":"Example"}]}`},
		{"non-string required entry", `{"personal":{"name":"Person"},"education":[{"institution":42}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if validProfile(profileRecord{Name: "Profile", Data: json.RawMessage(tt.data)}) {
				t.Fatal("malformed profile accepted")
			}
		})
	}
}

func TestValidProfileRejectsEmptyNestedRows(t *testing.T) {
	data := json.RawMessage(`{"personal":{"name":"Person"},"experience":[{}],"education":[{}]}`)
	if validProfile(profileRecord{Name: "Profile", Data: data}) {
		t.Fatal("empty nested entries must not be silently skipped")
	}
}

func TestValidProfileRejectsDatabaseConstraintValues(t *testing.T) {
	tests := []struct{ name, data string }{
		{"reversed education dates", `{"personal":{"name":"Person"},"education":[{"institution":"School","startDate":"2022-01","endDate":"2020-01"}]}`},
		{"invalid date", `{"personal":{"name":"Person"},"experience":[{"company":"Example","title":"Engineer","startDate":"2024-13"}]}`},
		{"duplicate skills ignoring case", `{"personal":{"name":"Person"},"skills":{"programming":["Go"," go "]}}`},
		{"duplicate languages ignoring case", `{"personal":{"name":"Person"},"languages":[{"language":"English","proficiency":"Fluent"},{"language":"english","proficiency":"Native"}]}`},
		{"unsupported proficiency", `{"personal":{"name":"Person"},"languages":[{"language":"English","proficiency":"Expert"}]}`},
		{"invalid search work mode", `{"personal":{"name":"Person"},"searches":[{"name":"Search","titles":["Engineer"],"workMode":["Anywhere"]}]}`},
		{"invalid salary settings", `{"personal":{"name":"Person"},"searches":[{"name":"Search","titles":["Engineer"],"salary":"100000","currency":"INR","period":"weekly"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if validProfile(profileRecord{Name: "Profile", Data: json.RawMessage(tt.data)}) {
				t.Fatal("constraint-invalid profile accepted")
			}
		})
	}
}

func TestValidProfileAcceptsMonthDatesAndEmptyOptionalDates(t *testing.T) {
	data := json.RawMessage(`{"personal":{"name":"Person"},"experience":[{"company":"Example","title":"Engineer","startDate":"2024-06","current":true}]}`)
	if !validProfile(profileRecord{Name: "Profile", Data: data}) {
		t.Fatal("valid month precision date and empty current end date rejected")
	}
}

func TestProfileValidationMessageExplainsInvalidValues(t *testing.T) {
	p := profileRecord{Name: "Profile", Data: json.RawMessage(`{"personal":{"name":"Person"},"languages":[{"language":"English","proficiency":"Expert"}]}`)}
	if got := profileValidationMessage(p); got == "" || got == "Profile name and personal full name are required; profile data must be a JSON object." {
		t.Fatalf("expected actionable invalid-value message, got %q", got)
	}
}
