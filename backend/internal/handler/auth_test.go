package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
