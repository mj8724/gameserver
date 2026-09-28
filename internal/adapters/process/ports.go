package process

import (
	"context"
	"sync"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Recent implements ports.LogSource over the bounded in-memory buffer.
func (s *Supervisor) Recent(limit int) []string { return s.Logs(limit) }

// Subscribe implements ports.LogSource. The returned subscription never closes
// its channel, so a log broadcast racing with Close cannot panic; consumers
// stop on their own cancellation instead.
func (s *Supervisor) Subscribe(buffer int) ports.LogSubscription {
	if buffer < 1 {
		buffer = 64
	}
	subscription := &logSubscription{lines: make(chan string, buffer), done: make(chan struct{})}
	subscription.cancel = s.AddListener(func(line string) {
		select {
		case <-subscription.done:
			return
		default:
		}
		select {
		case subscription.lines <- line:
		case <-subscription.done:
		}
	})
	return subscription
}

type logSubscription struct {
	lines  chan string
	done   chan struct{}
	once   sync.Once
	cancel func()
}

func (l *logSubscription) Lines() <-chan string { return l.lines }

func (l *logSubscription) Close() {
	l.once.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		close(l.done)
	})
}

// ProcessStatusFor implements ports.ProcessStatusProvider for one instance.
func (s *Supervisor) ProcessStatus(_ context.Context, instance domain.InstanceID) (ports.ProcessStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := ports.ProcessStatus{Status: s.status}
	owned := s.process
	if owned == nil || owned.instance != instance {
		return status, nil
	}
	select {
	case <-owned.done:
		return status, nil
	default:
	}
	status.Running = true
	status.PID = owned.pid
	if !owned.started.IsZero() {
		status.UptimeSeconds = int(time.Since(owned.started).Seconds())
	}
	return status, nil
}

var (
	_ ports.LogSource             = (*Supervisor)(nil)
	_ ports.ProcessStatusProvider = (*Supervisor)(nil)
)
