package ports

import (
	"context"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

type compileStateStore struct{}

func (compileStateStore) Load(context.Context, domain.InstanceID) (domain.InstanceState, error) {
	return domain.InstanceState{}, nil
}
func (compileStateStore) Save(context.Context, domain.InstanceState) error { return nil }

func TestStateStoreContract(t *testing.T) {
	var store StateStore = compileStateStore{}
	if _, err := store.Load(context.Background(), domain.InstanceID("pz_test")); err != nil {
		t.Fatal(err)
	}
}
