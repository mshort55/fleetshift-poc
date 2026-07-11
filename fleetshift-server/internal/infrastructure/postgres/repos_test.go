package postgres_test

import (
	"context"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/authmethodrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/deliveryrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/deploymentrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/extensionresourcerepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/fulfillmentrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/inventoryrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/resourceidentityrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/storetest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/targetrepotest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/postgres"
)

func newStore(t *testing.T) *postgres.Store {
	t.Helper()
	db := postgres.OpenTestDB(t)
	return &postgres.Store{DB: db}
}

func newTxRepo[T any](t *testing.T, accessor func(domain.Tx) T) T {
	t.Helper()
	store := newStore(t)
	tx, err := store.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	t.Cleanup(func() { tx.Rollback() })
	return accessor(tx)
}

func TestTargetRepo(t *testing.T) {
	t.Parallel()
	targetrepotest.Run(t, func(t *testing.T) domain.TargetRepository {
		return newTxRepo(t, domain.Tx.Targets)
	})
}

func TestTargetRepo_TransitionState_EmptyStateTreatedAsReady(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()

	// Bypass Create's empty→ready normalization to exercise the compare-and-swap
	// readiness convention for legacy/empty stored state.
	_, err := store.DB.ExecContext(ctx,
		`INSERT INTO targets (id, type, name, state, labels, properties, inventory_item_id, accepted_manifest_types)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"t-empty", "kubernetes", "empty-state", "", `{}`, `{}`, "target:t-empty", `[]`,
	)
	if err != nil {
		t.Fatalf("raw insert: %v", err)
	}

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := tx.Targets().TransitionState(ctx, "t-empty", domain.TargetStateReady, domain.TargetStateDraining); err != nil {
		t.Fatalf("TransitionState from empty state: %v", err)
	}
	got, err := tx.Targets().Get(ctx, "t-empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State() != domain.TargetStateDraining {
		t.Fatalf("State = %q, want draining", got.State())
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestFulfillmentRepo(t *testing.T) {
	t.Parallel()
	fulfillmentrepotest.Run(t, func(t *testing.T) domain.FulfillmentRepository {
		return newTxRepo(t, domain.Tx.Fulfillments)
	})
}

func TestDeploymentRepo(t *testing.T) {
	t.Parallel()
	deploymentrepotest.Run(t, func(t *testing.T) domain.Tx {
		store := newStore(t)
		tx, err := store.Begin(context.Background())
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		return tx
	})
}

func TestDeliveryRepo(t *testing.T) {
	t.Parallel()
	deliveryrepotest.Run(t, func(t *testing.T) domain.DeliveryRepository {
		return newTxRepo(t, domain.Tx.Deliveries)
	})
}

func TestInventoryRepo(t *testing.T) {
	t.Parallel()
	inventoryrepotest.Run(t, func(t *testing.T) domain.InventoryRepository {
		return newTxRepo(t, domain.Tx.Inventory)
	})
}

func TestStore(t *testing.T) {
	t.Parallel()
	storetest.Run(t, func(t *testing.T) domain.Store {
		return newStore(t)
	})
}

func TestResourceIdentityRepo(t *testing.T) {
	t.Parallel()
	resourceidentityrepotest.Run(t, func(t *testing.T) domain.Tx {
		store := newStore(t)
		tx, err := store.Begin(context.Background())
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		return tx
	})
}

func TestExtensionResourceRepo(t *testing.T) {
	t.Parallel()
	extensionresourcerepotest.Run(t, func(t *testing.T) domain.Tx {
		store := newStore(t)
		tx, err := store.Begin(context.Background())
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		return tx
	})
}

func TestAuthMethodRepo(t *testing.T) {
	t.Parallel()
	authmethodrepotest.Run(t, func(t *testing.T) domain.AuthMethodRepository {
		db := postgres.OpenTestDB(t)
		return &postgres.AuthMethodRepo{DB: db}
	})
}
