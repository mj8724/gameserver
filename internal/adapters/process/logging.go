package process

import (
	"context"
	"log"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// LoggingSupervisor decorates a supervisor and logs operation failures. The
// user-facing contract keeps its generic message ("服务器启动失败，请检查控制台输出"),
// so the underlying cause must be observable somewhere: this decorator is that
// place, and it also reports the resolved launch spec on start failures.
type LoggingSupervisor struct {
	Inner ports.ProcessSupervisor
}

// Start implements ports.ProcessSupervisor.
func (l *LoggingSupervisor) Start(ctx context.Context, spec ports.LaunchSpec) (ports.Process, error) {
	process, err := l.Inner.Start(ctx, spec)
	if err != nil {
		log.Printf("process start failed: executable=%q workdir=%q args=%d err=%v",
			spec.Executable, spec.WorkDir, len(spec.Args), err)
		return process, err
	}
	return process, nil
}

// Stop implements ports.ProcessSupervisor.
func (l *LoggingSupervisor) Stop(ctx context.Context, id domain.InstanceID) error {
	if err := l.Inner.Stop(ctx, id); err != nil {
		log.Printf("process stop failed: instance=%s err=%v", id, err)
		return err
	}
	return nil
}

// Kill implements ports.ProcessSupervisor.
func (l *LoggingSupervisor) Kill(ctx context.Context, id domain.InstanceID) error {
	if err := l.Inner.Kill(ctx, id); err != nil {
		log.Printf("process kill failed: instance=%s err=%v", id, err)
		return err
	}
	return nil
}

// SendInput writes console input to the running process.
func (l *LoggingSupervisor) SendInput(ctx context.Context, id domain.InstanceID, input string) error {
	if err := l.Inner.SendInput(ctx, id, input); err != nil {
		log.Printf("console input failed: instance=%s err=%v", id, err)
		return err
	}
	return nil
}

// Status implements ports.ProcessStatusProvider.
func (l *LoggingSupervisor) Status() (string, int, bool) {
	if provider, ok := l.Inner.(interface{ Status() (string, int, bool) }); ok {
		return provider.Status()
	}
	return "STOPPED", 0, false
}

// ProcessStatus implements ports.ProcessStatusProvider.
func (l *LoggingSupervisor) ProcessStatus(ctx context.Context, id domain.InstanceID) (ports.ProcessStatus, error) {
	if provider, ok := l.Inner.(ports.ProcessStatusProvider); ok {
		return provider.ProcessStatus(ctx, id)
	}
	return ports.ProcessStatus{Status: "STOPPED"}, nil
}

// Recent implements ports.LogSource.
func (l *LoggingSupervisor) Recent(limit int) []string {
	if source, ok := l.Inner.(ports.LogSource); ok {
		return source.Recent(limit)
	}
	return []string{}
}

// Subscribe implements ports.LogSource.
func (l *LoggingSupervisor) Subscribe(buffer int) ports.LogSubscription {
	if source, ok := l.Inner.(ports.LogSource); ok {
		return source.Subscribe(buffer)
	}
	return nil
}

var _ ports.ProcessSupervisor = (*LoggingSupervisor)(nil)
