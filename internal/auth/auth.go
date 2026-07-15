// Package auth handles users and cookie sessions. There is a single implicit
// permission level — every account can do everything — so users carry no role
// field at all.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SessionCookie is the name of the session cookie.
const SessionCookie = "dt_session"

// SessionTTL is how long a login lasts.
const SessionTTL = 30 * 24 * time.Hour

// ErrInvalidCredentials is returned for a bad username/password pair.
var ErrInvalidCredentials = errors.New("invalid username or password")

// User is an account. Every user has full access; there are no roles.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// Service provides user management and session tokens.
type Service struct {
	db     *sql.DB
	secret []byte
}

// New returns a Service signing sessions with secret.
func New(conn *sql.DB, secret []byte) *Service {
	return &Service{db: conn, secret: secret}
}

// CreateUser stores a new user with a bcrypt-hashed password.
func (s *Service) CreateUser(ctx context.Context, username, password string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("username is required")
	}
	if len(password) < 8 {
		return User{}, errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, string(hash)).Scan(&id)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: id, Username: username}, nil
}

// Authenticate checks a username/password pair.
func (s *Service) Authenticate(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash FROM users WHERE username = $1`,
		strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Burn comparable time so missing users aren't distinguishable.
		bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0GxaBEyJQov4rRSK3ZLbbiXVSyq"), []byte(password))
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("look up user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

// UserCount reports how many accounts exist.
func (s *Service) UserCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// GetUser loads a user by ID.
func (s *Service) GetUser(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username FROM users WHERE id = $1`, id).Scan(&u.ID, &u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("look up user: %w", err)
	}
	return u, nil
}

// token format: base64(userID:expiryUnix):base64(hmac(userID:expiryUnix))

func (s *Service) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// IssueToken creates a signed session token for a user.
func (s *Service) IssueToken(userID int64, now time.Time) string {
	payload := fmt.Sprintf("%d:%d", userID, now.Add(SessionTTL).Unix())
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + s.sign(payload)
}

// VerifyToken validates a session token and returns the user ID.
func (s *Service) VerifyToken(token string, now time.Time) (int64, error) {
	encoded, sig, ok := strings.Cut(token, ".")
	if !ok {
		return 0, errors.New("malformed token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, errors.New("malformed token")
	}
	payload := string(payloadBytes)
	if !hmac.Equal([]byte(s.sign(payload)), []byte(sig)) {
		return 0, errors.New("invalid token signature")
	}
	idStr, expStr, ok := strings.Cut(payload, ":")
	if !ok {
		return 0, errors.New("malformed token payload")
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() > exp {
		return 0, errors.New("token expired")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, errors.New("malformed token payload")
	}
	return id, nil
}

// SetCookie writes the session cookie on a response.
func SetCookie(w http.ResponseWriter, token string, now time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  now.Add(SessionTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie removes the session cookie.
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// UserID extracts and verifies the session from a request.
func (s *Service) UserID(r *http.Request, now time.Time) (int64, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return 0, errors.New("not logged in")
	}
	return s.VerifyToken(c.Value, now)
}
