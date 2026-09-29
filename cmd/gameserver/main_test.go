package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/adapters/instancefiles"
	"github.com/mj8724/gameserver/internal/adapters/localstate"
	"github.com/mj8724/gameserver/internal/adapters/oslock"
	"github.com/mj8724/gameserver/internal/adapters/pztemplate"
	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
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

// launchArtifactName mirrors the platform's directly executable server artifact.
func launchArtifactName() string {
	if runtime.GOOS == "windows" {
		return "ProjectZomboid64.exe"
	}
	return "ProjectZomboid64"
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
		LaunchExecutable:  launchArtifactName(),
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

	// D8 gate is POSIX-only: Windows has no POSIX modes and the adapters exempt
	// it, so the fail-closed phase and the chmod normalization only run on POSIX.
	if runtime.GOOS != "windows" {
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
	}

	status, body := call(t, client, "POST", server.URL+"/api/server/config", origin, `{"variables":{"MAX_PLAYERS":24}}`)
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
	} else if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
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
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
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
		"/api/server/config", "/api/server/logs", "/api/server/command", "/ws/console",
		// Stage 5 additions: game/version selection and progress tracking.
		"/api/templates", "/api/server/install"} {
		if !strings.Contains(ui, required) {
			t.Fatalf("UI no longer uses %s (legacy call surface changed)", required)
		}
	}
	// The console renders the catalogue: grouped options, typed controls and the
	// restart hint, without promising that a saved value is already live.
	for _, required := range []string{"options-groups", "option-search", "options-save", "install-bar",
		"version-select", "catalog_degraded", "pending_restart", "重启后生效", "留空表示保持当前值"} {
		if !strings.Contains(ui, required) {
			t.Fatalf("UI is missing the stage-5 element %q", required)
		}
	}
	if size := len(uiRaw); size > 256*1024 {
		t.Fatalf("static UI grew beyond the agreed budget: %d bytes", size)
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
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
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

// TestSteamcmdInstallConfigResolution pins the fail-closed installer wiring:
// without an executable or app id the service must not pretend installs work,
// and the SteamCMD tree must stay inside the instance root (ADR §4).
func TestSteamcmdInstallConfigResolution(t *testing.T) {
	installDir := func(domain.InstanceID) (string, error) { return filepath.Join("tmp", "install"), nil }
	serversRoot := filepath.Join("tmp", "data", "servers")

	if _, ok := steamcmdInstallConfig(runtimeConfig{}, serversRoot, "pz_01", "380870", installDir); ok {
		t.Fatal("missing executable must not configure the installer")
	}
	if _, ok := steamcmdInstallConfig(runtimeConfig{SteamCMDExecutable: "steamcmd"}, serversRoot, "pz_01", "", installDir); ok {
		t.Fatal("missing app id must not configure the installer")
	}
	config, ok := steamcmdInstallConfig(runtimeConfig{SteamCMDExecutable: "steamcmd"}, serversRoot, "pz_01", "380870", installDir)
	if !ok {
		t.Fatal("configured installer expected")
	}
	if want := filepath.Join(serversRoot, "pz_01", "steamcmd"); config.SteamDir != want {
		t.Fatalf("steam dir = %q, want %q", config.SteamDir, want)
	}
	if config.AppID != "380870" || config.Executable != "steamcmd" {
		t.Fatalf("config = %+v", config)
	}
	explicitDir := filepath.Join("opt", "steamcmd")
	override, ok := steamcmdInstallConfig(runtimeConfig{SteamCMDExecutable: "steamcmd", SteamCMDDir: explicitDir},
		serversRoot, "pz_01", "380870", installDir)
	if !ok || override.SteamDir != explicitDir {
		t.Fatalf("explicit steam dir override not honoured: %+v ok=%t", override, ok)
	}
}

// TestRuntimeConfiguresInstallerFromTemplate loads the shipped template so the
// app id is taken from real template metadata, not a hardcoded constant.
func TestRuntimeConfiguresInstallerFromTemplate(t *testing.T) {
	dataRoot, _ := legacyFixture(t)
	cfg := testConfig(t, dataRoot)
	cfg.SteamCMDExecutable = "steamcmd"
	rt, err := buildRuntime(cfg)
	if err != nil {
		t.Fatalf("buildRuntime: %v", err)
	}
	defer rt.handler.Close()
	templateAppID := ""
	templates, err := pztemplate.New(cfg.TemplatesDir)
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if template, ok := templates.Get("project_zomboid"); ok {
		templateAppID = template.Summary.AppID
	}
	if templateAppID == "" {
		t.Fatal("shipped template must carry steam.app_id so the installer can be configured")
	}
	defaults, err := localstate.DefaultState("pz_01")
	if err != nil {
		t.Fatalf("default state: %v", err)
	}
	states, err := localstate.NewStore(dataRoot, defaults)
	if err != nil {
		t.Fatalf("state store: %v", err)
	}
	files, err := instancefiles.New(filepath.Join(dataRoot, "servers"), "darwin")
	if err != nil {
		t.Fatalf("instance files: %v", err)
	}
	installer, err := resolveInstaller(cfg, filepath.Join(dataRoot, "servers"), "pz_01", states, templates, files)
	if err != nil {
		t.Fatalf("resolveInstaller: %v", err)
	}
	if installer == nil {
		t.Fatal("installer must be wired when executable and template app id are present")
	}
}

// ---- end-to-end install/start/stop with injected fakes -------------------

// fakeInstaller is driven from the install goroutine and inspected by the test,
// so every field is mutex guarded (the race detector caught this on CI).
type fakeInstaller struct {
	mu         sync.Mutex
	installed  bool
	requests   []ports.InstallRequest
	installDir func(domain.InstanceID) (string, error)
}

func (f *fakeInstaller) Install(_ context.Context, request ports.InstallRequest, progress func(ports.Progress)) error {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	progress(ports.Progress{Percent: 50, Message: "下载中"})
	if f.installDir == nil {
		return nil
	}
	installDir, err := f.installDir(request.InstanceID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		return err
	}
	// Mirror the real artifact name so the presence check flips to installed.
	if err := os.WriteFile(filepath.Join(installDir, launchArtifactName()), []byte("stub"), 0o700); err != nil {
		return err
	}
	f.mu.Lock()
	f.installed = true
	f.mu.Unlock()
	progress(ports.Progress{Percent: 100, Message: "完成"})
	return nil
}

func (f *fakeInstaller) request(index int) ports.InstallRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[index]
}

