package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"document-tools/internal/testdb"
)

func TestTokenRoundTrip(t *testing.T) {
	s := New(nil, []byte("secret"))
	now := time.Now()

	token := s.IssueToken(42, now)
	id, err := s.VerifyToken(token, now)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestTokenExpiry(t *testing.T) {
	s := New(nil, []byte("secret"))
	now := time.Now()
	token := s.IssueToken(42, now)

	if _, err := s.VerifyToken(token, now.Add(SessionTTL+time.Hour)); err == nil {
		t.Error("expected expired token to fail")
	}
}

func TestTokenTampering(t *testing.T) {
	s := New(nil, []byte("secret"))
	other := New(nil, []byte("different-secret"))
	now := time.Now()

	if _, err := s.VerifyToken(other.IssueToken(42, now), now); err == nil {
		t.Error("expected token from another secret to fail")
	}
	if _, err := s.VerifyToken("garbage", now); err == nil {
		t.Error("expected malformed token to fail")
	}
}

func TestUserLifecycle(t *testing.T) {
	conn := testdb.Open(t, "auth")
	s := New(conn, []byte("secret"))
	ctx := context.Background()

	if n, _ := s.UserCount(ctx); n != 0 {
		t.Fatalf("UserCount = %d, want 0", n)
	}

	user, err := s.CreateUser(ctx, "jacob", "correct-horse")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.Authenticate(ctx, "jacob", "correct-horse")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("id = %d, want %d", got.ID, user.ID)
	}

	if _, err := s.Authenticate(ctx, "jacob", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v", err)
	}
	if _, err := s.Authenticate(ctx, "nobody", "whatever"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown user err = %v", err)
	}

	if _, err := s.CreateUser(ctx, "x", "short"); err == nil {
		t.Error("expected short password to be rejected")
	}
	if _, err := s.CreateUser(ctx, "", "long-enough-pass"); err == nil {
		t.Error("expected empty username to be rejected")
	}
}
