package ports

import (
	"context"
	"errors"
	"time"
)

// ErrOperationNotAllowed is returned for any remote operation outside the
// reviewed allowlist. Remote execution never accepts arbitrary commands.
var ErrOperationNotAllowed = errors.New("operation is not allowed for remote execution")

// AuditEntry is one append-only record of a privileged action.
type AuditEntry struct {
	At        time.Time `json:"at"`
	Operator  string    `json:"operator"`
	NodeID    string    `json:"node_id,omitempty"`
	Operation string    `json:"operation"`
	RequestID string    `json:"request_id,omitempty"`
	InputHash string    `json:"input_hash,omitempty"`
	Outcome   string    `json:"outcome"`
	Detail    string    `json:"detail,omitempty"`
}

// AuditLog appends privileged-action records. Implementations must never
// rewrite, truncate or reorder existing records.
type AuditLog interface {
	Append(ctx context.Context, entry AuditEntry) error
	Recent(ctx context.Context, limit int) ([]AuditEntry, error)
}
