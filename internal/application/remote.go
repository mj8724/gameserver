package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mj8724/gameserver/internal/ports"
)

// RemoteOperation is one reviewed unit of remote execution. Remote callers can
// only request these operations; there is no generic command channel.
type RemoteOperation string

const (
	RemoteStart   RemoteOperation = "start"
	RemoteStop    RemoteOperation = "stop"
	RemoteRestart RemoteOperation = "restart"
	RemoteInstall RemoteOperation = "install"
	RemoteStatus  RemoteOperation = "status"
	RemoteQuery   RemoteOperation = "query"
)

// remoteAllowlist is the complete set of remotely executable operations.
var remoteAllowlist = map[RemoteOperation]bool{
	RemoteStart:   true,
	RemoteStop:    true,
	RemoteRestart: true,
	RemoteInstall: true,
	RemoteStatus:  true,
	RemoteQuery:   true,
}

// RemoteRequest is one authenticated remote invocation. Parameters are a closed
// set of scalars, never a command string.
type RemoteRequest struct {
	NodeID      string
	Fingerprint string
	Operator    string
	RequestID   string
	Operation   RemoteOperation
	Version     string
}

// RemoteResult is the outcome returned to the caller (and replayed on retry).
type RemoteResult struct {
	RequestID string `json:"request_id"`
	Operation string `json:"operation"`
	Executed  bool   `json:"executed"`
	Outcome   string `json:"outcome"`
	Detail    string `json:"detail,omitempty"`
}

// inputHash binds a request id to the exact input, so a replay with different
// parameters can never be answered with the first execution's result.
func (r RemoteRequest) inputHash() string {
	parts := []string{string(r.Operation), r.Version}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// ExecuteRemote runs one allowlisted operation for an authorized node, exactly
// once per request id, and appends an audit record for every attempt.
func (s *ControlService) ExecuteRemote(ctx context.Context, request RemoteRequest) (RemoteResult, error) {
	if s.deps.Nodes == nil {
		return RemoteResult{}, NewError(CodeOperationFailed, "节点存储不可用")
	}
	if s.deps.Audit == nil {
		return RemoteResult{}, NewError(CodeOperationFailed, "审计日志不可用")
	}
	if !remoteAllowlist[request.Operation] {
		s.audit(ctx, request, "rejected", "operation not in the remote allowlist")
		return RemoteResult{}, NewError(CodeOperationFailed, ports.ErrOperationNotAllowed.Error()+": "+string(request.Operation))
	}
	if err := s.deps.Nodes.Authorize(ctx, request.NodeID, request.Fingerprint); err != nil {
		s.audit(ctx, request, "unauthorized", err.Error())
		return RemoteResult{}, WrapError(CodeOperationFailed, "节点未获授权", err)
	}
	if strings.TrimSpace(request.RequestID) == "" {
		s.audit(ctx, request, "rejected", "missing request id")
		return RemoteResult{}, NewError(CodeOperationFailed, "远程请求必须携带 request_id")
	}

	record := ports.TaskRecord{
		RequestID: request.RequestID,
		NodeID:    request.NodeID,
		Operation: string(request.Operation),
		InputHash: request.inputHash(),
	}
	if s.deps.Tasks != nil {
		existing, existed, err := s.deps.Tasks.Begin(ctx, record)
		if err != nil {
			if errors.Is(err, ports.ErrTaskConflict) {
				s.audit(ctx, request, "conflict", "request id reused with different input")
				return RemoteResult{}, NewError(CodeOperationFailed, "request_id 已被不同参数使用")
			}
			return RemoteResult{}, WrapError(CodeOperationFailed, "任务台账不可用", err)
		}
		if existed {
			// Replay: report the stored outcome and never execute again.
			s.audit(ctx, request, "replayed", existing.State)
			outcome := existing.State
			if outcome == ports.TaskPending {
				outcome = "in_progress"
			}
			return RemoteResult{RequestID: request.RequestID, Operation: string(request.Operation), Executed: false, Outcome: outcome, Detail: existing.Result}, nil
		}
		defer func() { _ = err }()
		if err := s.executeRemoteOperation(ctx, request); err != nil {
			if s.deps.Tasks != nil {
				_ = s.deps.Tasks.Complete(ctx, request.RequestID, "", err.Error())
			}
			s.audit(ctx, request, "failed", err.Error())
			return RemoteResult{}, err
		}
		if s.deps.Tasks != nil {
			_ = s.deps.Tasks.Complete(ctx, request.RequestID, "ok", "")
		}
	} else if err := s.executeRemoteOperation(ctx, request); err != nil {
		s.audit(ctx, request, "failed", err.Error())
		return RemoteResult{}, err
	}
	s.audit(ctx, request, "executed", "")
	return RemoteResult{RequestID: request.RequestID, Operation: string(request.Operation), Executed: true, Outcome: ports.TaskCompleted}, nil
}

// executeRemoteOperation maps an allowlisted operation onto a local use case.
func (s *ControlService) executeRemoteOperation(ctx context.Context, request RemoteRequest) error {
	switch request.Operation {
	case RemoteStart:
		_, err := s.Start(ctx)
		return err
	case RemoteStop:
		_, err := s.Stop(ctx)
		return err
	case RemoteRestart:
		_, err := s.Restart(ctx)
		return err
	case RemoteInstall:
		_, err := s.BeginInstall(ctx, request.Version)
		return err
	case RemoteStatus:
		_, err := s.Status(ctx)
		return err
	case RemoteQuery:
		_, err := s.Status(ctx)
		return err
	default:
		return NewError(CodeOperationFailed, ports.ErrOperationNotAllowed.Error())
	}
}

func (s *ControlService) audit(ctx context.Context, request RemoteRequest, outcome, detail string) {
	if s.deps.Audit == nil {
		return
	}
	_ = s.deps.Audit.Append(ctx, ports.AuditEntry{
		At:        s.deps.Clock.Now(),
		Operator:  request.Operator,
		NodeID:    request.NodeID,
		Operation: string(request.Operation),
		RequestID: request.RequestID,
		InputHash: request.inputHash(),
		Outcome:   outcome,
		Detail:    detail,
	})
}

// AuditRecent exposes the trail for review.
func (s *ControlService) AuditRecent(ctx context.Context, limit int) ([]ports.AuditEntry, error) {
	if s.deps.Audit == nil {
		return []ports.AuditEntry{}, nil
	}
	entries, err := s.deps.Audit.Recent(ctx, limit)
	if err != nil {
		return nil, WrapError(CodeOperationFailed, "读取审计日志失败", err)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At.Before(entries[j].At) })
	return entries, nil
}

var _ = fmt.Sprintf

// RemoteExecutor is the remote-execution surface (M6.2).
type RemoteExecutor interface {
	ExecuteRemote(ctx context.Context, request RemoteRequest) (RemoteResult, error)
}

// AuditReader exposes the audit trail.
type AuditReader interface {
	AuditRecent(ctx context.Context, limit int) ([]ports.AuditEntry, error)
}

var _ RemoteExecutor = (*ControlService)(nil)
var _ AuditReader = (*ControlService)(nil)
