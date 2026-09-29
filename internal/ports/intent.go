package ports

import (
	"context"
	"errors"

	"github.com/mj8724/gameserver/internal/domain"
)

// ErrIntentAbsent is returned by IntentLog.Load when no intent was recorded.
var ErrIntentAbsent = errors.New("no install intent recorded")

// Install phases persisted by the intent log (ADR §5.4 D10). The legacy
// projection (IDLE/INSTALLING/COMPLETED/FAILED) is derived from these; only
// the values below exist in the intent file.
const (
	PhaseRequested = "REQUESTED"
	PhaseRunning   = "RUNNING"
	PhaseVerifying = "VERIFYING"
	PhaseDone      = "DONE"
	PhaseFailed    = "FAILED"
)

// InstallIntent is the durable record of one install attempt.
type InstallIntent struct {
	Phase       string `json:"phase"`
	Branch      string `json:"branch,omitempty"`
	BuildID     string `json:"build_id,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Message     string `json:"message,omitempty"`
	ErrorText   string `json:"error_text,omitempty"`
	ManifestSHA string `json:"manifest_sha256,omitempty"`
	TotalBytes  int64  `json:"total_bytes,omitempty"`
}

// IntentLog persists an install intent next to the instance state. A missing
// intent (ErrIntentAbsent on Load) means "nothing to reconcile".
type IntentLog interface {
	Load(ctx context.Context, id domain.InstanceID) (InstallIntent, error)
	Write(ctx context.Context, id domain.InstanceID, intent InstallIntent) error
	Clear(ctx context.Context, id domain.InstanceID) error
}
