package application

import (
	"context"
	"errors"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

// Install intent reconciliation (ADR §5.4 D10).
//
// The intent file records the last known artifact fingerprint at each phase:
// while RUNNING it holds the fingerprint observed *before* the attempt, so a
// digest change proves Steam reached its manifest-commit step (which happens
// only after content is in place). That lets a restart fold a crash-interrupted
// install into DONE without downloading anything twice.
//
// The caller must have passed the instance lock and ownership checks first:
// Start and BeginInstall both call ensureLock before this runs, so a stale
// owner surfaces as RECOVERY_REQUIRED and never gets overwritten by a resume.

// reconcileInstallIntent resolves an in-flight intent recorded by a previous
// process. Terminal records are cleared; RUNNING/VERIFYING/REQUESTED are folded
// into DONE or FAILED based on the current artifact fingerprint.
func (s *ControlService) reconcileInstallIntent(ctx context.Context) error {
	if s.deps.Intents == nil {
		return nil
	}
	intent, err := s.deps.Intents.Load(ctx, s.deps.Instance)
	if errors.Is(err, ports.ErrIntentAbsent) {
		return nil
	}
	if err != nil {
		return WrapError(CodeOperationFailed, "读取安装意图失败", err)
	}

	switch intent.Phase {
	case "", ports.PhaseDone, ports.PhaseFailed:
		return s.clearIntent(ctx)
	case ports.PhaseRequested, ports.PhaseRunning, ports.PhaseVerifying:
	default:
		return WrapError(CodeOperationFailed, "安装意图阶段未知", errors.New("unknown intent phase "+intent.Phase))
	}

	current, fpErr := s.deps.Files.Fingerprint(s.deps.Instance)
	if fpErr != nil {
		return WrapError(CodeOperationFailed, "无法核对安装产物", fpErr)
	}
	// A manifest digest different from the recorded pre-attempt fingerprint
	// means the interrupted attempt committed content.
	committed := current.ManifestSHA != ""
	switch intent.Phase {
	case ports.PhaseRunning, ports.PhaseVerifying:
		committed = committed && current.ManifestSHA != intent.ManifestSHA
	case ports.PhaseRequested:
		// The attempt never reported progress; nothing can have been committed.
		committed = false
	}

	updated := intent
	updated.UpdatedAt = s.deps.Clock.Now().Format(time.RFC3339)
	if committed {
		updated.Phase = ports.PhaseDone
		updated.ManifestSHA = current.ManifestSHA
		updated.TotalBytes = current.TotalBytes
		updated.BuildID = firstNonEmpty(current.BuildID, intent.BuildID)
		updated.Message = "安装完成（重启对账收敛，未重复下载）"
		updated.ErrorText = ""
		if err := s.deps.Intents.Write(ctx, s.deps.Instance, updated); err != nil {
			return WrapError(CodeOperationFailed, "写入安装意图失败", err)
		}
		s.projectInstall(installProjection{status: "COMPLETED", progress: 100, message: updated.Message, version: intent.Branch})
		return s.clearIntent(ctx)
	}

	updated.Phase = ports.PhaseFailed
	updated.Message = "安装中断：启动对账未发现完整产物，请重试安装"
	updated.ErrorText = "reconciled after process restart; artifacts incomplete"
	if err := s.deps.Intents.Write(ctx, s.deps.Instance, updated); err != nil {
		return WrapError(CodeOperationFailed, "写入安装意图失败", err)
	}
	s.projectInstall(installProjection{status: "FAILED", message: updated.Message, errText: updated.ErrorText, version: intent.Branch})
	return s.clearIntent(ctx)
}

func (s *ControlService) clearIntent(ctx context.Context) error {
	if s.deps.Intents == nil {
		return nil
	}
	if err := s.deps.Intents.Clear(ctx, s.deps.Instance); err != nil {
		return WrapError(CodeOperationFailed, "清理安装意图失败", err)
	}
	return nil
}

type installProjection struct {
	status   string
	progress float64
	message  string
	errText  string
	version  string
}

// projectInstall reflects a reconciled terminal state through the legacy
// install_task projection without inventing new field names.
func (s *ControlService) projectInstall(p installProjection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.install.running = false
	s.install.cancel = nil
	if p.version != "" {
		s.install.version = p.version
	}
	s.install.status = p.status
	if p.progress > 0 {
		s.install.progress = p.progress
	}
	if p.message != "" {
		s.install.message = p.message
	}
	if p.errText != "" {
		text := p.errText
		s.install.errText = &text
	} else {
		s.install.errText = nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// persistIntentPhase records the intent for the in-flight install. It is a
// best-effort persistence: callers ignore failures from the terminal write so
// a disk error never turns a successful install into a reported failure.
func (s *ControlService) persistIntentPhase(phase, branch, buildID string, started time.Time, manifestSHA string, totalBytes int64, message string) error {
	if s.deps.Intents == nil {
		return nil
	}
	intent := ports.InstallIntent{
		Phase:       phase,
		Branch:      branch,
		BuildID:     buildID,
		StartedAt:   started.Format(time.RFC3339),
		UpdatedAt:   s.deps.Clock.Now().Format(time.RFC3339),
		Message:     message,
		ManifestSHA: manifestSHA,
		TotalBytes:  totalBytes,
	}
	return s.deps.Intents.Write(context.Background(), s.deps.Instance, intent)
}
