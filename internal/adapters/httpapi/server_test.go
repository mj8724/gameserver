package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/mj8724/gameserver/internal/application"
	"github.com/mj8724/gameserver/internal/ports"
)

const testPassword = "control-plane-sentinel-password"

var testOrigin = "http://example.test"

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type fakeControl struct {
	mu           sync.Mutex
	status       application.StatusResponse
	install      application.InstallStatus
	config       application.ConfigSnapshot
	templates    []application.TemplateSummary
	logs         []string
	fail         map[string]error
	calls        map[string]int
	inputs       []string
	commands     []string
	update       application.ConfigUpdate
	added        application.AddModRequest
	downloaded   string
	removed      string
	months       int
	inputRunning bool
	subscription *fakeSubscription
	subscribeN   atomic.Int64
}

func newFakeControl() *fakeControl {
	return &fakeControl{
		status: application.StatusResponse{
			"instance_id": "pz_01", "status": "STOPPED", "running": false, "is_installed": true,
			"pid": nil, "cpu_percent": 0.0, "memory_mb": 0.0, "uptime_seconds": 0,
			"ports":     map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
			"variables": map[string]any{"SERVER_NAME": "servertest", "SERVER_PASSWORD": "private-sentinel", "ADMIN_PASSWORD": "game-admin-sentinel"},
		},
		install: application.InstallStatus{Status: "IDLE", Progress: 0, Message: "尚未运行"},
		config: application.ConfigSnapshot{
			Variables:    map[string]any{"SERVER_NAME": "servertest", "SERVER_PASSWORD": "private-sentinel", "ADMIN_PASSWORD": "game-admin-sentinel"},
			Ports:        map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
			AllowedPorts: []string{"SERVER_PORT", "DIRECT_PORT"},
			Fields: []application.ConfigField{
				{Key: "SERVER_NAME", Label: "服务器名称", Type: "string", Value: "servertest", Default: "servertest", UserEditable: true},
				{Key: "SERVER_PASSWORD", Label: "入服密码", Type: "password", Value: "secret", Default: "secret", UserEditable: true},
				{Key: "ADMIN_PASSWORD", Label: "管理密码", Type: "password", Value: "hidden", Default: "hidden", Required: true, UserEditable: true},
				{Key: "MAX_PLAYERS", Label: "玩家数", Type: "number", Value: 16, Validation: map[string]any{"min": 1, "max": 64}, UserEditable: true},
				{Key: "PVP_ENABLED", Label: "PvP", Type: "boolean", Value: true, UserEditable: true},
				{Key: "LOCKED", Label: "只读", Type: "string", Value: "no", UserEditable: false},
			},
			Template: application.ConfigTemplate{ID: "project_zomboid", Name: "Project Zomboid"},
		},
		logs: []string{"log-a", "log-b"}, fail: make(map[string]error), calls: make(map[string]int), inputRunning: true,
	}
}

