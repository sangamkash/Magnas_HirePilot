package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionTokenValidation(t *testing.T) {
	h := &authHandler{secret: []byte("01234567890123456789012345678901")}
	r := httptest.NewRecorder()
	h.setCookie(r, "user-123")
	cookie := r.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != 2 {
		t.Fatalf("cookie security flags missing: %#v", cookie)
	}
	if userID, ok := h.parseToken(cookie.Value); !ok || userID != "user-123" {
		t.Fatalf("valid token rejected: %q %v", userID, ok)
	}
	if _, ok := h.parseToken("invalid.token"); ok {
		t.Fatal("accepted invalid token")
	}
}

func TestExpiredSessionTokenRejected(t *testing.T) {
	h := &authHandler{secret: []byte("01234567890123456789012345678901")}
	payload, _ := json.Marshal(sessionClaims{UserID: "u", Expires: time.Now().Add(-time.Minute).Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(encoded))
	token := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, ok := h.parseToken(token); ok {
		t.Fatal("accepted expired token")
	}
}

func TestProtectedAPIRequiresValidSession(t *testing.T) {
	h := &authHandler{secret: []byte("01234567890123456789012345678901")}
	protected := h.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Context().Value(userIDKey{}); got != "user-123" {
			t.Errorf("user id in context = %v", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	unauthenticated := httptest.NewRecorder()
	protected.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/private", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	authenticated := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	cookieRecorder := httptest.NewRecorder()
	h.setCookie(cookieRecorder, "user-123")
	request.AddCookie(cookieRecorder.Result().Cookies()[0])
	protected.ServeHTTP(authenticated, request)
	if authenticated.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", authenticated.Code)
	}
}

func TestDecodeJSONReturnsActionableErrors(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"wrong field type", `{"username":42,"password":"long-enough-password"}`, "Invalid value for username: expected string."},
		{"unknown field", `{"usernmae":"person","password":"long-enough-password"}`, "unsupported field"},
		{"malformed JSON", `{"username":`, "invalid JSON"},
		{"trailing value", `{"username":"person","password":"long-enough-password"} {}`, "exactly one JSON value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if decodeJSON(r, request, &credentials{}) {
				t.Fatal("expected request rejection")
			}
			if !strings.Contains(r.Body.String(), tt.want) {
				t.Fatalf("response %q does not contain %q", r.Body.String(), tt.want)
			}
		})
	}
}

func TestDecodeJSONAcceptsProfileSizedPayload(t *testing.T) {
	// Simulate a profile with enough description text to exceed the old 4 KiB cap.
	body, err := json.Marshal(map[string]any{
		"name": "Backend Engineer",
		"data": map[string]any{
			"personal":   map[string]string{"name": "Test User"},
			"experience": []map[string]string{{"description": strings.Repeat("career detail ", 500)}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 4096 {
		t.Fatalf("test payload should exceed old 4 KiB limit, got %d bytes", len(body))
	}
	var decoded map[string]any
	r := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", strings.NewReader(string(body)))
	if !decodeJSON(r, request, &decoded) {
		t.Fatalf("profile-sized payload rejected: %s", r.Body.String())
	}
}
