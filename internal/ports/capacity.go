package ports

import (
	"context"

	"github.com/mj8724/gameserver/internal/domain"
)

// CapacityChecker is the M3.5 capacity policy seam: entry and pre-write
// double-checks plus a status projection. It never deletes data.
type CapacityChecker interface {
	CheckBeforeWrite(ctx context.Context, instance domain.InstanceID) error
	Status(ctx context.Context, instance domain.InstanceID) (CapacityStatus, error)
}

// CapacityStatus is the additive status projection for the capacity policy.
type CapacityStatus struct {
	State       string  `json:"state"` // ok | soft | hard | disabled | unknown
	UsedMB      float64 `json:"used_mb"`
	QuotaGB     float64 `json:"quota_gb,omitempty"`
	HardPercent float64 `json:"hard_percent,omitempty"`
	SoftPercent float64 `json:"soft_percent,omitempty"`
}