func (f *fakeControl) call(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[key]++
	return f.fail[key]
}
func (f *fakeControl) Status(context.Context) (application.StatusResponse, error) {
	return f.status, f.call("status")
}
func (f *fakeControl) Templates(context.Context) ([]application.TemplateSummary, error) {
	return f.templates, f.call("templates")
}
func (f *fakeControl) BeginInstall(context.Context, string) (application.InstallAccepted, error) {
	err := f.call("install")
	return application.InstallAccepted{Message: "安装/更新任务已启动", Status: "INSTALLING"}, err
}
func (f *fakeControl) InstallState(context.Context) (application.InstallStatus, error) {
	return f.install, f.call("install_status")
}
func (f *fakeControl) Start(context.Context) (application.StartResult, error) {
	err := f.call("start")
	return application.StartResult{Message: "启动指令已执行", Running: true}, err
}
func (f *fakeControl) Stop(context.Context) (application.OperationResult, error) {
	err := f.call("stop")
	return application.OperationResult{Message: "服务器已停止", Success: true}, err
}
func (f *fakeControl) Restart(context.Context) (application.OperationResult, error) {
	err := f.call("restart")
	return application.OperationResult{Message: "服务器已重启", Success: true}, err
}
func (f *fakeControl) Kill(context.Context) (application.OperationResult, error) {
	err := f.call("kill")
	return application.OperationResult{Message: "强制终止指令已执行", Success: false}, err
}
func (f *fakeControl) SendCommand(_ context.Context, text string) (application.CommandResult, error) {
	f.mu.Lock()
	f.commands = append(f.commands, text)
	f.mu.Unlock()
	return application.CommandResult{Success: true}, f.call("command")
}
func (f *fakeControl) SendConsoleInput(_ context.Context, text string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, text)
	return f.inputRunning, f.fail["input"]
}
func (f *fakeControl) Logs(_ context.Context, limit int) ([]string, error) {
	if err := f.call("logs"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	logs := append([]string(nil), f.logs...)
	if limit < len(logs) {
		logs = logs[len(logs)-limit:]
	}
	return logs, nil
}
func (f *fakeControl) Config(context.Context) (application.ConfigSnapshot, error) {
	return f.config, f.call("config")
}
func (f *fakeControl) UpdateConfig(_ context.Context, u application.ConfigUpdate) (application.ConfigUpdateResult, error) {
	f.update = u
	return application.ConfigUpdateResult{Message: "配置已保存并同步", State: map[string]any{"variables": map[string]any{"ADMIN_PASSWORD": "game-admin-sentinel", "SERVER_PASSWORD": "private-sentinel"}}}, f.call("update")
}
func (f *fakeControl) DownloadMod(_ context.Context, id string, name *string) (application.ModsResult, error) {
	f.downloaded = id
	return application.ModsResult{Message: "模组已下载并登记", Mods: map[string]any{"workshop_ids": []string{id}, "mod_names": []string{}}}, f.call("download_mod")
}

func (f *fakeControl) AddMod(_ context.Context, req application.AddModRequest) (application.ModsResult, error) {
	f.added = req
	return application.ModsResult{Message: "模组已登记（尚未下载）", Mods: map[string]any{"workshop_ids": []string{req.WorkshopID}, "mod_names": []string{}}}, f.call("add_mod")
}
func (f *fakeControl) RemoveMod(_ context.Context, id string) (application.ModsResult, error) {
	f.removed = id
	return application.ModsResult{Message: "模组已移除", Mods: map[string]any{"workshop_ids": []string{}, "mod_names": []string{}}}, f.call("remove_mod")
}
func (f *fakeControl) Renew(_ context.Context, months int) (application.RenewalResult, error) {
	f.months = months
	return application.RenewalResult{Message: "模拟续费成功", Billing: map[string]any{"status": "ACTIVE", "expire_days_left": 60}}, f.call("renew")
}
func (f *fakeControl) SubscribeConsole(_ context.Context, limit int) (application.ConsoleSubscription, error) {
	f.subscribeN.Add(1)
	if err := f.call("subscribe"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscription == nil {
		f.subscription = newFakeSubscription(nil)
	}
	f.subscription.limit = limit
	return f.subscription, nil
}

var _ application.Control = (*fakeControl)(nil)

type fakeSubscription struct {
	replay []string
	events chan string
	closed chan struct{}
	once   sync.Once
	limit  int
}

func newFakeSubscription(lines []string) *fakeSubscription {
	return &fakeSubscription{replay: append([]string(nil), lines...), events: make(chan string, 128), closed: make(chan struct{})}
}
func (s *fakeSubscription) Replay() []string {
	out := append([]string(nil), s.replay...)
	if s.limit > 0 && len(out) > s.limit {
		out = out[len(out)-s.limit:]
	}
	return out
}
func (s *fakeSubscription) Events() <-chan string { return s.events }
func (s *fakeSubscription) Close()                { s.once.Do(func() { close(s.closed) }) }
func (s *fakeSubscription) Publish(line string)   { s.events <- line }

func harness(t *testing.T, password string, secure bool, assets ports.StaticAssets) (*Server, *fakeControl, *application.Authenticator) {
	t.Helper()
	clock := &testClock{now: time.Unix(1_800_000_000, 0)}
	auth, err := application.NewAuthenticatorWithClock(application.AuthConfig{AdminPassword: password}, clock)
	if err != nil {
		t.Fatal("create authenticator")
	}
	control := newFakeControl()
	server, err := NewServer(control, auth, assets, Config{SecureCookies: secure})
	if err != nil {
		t.Fatal("create server")
	}
	t.Cleanup(server.Close)
	return server, control, auth
}

func token(t *testing.T, auth *application.Authenticator) string {
	t.Helper()
	value, err := auth.Login(testPassword)
	if err != nil {
		t.Fatal("issue session")
	}
	return string(value)
}
func httpRequest(t *testing.T, h http.Handler, method, path, body, cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "example.test"
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "gameserver_session", Value: cookie})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
func jsonStatus(t *testing.T, r *httptest.ResponseRecorder, status int, out any) {
	t.Helper()
	if r.Code != status {
		t.Fatalf("status=%d want=%d body=%s", r.Code, status, r.Body.String())
	}
	if !strings.HasPrefix(r.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type=%q", r.Header().Get("Content-Type"))
	}
	if err := json.Unmarshal(r.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %q: %v", r.Body.String(), err)
	}
}
func detail(t *testing.T, r *httptest.ResponseRecorder, status int, want string) {
	t.Helper()
	var got map[string]any
	jsonStatus(t, r, status, &got)
	if len(got) != 1 || got["detail"] != want {
		t.Fatalf("detail body=%#v want=%q", got, want)
	}
}

func TestRouteTableCompatibilityAndStrictMethods(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<title>ui</title>")}, "app.js": {Data: []byte("script")}}
	s, control, auth := harness(t, testPassword, false, assets)
	control.templates = []application.TemplateSummary{{ID: "project_zomboid", SupportedOS: []string{"windows", "linux", "darwin"}, AppID: "380870"}}
	cookie := token(t, auth)
	cases := []struct {
		name, method, path, body string
		status                   int
	}{
		{"auth status", "GET", "/api/auth/status", "", 200}, {"auth login", "POST", "/api/auth/login", `{"password":"` + testPassword + `"}`, 200}, {"auth logout", "POST", "/api/auth/logout", `{}`, 200},
		{"status", "GET", "/api/status", "", 200}, {"templates", "GET", "/api/templates", "", 200}, {"install start", "POST", "/api/server/install", `{}`, 202}, {"install status", "GET", "/api/server/install", "", 200},
		{"start", "POST", "/api/server/start", `{}`, 200}, {"stop", "POST", "/api/server/stop", `{}`, 200}, {"restart", "POST", "/api/server/restart", `{}`, 200}, {"kill", "POST", "/api/server/kill", `{}`, 200},
		{"command", "POST", "/api/server/command", `{"command":"status"}`, 200}, {"logs", "GET", "/api/server/logs?limit=200", "", 200}, {"config get", "GET", "/api/server/config", "", 200}, {"config update", "POST", "/api/server/config", `{}`, 200},
		{"mods add", "POST", "/api/server/mods", `{"workshop_id":"42"}`, 200}, {"mods delete", "DELETE", "/api/server/mods/42", "", 200}, {"renew", "POST", "/api/server/renew", `{}`, 200}, {"root", "GET", "/", "", 200}, {"static", "GET", "/static/app.js", "", 200},
		{"wrong method", "POST", "/api/status", `{}`, 405}, {"unknown path", "GET", "/api/not-real", "", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origin := ""
			if tc.method != "GET" {
				origin = testOrigin
			}
			c := cookie
			if strings.HasPrefix(tc.path, "/api/auth/") {
				c = ""
			}
			r := httpRequest(t, s, tc.method, tc.path, tc.body, c, origin)
			if r.Code != tc.status {
				t.Fatalf("%s %s = %d want %d: %s", tc.method, tc.path, r.Code, tc.status, r.Body.String())
			}
			if strings.HasPrefix(tc.path, "/api/") && !strings.HasPrefix(r.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("API response not JSON: %q", r.Header().Get("Content-Type"))
			}
			if tc.path == "/api/templates" {
				var got []application.TemplateSummary
				if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil || len(got) != 1 || strings.Join(got[0].SupportedOS, ",") != "windows,linux,darwin" {
					t.Fatalf("template compatibility metadata: %#v %v", got, err)
				}
			}
		})
	}
	if control.calls["install"] == 0 || control.calls["start"] == 0 {
		t.Fatal("empty {} UI action posts were not delegated")
	}
}