func (f *fakeInstaller) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// fakeSupervisor is called from request handlers and from the readiness
// goroutine, so all state is mutex guarded.
type fakeSupervisor struct {
	mu        sync.Mutex
	running   bool
	pid       int
	specs     []ports.LaunchSpec
	inputs    []string
	logs      []string
	listeners int
}

func (f *fakeSupervisor) Start(_ context.Context, spec ports.LaunchSpec) (ports.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, spec)
	f.running = true
	f.pid = 4242
	f.logs = append(f.logs, "fake server started")
	return ports.Process{PID: f.pid}, nil
}
func (f *fakeSupervisor) Stop(context.Context, domain.InstanceID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = false
	f.pid = 0
	f.logs = append(f.logs, "fake server stopped")
	return nil
}
func (f *fakeSupervisor) Kill(context.Context, domain.InstanceID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = false
	f.pid = 0
	return nil
}
func (f *fakeSupervisor) SendInput(_ context.Context, _ domain.InstanceID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, text)
	return nil
}
func (f *fakeSupervisor) Recent(limit int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit > 0 && len(f.logs) > limit {
		return append([]string(nil), f.logs[len(f.logs)-limit:]...)
	}
	return append([]string(nil), f.logs...)
}
func (f *fakeSupervisor) Subscribe(buffer int) ports.LogSubscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listeners++
	return &fakeSubscription{lines: make(chan string, buffer), closed: make(chan struct{})}
}
func (f *fakeSupervisor) ProcessStatus(context.Context, domain.InstanceID) (ports.ProcessStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running {
		return ports.ProcessStatus{Running: true, Status: "RUNNING", PID: f.pid, CPUPercent: 1.5, MemoryMB: 512}, nil
	}
	return ports.ProcessStatus{Running: false, Status: "STOPPED"}, nil
}
func (f *fakeSupervisor) specCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.specs)
}
func (f *fakeSupervisor) spec(index int) ports.LaunchSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.specs[index]
}
func (f *fakeSupervisor) inputCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inputs)
}
func (f *fakeSupervisor) input(index int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inputs[index]
}

