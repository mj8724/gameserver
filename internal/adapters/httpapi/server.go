// Package httpapi implements the inbound HTTP and WebSocket transports. It
// delegates application behavior to application.Control and serves static
// files only through the injected StaticAssets port.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/mj8724/gameserver/internal/application"
	"github.com/mj8724/gameserver/internal/ports"
)

const (
	sessionCookieName = "gameserver_session"
	maxRequestBody    = 1 << 20
	maxWebSocketFrame = 16 << 10
	writeTimeout      = 5 * time.Second
)

// Config contains transport-only security configuration.
type Config struct {
	SecureCookies bool
}

// Server is a configured HTTP handler. Close cancels active WebSocket
// sessions and their application subscriptions; call it during shutdown.
type Server struct {
	control application.Control
	auth    *application.Authenticator
	assets  ports.StaticAssets
	config  Config
	mux     *http.ServeMux

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	conns  map[*websocket.Conn]struct{}
}

// NewServer constructs a ServeMux-backed inbound adapter. It deliberately
// does not construct application services or outbound adapters.
func NewServer(control application.Control, auth *application.Authenticator, assets ports.StaticAssets, config Config) (*Server, error) {
	if control == nil {
		return nil, errors.New("application control is required")
	}
	if auth == nil {
		return nil, errors.New("authenticator is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		control: control,
		auth:    auth,
		assets:  assets,
		config:  config,
		mux:     http.NewServeMux(),
		ctx:     ctx,
		cancel:  cancel,
		conns:   make(map[*websocket.Conn]struct{}),
	}
	s.registerRoutes()
	return s, nil
}

// Close initiates a bounded, leak-free WebSocket shutdown. It is safe to call
// more than once.
func (s *Server) Close() {
	s.cancel()
	s.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.CloseNow()
	}
}

