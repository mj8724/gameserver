package application

import (
	"context"

	"github.com/mj8724/gameserver/internal/ports"
)

// Control composes the application use cases exposed by the inbound HTTP and
// console adapters. Implementations coordinate domain state and outbound
// ports; this interface contains no HTTP, cookie, route, or WebSocket types.
type Control interface {
	Status(context.Context) (StatusResponse, error)
	Templates(context.Context) ([]TemplateSummary, error)
	BeginInstall(ctx context.Context, version string) (InstallAccepted, error)
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
	DownloadMod(context.Context, string, *string) (ModsResult, error)
	RemoveMod(context.Context, string) (ModsResult, error)
	Renew(context.Context, int) (RenewalResult, error)
	SubscribeConsole(context.Context, int) (ConsoleSubscription, error)
}

// InstanceLister projects the host registry plus live state (M5.3).
type InstanceLister interface {
	ListInstances(ctx context.Context) ([]ports.InstanceView, error)
}

// InstanceSummary is the multi-instance projection (M5.3): the registry record
// plus the instance's own running/ready state, so one call answers "which
// instances exist and what are they doing".
type InstanceSummary struct {
	ID         string         `json:"instance_id"`
	Name       string         `json:"name"`
	TemplateID string         `json:"template_id"`
	DataRoot   string         `json:"data_root"`
	Running    bool           `json:"running"`
	Ready      bool           `json:"ready"`
	Status     string         `json:"status"`
	Ports      map[string]int `json:"ports,omitempty"`
	// Active marks the instance this control session currently operates on.
	Active bool `json:"active"`
}

// StatusResponse is the additive, extensible status projection returned by
// the application. The adapter must redact known secret fields before writing
// it to a client.
type StatusResponse map[string]any

// TemplateVersion is one selectable server branch (evidence-backed).
type TemplateVersion struct {
	Label       string `json:"label"`
	Branch      string `json:"branch"`
	BuildID     string `json:"build_id,omitempty"`
	Default     bool   `json:"default"`
	EvidenceRef string `json:"evidence_ref"`
}

// TemplateSummary is the legacy template list projection. supported_os is
// compatibility metadata, not a platform support claim.
type TemplateSummary struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Icon        string            `json:"icon"`
	Author      string            `json:"author"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	SupportedOS []string          `json:"supported_os"`
	AppID       string            `json:"app_id"`
	Versions    []TemplateVersion `json:"versions,omitempty"`
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
	// Version echoes the selected Steam branch (empty = template default).
	Version string `json:"version,omitempty"`
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
	// Options and Groups are additive: the catalogue-driven view of the vendor
	// configuration. fields[] keeps its legacy shape for existing consumers.
	Options []ConfigOption `json:"options,omitempty"`
	Groups  []string       `json:"groups,omitempty"`
	// PendingRestart reports that an option requiring a restart was saved while
	// the server was running; the console must say so instead of implying the
	// change is already live.
	PendingRestart bool `json:"pending_restart"`
	// CatalogDegraded reports that the option catalogue could not be loaded.
	CatalogDegraded bool `json:"catalog_degraded,omitempty"`
}

// ConfigOption is one catalogue entry projected for the console, with the value
// read back from the vendor file (never from the request).
type ConfigOption struct {
	Key             string   `json:"key"`
	Target          string   `json:"target"`
	Label           string   `json:"label"`
	Type            string   `json:"type"`
	Secret          bool     `json:"secret"`
	Value           any      `json:"value"`
	Default         any      `json:"default,omitempty"`
	Min             *float64 `json:"min,omitempty"`
	Max             *float64 `json:"max,omitempty"`
	Enum            []string `json:"enum,omitempty"`
	Group           string   `json:"group"`
	Description     string   `json:"description,omitempty"`
	RequiresRestart bool     `json:"requires_restart"`
	Writable        string   `json:"writable"`
	Clearable       bool     `json:"clearable"`
	Source          string   `json:"source"`
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
	// Options carries catalogue-driven values (plan D-D). It is a separate
	// write path from Variables: one physical key, one writer.
	Options map[string]string
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
