package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const authCookie = "hirepilot_session"
const sessionLifetime = 12 * time.Hour

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

type authHandler struct {
	db     *pgxpool.Pool
	secret []byte
	secure bool
}

type authUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}
type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type sessionClaims struct {
	UserID  string `json:"sub"`
	Expires int64  `json:"exp"`
}

func NewAuth(db *pgxpool.Pool, secret string, secure bool) http.Handler {
	h := &authHandler{db: db, secret: []byte(secret), secure: secure}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, response{Status: "ok"}) })
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := contextWithTimeout(r)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, response{Status: "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "ready"})
	})
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.Handle("GET /api/v1/auth/me", h.requireAuth(http.HandlerFunc(h.me)))
	mux.Handle("/api/", h.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeError(w, http.StatusNotFound, "Not found.") })))
	return mux
}

// contextWithTimeout limits readiness probes without exposing database errors.
func contextWithTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 2*time.Second)
}

func (h *authHandler) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if !usernamePattern.MatchString(input.Username) || len(input.Password) < 12 || len(input.Password) > 72 {
		writeError(w, http.StatusBadRequest, "Username must be 3–32 letters, numbers, or underscores; password must be 12–72 bytes.")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Unable to create account.")
		return
	}
	var user authUser
	err = h.db.QueryRow(r.Context(), `INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id::text, username`, input.Username, string(passwordHash)).Scan(&user.ID, &user.Username)
	if err != nil {
		if strings.Contains(err.Error(), "users_username_lower_idx") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "That username is unavailable.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Unable to create account.")
		return
	}
	h.setCookie(w, user.ID)
	writeJSON(w, http.StatusCreated, user)
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Username) > 32 || len(input.Password) > 72 {
		writeError(w, http.StatusUnauthorized, "Invalid username or password.")
		return
	}
	var user authUser
	var hash string
	err := h.db.QueryRow(r.Context(), `SELECT id::text, username, password_hash FROM users WHERE lower(username)=lower($1)`, strings.TrimSpace(input.Username)).Scan(&user.ID, &user.Username, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "Invalid username or password.")
		return
	}
	h.setCookie(w, user.ID)
	writeJSON(w, http.StatusOK, user)
}

func (h *authHandler) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: authCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	id := r.Context().Value(userIDKey{}).(string)
	var user authUser
	if err := h.db.QueryRow(r.Context(), `SELECT id::text, username FROM users WHERE id=$1`, id).Scan(&user.ID, &user.Username); err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication required.")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type userIDKey struct{}

func (h *authHandler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Authentication required.")
			return
		}
		id, ok := h.parseToken(cookie.Value)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Authentication required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, id)))
	})
}
func (h *authHandler) setCookie(w http.ResponseWriter, id string) {
	payload, _ := json.Marshal(sessionClaims{UserID: id, Expires: time.Now().Add(sessionLifetime).Unix()})
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(body))
	value := body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{Name: authCookie, Value: value, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionLifetime.Seconds()), Expires: time.Now().Add(sessionLifetime)})
}
func (h *authHandler) parseToken(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(parts[0]))
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return "", false
	}
	var claims sessionClaims
	if json.Unmarshal(payload, &claims) != nil || claims.UserID == "" || claims.Expires <= time.Now().Unix() {
		return "", false
	}
	return claims.UserID, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return false
	}
	return true
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