// ServeHTTP rejects static traversal before ServeMux's canonical-path redirect
// behavior, then dispatches the registered route table.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isStaticRequestPath(r.URL.Path) && containsTraversal(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if escaped := r.URL.EscapedPath(); escaped != r.URL.Path {
		if decoded, err := url.PathUnescape(escaped); err != nil || (isStaticRequestPath(decoded) && containsTraversal(decoded)) {
			http.NotFound(w, r)
			return
		}
	}
	if !knownPath(r.URL.Path) {
		if isAPIPath(r.URL.Path) {
			writeDetail(w, http.StatusNotFound, "Not Found")
			return
		}
		http.NotFound(w, r)
		return
	}
	allowed := allowedMethods(r.URL.Path)
	method := r.Method
	if method == http.MethodHead && contains(allowed, http.MethodGet) {
		method = http.MethodGet
	}
	if !contains(allowed, method) {
		if isAPIPath(r.URL.Path) {
			writeDetail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/auth/status", s.authStatus)
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)

	s.mux.HandleFunc("GET /api/status", s.status)
	s.mux.HandleFunc("GET /api/templates", s.templates)
	s.mux.HandleFunc("POST /api/server/install", s.install)
	s.mux.HandleFunc("GET /api/server/install", s.installStatus)
	s.mux.HandleFunc("POST /api/server/start", s.start)
	s.mux.HandleFunc("POST /api/server/stop", s.stop)
	s.mux.HandleFunc("POST /api/server/restart", s.restart)
	s.mux.HandleFunc("POST /api/server/kill", s.kill)
	s.mux.HandleFunc("POST /api/server/command", s.command)
	s.mux.HandleFunc("GET /api/server/logs", s.logs)
	s.mux.HandleFunc("GET /api/server/config", s.getConfig)
	s.mux.HandleFunc("POST /api/server/config", s.updateConfig)
	s.mux.HandleFunc("POST /api/server/mods", s.addMod)
	s.mux.HandleFunc("DELETE /api/server/mods/{workshop_id}", s.removeMod)
	s.mux.HandleFunc("POST /api/server/renew", s.renew)

	s.mux.HandleFunc("GET /ws/console", s.console)
	s.mux.HandleFunc("GET /static/", s.staticFile)
	s.mux.HandleFunc("GET /{$}", s.root)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if !checkOrigin(r, false) {
		writeDetail(w, http.StatusForbidden, "跨站请求已拒绝")
		return
	}
	authenticated := false
	if s.auth.Configured() {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && s.auth.Authenticate(cookie.Value) == nil {
			authenticated = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": authenticated})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !checkOrigin(r, true) {
		writeDetail(w, http.StatusForbidden, "跨站请求已拒绝")
		return
	}
	var req loginRequest
	if err := decodeStrictJSON(r, &req, false); err != nil || req.Password == nil {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return
	}
	token, err := s.auth.Login(*req.Password)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    string(token),
		Path:     "/",
		MaxAge:   int(application.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !checkOrigin(r, true) {
		writeDetail(w, http.StatusForbidden, "跨站请求已拒绝")
		return
	}
	if !acceptEmptyJSON(w, r) {
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.auth.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, false) {
		return
	}
	result, err := s.control.Status(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	state := cloneObject(result)
	state["variables"] = redactVariables(state["variables"])
	install, err := s.control.InstallState(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	state["install_task"] = install
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) templates(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, false) {
		return
	}
	result, err := s.control.Templates(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if result == nil {
		result = []application.TemplateSummary{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	if !acceptEmptyJSON(w, r) {
		return
	}
	result, err := s.control.BeginInstall(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) installStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, false) {
		return
	}
	result, err := s.control.InstallState(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) || !acceptEmptyJSON(w, r) {
		return
	}
	result, err := s.control.Start(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) || !acceptEmptyJSON(w, r) {
		return
	}
	result, err := s.control.Stop(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) || !acceptEmptyJSON(w, r) {
		return
	}
	result, err := s.control.Restart(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) kill(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) || !acceptEmptyJSON(w, r) {
		return
	}
	result, err := s.control.Kill(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	var req commandRequest
	if err := decodeStrictJSON(r, &req, false); err != nil || req.Command == nil || runeLength(*req.Command) < 1 || runeLength(*req.Command) > 4096 {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return
	}
	result, err := s.control.SendCommand(r.Context(), *req.Command)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, false) {
		return
	}
	limit := 150
	if value, ok := r.URL.Query()["limit"]; ok && len(value) > 0 {
		parsed, err := strconv.Atoi(value[0])
		if err != nil {
			writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
			return
		}
		limit = parsed
	}
	limit = max(1, min(limit, 1000))
	result, err := s.control.Logs(r.Context(), limit)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if result == nil {
		result = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"logs": result})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, false) {
		return
	}
	result, err := s.control.Config(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	result.Variables = redactVariables(result.Variables)
	fields := make([]application.ConfigField, 0, len(result.Fields))
	for _, field := range result.Fields {
		if !field.UserEditable {
			continue
		}
		if field.Type == "password" {
			field.Value = nil
			field.Default = nil
		}
		fields = append(fields, field)
	}
	result.Fields = fields
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	var req configRequest
	if err := decodeStrictJSON(r, &req, false); err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return
	}
	update := application.ConfigUpdate{Variables: map[string]any{}}
	if len(req.Variables) != 0 && string(req.Variables) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(req.Variables))
		decoder.UseNumber()
		if err := decoder.Decode(&update.Variables); err != nil || update.Variables == nil {
			writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
			return
		}
	}
	if req.Ports != nil && string(req.Ports) != "null" {
		values, ok := strictPorts(req.Ports)
		if !ok {
			writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
			return
		}
		update.Ports = values
	}
	if len(req.Options) != 0 && string(req.Options) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(req.Options))
		options := map[string]string{}
		if err := decoder.Decode(&options); err != nil {
			writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
			return
		}
		update.Options = options
	}
	snapshot, err := s.control.Config(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if err := application.ValidateConfigUpdate(snapshot, update); err != nil {
		writeApplicationError(w, err)
		return
	}
	result, err := s.control.UpdateConfig(r.Context(), update)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	result.State = cloneObject(result.State)
	result.State["variables"] = redactVariables(result.State["variables"])
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) addMod(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	var req addModRequest
	if err := decodeStrictJSON(r, &req, false); err != nil || req.WorkshopID == nil || runeLength(*req.WorkshopID) < 1 || runeLength(*req.WorkshopID) > 32 || (req.ModName != nil && runeLength(*req.ModName) > 128) {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return
	}
	if !asciiDigits(*req.WorkshopID) {
		writeDetail(w, http.StatusUnprocessableEntity, "Workshop ID 必须为数字")
		return
	}
	result, err := s.control.AddMod(r.Context(), application.AddModRequest{WorkshopID: *req.WorkshopID, ModName: req.ModName})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) removeMod(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	result, err := s.control.RemoveMod(r.Context(), r.PathValue("workshop_id"))
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) renew(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r, true) {
		return
	}
	var req renewRequest
	if err := decodeStrictJSON(r, &req, false); err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return
	}
	months := 1
	if len(req.Months) != 0 {
		if string(req.Months) == "null" || json.Unmarshal(req.Months, &months) != nil || months < 1 || months > 24 {
			writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
			return
		}
	}
	result, err := s.control.Renew(r.Context(), months)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if !s.serveAsset(w, r, "index.html", true) {
		return
	}
}