type fakeSubscription struct {
	lines  chan string
	closed chan struct{}
}

func (s *fakeSubscription) Lines() <-chan string { return s.lines }
func (s *fakeSubscription) Close()               { close(s.closed) }

type fakeReadiness struct{ ready bool }

func (f fakeReadiness) Ready(context.Context, domain.InstanceID) (bool, error) { return f.ready, nil }

// TestM2OfflineInstallStartStopLifecycle drives install -> start -> command ->
// stop through the real HTTP surface with only the outbound game adapters
// replaced, so the transport, control service, state store and PZ INI adapter
// are the production ones.
func TestM2OfflineInstallStartStopLifecycle(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
	}
	platform := runtime.GOOS
	files, err := instancefiles.New(serversRoot, platform)
	if err != nil {
		t.Fatalf("instance files: %v", err)
	}
	installer := &fakeInstaller{installDir: files.InstallDir}
	supervisor := &fakeSupervisor{}
	rt, err := buildRuntimeWith(testConfig(t, dataRoot), runtimeOverrides{
		Installer: installer, Processes: supervisor, ProcessStatus: supervisor, Logs: supervisor,
		Readiness: fakeReadiness{ready: true},
	})
	if err != nil {
		t.Fatalf("buildRuntimeWith: %v", err)
	}
	defer rt.handler.Close()
	server := httptest.NewServer(rt.handler)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := server.Client()
	client.Jar = jar
	origin := server.URL

	if status, _ := call(t, client, "POST", origin+"/api/auth/login", origin, `{"password":"m2-offline-admin"}`); status != 200 {
		t.Fatalf("login = %d", status)
	}
	if status, body := call(t, client, "POST", origin+"/api/server/start", origin, `{}`); status == 200 {
		t.Fatalf("start before install must fail, got %d %s", status, body)
	}
	if status, body := call(t, client, "POST", origin+"/api/server/install", origin, `{}`); status != 202 && status != 200 {
		t.Fatalf("install = %d %s", status, body)
	}
	// The install runs in a goroutine, so wait for it instead of assuming it was
	// scheduled before the POST returned (this raced on CI).
	requestDeadline := time.Now().Add(5 * time.Second)
	for installer.requestCount() == 0 && time.Now().Before(requestDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if installer.requestCount() != 1 {
		t.Fatalf("installer requests = %d", installer.requestCount())
	}
	dir, err := files.InstallDir("pz_01")
	if err != nil {
		t.Fatalf("install dir: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join("pz_01", "server_files")) {
		t.Fatalf("install dir = %q", dir)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, body := call(t, client, "GET", origin+"/api/status", origin, "")
		if status == 200 && strings.Contains(body, `"is_installed":true`) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	status, body := call(t, client, "GET", origin+"/api/status", origin, "")
	if status != 200 || !strings.Contains(body, `"is_installed":true`) {
		t.Fatalf("install did not complete: %d %s", status, body)
	}

	if status, body := call(t, client, "POST", origin+"/api/server/start", origin, `{}`); status != 200 {
		t.Fatalf("start = %d %s", status, body)
	}
	if supervisor.specCount() != 1 {
		t.Fatalf("launch specs = %d", supervisor.specCount())
	}
	spec := supervisor.spec(0)
	if !strings.HasSuffix(spec.Executable, "ProjectZomboid64") && !strings.HasSuffix(spec.Executable, "ProjectZomboid64.exe") {
		t.Fatalf("launch executable = %q", spec.Executable)
	}
	joined := strings.Join(spec.Args, " ")
	for _, want := range []string{"-cachedir=", "-servername=servertest", "-adminpassword="} {
		if !strings.Contains(joined, want) {
			t.Fatalf("launch argv missing %s: %v", want, spec.Args)
		}
	}
	status, body = call(t, client, "GET", origin+"/api/status", origin, "")
	if status != 200 || !strings.Contains(body, `"running":true`) || !strings.Contains(body, `"pid":4242`) {
		t.Fatalf("running status = %d %s", status, body)
	}
	if status, body := call(t, client, "POST", origin+"/api/server/command", origin, `{"command":"save"}`); status != 200 {
		t.Fatalf("command = %d %s", status, body)
	}
	if supervisor.inputCount() != 1 || supervisor.input(0) != "save" {
		t.Fatalf("console inputs = %d", supervisor.inputCount())
	}
	if status, body := call(t, client, "POST", origin+"/api/server/stop", origin, `{}`); status != 200 {
		t.Fatalf("stop = %d %s", status, body)
	}
	status, body = call(t, client, "GET", origin+"/api/status", origin, "")
	if status != 200 || !strings.Contains(body, `"running":false`) {
		t.Fatalf("stopped status = %d %s", status, body)
	}
	if !strings.Contains(body, `"status":"STOPPED"`) {
		t.Fatalf("status enum after stop = %s", body)
	}
}

// TestLauncherDescriptorVectorUsesBundledJRE proves the descriptor vector runs
// the bundled JRE with typed argv and never falls back to the vendor .bat or a
// shell: the recorded executable set must be exactly the JRE.
func TestLauncherDescriptorVectorUsesBundledJRE(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
	}
	installDir, err := instancefiles.New(serversRoot, runtime.GOOS)
	if err != nil {
		t.Fatalf("instance files: %v", err)
	}
	dir, err := installDir.InstallDir("pz_01")
	if err != nil {
		t.Fatalf("install dir: %v", err)
	}
	javaName := "java"
	if runtime.GOOS == "windows" {
		javaName = "java.exe"
	}
	// Vendor-shaped artifacts plus the presence marker IsInstalled checks.
	for _, path := range []string{filepath.Join(dir, "jre64", "bin", javaName), filepath.Join(dir, "ProjectZomboid64")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stub"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	descriptor := `{"mainClass":"zombie/network/GameServer","classpath":["java/.","java/projectzomboid.jar"],
 "vmArgs":["-Djava.awt.headless=true","-Xmx3072m","-Dzomboid.steam=1","-Dzomboid.znetlog=1","-Djava.library.path=natives/","-XX:-CreateCoredumpOnCrash","-XX:-OmitStackTraceInFastThrow"],
 "windows":{"10":{"vmArgs":["-XX:+UseZGC"]}}}`
	if err := os.WriteFile(filepath.Join(dir, "ProjectZomboid64.json"), []byte(descriptor), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, dataRoot)
	cfg.LaunchVector = "launcher-descriptor"
	cfg.LaunchExecutable = ""
	cfg.LaunchDirectExec = false
	supervisor := &fakeSupervisor{}
	rt, err := buildRuntimeWith(cfg, runtimeOverrides{Processes: supervisor, ProcessStatus: supervisor, Logs: supervisor})
	if err != nil {
		t.Fatalf("buildRuntimeWith: %v", err)
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
	if status, body := call(t, client, "POST", server.URL+"/api/server/start", server.URL, `{}`); status != 200 {
		t.Fatalf("start = %d %s", status, body)
	}
	if supervisor.specCount() != 1 {
		t.Fatalf("launch specs = %d", supervisor.specCount())
	}
	spec := supervisor.spec(0)
	if !strings.HasSuffix(spec.Executable, filepath.Join("jre64", "bin", javaName)) {
		t.Fatalf("descriptor vector must execute the bundled JRE, got %q", spec.Executable)
	}
	if spec.WorkDir != dir {
		t.Fatalf("workdir = %q, want %q", spec.WorkDir, dir)
	}
	joined := strings.Join(spec.Args, " ")
	for _, want := range []string{"-cp java/", "zombie/network/GameServer", "-statistic 0", "-cachedir=", "-adminpassword="} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q: %v", want, spec.Args)
		}
	}
	for _, forbidden := range []string{".bat", "cmd.exe", "cmd /c", "StartServer64"} {
		if strings.Contains(strings.ToLower(joined), strings.ToLower(forbidden)) || strings.Contains(strings.ToLower(spec.Executable), strings.ToLower(forbidden)) {
			t.Fatalf("descriptor vector must never use %q (executable=%q argv=%v)", forbidden, spec.Executable, spec.Args)
		}
	}
}

// TestOptionCatalogStaysOutOfTemplates pins two catalogue guarantees: the
// catalogue is not a game template (it must never surface in /api/templates),
// and the template loader stays warning-free.
func TestOptionCatalogStaysOutOfTemplates(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	catalogPath := filepath.Join(repo, "catalogs", "project_zomboid.options.yaml")
	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatalf("option catalogue missing: %v", err)
	}
	templates, err := pztemplate.New(filepath.Join(repo, "templates"))
	if err != nil {
		t.Fatalf("template catalog: %v", err)
	}
	if warnings := templates.Warnings(); len(warnings) != 0 {
		t.Fatalf("template loader must stay warning-free: %v", warnings)
	}
	if _, ok := templates.Get("project_zomboid.options"); ok {
		t.Fatal("the option catalogue must not be loaded as a game template")
	}
	for _, summary := range templates.List() {
		if strings.Contains(string(summary.ID), "options") {
			t.Fatalf("catalogue leaked into templates: %s", summary.ID)
		}
	}
}

// GET /api/server/config keeps the legacy fields[] shape and adds the
// catalogue-driven options[]/groups[] read back from the vendor files.
func TestM2OfflineConfigExposesCatalogueOptions(t *testing.T) {
	dataRoot, _ := legacyFixture(t)
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
	}
	cfg := testConfig(t, dataRoot)
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CatalogsDir = filepath.Join(repo, "catalogs")
	rt, err := buildRuntime(cfg)
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
	status, body := call(t, client, "GET", server.URL+"/api/server/config", server.URL, "")
	if status != 200 {
		t.Fatalf("config = %d %s", status, body)
	}
	var payload struct {
		Fields  []map[string]any `json:"fields"`
		Options []struct {
			Key             string   `json:"key"`
			Target          string   `json:"target"`
			Type            string   `json:"type"`
			Secret          bool     `json:"secret"`
			Value           any      `json:"value"`
			Group           string   `json:"group"`
			Writable        string   `json:"writable"`
			RequiresRestart bool     `json:"requires_restart"`
			Source          string   `json:"source"`
			Enum            []string `json:"enum"`
		} `json:"options"`
		Groups          []string `json:"groups"`
		CatalogDegraded bool     `json:"catalog_degraded"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("config json: %v", err)
	}
	if payload.CatalogDegraded {
		t.Fatal("catalogue must load from the repository directory")
	}
	if len(payload.Fields) == 0 {
		t.Fatal("legacy fields[] must stay present")
	}
	for _, field := range payload.Fields {
		for _, required := range []string{"key", "label", "type", "value", "user_editable"} {
			if _, ok := field[required]; !ok {
				t.Fatalf("legacy field missing %q: %v", required, field)
			}
		}
	}
	if len(payload.Options) < 400 {
		t.Fatalf("options[] too small: %d", len(payload.Options))
	}
	if len(payload.Groups) == 0 {
		t.Fatal("groups[] must be reported")
	}
	readonly, secret, sandbox := 0, 0, 0
	for _, option := range payload.Options {
		if option.Writable != "rw" {
			readonly++
		}
		if option.Secret {
			secret++
			if option.Value != nil {
				t.Fatalf("secret option leaked a value: %+v", option)
			}
		}
		if option.Target == "sandboxvars" {
			sandbox++
		}
		if option.Source != "file" {
			t.Fatalf("option value must be file-backed: %+v", option)
		}
	}
	if readonly == 0 || secret == 0 || sandbox == 0 {
		t.Fatalf("expected read-only/secret/sandbox entries: ro=%d secret=%d sandbox=%d", readonly, secret, sandbox)
	}
}

