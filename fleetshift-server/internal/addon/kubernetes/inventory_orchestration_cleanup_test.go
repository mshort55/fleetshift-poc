package kubernetes_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/memworkflow"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
)

// TestOrchestrationCleanupTargetIndexedInventory_RealKubernetesCleaner
// runs the orchestration CleanupTargetIndexedInventory activity with a
// real TargetOutputHookService + KubernetesTargetIndexedInventoryCleaner
// against extension-resource inventory (not delivery-owned InventoryItem
// rows), including the prod vs prod-old segment-boundary case.
func TestOrchestrationCleanupTargetIndexedInventory_RealKubernetesCleaner(t *testing.T) {
	store := &sqlite.Store{DB: sqlite.OpenTestDB(t)}
	seedKubernetesObjectType(t, store)

	ctx := context.Background()
	now := time.Now()
	reports := application.NewInventoryReportService(store)
	subtrees := application.NewTargetInventoryCleanupService(store)

	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	podProd, err := kubernetes.ObjectResourceName(kubernetes.KubernetesObjectIdentity{
		TargetID: "prod", GVR: podsGVR, Namespace: "default", Name: "web", UID: "uid-prod",
	})
	if err != nil {
		t.Fatalf("ObjectResourceName prod: %v", err)
	}
	podSibling, err := kubernetes.ObjectResourceName(kubernetes.KubernetesObjectIdentity{
		TargetID: "prod-old", GVR: podsGVR, Namespace: "default", Name: "web", UID: "uid-old",
	})
	if err != nil {
		t.Fatalf("ObjectResourceName sibling: %v", err)
	}

	if err := reports.ReplaceCollection(ctx, application.InventoryCollectionReplacementInput{
		ResourceType: kubernetes.ObjectResourceType,
		Collection:   podProd.Collection(),
		Reports:      []application.InventoryReplacementInput{{Name: &podProd, ObservedAt: now}},
	}); err != nil {
		t.Fatalf("seed prod: %v", err)
	}
	if err := reports.ReplaceCollection(ctx, application.InventoryCollectionReplacementInput{
		ResourceType: kubernetes.ObjectResourceType,
		Collection:   podSibling.Collection(),
		Reports:      []application.InventoryReplacementInput{{Name: &podSibling, ObservedAt: now}},
	}); err != nil {
		t.Fatalf("seed sibling: %v", err)
	}

	hooks := application.NewTargetOutputHookService(
		application.WithTargetIndexedInventoryCleaner(
			kubernetes.TargetType,
			kubernetes.NewKubernetesTargetIndexedInventoryCleaner(subtrees),
		),
	)

	reg := &memworkflow.Registry{}
	wf := domain.NewOrchestrationWorkflowSpec(
		store,
		noopDeliveryAgent{},
		domain.StrategyFactory{Store: store},
		reg,
		domain.WithTargetOutputHooks(hooks),
	)

	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:   "prod",
		Name: "prod",
		Type: kubernetes.TargetType,
	})
	if _, err := wf.CleanupTargetIndexedInventory().Run(ctx, domain.TerminatingTargets{
		Targets: []domain.TargetInfo{target},
	}); err != nil {
		t.Fatalf("CleanupTargetIndexedInventory: %v", err)
	}

	readTx, err := store.BeginReadOnly(ctx)
	if err != nil {
		t.Fatalf("begin read: %v", err)
	}
	defer readTx.Rollback()

	if _, err := readTx.ExtensionResources().Get(ctx, kubernetes.ObjectResourceType.FullName(podProd)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected prod object deleted, got err=%v", err)
	}
	if _, err := readTx.ExtensionResources().Get(ctx, kubernetes.ObjectResourceType.FullName(podSibling)); err != nil {
		t.Fatalf("expected prod-old sibling to survive, got err=%v", err)
	}
}

// noopDeliveryAgent satisfies domain.DeliveryAgent for orchestration
// specs that only exercise cleanup activities.
type noopDeliveryAgent struct{}

func (noopDeliveryAgent) Deliver(context.Context, domain.TargetInfo, domain.DeliveryID, []domain.Manifest, domain.DeliveryAuth, *domain.Attestation, domain.Generation) error {
	return nil
}

func (noopDeliveryAgent) Remove(context.Context, domain.TargetInfo, domain.DeliveryID, []domain.Manifest, domain.DeliveryAuth, *domain.Attestation, domain.Generation) error {
	return nil
}
