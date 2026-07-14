package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
)

type recordingTargetRuntimeHooks struct {
	ready    []domain.TargetInfo
	drained  []domain.TargetInfo
	drainErr error
	order    *[]string
}

func (n *recordingTargetRuntimeHooks) NotifyTargetReady(_ context.Context, target domain.TargetInfo) {
	n.ready = append(n.ready, target)
	if n.order != nil {
		*n.order = append(*n.order, "ready:"+string(target.ID()))
	}
}

func (n *recordingTargetRuntimeHooks) OnTargetDraining(_ context.Context, target domain.TargetInfo) error {
	n.drained = append(n.drained, target)
	if n.order != nil {
		*n.order = append(*n.order, "draining:"+string(target.ID()))
	}
	return n.drainErr
}

func testOutputHookTarget(targetType domain.TargetType) domain.TargetInfo {
	return domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:    "target-1",
		Name:  "target-1",
		Type:  targetType,
		State: domain.TargetStateReady,
	})
}

func testOutputHookStore(t *testing.T) domain.Store {
	t.Helper()
	return &sqlite.Store{DB: sqlite.OpenTestDB(t)}
}

func seedOutputHookTarget(t *testing.T, store domain.Store, target domain.TargetInfo) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := tx.Targets().Create(ctx, target); err != nil {
		t.Fatalf("Create target: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestTargetOutputHookService_AfterTargetRegistered_NotifiesRuntime(t *testing.T) {
	notifier := &recordingTargetRuntimeHooks{}
	svc := application.NewTargetOutputHookService(
		testOutputHookStore(t),
		application.WithTargetRuntimeHooks(notifier),
	)

	svc.AfterTargetRegistered(context.Background(), testOutputHookTarget("vm"))

	if len(notifier.ready) != 1 || notifier.ready[0].ID() != "target-1" {
		t.Fatalf("ready calls = %v, want one call for target-1", notifier.ready)
	}
	if len(notifier.drained) != 0 {
		t.Fatalf("OnTargetDraining calls = %v, want none", notifier.drained)
	}
}

func TestTargetOutputHookService_DefaultNoOpNotifier(t *testing.T) {
	// Exercises NoOpTargetRuntimeHooks ready + OnTargetDraining paths.
	store := testOutputHookStore(t)
	svc := application.NewTargetOutputHookService(store)
	svc.AfterTargetRegistered(context.Background(), testOutputHookTarget("vm"))
	if err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("vm")); err != nil {
		t.Fatalf("BeforeTargetDeleted with default notifier: %v", err)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_DrainsRuntime(t *testing.T) {
	order := []string{}
	store := testOutputHookStore(t)
	target := testOutputHookTarget("vm")
	seedOutputHookTarget(t, store, target)

	notifier := &recordingTargetRuntimeHooks{order: &order}
	svc := application.NewTargetOutputHookService(
		store,
		application.WithTargetRuntimeHooks(notifier),
	)

	err := svc.BeforeTargetDeleted(context.Background(), target)
	if err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}

	tx, err := store.BeginReadOnly(context.Background())
	if err != nil {
		t.Fatalf("begin read: %v", err)
	}
	defer tx.Rollback()
	got, err := tx.Targets().Get(context.Background(), target.ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State() != domain.TargetStateDraining {
		t.Fatalf("State = %q, want %q", got.State(), domain.TargetStateDraining)
	}

	if len(notifier.drained) != 1 || notifier.drained[0].ID() != "target-1" {
		t.Fatalf("OnTargetDraining calls = %v, want one call for target-1", notifier.drained)
	}
	want := []string{"draining:target-1"}
	if len(order) != len(want) || order[0] != want[0] {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_OnTargetDrainingError(t *testing.T) {
	store := testOutputHookStore(t)
	target := testOutputHookTarget("vm")
	seedOutputHookTarget(t, store, target)

	drainErr := errors.New("stop timed out")
	notifier := &recordingTargetRuntimeHooks{drainErr: drainErr}
	svc := application.NewTargetOutputHookService(
		store,
		application.WithTargetRuntimeHooks(notifier),
	)

	err := svc.BeforeTargetDeleted(context.Background(), target)
	if !errors.Is(err, drainErr) {
		t.Fatalf("BeforeTargetDeleted error = %v, want OnTargetDraining error", err)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_IdempotentWhenAlreadyDraining(t *testing.T) {
	store := testOutputHookStore(t)
	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID: "target-1", Name: "target-1", Type: "vm", State: domain.TargetStateDraining,
	})
	seedOutputHookTarget(t, store, target)

	svc := application.NewTargetOutputHookService(store)
	if err := svc.BeforeTargetDeleted(context.Background(), target); err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_DoesNotOverwriteProperties(t *testing.T) {
	store := testOutputHookStore(t)
	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:         "target-1",
		Name:       "target-1",
		Type:       "vm",
		State:      domain.TargetStateReady,
		Properties: map[string]string{"fleetshift.inventory.mode": "local", "token": "secret"},
	})
	seedOutputHookTarget(t, store, target)

	// Simulate a concurrent property update after the drain snapshot.
	ctx := context.Background()
	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	updated := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID: target.ID(), Type: target.Type(), Name: target.Name(), State: domain.TargetStateReady,
		Properties: map[string]string{"fleetshift.inventory.mode": "external", "token": "rotated"},
	})
	if err := tx.Targets().CreateOrUpdate(ctx, updated); err != nil {
		t.Fatalf("CreateOrUpdate: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Cleanup still holds the stale snapshot (old properties).
	stale := testOutputHookTarget("vm")
	svc := application.NewTargetOutputHookService(store)
	if err := svc.BeforeTargetDeleted(context.Background(), stale); err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}

	readTx, err := store.BeginReadOnly(ctx)
	if err != nil {
		t.Fatalf("begin read: %v", err)
	}
	defer readTx.Rollback()
	got, err := readTx.Targets().Get(ctx, target.ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State() != domain.TargetStateDraining {
		t.Fatalf("State = %q, want draining", got.State())
	}
	if got.Properties()["fleetshift.inventory.mode"] != "external" || got.Properties()["token"] != "rotated" {
		t.Fatalf("Properties = %v, want concurrent updates preserved", got.Properties())
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_MissingTargetContinues(t *testing.T) {
	store := testOutputHookStore(t)
	order := []string{}
	notifier := &recordingTargetRuntimeHooks{order: &order}
	svc := application.NewTargetOutputHookService(
		store,
		application.WithTargetRuntimeHooks(notifier),
	)

	// Target row already gone (e.g. retry after partial delete).
	if err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("vm")); err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}
	want := []string{"draining:target-1"}
	if len(order) != len(want) || order[0] != want[0] {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_IllegalStateBlocks(t *testing.T) {
	store := testOutputHookStore(t)
	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID: "target-1", Name: "target-1", Type: "vm", State: domain.TargetStateInitializing,
	})
	seedOutputHookTarget(t, store, target)

	notifier := &recordingTargetRuntimeHooks{}
	svc := application.NewTargetOutputHookService(
		store,
		application.WithTargetRuntimeHooks(notifier),
	)

	err := svc.BeforeTargetDeleted(context.Background(), target)
	if !errors.Is(err, domain.ErrIllegalStateTransition) {
		t.Fatalf("BeforeTargetDeleted error = %v, want ErrIllegalStateTransition", err)
	}
	if len(notifier.drained) != 0 {
		t.Fatalf("OnTargetDraining calls = %v, want none when drain compare-and-swap fails", notifier.drained)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_BeginFailureBlocks(t *testing.T) {
	beginErr := errors.New("begin failed")
	notifier := &recordingTargetRuntimeHooks{}
	svc := application.NewTargetOutputHookService(
		&beginFailStore{err: beginErr},
		application.WithTargetRuntimeHooks(notifier),
	)

	err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("vm"))
	if !errors.Is(err, beginErr) {
		t.Fatalf("BeforeTargetDeleted error = %v, want begin failure", err)
	}
	if len(notifier.drained) != 0 {
		t.Fatalf("OnTargetDraining must not run after begin failure")
	}
}

type beginFailStore struct{ err error }

func (s *beginFailStore) Begin(context.Context) (domain.Tx, error) { return nil, s.err }
func (s *beginFailStore) BeginReadOnly(context.Context) (domain.Tx, error) {
	return nil, errors.New("BeginReadOnly unused")
}