func (s *Server) staticFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name == r.URL.Path || !validAssetName(name) {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, name, false)
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, name string, root bool) bool {
	if s.assets == nil {
		if root {
			writeFallback(w)
			return true
		}
		http.NotFound(w, r)
		return false
	}
	file, err := s.assets.Open(name)
	if err != nil {
		if root {
			writeFallback(w)
			return true
		}
		http.NotFound(w, r)
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		if root {
			writeFallback(w)
			return true
		}
		http.NotFound(w, r)
		return false
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		if root {
			writeFallback(w)
			return true
		}
		http.NotFound(w, r)
		return false
	}
	if contentType := mime.TypeByExtension(extension(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(contents))
	return true
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request, mutating bool) bool {
	if !checkOrigin(r, mutating) {
		writeDetail(w, http.StatusForbidden, "跨站请求已拒绝")
		return false
	}
	if !s.auth.Configured() {
		writeDetail(w, http.StatusServiceUnavailable, "尚未配置 GAMESERVER_ADMIN_PASSWORD")
		return false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || s.auth.Authenticate(cookie.Value) != nil {
		writeDetail(w, http.StatusUnauthorized, "请先登录")
		return false
	}
	return true
}

func (s *Server) console(w http.ResponseWriter, r *http.Request) {
	if !checkOrigin(r, true) {
		writeDetail(w, http.StatusForbidden, "跨站请求已拒绝")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return
	}
	if !s.registerConnection(conn) {
		return
	}
	defer s.unregisterConnection(conn)
	defer conn.CloseNow()

	if !s.auth.Configured() || s.auth.Authenticate(cookieValue(r, sessionCookieName)) != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "policy violation")
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	conn.SetReadLimit(maxWebSocketFrame)
	subscription, err := s.control.SubscribeConsole(ctx, 100)
	if err != nil || subscription == nil {
		_ = conn.Close(websocket.StatusInternalError, "console unavailable")
		return
	}
	defer subscription.Close()

	replay := subscription.Replay()
	if len(replay) > 100 {
		replay = replay[len(replay)-100:]
	}
	for _, line := range replay {
		if err := writeSocketJSON(ctx, conn, map[string]any{"type": "log", "data": line}); err != nil {
			return
		}
	}

	incoming := make(chan websocketMessage, 1)
	go readSocket(ctx, conn, incoming)
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-subscription.Events():
			if !ok {
				return
			}
			if err := writeSocketJSON(ctx, conn, map[string]any{"type": "log", "data": line}); err != nil {
				return
			}
		case message := <-incoming:
			if message.err != nil {
				return
			}
			if message.kind != websocket.MessageText {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid message")
				return
			}
			kind, data, err := parseConsoleMessage(message.data)
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid message")
				return
			}
			switch kind {
			case "ping":
				if err := writeSocketJSON(ctx, conn, map[string]string{"type": "pong"}); err != nil {
					return
				}
			case "input":
				if data == "" {
					continue
				}
				running, err := s.control.SendConsoleInput(ctx, data)
				if err != nil {
					_ = conn.Close(websocket.StatusInternalError, "console input failed")
					return
				}
				if !running {
					if err := writeSocketJSON(ctx, conn, map[string]string{"type": "error", "data": "server_not_running"}); err != nil {
						return
					}
				}
			}
		}
	}
}

