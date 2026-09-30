package application

import (
	"context"
	"errors"
	"sort"

	"github.com/mj8724/gameserver/internal/domain"
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

// CreateInstance registers a new instance and allocates its port pair (M5.1).
// It refuses duplicates and unknown templates; it never deletes or overwrites.
func (s *ControlService) CreateInstance(ctx context.Context, id string, templateID domain.TemplateID) (ports.InstanceView, error) {
	if s.deps.Registry == nil {
		return ports.InstanceView{}, NewError(CodeOperationFailed, "实例注册表不可用")
	}
	if _, ok := s.deps.Templates.Get(templateID); !ok {
		return ports.InstanceView{}, NewError(CodeOperationFailed, "模板不存在："+string(templateID))
	}
	primaryKey, directKey := "SERVER_PORT", "DIRECT_PORT"
	allocated := map[string]int{}
	if s.deps.Ports != nil {
		ports_, err := s.deps.Ports.Allocate(ctx, primaryKey, directKey)
		if err != nil {
			return ports.InstanceView{}, WrapError(CodeOperationFailed, "端口分配失败", err)
		}
		allocated = ports_
	}
	record := ports.InstanceRecord{ID: domain.InstanceID(id), TemplateID: string(templateID), Name: id}
	if err := s.deps.Registry.Add(ctx, record); err != nil {
		if errors.Is(err, ports.ErrInstanceExists) {
			return ports.InstanceView{}, NewError(CodeOperationFailed, "实例已存在："+id)
		}
		return ports.InstanceView{}, WrapError(CodeOperationFailed, "写入实例注册表失败", err)
	}
	return ports.InstanceView{Record: record, Status: "registered", Ports: allocated}, nil
}

// RemoveInstance unregisters an instance. Data on disk is never deleted.
func (s *ControlService) RemoveInstance(ctx context.Context, id domain.InstanceID) error {
	if s.deps.Registry == nil {
		return NewError(CodeOperationFailed, "实例注册表不可用")
	}
	if id == s.deps.Instance {
		return NewError(CodeOperationFailed, "不能注销当前活跃实例")
	}
	if err := s.deps.Registry.Remove(ctx, id); err != nil {
		if errors.Is(err, ports.ErrInstanceUnknown) {
			return NewError(CodeOperationFailed, "实例未注册："+string(id))
		}
		return WrapError(CodeOperationFailed, "移除实例注册失败", err)
	}
	return nil
}
