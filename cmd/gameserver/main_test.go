package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mj8724/gameserver/internal/adapters/oslock"
)

// legacyFixture writes the Python-era layout an operator would really have:
// a permissive instance directory, a legacy instance.json without the newer
// mods/billing keys, and an INI with comments plus an unmanaged key.
func legacyFixture(t *testing.T) (dataRoot, serversRoot string) {
	t.Helper()
	root := t.TempDir()
	dataRoot = filepath.Join(root, "data")
	serversRoot = filepath.Join(dataRoot, "servers")
	instanceDir := filepath.Join(serversRoot, "pz_01")
	for _, dir := range []string{filepath.Join(instanceDir, "Zomboid", "Server"), filepath.Join(instanceDir, "server_files")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	legacy := `{"instance_id":"pz_01","name":"Project Zomboid Dedicated Server","template_id":"project_zomboid",
 "variables":{"SERVER_NAME":"servertest","MAX_PLAYERS":16},"ports":{"SERVER_PORT":16261,"DIRECT_PORT":16262},
 "quota_gb":30.0}`
	if err := os.WriteFile(filepath.Join(instanceDir, "instance.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}
	ini := "# legacy comment must survive\nPublic=true\nMaxPlayers=16\nUnknownKey=keep-me\n"
	if err := os.WriteFile(filepath.Join(instanceDir, "Zomboid", "Server", "servertest.ini"), []byte(ini), 0o644); err != nil {
		t.Fatalf("write ini: %v", err)
	}
	return dataRoot, serversRoot
}

func testConfig(t *testing.T, dataRoot string) runtimeConfig {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return runtimeConfig{
		Host:              "127.0.0.1",
		Port:              0,
		DataRoot:          dataRoot,
		StaticDir:         filepath.Join(repo, "static"),
		TemplatesDir:      filepath.Join(repo, "templates"),
		Instance:          "pz_01",
		AdminPass:         "m2-offline-admin",
		LaunchExecutable:  "ProjectZomboid64",
		LaunchDirectExec:  true,
		LaunchEvidenceRef: "manifest#m2-offline",
	}
}

func call(t *testing.T, client *http.Client, method, url, origin string, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, url, err)
	}
	return response.StatusCode, string(payload)
}

// TestM2OfflineHTTPBlackBox walks the M2-API/M2-CONFIG/M2-SECRET/M2-AUTH rows
// against the real component graph: real state store, real PZ INI adapter, real
// templates and the real handler — only the port is injected by httptest.
func TestM2OfflineHTTPBlackBox(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	rt, err := buildRuntime(testConfig(t, dataRoot))
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	defer rt.handler.Close()

	server := httptest.NewServer(rt.handler)
	defer server.Close()
	client := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client.Jar = jar
	origin := server.URL

	if status, body := call(t, client, "GET", server.URL+"/api/auth/status", "", ""); status != 200 || body != `{"authenticated":false}` {
		t.Fatalf("anonymous auth status = %d %s", status, body)
	}
	if status, _ := call(t, client, "GET", server.URL+"/api/status", "", ""); status != 401 {
		t.Fatalf("unauthenticated status = %d, want 401", status)
	}
	if status, body := call(t, client, "POST", server.URL+"/api/auth/login", "", `{}`); status != 403 || body != `{"detail":"跨站请求已拒绝"}` {
		t.Fatalf("missing origin login = %d %s", status, body)
	}
	if status, body := call(t, client, "POST", server.URL+"/api/auth/login", server.URL, `{"password":"wrong"}`); status != 401 {
		t.Fatalf("wrong password = %d %s", status, body)
	}
	if status, _ := call(t, client, "POST", server.URL+"/api/auth/login", server.URL, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("login = %d", status)
	}

	if status, body := call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"MAX_PLAYERS":99}}`); status != 422 || body != `{"detail":"MAX_PLAYERS 超出允许范围"}` {
		t.Fatalf("range validation = %d %s", status, body)
	}
	if status, _ := call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"NOPE":1}}`); status != 422 {
		t.Fatalf("unknown field = %d, want 422", status)
	}

	// D8 gate: a legacy root with permissive modes must fail closed (no partial
	// write, no silent chmod) until the operator normalizes it.
	status, body := call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"MAX_PLAYERS":24}}`)
	if status != 500 || body != `{"detail":"保存配置失败"}` {
		t.Fatalf("permissive root update = %d %s, want fail-closed 500", status, body)
	}
	if _, err := os.Stat(filepath.Join(serversRoot, "pz_01", "state", "instance.json")); !os.IsNotExist(err) {
		t.Fatalf("fail-closed update still wrote state (err=%v)", err)
	}
	if info, err := os.Stat(filepath.Join(serversRoot, "pz_01")); err != nil {
		t.Fatalf("stat instance dir: %v", err)
	} else if info.Mode().Perm() != 0o755 {
		t.Fatalf("instance dir mode changed implicitly to %o", info.Mode().Perm())
	}
	if changed, err := normalizePermissions(dataRoot, "pz_01"); err != nil || changed == 0 {
		t.Fatalf("normalizePermissions = %d, %v", changed, err)
	}

	status, body = call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"MAX_PLAYERS":24}}`)
	if status != 200 || !strings.Contains(body, "配置已保存并同步") {
		t.Fatalf("valid update after normalize = %d %s", status, body)
	}

	state, err := os.ReadFile(filepath.Join(serversRoot, "pz_01", "state", "instance.json"))
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	if !strings.Contains(string(state), `"MAX_PLAYERS": 24`) {
		t.Fatalf("state not persisted: %s", state)
	}
	if info, err := os.Stat(filepath.Join(serversRoot, "pz_01", "state", "instance.json")); err != nil {
		t.Fatalf("stat state: %v", err)
	} else if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %o, want 600", perm)
	}
	ini, err := os.ReadFile(filepath.Join(serversRoot, "pz_01", "Zomboid", "Server", "servertest.ini"))
	if err != nil {
		t.Fatalf("read ini: %v", err)
	}
	for _, want := range []string{"# legacy comment must survive", "UnknownKey=keep-me", "MaxPlayers=24"} {
		if !strings.Contains(string(ini), want) {
			t.Fatalf("ini missing %q:\n%s", want, ini)
		}
	}

	status, body = call(t, client, "GET", server.URL+"/api/status", origin, "")
	if status != 200 {
		t.Fatalf("status = %d %s", status, body)
	}
	var projection map[string]any
	if err := json.Unmarshal([]byte(body), &projection); err != nil {
		t.Fatalf("status json: %v", err)
	}
	for _, field := range []string{"billing", "cpu_percent", "disk", "install_task", "is_installed", "memory_mb",
		"mods", "name", "pid", "platform", "ports", "readiness", "ready", "running", "status", "steamcmd"} {
		if _, ok := projection[field]; !ok {
			t.Fatalf("status missing legacy field %q: %s", field, body)
		}
	}
	if strings.Contains(body, "m2-offline-admin") {
		t.Fatal("status leaked the admin password value")
	}
	if !strings.Contains(body, `"ADMIN_PASSWORD":null`) && !strings.Contains(body, `"ADMIN_PASSWORD":""`) {
		t.Fatalf("status did not redact ADMIN_PASSWORD: %s", body)
	}

	status, body = call(t, client, "GET", server.URL+"/api/server/config", origin, "")
	if status != 200 {
		t.Fatalf("config = %d %s", status, body)
	}
	if !strings.Contains(body, `"ADMIN_PASSWORD":null`) || !strings.Contains(body, `"SERVER_PASSWORD":""`) {
		t.Fatalf("password projection wrong: %s", body)
	}
	if strings.Contains(body, "m2-offline-admin") {
		t.Fatal("config projection leaked the admin password")
	}

	status, body = call(t, client, "GET", server.URL+"/api/server/logs", origin, "")
	if status != 200 || !strings.Contains(body, `"logs"`) {
		t.Fatalf("logs = %d %s", status, body)
	}
	if status, _ := call(t, client, "POST", server.URL+"/api/server/command", origin, `{"command":"help"}`); status != 400 {
		t.Fatalf("command while stopped = %d, want 400", status)
	}

	if status, _ := call(t, client, "POST", server.URL+"/api/auth/logout", origin, `{}`); status != 200 {
		t.Fatalf("logout = %d", status)
	}
	if status, body := call(t, client, "GET", server.URL+"/api/status", origin, ""); status != 401 || body != `{"detail":"请先登录"}` {
		t.Fatalf("replay after logout = %d %s", status, body)
	}

	manager, err := oslock.NewManager(serversRoot, "test-runtime")
	if err != nil {
		t.Fatalf("lock manager: %v", err)
	}
	lease, err := manager.AcquireInstance("pz_01", oslock.OwnerMeta{OperationID: "m2-offline-blackbox"})
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = lease.Release() }()

	if status, _ := call(t, client, "POST", server.URL+"/api/auth/login", origin, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("re-login = %d", status)
	}
	status, body = call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"MAX_PLAYERS":32}}`)
	if status != 409 || body != `{"detail":"instance owned by another process"}` {
		t.Fatalf("locked mutation = %d %s", status, body)
	}
	stored, err := os.ReadFile(filepath.Join(serversRoot, "pz_01", "state", "instance.json"))
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if strings.Contains(string(stored), `"MAX_PLAYERS": 32`) {
		t.Fatal("locked mutation still wrote state")
	}
	if status, _ := call(t, client, "GET", server.URL+"/api/status", origin, ""); status != 200 {
		t.Fatalf("read-only status while locked = %d, want 200", status)
	}
	fmt.Fprintln(os.Stderr, "blackbox evidence: routes, exact error strings, projection, persistence, permissions, lock fencing")
}

// TestM2OfflineRestartPersistence covers the offline part of M2-RESTART: a
// normal stop followed by a fresh process over the same data root must read the
// committed state back and must start with a clean ownership reconciliation.
func TestM2OfflineRestartPersistence(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
		t.Fatalf("normalizePermissions: %v", err)
	}

	first, err := buildRuntime(testConfig(t, dataRoot))
	if err != nil {
		t.Fatalf("first runtime: %v", err)
	}
	firstServer := httptest.NewServer(first.handler)
	jar, _ := cookiejar.New(nil)
	client := firstServer.Client()
	client.Jar = jar
	if status, _ := call(t, client, "POST", firstServer.URL+"/api/auth/login", firstServer.URL, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("first login = %d", status)
	}
	if status, body := call(t, client, "POST", firstServer.URL+"/api/server/config", firstServer.URL, `{"variables":{"MAX_PLAYERS":24}}`); status != 200 {
		t.Fatalf("first update = %d %s", status, body)
	}
	first.handler.Close()
	firstServer.Close()

	second, err := buildRuntime(testConfig(t, dataRoot))
	if err != nil {
		t.Fatalf("second runtime: %v", err)
	}
	defer second.handler.Close()
	secondServer := httptest.NewServer(second.handler)
	defer secondServer.Close()
	jar2, _ := cookiejar.New(nil)
	client2 := secondServer.Client()
	client2.Jar = jar2
	if status, _ := call(t, client2, "POST", secondServer.URL+"/api/auth/login", secondServer.URL, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("second login = %d", status)
	}
	status, body := call(t, client2, "GET", secondServer.URL+"/api/server/config", secondServer.URL, "")
	if status != 200 || !strings.Contains(body, `"MAX_PLAYERS":24`) {
		t.Fatalf("config after restart = %d %s", status, body)
	}
	if status, body := call(t, client2, "POST", secondServer.URL+"/api/server/config", secondServer.URL, `{"variables":{"MAX_PLAYERS":32}}`); status != 200 {
		t.Fatalf("post-restart mutation = %d %s (ownership reconciliation must be clean)", status, body)
	}
	if _, err := os.Stat(filepath.Join(serversRoot, ".owners", "pz_01.json")); !os.IsNotExist(err) {
		t.Fatalf("ownership record must not outlive a clean run (err=%v)", err)
	}
}

// TestStaticUIContract checks the shipped UI against the served contract:
// every endpoint the UI calls must exist in the route table, the login-error
// branch must key off the exact server string, D3 error frames must not be
// rendered as logs, and password fields must never be prefilled.
func TestStaticUIContract(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	uiRaw, err := os.ReadFile(filepath.Join(repo, "static", "index.html"))
	if err != nil {
		t.Fatalf("read static UI: %v", err)
	}
	ui := string(uiRaw)
	server, err := os.ReadFile(filepath.Join(repo, "internal", "adapters", "httpapi", "server.go"))
	if err != nil {
		t.Fatalf("read route table: %v", err)
	}
	routes := string(server)

	endpointPattern := regexp.MustCompile(`/api/[a-zA-Z0-9_/]+`)
	seen := map[string]bool{}
	for _, match := range endpointPattern.FindAllString(ui, -1) {
		seen[match] = true
	}
	if len(seen) < 6 {
		t.Fatalf("expected the UI to call the legacy endpoints, found %v", seen)
	}
	for endpoint := range seen {
		if !strings.Contains(routes, endpoint) {
			t.Fatalf("UI calls %s but the route table has no such route", endpoint)
		}
	}
	for _, required := range []string{"/api/auth/status", "/api/auth/login", "/api/auth/logout", "/api/status",
		"/api/server/config", "/api/server/logs", "/api/server/command", "/ws/console"} {
		if !strings.Contains(ui, required) {
			t.Fatalf("UI no longer uses %s (legacy call surface changed)", required)
		}
	}
	if !strings.Contains(ui, `error.message==='请先登录'`) || !strings.Contains(routes, "请先登录") {
		t.Fatal("login-expiry branch or its exact server string drifted")
	}
	if !strings.Contains(ui, `input.value=field.value??''`) || strings.Contains(ui, "input.value=field.default") {
		t.Fatal("config fields must render from value, never from the template default")
	}
	if !strings.Contains(ui, `type==='password'){input.placeholder='留空表示保持当前口令'}`) {
		t.Fatal("password fields must stay empty and explain the keep-current semantics")
	}

	handlerStart := strings.Index(ui, "socket.onmessage=")
	if handlerStart < 0 {
		t.Fatal("UI has no console onmessage handler")
	}
	handlerEnd := strings.Index(ui[handlerStart:], "};")
	if handlerEnd < 0 {
		t.Fatal("cannot delimit the onmessage handler")
	}
	handler := ui[handlerStart : handlerStart+handlerEnd]
	if !strings.Contains(handler, `if(message.type==='log')`) {
		t.Fatal("D3 error frames must be filtered by type==='log'")
	}
	if strings.Count(handler, ".textContent+=") != 1 {
		t.Fatalf("only the log branch may render frames, handler=%s", handler)
	}
}

// TestM2OfflineRecoveryRequiredReconciliation covers the offline part of
// M2-RESTART row 2: a stale ownership record (dead pid) must keep mutations
// fail-closed, must not be auto-cleaned, and must still allow read-only status.
func TestM2OfflineRecoveryRequiredReconciliation(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
		t.Fatalf("normalizePermissions: %v", err)
	}
	manager, err := oslock.NewManager(serversRoot, "cursor-process")
	if err != nil {
		t.Fatalf("lock manager: %v", err)
	}
	ownerPath, err := manager.OwnerRecordPath("pz_01")
	if err != nil {
		t.Fatalf("owner path: %v", err)
	}
	stale := fmt.Sprintf(`{"schema":"gameserver-owner/1","service_id":"crashed-process","pid":%d,"session_token":"stale",
 "started_at":"2020-01-01T00:00:00Z","root":%q}`, 999999, filepath.Join(serversRoot, "pz_01"))
	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o700); err != nil {
		t.Fatalf("mkdir owners: %v", err)
	}
	if err := os.WriteFile(ownerPath, []byte(stale), 0o600); err != nil {
		t.Fatalf("write stale owner: %v", err)
	}

	rt, err := buildRuntime(testConfig(t, dataRoot))
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	defer rt.handler.Close()
	server := httptest.NewServer(rt.handler)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := server.Client()
	client.Jar = jar
	if status, _ := call(t, client, "POST", server.URL+"/api/auth/login", server.URL, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("login = %d", status)
	}
	if status, body := call(t, client, "GET", server.URL+"/api/status", server.URL, ""); status != 200 || !strings.Contains(body, "status") {
		t.Fatalf("read-only status = %d %s", status, body)
	}
	status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"variables":{"MAX_PLAYERS":24}}`)
	if status != 409 {
		t.Fatalf("stale owner mutation = %d %s, want 409 fail-closed", status, body)
	}
	if _, err := os.Stat(ownerPath); err != nil {
		t.Fatalf("stale ownership record must not be auto-deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serversRoot, "pz_01", "state", "instance.json")); !os.IsNotExist(err) {
		t.Fatalf("fail-closed mutation must not write state (err=%v)", err)
	}
}
