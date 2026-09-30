package application

import (
	"context"
	"sort"

	"github.com/mj8724/gameserver/internal/ports"
)

// ListInstances aggregates the host registry with each instance's live state so
// the control surface can show every instance in one call (M5.3). It is
// additive: the single-instance Control interface is untouched, and a host
// without a registry simply reports the active instance.
func (s *ControlService) ListInstances(ctx context.Context) ([]ports.InstanceView, error) {
	views := []ports.InstanceView{}
	if s.deps.Registry != nil {
		records, err := s.deps.Registry.List(ctx)
		if err != nil {
			return nil, WrapError(CodeOperationFailed, "读取实例注册表失败", err)
		}
		for _, record := range records {
			view := ports.InstanceView{Record: record}
			view.Active = record.ID == s.deps.Instance
			if view.Active {
				// Only the active instance has a live process/state in this
				// process; other instances are reported by their registry record.
				status := s.processStatus(ctx)
				view.Running = status.Running
				view.Status = status.Status
				s.mu.Lock()
				view.Ready = s.ready
				s.mu.Unlock()
				if state, err := s.load(ctx); err == nil {
					view.Ports = state.Ports
				}
			} else {
				view.Status = "registered"
			}
			views = append(views, view)
		}
	} else {
		status := s.processStatus(ctx)
		views = append(views, ports.InstanceView{
			Record:  ports.InstanceRecord{ID: s.deps.Instance, DataRoot: ""},
			Running: status.Running,
			Status:  status.Status,
			Ready:   func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.ready }(),
			Active:  true,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Record.ID < views[j].Record.ID })
	return views, nil
}

var _ ports.InstanceLister = (*ControlService)(nil)
