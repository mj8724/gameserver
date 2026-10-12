package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Capacity errors carried through the use-case error codes.
var (
	errCapacityExceeded = errors.New("capacity exceeded")
)

// CapacityConfig is the validated capacity policy (M3.5). Thresholds are
// optional: zero or negative values disable the corresponding check so the
// historical behaviour (no limits) stays the default.
type CapacityConfig struct {
	// SoftPercent alerts (status flag) when usage crosses it.
	SoftPercent float64
	// HardPercent refuses new writes when usage crosses it.
	HardPercent float64
}

// capacityGuard double-checks capacity at API entry and again right before a
// write lands (the check-and-write race is covered by the second check). It
// never deletes anything; recovery is operator-driven.
// NewCapacity builds the double-checked capacity policy from configured
// thresholds and the effective quota. Low-Level guard: usage comes from the
// same files adapter that projects DiskUsageMB so the two views agree.
func NewCapacity(softPercent, hardPercent, quotaGB float64, usage diskUsageProvider) ports.CapacityChecker {
	return &capacityGuard{usage: usage, quota: quotaGB, config: CapacityConfig{SoftPercent: softPercent, HardPercent: hardPercent}}
}

type capacityGuard struct {
	usage  diskUsageProvider
	quota  float64 // effective quota in GB (>0 when enforced)
	config CapacityConfig
}

// CheckBeforeWrite implements ports.CapacityChecker.
func (g *capacityGuard) CheckBeforeWrite(ctx context.Context, instance domain.InstanceID) error {
	return g.check(ctx, instance)
}

func (g *capacityGuard) check(ctx context.Context, instance domain.InstanceID) error {
	if g == nil || g.usage == nil || g.quota <= 0 || g.config.HardPercent <= 0 {
		return nil
	}
	used, err := g.usage.DiskUsageMB(instance)
	if err != nil {
		// A failing measurement fails closed for writes that grow the disk.
		return fmt.Errorf("容量测量失败：%w", err)
	}
	hardMB := g.quota * 1024 * g.config.HardPercent / 100
	if used >= hardMB {
		return fmt.Errorf("%w：磁盘用量已达硬阈值（%.1f MB / %.0f MB）", errCapacityExceeded, used, hardMB)
	}
	return nil
}

// Status implements ports.CapacityChecker.
func (g *capacityGuard) Status(ctx context.Context, instance domain.InstanceID) (ports.CapacityStatus, error) {
	state, used, err := g.status(ctx, instance)
	if err != nil {
		return ports.CapacityStatus{}, err
	}
	return ports.CapacityStatus{State: state, UsedMB: used, QuotaGB: g.quota, SoftPercent: g.config.SoftPercent, HardPercent: g.config.HardPercent}, nil
}

func (g *capacityGuard) status(ctx context.Context, instance domain.InstanceID) (string, float64, error) {
	if g == nil || g.usage == nil || g.quota <= 0 {
		return "disabled", 0, nil
	}
	used, err := g.usage.DiskUsageMB(instance)
	if err != nil {
		return "unknown", 0, err
	}
	if g.config.HardPercent > 0 && used >= g.quota*1024*g.config.HardPercent/100 {
		return "hard", used, nil
	}
	if g.config.SoftPercent > 0 && used >= g.quota*1024*g.config.SoftPercent/100 {
		return "soft", used, nil
	}
	return "ok", used, nil
}
