package application

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

const (
	// SessionTTL is the fixed control-plane session lifetime.
	SessionTTL      = 12 * time.Hour
	sessionIDBytes  = 32
	sessionMACBytes = sha256.Size
)

// AuthConfig configures control-plane password validation. An empty password
// intentionally disables authentication and all protected operations.
type AuthConfig struct {
	AdminPassword string
}

// SessionToken is an opaque, signed cookie value. Callers must not log it.
type SessionToken string

// Authenticator creates and validates process-local sessions.
type Authenticator struct {
	password string
	key      [32]byte
	clock    interface{ Now() time.Time }

	mu       sync.Mutex
	sessions map[string]time.Time
}

// NewAuthenticator creates an authenticator with a fresh process-local HMAC
// key. Passing a ports.Clock permits deterministic expiry tests.
func NewAuthenticator(config AuthConfig, clock ports.Clock) (*Authenticator, error) {
	return NewAuthenticatorWithClock(config, clock)
}

// NewAuthenticatorWithClock also accepts small clock test doubles without
// coupling callers to a concrete clock adapter.
func NewAuthenticatorWithClock(config AuthConfig, clock interface{ Now() time.Time }) (*Authenticator, error) {
	if clock == nil {
		clock = wallClock{}
	}
	a := &Authenticator{password: config.AdminPassword, clock: clock, sessions: make(map[string]time.Time)}
	if _, err := rand.Read(a.key[:]); err != nil {
		return nil, errors.New("generate session signing key")
	}
	return a, nil
}

// Configured reports whether a control-plane password has been configured.
func (a *Authenticator) Configured() bool { return a != nil && a.password != "" }

// Login checks the configured password in constant time, then issues a
// timestamped, random-SID HMAC cookie token. Neither password nor token is
// incorporated into an error.
func (a *Authenticator) Login(password string) (SessionToken, error) {
	if !a.Configured() {
		return "", NewError(CodeAdminPasswordNotConfigured, "")
	}
	providedDigest := sha256.Sum256([]byte(password))
	configuredDigest := sha256.Sum256([]byte(a.password))
	if subtle.ConstantTimeCompare(providedDigest[:], configuredDigest[:]) != 1 {
		return "", NewError(CodeInvalidAdminPassword, "")
	}

	sid := make([]byte, sessionIDBytes)
	if _, err := rand.Read(sid); err != nil {
		return "", WrapError(CodeOperationFailed, "could not create session", err)
	}
	now := a.clock.Now()
	issuedAt := now.Unix()
	sidText := hex.EncodeToString(sid)
	timestamp := strconv.FormatInt(issuedAt, 10)
	payload := sidText + "." + timestamp
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte(payload))
	token := SessionToken(payload + "." + hex.EncodeToString(mac.Sum(nil)))
	expiresAt := time.Unix(issuedAt, 0).Add(SessionTTL)

	a.mu.Lock()
	a.pruneExpiredLocked(now)
	a.sessions[sidText] = expiresAt
	a.mu.Unlock()
	return token, nil
}

// Authenticate validates format, timestamp, constant-time signature, and
// server-side SID membership. Future, expired, revoked, and old-process tokens
// all fail closed.
func (a *Authenticator) Authenticate(token string) error {
	if a == nil || !a.Configured() {
		return NewError(CodeAdminPasswordNotConfigured, "")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[0]) != sessionIDBytes*2 || len(parts[2]) != sessionMACBytes*2 {
		return NewError(CodeUnauthenticated, "")
	}
	sidBytes, err := hex.DecodeString(parts[0])
	if err != nil || len(sidBytes) != sessionIDBytes {
		return NewError(CodeUnauthenticated, "")
	}
	sig, err := hex.DecodeString(parts[2])
	if err != nil || len(sig) != sessionMACBytes {
		return NewError(CodeUnauthenticated, "")
	}
	issuedAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return NewError(CodeUnauthenticated, "")
	}
	now := a.clock.Now()
	if issuedAt > now.Unix() {
		return NewError(CodeUnauthenticated, "")
	}
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return NewError(CodeUnauthenticated, "")
	}
	issued := time.Unix(issuedAt, 0)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneExpiredLocked(now)
	expiresAt, ok := a.sessions[parts[0]]
	if !ok || !now.Before(expiresAt) || now.Before(issued) || now.Sub(issued) >= SessionTTL {
		return NewError(CodeUnauthenticated, "")
	}
	return nil
}

// Logout revokes a live session SID so captured cookie values cannot be
// replayed. Invalid tokens are idempotently ignored.
func (a *Authenticator) Logout(token string) {
	if a == nil {
		return
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[0]) != sessionIDBytes*2 {
		return
	}
	sig, err := hex.DecodeString(parts[2])
	if err != nil || len(sig) != sessionMACBytes {
		return
	}
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return
	}
	a.mu.Lock()
	delete(a.sessions, parts[0])
	a.mu.Unlock()
}

// ActiveSessions returns the current live session count for diagnostics/tests;
// it never exposes session IDs or cookie values.
func (a *Authenticator) ActiveSessions() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneExpiredLocked(a.clock.Now())
	return len(a.sessions)
}

func (a *Authenticator) pruneExpiredLocked(now time.Time) {
	for sid, expiresAt := range a.sessions {
		if !now.Before(expiresAt) {
			delete(a.sessions, sid)
		}
	}
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