type loginRequest struct {
	Password *string `json:"password"`
}

type commandRequest struct {
	Command *string `json:"command"`
}

type configRequest struct {
	Variables json.RawMessage `json:"variables"`
	Ports     json.RawMessage `json:"ports"`
	Options   json.RawMessage `json:"options"`
}

type addModRequest struct {
	WorkshopID *string `json:"workshop_id"`
	ModName    *string `json:"mod_name"`
}

type renewRequest struct {
	Months json.RawMessage `json:"months"`
}

type websocketMessage struct {
	kind websocket.MessageType
	data []byte
	err  error
}

func decodeStrictJSON(r *http.Request, destination any, emptyMeansObject bool) error {
	if r.Body == nil || r.Body == http.NoBody {
		if emptyMeansObject {
			return nil
		}
		return errors.New("request body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxRequestBody {
		return errors.New("request body is too large")
	}
	if emptyMeansObject && len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func acceptEmptyJSON(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil || len(body) > maxRequestBody {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var object map[string]json.RawMessage
	if err := decodeStrictJSON(r, &object, false); err != nil || object == nil || len(object) != 0 {
		writeDetail(w, http.StatusUnprocessableEntity, "请求数据无效")
		return false
	}
	return true
}

func checkOrigin(r *http.Request, mutating bool) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return !mutating
	}
	if len(values) != 1 {
		return false
	}
	origin := strings.TrimSpace(values[0])
	if origin == "" || strings.ContainsAny(origin, "\r\n") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host := r.Host
	if host == "" && r.URL != nil {
		host = r.URL.Host
	}
	return host != "" && strings.EqualFold(parsed.Host, host)
}

func writeApplicationError(w http.ResponseWriter, err error) {
	var useCase *application.UseCaseError
	if !errors.As(err, &useCase) {
		writeDetail(w, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	status, detail := applicationErrorDetails(useCase)
	writeDetail(w, status, detail)
}

func applicationErrorDetails(err *application.UseCaseError) (int, string) {
	switch err.Code {
	case application.CodeAdminPasswordNotConfigured:
		return http.StatusServiceUnavailable, "尚未配置 GAMESERVER_ADMIN_PASSWORD"
	case application.CodeInvalidAdminPassword:
		return http.StatusUnauthorized, "管理员密码错误"
	case application.CodeUnauthenticated:
		return http.StatusUnauthorized, "请先登录"
	case application.CodeInstallAlreadyRunning:
		return http.StatusConflict, "已有安装任务在运行中"
	case application.CodeInstallWhileRunning:
		return http.StatusConflict, "服务器运行中，请先关机后再更新"
	case application.CodeServerInstalling:
		return http.StatusConflict, "正在安装中，请稍候启动"
	case application.CodeServerNotInstalled:
		return http.StatusBadRequest, "游戏服务端未安装，请先安装服务端"
	case application.CodeStartFailed:
		return http.StatusInternalServerError, "服务器启动失败，请检查控制台输出"
	case application.CodeStopFailed:
		return http.StatusInternalServerError, "服务器停止失败"
	case application.CodeRestartStopFailed:
		return http.StatusInternalServerError, "停止服务器失败，已取消重启"
	case application.CodeRestartNotInstalled:
		return http.StatusBadRequest, "游戏服务端未安装"
	case application.CodeRestartFailed:
		return http.StatusInternalServerError, "服务器重启失败，请检查控制台输出"
	case application.CodeServerNotRunning:
		return http.StatusBadRequest, "服务器未运行，无法发送控制台指令"
	case application.CodeWorkshopIDNotNumeric:
		return http.StatusUnprocessableEntity, "Workshop ID 必须为数字"
	case application.CodeRenewalOutOfRange:
		return http.StatusUnprocessableEntity, "请求数据无效"
	case application.CodeValidation:
		if err.Message == "" {
			return http.StatusUnprocessableEntity, "请求数据无效"
		}
		return http.StatusUnprocessableEntity, err.Message
	case application.CodeOptionReadOnly:
		return http.StatusConflict, messageOr(err, "配置项由其他页面管理")
	case application.CodeOptionUnknown:
		return http.StatusUnprocessableEntity, messageOr(err, "未知配置项")
	case application.CodeOptionValue:
		return http.StatusUnprocessableEntity, messageOr(err, "配置项取值无效")
	case application.CodeInstanceOwned:
		return http.StatusConflict, "instance owned by another process"
	case application.CodeRecoveryRequired:
		return http.StatusConflict, "recovery required"
	case application.CodeOperationFailed:
		// Operation failures carry their own operator-facing message; falling
		// through to the generic text would hide the real cause (for example a
		// fail-closed permission gate) behind "服务器内部错误".
		if err.Message == "" {
			return http.StatusInternalServerError, "服务器内部错误"
		}
		return http.StatusInternalServerError, err.Message
	default:
		return http.StatusInternalServerError, "服务器内部错误"
	}
}

// messageOr prefers the operator-facing message and falls back to a stable
// default, so new coded errors never surface as a generic 500.
func messageOr(err *application.UseCaseError, fallback string) string {
	if err != nil && err.Message != "" {
		return err.Message
	}
	return fallback
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		writeJSONBytes(w, http.StatusInternalServerError, []byte(`{"detail":"服务器内部错误"}`))
		return
	}
	writeJSONBytes(w, status, encoded)
}

func writeJSONBytes(w http.ResponseWriter, status int, encoded []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

func writeFallback(w http.ResponseWriter) {
	writeJSONBytes(w, http.StatusOK, []byte(`{"message":"Gameserver Backend Running"}`))
}

func cloneObject(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+1)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func redactVariables(value any) map[string]any {
	output := make(map[string]any)
	switch variables := value.(type) {
	case map[string]any:
		for key, item := range variables {
			output[key] = item
		}
	case map[string]string:
		for key, item := range variables {
			output[key] = item
		}
	}
	for key, item := range output {
		switch key {
		case "SERVER_PASSWORD":
			output[key] = ""
		case "ADMIN_PASSWORD":
			if truthy(item) {
				output[key] = nil
			} else {
				output[key] = ""
			}
		}
	}
	return output
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case json.Number:
		return typed != "0"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return true
	}
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func runeLength(value string) int { return len([]rune(value)) }

func strictPorts(raw json.RawMessage) (map[string]int, bool) {
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &encoded); err != nil || encoded == nil {
		return nil, false
	}
	ports := make(map[string]int, len(encoded))
	for key, value := range encoded {
		var number int
		if len(value) == 0 || string(value) == "null" || json.Unmarshal(value, &number) != nil {
			return nil, false
		}
		ports[key] = number
	}
	return ports, true
}

func validAssetName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || !safeAssetSegments(name) {
		return false
	}
	candidate := name
	for range 8 {
		decoded, err := url.PathUnescape(candidate)
		if err != nil || decoded == candidate {
			return true
		}
		if !safeAssetSegments(decoded) {
			return false
		}
		candidate = decoded
	}
	// Refuse excessive multi-encoded paths instead of passing an ambiguous
	// name to an injected asset store.
	return false
}

func safeAssetSegments(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00:") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func extension(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 && !strings.Contains(name[index:], "/") {
		return strings.ToLower(name[index:])
	}
	return ""
}

