package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

type recordingTargetRuntimeNotifier struct {
	ready       []domain.TargetInfo
	terminating []domain.TargetInfo
	order       *[]string
}

func (n *recordingTargetRuntimeNotifier) NotifyTargetReady(_ context.Context, target domain.TargetInfo) {
	n.ready = append(n.ready, target)
	if n.order != nil {
		*n.order = append(*n.order, "ready:"+string(target.ID()))
	}
}

func (n *recordingTargetRuntimeNotifier) NotifyTargetTerminating(_ context.Context, target domain.TargetInfo) {
	n.terminating = append(n.terminating, target)
	if n.order != nil {
		*n.order = append(*n.order, "terminating:"+string(target.ID()))
	}
}

type recordingIndexedInventoryCleaner struct {
	calls []domain.TargetInfo
	err   error
	order *[]string
}

func (c *recordingIndexedInventoryCleaner) CleanupIndexedInventory(_ context.Context, target domain.TargetInfo) error {
	c.calls = append(c.calls, target)
	if c.order != nil {
		*c.order = append(*c.order, "cleaner:"+string(target.ID()))
	}
	return c.err
}

func testOutputHookTarget(targetType domain.TargetType) domain.TargetInfo {
	return domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:   "target-1",
		Name: "target-1",
		Type: targetType,
	})
}

func TestTargetOutputHookService_AfterTargetRegistered_NotifiesRuntime(t *testing.T) {
	notifier := &recordingTargetRuntimeNotifier{}
	svc := application.NewTargetOutputHookService(
		application.WithTargetRuntimeNotifier(notifier),
	)

	svc.AfterTargetRegistered(context.Background(), testOutputHookTarget("kubernetes"))

	if len(notifier.ready) != 1 || notifier.ready[0].ID() != "target-1" {
		t.Fatalf("ready calls = %v, want one call for target-1", notifier.ready)
	}
	if len(notifier.terminating) != 0 {
		t.Fatalf("terminating calls = %v, want none", notifier.terminating)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_NotifiesRuntimeBeforeCleaner(t *testing.T) {
	order := []string{}
	notifier := &recordingTargetRuntimeNotifier{order: &order}
	cleaner := &recordingIndexedInventoryCleaner{order: &order}
	svc := application.NewTargetOutputHookService(
		application.WithTargetRuntimeNotifier(notifier),
		application.WithTargetIndexedInventoryCleaner("kubernetes", cleaner),
	)

	err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("kubernetes"))
	if err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}

	if len(notifier.terminating) != 1 || notifier.terminating[0].ID() != "target-1" {
		t.Fatalf("terminating calls = %v, want one call for target-1", notifier.terminating)
	}
	if len(cleaner.calls) != 1 || cleaner.calls[0].ID() != "target-1" {
		t.Fatalf("cleaner calls = %v, want one call for target-1", cleaner.calls)
	}
	want := []string{"terminating:target-1", "cleaner:target-1"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_UndeclaredTypeSkipsCleaner(t *testing.T) {
	notifier := &recordingTargetRuntimeNotifier{}
	cleaner := &recordingIndexedInventoryCleaner{}
	svc := application.NewTargetOutputHookService(
		application.WithTargetRuntimeNotifier(notifier),
		application.WithTargetIndexedInventoryCleaner("kubernetes", cleaner),
	)

	err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("unmanaged"))
	if err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}

	if len(notifier.terminating) != 1 {
		t.Fatalf("terminating calls = %v, want one call", notifier.terminating)
	}
	if len(cleaner.calls) != 0 {
		t.Fatalf("cleaner calls = %v, want none", cleaner.calls)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_NilCleanerFails(t *testing.T) {
	notifier := &recordingTargetRuntimeNotifier{}
	svc := application.NewTargetOutputHookService(
		application.WithTargetRuntimeNotifier(notifier),
		application.WithTargetIndexedInventoryCleaner("kubernetes", nil),
	)

	err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("kubernetes"))
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("BeforeTargetDeleted error = %v, want ErrInvalidArgument", err)
	}
	if len(notifier.terminating) != 1 {
		t.Fatalf("terminating calls = %v, want one call", notifier.terminating)
	}
}

func TestTargetOutputHookService_BeforeTargetDeleted_CleanerErrorFails(t *testing.T) {
	cleanerErr := errors.New("cleanup failed")
	svc := application.NewTargetOutputHookService(
		application.WithTargetIndexedInventoryCleaner("kubernetes", &recordingIndexedInventoryCleaner{err: cleanerErr}),
	)

	err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("kubernetes"))
	if !errors.Is(err, cleanerErr) {
		t.Fatalf("BeforeTargetDeleted error = %v, want wrapped cleaner error", err)
	}
}

func TestTargetOutputHookService_WithTargetIndexedInventoryCleaners_RegistersEveryEntry(t *testing.T) {
	kubernetesCleaner := &recordingIndexedInventoryCleaner{}
	gcphcpCleaner := &recordingIndexedInventoryCleaner{}
	svc := application.NewTargetOutputHookService(
		application.WithTargetIndexedInventoryCleaners(map[domain.TargetType]application.TargetIndexedInventoryCleaner{
			"kubernetes": kubernetesCleaner,
			"gcphcp":     gcphcpCleaner,
		}),
	)

	if err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("kubernetes")); err != nil {
		t.Fatalf("BeforeTargetDeleted(kubernetes): %v", err)
	}
	if err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("gcphcp")); err != nil {
		t.Fatalf("BeforeTargetDeleted(gcphcp): %v", err)
	}

	if len(kubernetesCleaner.calls) != 1 {
		t.Errorf("kubernetes cleaner calls = %d, want 1", len(kubernetesCleaner.calls))
	}
	if len(gcphcpCleaner.calls) != 1 {
		t.Errorf("gcphcp cleaner calls = %d, want 1", len(gcphcpCleaner.calls))
	}
}

func TestTargetOutputHookService_WithTargetIndexedInventoryCleaners_CopiesMap(t *testing.T) {
	cleaner := &recordingIndexedInventoryCleaner{}
	cleaners := map[domain.TargetType]application.TargetIndexedInventoryCleaner{"kubernetes": cleaner}
	svc := application.NewTargetOutputHookService(
		application.WithTargetIndexedInventoryCleaners(cleaners),
	)

	// Mutate the caller's map after construction; per this option's doc,
	// the service must not be affected because it copies the map.
	cleaners["kubernetes"] = nil

	if err := svc.BeforeTargetDeleted(context.Background(), testOutputHookTarget("kubernetes")); err != nil {
		t.Fatalf("BeforeTargetDeleted: %v", err)
	}
	if len(cleaner.calls) != 1 {
		t.Fatalf("cleaner calls = %d, want 1 (service must copy the map, not alias it)", len(cleaner.calls))
	}
}
