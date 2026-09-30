package ports

import (
	"context"
	"errors"
	"time"
)

// Node identity and durable task ledger (M6). The identity is a keypair whose
// private half never leaves the node; the ledger makes remote operations
// idempotent so a lost connection can be retried without repeating an action.
var (
	ErrNodeUnknown  = errors.New("node is not registered")
	ErrNodeRevoked  = errors.New("node is revoked")
	ErrTaskConflict = errors.New("request id already used with different input")
)

// NodeIdentity is the local node's keypair material.
type NodeIdentity struct {
	NodeID      string    `json:"node_id"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"public_key"`
	CreatedAt   time.Time `json:"created_at"`
	RotatedAt   time.Time `json:"rotated_at,omitempty"`
	// PrivateKeyPath is where the secret half lives (0600, never returned).
	PrivateKeyPath string `json:"-"`
}

// RegisteredNode is a peer node accepted by this control plane.
type RegisteredNode struct {
	NodeID       string    `json:"node_id"`
	Fingerprint  string    `json:"fingerprint"`
	PublicKey    string    `json:"public_key"`
	RegisteredAt time.Time `json:"registered_at"`
	RevokedAt    time.Time `json:"revoked_at,omitempty"`
}

// NodeStore persists this node's identity and its peer registry.
type NodeStore interface {
	Identity(ctx context.Context) (NodeIdentity, error)
	// Rotate generates a new keypair, keeping the node id and revoking nothing.
	Rotate(ctx context.Context) (NodeIdentity, error)
	Register(ctx context.Context, node RegisteredNode) error
	Revoke(ctx context.Context, nodeID string) error
	// Authorize reports whether a peer may act, failing closed for unknown and
	// revoked nodes.
	Authorize(ctx context.Context, nodeID, fingerprint string) error
	List(ctx context.Context) ([]RegisteredNode, error)
}

// TaskState is the durable lifecycle of one remote request.
const (
	TaskPending   = "pending"
	TaskCompleted = "completed"
	TaskFailed    = "failed"
)

// TaskRecord is one idempotent remote task keyed by request id.
type TaskRecord struct {
	RequestID string    `json:"request_id"`
	NodeID    string    `json:"node_id"`
	Operation string    `json:"operation"`
	InputHash string    `json:"input_hash"`
	State     string    `json:"state"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskLedger records remote tasks so a replay returns the stored result
// instead of executing the operation twice.
type TaskLedger interface {
	// Begin claims a request id. It returns the existing record when the same
	// id already ran (with the same input hash), ErrTaskConflict when the id
	// was reused with different input, and a fresh pending record otherwise.
	Begin(ctx context.Context, record TaskRecord) (TaskRecord, bool, error)
	Complete(ctx context.Context, requestID, result, errText string) error
	Get(ctx context.Context, requestID string) (TaskRecord, error)
	List(ctx context.Context) ([]TaskRecord, error)
}
