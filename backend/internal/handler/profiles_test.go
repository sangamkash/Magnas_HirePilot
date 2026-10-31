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
