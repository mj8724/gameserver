package application

import (
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time      { return c.now }
func (c *fakeClock) Add(d time.Duration) { c.now = c.now.Add(d) }

func newTestAuthenticator(t *testing.T, password string, clock *fakeClock) *Authenticator {
	t.Helper()
	auth, err := NewAuthenticatorWithClock(AuthConfig{AdminPassword: password}, clock)
	if err != nil {
		t.Fatal("NewAuthenticatorWithClock failed")
	}
	return auth
}

func TestAuthenticatorRejectsMissingAndIncorrectConfiguredPassword(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	missing := newTestAuthenticator(t, "", clock)
	if _, err := missing.Login("anything"); codeForTest(err) != CodeAdminPasswordNotConfigured {
		t.Fatalf("unconfigured Login() error code = %q, want %q", codeForTest(err), CodeAdminPasswordNotConfigured)
	}
	if err := missing.Authenticate("malformed"); codeForTest(err) != CodeAdminPasswordNotConfigured {
		t.Fatalf("unconfigured Authenticate() error code = %q, want %q", codeForTest(err), CodeAdminPasswordNotConfigured)
	}

	const password = "sentinel-control-password"
	auth := newTestAuthenticator(t, password, clock)
	if _, err := auth.Login("wrong"); codeForTest(err) != CodeInvalidAdminPassword {
		t.Fatalf("incorrect Login() error code = %q, want %q", codeForTest(err), CodeInvalidAdminPassword)
	}
	if strings.Contains(authErrorForTest(auth, "wrong"), password) {
		t.Fatal("authentication error exposed configured password")
	}
}

func TestAuthenticatorAcceptsTokenAndRejectsTamperedSignature(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	auth := newTestAuthenticator(t, "test-password", clock)
	token, err := auth.Login("test-password")
	if err != nil {
		t.Fatal("Login() failed")
	}
	if err := auth.Authenticate(string(token)); err != nil {
		t.Fatal("Authenticate() rejected freshly issued token")
	}
	parts := strings.Split(string(token), ".")
	if len(parts) != 3 {
		t.Fatalf("token format has %d components, want SID.timestamp.signature", len(parts))
	}
	last := parts[2][len(parts[2])-1]
	if last == '0' {
		parts[2] = parts[2][:len(parts[2])-1] + "1"
	} else {
		parts[2] = parts[2][:len(parts[2])-1] + "0"
	}
	if err := auth.Authenticate(strings.Join(parts, ".")); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("tampered token error code = %q, want %q", codeForTest(err), CodeUnauthenticated)
	}
}

func TestAuthenticatorRejectsFutureAndExpiredTimestamps(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	clock := &fakeClock{now: base}
	auth := newTestAuthenticator(t, "test-password", clock)
	token, err := auth.Login("test-password")
	if err != nil {
		t.Fatal("Login() failed")
	}

	clock.now = base.Add(-time.Second)
	if err := auth.Authenticate(string(token)); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("future timestamp error code = %q, want %q", codeForTest(err), CodeUnauthenticated)
	}
	clock.now = base.Add(SessionTTL)
	if err := auth.Authenticate(string(token)); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("expired timestamp error code = %q, want %q", codeForTest(err), CodeUnauthenticated)
	}
	if got := auth.ActiveSessions(); got != 0 {
		t.Fatalf("expired session count = %d, want 0", got)
	}
}

func TestAuthenticatorLogoutRevokesReplay(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	auth := newTestAuthenticator(t, "test-password", clock)
	token, err := auth.Login("test-password")
	if err != nil {
		t.Fatal("Login() failed")
	}
	if err := auth.Authenticate(string(token)); err != nil {
		t.Fatal("Authenticate() rejected freshly issued token")
	}
	auth.Logout(string(token))
	if err := auth.Authenticate(string(token)); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("replayed token after logout error code = %q, want %q", codeForTest(err), CodeUnauthenticated)
	}
}

func TestAuthenticatorRestartInvalidatesOldTokens(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	firstProcess := newTestAuthenticator(t, "test-password", clock)
	token, err := firstProcess.Login("test-password")
	if err != nil {
		t.Fatal("Login() failed")
	}
	secondProcess := newTestAuthenticator(t, "test-password", clock)
	if err := secondProcess.Authenticate(string(token)); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("new authenticator accepted old process token (code %q)", codeForTest(err))
	}
	if got := firstProcess.ActiveSessions(); got != 1 {
		t.Fatalf("first authenticator lost its in-memory session, count = %d", got)
	}
}

func TestAuthenticatorTokenExpiresAtTwelveHours(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	auth := newTestAuthenticator(t, "test-password", clock)
	token, err := auth.Login("test-password")
	if err != nil {
		t.Fatal("Login() failed")
	}
	clock.Add(SessionTTL - time.Second)
	if err := auth.Authenticate(string(token)); err != nil {
		t.Fatal("session expired before 12-hour TTL")
	}
	clock.Add(time.Second)
	if err := auth.Authenticate(string(token)); codeForTest(err) != CodeUnauthenticated {
		t.Fatalf("session at TTL boundary error code = %q, want %q", codeForTest(err), CodeUnauthenticated)
	}
}

func codeForTest(err error) ErrorCode {
	if code, ok := ErrorCodeOf(err); ok {
		return code
	}
	return ""
}

func authErrorForTest(auth *Authenticator, password string) string {
	_, err := auth.Login(password)
	if err == nil {
		return ""
	}
	return err.Error()
}