func isStaticRequestPath(path string) bool {
	return path == "/static" || strings.HasPrefix(path, "/static/")
}

func isAPIPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/api/") || requestPath == "/ws/console"
}

func knownPath(requestPath string) bool {
	if isStaticRequestPath(requestPath) || requestPath == "/" {
		return true
	}
	switch requestPath {
	case "/api/auth/status", "/api/auth/login", "/api/auth/logout",
		"/api/status", "/api/templates", "/api/server/install", "/api/server/start",
		"/api/server/stop", "/api/server/restart", "/api/server/kill", "/api/server/command",
		"/api/server/logs", "/api/server/config", "/api/server/mods", "/api/server/renew", "/ws/console":
		return true
	}
	return isModItemPath(requestPath)
}

func allowedMethods(requestPath string) []string {
	switch requestPath {
	case "/", "/api/auth/status", "/api/status", "/api/templates", "/api/server/logs", "/api/server/config", "/api/server/install", "/static", "/static/":
		if requestPath == "/api/server/config" || requestPath == "/api/server/install" {
			return []string{http.MethodGet, http.MethodPost}
		}
		if strings.HasPrefix(requestPath, "/api/") {
			return []string{http.MethodGet}
		}
		return []string{http.MethodGet, http.MethodHead}
	case "/api/auth/login", "/api/auth/logout", "/api/server/start", "/api/server/stop", "/api/server/restart", "/api/server/kill", "/api/server/command", "/api/server/mods", "/api/server/renew", "/ws/console":
		if requestPath == "/api/server/mods" {
			return []string{http.MethodPost}
		}
		if requestPath == "/ws/console" {
			return []string{http.MethodGet}
		}
		return []string{http.MethodPost}
	default:
		if isModItemPath(requestPath) {
			return []string{http.MethodDelete}
		}
		if isStaticRequestPath(requestPath) {
			return []string{http.MethodGet, http.MethodHead}
		}
		return []string{}
	}
}

