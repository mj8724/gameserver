package application

import (
	"context"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

type memoryStateStore struct {
	state domain.InstanceState
}

func (s memoryStateStore) Load(_ context.Context, _ domain.InstanceID) (domain.InstanceState, error) {
	return s.state, nil
}

func (memoryStateStore) Save(context.Context, domain.InstanceState) error { return nil }

func TestInstanceStateUsesInjectedStore(t *testing.T) {
	want := domain.InstanceState{ID: domain.InstanceID("pz_test"), Name: "test"}
	service, err := New(memoryStateStore{state: want})
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.InstanceState(context.Background(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Name != want.Name {
		t.Fatalf("InstanceState() = %#v, want %#v", got, want)
	}
}