func TestAuthenticatorHTTPLoginLogoutCookieAndReplay(t *testing.T) {
	s, _, auth := harness(t, testPassword, true, nil)
	login := httpRequest(t, s, "POST", "/api/auth/login", `{"password":"`+testPassword+`"}`, "", testOrigin)
	var body map[string]bool
	jsonStatus(t, login, 200, &body)
	if !body["authenticated"] {
		t.Fatal("login false")
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "gameserver_session" || c.Path != "/" || c.MaxAge != 43200 || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie attrs mismatch: %#v", c)
	}
	logout := httpRequest(t, s, "POST", "/api/auth/logout", `{}`, c.Value, testOrigin)
	jsonStatus(t, logout, 200, &body)
	if body["authenticated"] {
		t.Fatal("logout response true")
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Path != "/" {
		t.Fatalf("cookie not cleared: %#v", cleared)
	}
	detail(t, httpRequest(t, s, "GET", "/api/status", "", c.Value, ""), 401, "请先登录")
	if auth.ActiveSessions() != 0 {
		t.Fatal("logout did not revoke session")
	}
}

func TestAuthOriginAndRequestValidation(t *testing.T) {
	s, control, auth := harness(t, testPassword, false, nil)
	c := token(t, auth)
	detail(t, httpRequest(t, s, "POST", "/api/auth/login", `{"password":"x"}`, "", ""), 403, "跨站请求已拒绝")
	detail(t, httpRequest(t, s, "POST", "/api/auth/login", `{"password":"x"}`, "", "http://evil.test"), 403, "跨站请求已拒绝")
	detail(t, httpRequest(t, s, "POST", "/api/auth/login", `{"password":"wrong"}`, "", testOrigin), 401, "管理员密码错误")
	detail(t, httpRequest(t, s, "POST", "/api/auth/login", `{"password":"x","extra":1}`, "", testOrigin), 422, "请求数据无效")
	detail(t, httpRequest(t, s, "GET", "/api/status", "", "", ""), 401, "请先登录")
	detail(t, httpRequest(t, s, "GET", "/api/status", "", c, "http://evil.test"), 403, "跨站请求已拒绝")
	if got := httpRequest(t, s, "GET", "/api/status", "", c, ""); got.Code != 200 {
		t.Fatalf("same-origin read without Origin=%d", got.Code)
	}
	detail(t, httpRequest(t, s, "POST", "/api/server/start", `{}`, c, ""), 403, "跨站请求已拒绝")
	detail(t, httpRequest(t, s, "POST", "/api/server/start", `{}`, c, "http://evil.test"), 403, "跨站请求已拒绝")
	if control.calls["start"] != 0 {
		t.Fatal("origin-rejected request reached use case")
	}
	unconfigured, _, _ := harness(t, "", false, nil)
	detail(t, httpRequest(t, unconfigured, "GET", "/api/status", "", "", ""), 503, "尚未配置 GAMESERVER_ADMIN_PASSWORD")
	detail(t, httpRequest(t, unconfigured, "POST", "/api/auth/login", `{"password":"x"}`, "", testOrigin), 503, "尚未配置 GAMESERVER_ADMIN_PASSWORD")
	for _, tc := range []struct{ path, body string }{{"/api/server/command", `{"command":"ok","extra":true}`}, {"/api/server/config", `{"unknown":true}`}, {"/api/server/start", `{"unknown":true}`}, {"/api/server/command", `{}`}, {"/api/server/command", `{"command":"` + strings.Repeat("x", 4097) + `"}`}} {
		detail(t, httpRequest(t, s, "POST", tc.path, tc.body, c, testOrigin), 422, "请求数据无效")
	}
	if got := httpRequest(t, s, "GET", "/api/status", "", c, ""); got.Code != 200 {
		t.Fatal("GET Content-Type application/json rejected")
	}
}

func TestApplicationErrorsAndConfigurationRedaction(t *testing.T) {
	cases := []struct {
		code   application.ErrorCode
		status int
		detail string
	}{
		{application.CodeInstallAlreadyRunning, 409, "已有安装任务在运行中"}, {application.CodeInstallWhileRunning, 409, "服务器运行中，请先关机后再更新"}, {application.CodeServerInstalling, 409, "正在安装中，请稍候启动"},
		{application.CodeServerNotInstalled, 400, "游戏服务端未安装，请先安装服务端"}, {application.CodeStartFailed, 500, "服务器启动失败，请检查控制台输出"}, {application.CodeStopFailed, 500, "服务器停止失败"},
		{application.CodeRestartStopFailed, 500, "停止服务器失败，已取消重启"}, {application.CodeRestartNotInstalled, 400, "游戏服务端未安装"}, {application.CodeRestartFailed, 500, "服务器重启失败，请检查控制台输出"},
		{application.CodeServerNotRunning, 400, "服务器未运行，无法发送控制台指令"}, {application.CodeWorkshopIDNotNumeric, 422, "Workshop ID 必须为数字"},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			control := newFakeControl()
			control.fail["start"] = application.NewError(tc.code, "")
			s, auth := harnessWithControl(t, control, testPassword, nil)
			detail(t, httpRequest(t, s, "POST", "/api/server/start", `{}`, token(t, auth), testOrigin), tc.status, tc.detail)
		})
	}
	s, control, auth := harness(t, testPassword, false, nil)
	c := token(t, auth)
	var status map[string]any
	jsonStatus(t, httpRequest(t, s, "GET", "/api/status", "", c, ""), 200, &status)
	vars := status["variables"].(map[string]any)
	if vars["SERVER_PASSWORD"] != "" || vars["ADMIN_PASSWORD"] != nil {
		t.Fatalf("status secret leak: %#v", vars)
	}
	task := status["install_task"].(map[string]any)
	if _, ok := task["progress"].(float64); !ok {
		t.Fatalf("progress type %T", task["progress"])
	}
	var cfg map[string]any
	resp := httpRequest(t, s, "GET", "/api/server/config", "", c, "")
	jsonStatus(t, resp, 200, &cfg)
	fields := cfg["fields"].([]any)
	if len(fields) != 5 {
		t.Fatalf("user editable fields=%d", len(fields))
	}
	pass := fields[1].(map[string]any)
	if pass["value"] != nil {
		t.Fatalf("password value=%#v", pass["value"])
	}
	if _, ok := pass["default"]; ok {
		t.Fatal("password default exposed")
	}
	if strings.Contains(resp.Body.String(), "sentinel") {
		t.Fatal("secret in config response")
	}
	update := httpRequest(t, s, "POST", "/api/server/config", `{"variables":{"MAX_PLAYERS":16},"ports":{"SERVER_PORT":17000}}`, c, testOrigin)
	if update.Code != 200 {
		t.Fatalf("valid config update=%d %s", update.Code, update.Body.String())
	}
	if control.update.Ports["SERVER_PORT"] != 17000 || control.update.Variables["MAX_PLAYERS"].(json.Number) != "16" {
		t.Fatalf("update parse=%#v", control.update)
	}
	bad := httpRequest(t, s, "POST", "/api/server/config", `{"variables":{"MAX_PLAYERS":true}}`, c, testOrigin)
	detail(t, bad, 422, "MAX_PLAYERS 必须是整数")
	workshop := httpRequest(t, s, "POST", "/api/server/mods", `{"workshop_id":"abc"}`, c, testOrigin)
	detail(t, workshop, 422, "Workshop ID 必须为数字")
	if r := httpRequest(t, s, "POST", "/api/server/renew", `{}`, c, testOrigin); r.Code != 200 || control.months != 1 {
		t.Fatalf("renewal default status=%d months=%d", r.Code, control.months)
	}
	detail(t, httpRequest(t, s, "POST", "/api/server/renew", `{"months":25}`, c, testOrigin), 422, "请求数据无效")
}

