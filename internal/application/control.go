package application

import "context"

// Control composes the application use cases exposed by the inbound HTTP and
// console adapters. Implementations coordinate domain state and outbound
// ports; this interface contains no HTTP, cookie, route, or WebSocket types.
type Control interface {
	Status(context.Context) (StatusResponse, error)
	Templates(context.Context) ([]TemplateSummary, error)
	BeginInstall(context.Context) (InstallAccepted, error)
	InstallState(context.Context) (InstallStatus, error)
	Start(context.Context) (StartResult, error)
	Stop(context.Context) (OperationResult, error)
	Restart(context.Context) (OperationResult, error)
	Kill(context.Context) (OperationResult, error)
	SendCommand(context.Context, string) (CommandResult, error)
	SendConsoleInput(context.Context, string) (bool, error)
	Logs(context.Context, int) ([]string, error)
	Config(context.Context) (ConfigSnapshot, error)
	UpdateConfig(context.Context, ConfigUpdate) (ConfigUpdateResult, error)
	AddMod(context.Context, AddModRequest) (ModsResult, error)
	RemoveMod(context.Context, string) (ModsResult, error)
	Renew(context.Context, int) (RenewalResult, error)
	SubscribeConsole(context.Context, int) (ConsoleSubscription, error)
}

// StatusResponse is the additive, extensible status projection returned by
// the application. The adapter must redact known secret fields before writing
// it to a client.
type StatusResponse map[string]any

// TemplateSummary is the legacy template list projection. supported_os is
// compatibility metadata, not a platform support claim.
type TemplateSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Icon        string   `json:"icon"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	SupportedOS []string `json:"supported_os"`
	AppID       string   `json:"app_id"`
}

// InstallAccepted is the accepted (HTTP 202) response for asynchronous install.
type InstallAccepted struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// InstallStatus is the observable in-memory installer state.
type InstallStatus struct {
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	Message  string  `json:"message"`
	Error    *string `json:"error"`
}

// StartResult is the start endpoint response.
type StartResult struct {
	Message string `json:"message"`
	Running bool   `json:"running"`
}

// OperationResult is used by stop, restart, and kill responses.
type OperationResult struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// CommandResult is the REST console-command response.
type CommandResult struct {
	Success bool `json:"success"`
}

// ConfigSnapshot is the UI-facing configuration projection. Fields preserve
// the registered template metadata while the transport removes defaults from
// password fields and always masks their values.
type ConfigSnapshot struct {
	Variables    map[string]any `json:"variables"`
	Ports        map[string]int `json:"ports"`
	Fields       []ConfigField  `json:"fields"`
	AllowedPorts []string       `json:"-"`
	Template     ConfigTemplate `json:"template"`
}

// ConfigField is one editable configuration definition as projected to the UI.
type ConfigField struct {
	Key          string         `json:"key"`
	Label        string         `json:"label"`
	Type         string         `json:"type"`
	Value        any            `json:"value"`
	Default      any            `json:"default,omitempty"`
	Description  string         `json:"description,omitempty"`
	Required     bool           `json:"required,omitempty"`
	UserEditable bool           `json:"user_editable"`
	Validation   map[string]any `json:"validation,omitempty"`
}

// ConfigTemplate identifies the instance's selected template.
type ConfigTemplate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ConfigUpdate contains only caller-supplied editable values. A nil Ports map
// means the request omitted ports; empty maps are allowed and mean no update.
type ConfigUpdate struct {
	Variables map[string]any
	Ports     map[string]int
}

// ConfigUpdateResult preserves the legacy update response fields.
type ConfigUpdateResult struct {
	Message string         `json:"message"`
	State   map[string]any `json:"state"`
}

// AddModRequest is a workshop item registration request; it does not request a
// download.
type AddModRequest struct {
	WorkshopID string
	ModName    *string
}

// ModsResult preserves the legacy mods response.
type ModsResult struct {
	Message string         `json:"message"`
	Mods    map[string]any `json:"mods"`
}

// RenewalResult preserves the billing response.
type RenewalResult struct {
	Message string         `json:"message"`
	Billing map[string]any `json:"billing"`
}

// ConsoleSubscription is the application-owned seam between process logs and
// the WebSocket transport. Replay returns at most the requested newest lines;
// Events is closed on cancellation/disconnect; Close removes the listener and
// releases all subscription resources and is safe to call more than once.
type ConsoleSubscription interface {
	Replay() []string
	Events() <-chan string
	Close()
}