// The options write path is independent from the legacy variables path: it is
// catalogue-validated, refuses foreign writers with 409 and reports unknown or
// invalid entries as 422, while still accepting a secret left blank.
func TestM2OfflineOptionsWritePath(t *testing.T) {
	dataRoot, serversRoot := legacyFixture(t)
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
	}
	serverDir := filepath.Join(serversRoot, "pz_01", "Zomboid", "Server")
	sandbox := filepath.Join(serverDir, "servertest_SandboxVars.lua")
	if err := os.WriteFile(sandbox, []byte("SandboxVars = {\n    -- keep me\n    Zombies = 4,\n    Basement = \"None\",\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, dataRoot)
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CatalogsDir = filepath.Join(repo, "catalogs")
	supervisor := &fakeSupervisor{}
	rt, err := buildRuntimeWith(cfg, runtimeOverrides{Processes: supervisor, ProcessStatus: supervisor, Logs: supervisor})
	if err != nil {
		t.Fatalf("buildRuntimeWith: %v", err)
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

	// A read-only (variables-owned) key is refused by the options path.
	if status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"options":{"MaxPlayers":"32"}}`); status != 409 {
		t.Fatalf("read-only option = %d %s, want 409", status, body)
	} else if !strings.Contains(body, "MaxPlayers") {
		t.Fatalf("read-only detail must name the option: %s", body)
	}
	// Unknown and invalid entries are 422.
	if status, _ := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"options":{"NotAnOption":"1"}}`); status != 422 {
		t.Fatalf("unknown option = %d, want 422", status)
	}
	if status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"options":{"Zombies":"many"}}`); status != 422 {
		t.Fatalf("invalid value = %d %s, want 422", status, body)
	}
	// A valid sandbox value is written to the vendor file and read back.
	if status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"options":{"Zombies":"7"}}`); status != 200 {
		t.Fatalf("valid option = %d %s", status, body)
	}
	raw, err := os.ReadFile(sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Zombies = 7") || !strings.Contains(string(raw), "-- keep me") {
		t.Fatalf("sandbox write wrong:\n%s", raw)
	}
	// A blank secret means "keep the current value" and must not fail.
	if status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"options":{"RCONPassword":""}}`); status != 200 {
		t.Fatalf("blank secret = %d %s", status, body)
	}
	// The legacy variables path still works and keeps its own response shape.
	status, body := call(t, client, "POST", server.URL+"/api/server/config", server.URL, `{"variables":{"MAX_PLAYERS":24}}`)
	if status != 200 || !strings.Contains(body, "配置已保存并同步") {
		t.Fatalf("legacy variables update = %d %s", status, body)
	}
	// Status reports the restart hint field without breaking legacy fields.
	status, body = call(t, client, "GET", server.URL+"/api/status", server.URL, "")
	if status != 200 || !strings.Contains(body, `"pending_restart"`) {
		t.Fatalf("status must expose pending_restart: %d %s", status, body)
	}
}

// The install endpoint accepts an optional evidence-backed version selection:
// {} keeps working, a known branch is echoed, and an unknown branch is a 422
// validation error rather than a silent fallback.
func TestM2OfflineInstallVersionSelection(t *testing.T) {
	dataRoot, _ := legacyFixture(t)
	if runtime.GOOS != "windows" {
		if _, err := normalizePermissions(dataRoot, "pz_01"); err != nil {
			t.Fatalf("normalizePermissions: %v", err)
		}
	}
	cfg := testConfig(t, dataRoot)
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CatalogsDir = filepath.Join(repo, "catalogs")
	installer := &fakeInstaller{}
	rt, err := buildRuntimeWith(cfg, runtimeOverrides{Installer: installer})
	if err != nil {
		t.Fatalf("buildRuntimeWith: %v", err)
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

	// The template list advertises the evidence-backed branches.
	status, body := call(t, client, "GET", server.URL+"/api/templates", server.URL, "")
	if status != 200 || !strings.Contains(body, `"versions"`) || !strings.Contains(body, `"legacy41"`) {
		t.Fatalf("templates must advertise versions: %d %s", status, strings.TrimSpace(body)[:200])
	}

	// An unknown branch is rejected before any download starts.
	if status, body := call(t, client, "POST", server.URL+"/api/server/install", server.URL, `{"version":"nope"}`); status != 422 {
		t.Fatalf("unknown version = %d %s, want 422", status, body)
	}
	if len(installer.requests) != 0 {
		t.Fatal("a rejected version must not start an install")
	}
	// A non-string version is a strict-JSON violation.
	if status, _ := call(t, client, "POST", server.URL+"/api/server/install", server.URL, `{"version":5}`); status != 422 {
		t.Fatalf("non-string version = %d, want 422", status)
	}
	// A known branch is accepted and echoed back.
	status, body = call(t, client, "POST", server.URL+"/api/server/install", server.URL, `{"version":"legacy41"}`)
	if status != 202 && status != 200 {
		t.Fatalf("known version = %d %s", status, body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for installer.requestCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if installer.requestCount() != 1 {
		t.Fatalf("installer requests = %d", installer.requestCount())
	}
	if installer.request(0).Version != "legacy41" {
		t.Fatalf("installer version = %q, want legacy41", installer.request(0).Version)
	}
	status, body = call(t, client, "GET", server.URL+"/api/server/install", server.URL, "")
	if status != 200 || !strings.Contains(body, `"version"`) {
		t.Fatalf("install status must echo the version: %d %s", status, body)
	}
}