func harnessWithControl(t *testing.T, control *fakeControl, password string, assets ports.StaticAssets) (*Server, *application.Authenticator) {
	t.Helper()
	auth, err := application.NewAuthenticatorWithClock(application.AuthConfig{AdminPassword: password}, &testClock{now: time.Unix(1_800_000_000, 0)})
	if err != nil {
		t.Fatal("auth")
	}
	s, err := NewServer(control, auth, assets, Config{})
	if err != nil {
		t.Fatal("server")
	}
	t.Cleanup(s.Close)
	return s, auth
}

func TestStaticAssetsFallbackAndSafePaths(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<title>root</title>")}, "app.js": {Data: []byte("console.log('static')")}}
	s, _, _ := harness(t, testPassword, false, assets)
	root := httptest.NewRecorder()
	s.ServeHTTP(root, httptest.NewRequest("GET", "/", nil))
	if root.Code != 200 || !strings.Contains(root.Body.String(), "<title>root") {
		t.Fatalf("root: %d %q", root.Code, root.Body.String())
	}
	static := httptest.NewRecorder()
	s.ServeHTTP(static, httptest.NewRequest("GET", "/static/app.js", nil))
	if static.Code != 200 || !strings.Contains(static.Body.String(), "console.log") {
		t.Fatalf("static: %d %q", static.Code, static.Body.String())
	}
	missing, _, _ := harness(t, testPassword, false, fstest.MapFS{})
	fallback := httptest.NewRecorder()
	missing.ServeHTTP(fallback, httptest.NewRequest("GET", "/", nil))
	if fallback.Code != 200 || fallback.Body.String() != `{"message":"Gameserver Backend Running"}` {
		t.Fatalf("fallback=%d %q", fallback.Code, fallback.Body.String())
	}
	for _, p := range []string{"/static/../secret", "/static/%2e%2e/secret", "/static/%2e%2e%2fsecret", "/static/%5c..%5csecret"} {
		r := httptest.NewRecorder()
		missing.ServeHTTP(r, httptest.NewRequest("GET", p, nil))
		if r.Code != 404 {
			t.Errorf("traversal %q status=%d", p, r.Code)
		}
	}
}

