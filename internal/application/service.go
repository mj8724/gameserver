// Package application coordinates use cases through domain types and outbound ports.
package application

import (
	"context"
	"errors"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Service is the application layer's composition of required outbound ports.
type Service struct {
	states ports.StateStore
}

// New constructs a Service from injected outbound ports.
func New(states ports.StateStore) (*Service, error) {
	if states == nil {
		return nil, errors.New("state store is required")
	}
	return &Service{states: states}, nil
}

// InstanceState returns one instance's persisted state.
func (s *Service) InstanceState(ctx context.Context, id domain.InstanceID) (domain.InstanceState, error) {
	return s.states.Load(ctx, id)
}