func isModItemPath(requestPath string) bool {
	const prefix = "/api/server/mods/"
	if !strings.HasPrefix(requestPath, prefix) {
		return false
	}
	item := strings.TrimPrefix(requestPath, prefix)
	if item == "" || item == "." || item == ".." || strings.ContainsAny(item, "/\\") || path.Clean(requestPath) != requestPath {
		return false
	}
	for range 8 {
		decoded, err := url.PathUnescape(item)
		if err != nil || decoded == item {
			return true
		}
		if decoded == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\") {
			return false
		}
		item = decoded
	}
	return false
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func containsTraversal(path string) bool {
	if !isStaticRequestPath(path) {
		return false
	}
	name := strings.TrimPrefix(path, "/static")
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(name, "\\", "/")
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func cookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Server) registerConnection(conn *websocket.Conn) bool {
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = conn.CloseNow()
		return false
	}
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	return true
}

func (s *Server) unregisterConnection(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func readSocket(ctx context.Context, conn *websocket.Conn, destination chan<- websocketMessage) {
	for {
		kind, data, err := conn.Read(ctx)
		message := websocketMessage{kind: kind, data: data, err: err}
		select {
		case destination <- message:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func writeSocketJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode console frame: %w", err)
	}
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, encoded)
}

func parseConsoleMessage(data []byte) (string, string, error) {
	var message struct {
		Type *string `json:"type"`
		Data *string `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil || message.Type == nil {
		return "", "", errors.New("invalid console message")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", "", errors.New("invalid console message")
	}
	switch *message.Type {
	case "ping":
		return "ping", "", nil
	case "input":
		if message.Data == nil {
			return "", "", errors.New("input data is required")
		}
		return "input", *message.Data, nil
	default:
		return "", "", errors.New("unknown console message")
	}
}

var _ http.Handler = (*Server)(nil)