func wsHarness(t *testing.T) (*Server, *fakeControl, *application.Authenticator, *httptest.Server) {
	t.Helper()
	lines := make([]string, 130)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d", i)
	}
	control := newFakeControl()
	control.subscription = newFakeSubscription(lines)
	auth, err := application.NewAuthenticatorWithClock(application.AuthConfig{AdminPassword: testPassword}, &testClock{now: time.Unix(1_800_000_000, 0)})
	if err != nil {
		t.Fatal("auth")
	}
	s, err := NewServer(control, auth, nil, Config{})
	if err != nil {
		t.Fatal("server")
	}
	httpServer := httptest.NewServer(s)
	t.Cleanup(func() { s.Close(); httpServer.Close() })
	return s, control, auth, httpServer
}
func dialWS(t *testing.T, s *httptest.Server, cookie, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u, _ := url.Parse(s.URL)
	u.Scheme = "ws"
	u.Path = "/ws/console"
	headers := make(http.Header)
	if cookie != "" {
		headers.Set("Cookie", (&http.Cookie{Name: "gameserver_session", Value: cookie}).String())
	}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	return websocket.Dial(context.Background(), u.String(), &websocket.DialOptions{HTTPHeader: headers})
}
func readWS(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	kind, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	if kind != websocket.MessageText {
		t.Fatalf("ws kind=%v", kind)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode ws %q: %v", b, err)
	}
	return v
}
func wait(t *testing.T, desc string, fn func() bool) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting %s", desc)
}

func TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup(t *testing.T) {
	_, control, auth, server := wsHarness(t)
	bad, response, err := dialWS(t, server, "", server.URL)
	if err != nil {
		t.Fatalf("handshake should accept then close 1008: %v response=%v", err, response)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = bad.Read(ctx)
	cancel()
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("unauth close=%d err=%v", websocket.CloseStatus(err), err)
	}
	bad.CloseNow()
	if control.subscribeN.Load() != 0 {
		t.Fatal("unauthenticated socket subscribed")
	}
	if _, response, err = dialWS(t, server, token(t, auth), ""); err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("missing origin accepted or wrong status: resp=%v err=%v", response, err)
	}
	if _, response, err = dialWS(t, server, "", "http://evil.test"); err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("cross-origin accepted or wrong status: resp=%v err=%v", response, err)
	}
	conn, response, err := dialWS(t, server, token(t, auth), server.URL)
	if err != nil || response == nil || response.StatusCode != 101 {
		t.Fatalf("auth websocket dial: %v %v", response, err)
	}
	for i := 0; i < 100; i++ {
		frame := readWS(t, conn)
		if frame["type"] != "log" || frame["data"] != fmt.Sprintf("line-%03d", i+30) {
			t.Fatalf("replay[%d]=%#v", i, frame)
		}
	}
	control.subscription.Publish("live-line")
	if frame := readWS(t, conn); frame["type"] != "log" || frame["data"] != "live-line" {
		t.Fatalf("live log=%#v", frame)
	}
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"ping"}`))
	if frame := readWS(t, conn); frame["type"] != "pong" {
		t.Fatalf("ping frame=%#v", frame)
	}
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"input","data":"say hi"}`))
	wait(t, "input route", func() bool { control.mu.Lock(); defer control.mu.Unlock(); return len(control.inputs) == 1 })
	control.mu.Lock()
	control.inputRunning = false
	control.mu.Unlock()
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"input","data":"status"}`))
	if frame := readWS(t, conn); frame["type"] != "error" || frame["data"] != "server_not_running" {
		t.Fatalf("stopped input response=%#v", frame)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "done")
	select {
	case <-control.subscription.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription listener not removed after disconnect")
	}
	control.mu.Lock()
	n := len(control.inputs)
	control.mu.Unlock()
	if n != 2 {
		t.Fatalf("inputs=%d; arbitrary/non-input text was processed", n)
	}
}

func TestConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands(t *testing.T) {
	for name, payload := range map[string]string{"malformed": "{", "unknown": `{"type":"exec","data":"danger"}`, "plain-text": "arbitrary raw command", "extra": `{"type":"input","data":"ok","extra":true}`} {
		t.Run(name, func(t *testing.T) {
			_, control, auth, server := wsHarness(t)
			conn, _, err := dialWS(t, server, token(t, auth), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 100; i++ {
				_ = readWS(t, conn)
			}
			_ = conn.Write(context.Background(), websocket.MessageText, []byte(payload))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _, err = conn.Read(ctx)
			cancel()
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("invalid message close=%d: %v", websocket.CloseStatus(err), err)
			}
			control.mu.Lock()
			n := len(control.inputs)
			control.mu.Unlock()
			if n != 0 {
				t.Fatalf("invalid message converted into %d console inputs", n)
			}
			select {
			case <-control.subscription.closed:
			case <-time.After(3 * time.Second):
				t.Fatal("listener was not removed")
			}
		})
	}
}

func TestConsoleServerCancellationClosesConnectionAndSubscription(t *testing.T) {
	s, control, auth, server := wsHarness(t)
	conn, _, err := dialWS(t, server, token(t, auth), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		_ = readWS(t, conn)
	}
	s.Close()
	select {
	case <-control.subscription.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription not closed on cancel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = conn.Read(ctx)
	cancel()
	if err == nil {
		t.Fatal("connection remained open after cancellation")
	}
	conn.CloseNow()
}

func TestUnknownInternalErrorsDoNotLeakAndReadOnlyOriginsCompareHostPort(t *testing.T) {
	r := httptest.NewRecorder()
	writeApplicationError(r, errors.New("game-admin-sentinel"))
	detail(t, r, 500, "服务器内部错误")
	if strings.Contains(r.Body.String(), "sentinel") {
		t.Fatal("internal error leak")
	}
	req := httptest.NewRequest("GET", "http://example.test:8769/ws/console", nil)
	req.Host = "example.test:8769"
	req.Header.Set("Origin", "http://example.test:8769")
	if !checkOrigin(req, true) {
		t.Fatal("matching origin host and port rejected")
	}
	for _, origin := range []string{"", "http://example.test", "http://evil.test:8769", "null", "http://example.test:8769/path"} {
		req.Header.Set("Origin", origin)
		if checkOrigin(req, true) {
			t.Errorf("invalid origin accepted: %q", origin)
		}
	}
}
