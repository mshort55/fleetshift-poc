package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
)

func newTargetInventoryCleanupStore(t *testing.T) domain.Store {
	t.Helper()
	return &sqlite.Store{DB: sqlite.OpenTestDB(t)}
}

// seedManagedPlusInventoryType registers an extension resource type
// that supports both management and inventory reporting.
func seedManagedPlusInventoryType(t *testing.T, store domain.Store, rt domain.ResourceType) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	relation := domain.NewRegisteredSelfTarget(domain.TargetID("addon-widget"), domain.ManifestType("api.test.widget"))
	def := domain.NewExtensionResourceType(rt, "v1", "widgets", time.Now(),
		domain.WithManagement(relation, domain.Signature{
			Signer:         domain.FederatedIdentity{Subject: "addon-svc", Issuer: "https://issuer.test"},
			ContentHash:    []byte("hash"),
			SignatureBytes: []byte("sig"),
		}),
		domain.WithInventory())
	if err := tx.ExtensionResources().CreateType(ctx, def); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_DeletesOwnedSubtree(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)
	seedInventoryType(t, store)

	ctx := context.Background()
	now := time.Now()
	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	name := domain.ResourceName("targets/prod/apiResources/core~v1~pods/objects/pod1")
	sibling := domain.ResourceName("targets/prod-old/apiResources/core~v1~pods/objects/pod2")
	if err := tx.ExtensionResources().ReplaceInventory(ctx, []domain.InventoryReplacement{
		{ResourceType: inventoryReportTestType, Name: name, CandidateUID: domain.NewExtensionResourceUID(), ObservedAt: now, ReceivedAt: now},
		{ResourceType: inventoryReportTestType, Name: sibling, CandidateUID: domain.NewExtensionResourceUID(), ObservedAt: now, ReceivedAt: now},
	}); err != nil {
		t.Fatalf("seed ReplaceInventory: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	svc := application.NewTargetInventoryCleanupService(store)
	if err := svc.DeleteOwnedInventorySubtree(ctx, domain.AddonID("kind.fleetshift.io"), domain.InventorySubtreeRef{
		ResourceType: inventoryReportTestType,
		Parent:       "targets/prod",
	}); err != nil {
		t.Fatalf("DeleteOwnedInventorySubtree: %v", err)
	}

	readTx, err := store.BeginReadOnly(ctx)
	if err != nil {
		t.Fatalf("begin read tx: %v", err)
	}
	defer readTx.Rollback()
	if _, err := readTx.ExtensionResources().Get(ctx, inventoryReportTestType.FullName(name)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected %s to be deleted, got err=%v", name, err)
	}
	if _, err := readTx.ExtensionResources().Get(ctx, inventoryReportTestType.FullName(sibling)); err != nil {
		t.Fatalf("expected sibling %s to survive (segment-boundary safety), got err=%v", sibling, err)
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_RejectsWrongOwner(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)
	seedInventoryType(t, store)

	err := application.NewTargetInventoryCleanupService(store).DeleteOwnedInventorySubtree(
		context.Background(), domain.AddonID("someone-else.fleetshift.io"), domain.InventorySubtreeRef{
			ResourceType: inventoryReportTestType,
			Parent:       "targets/prod",
		})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("DeleteOwnedInventorySubtree error = %v, want ErrInvalidArgument", err)
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_RejectsMissingResourceType(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)

	err := application.NewTargetInventoryCleanupService(store).DeleteOwnedInventorySubtree(
		context.Background(), domain.AddonID("kind.fleetshift.io"), domain.InventorySubtreeRef{
			ResourceType: "kind.fleetshift.io/DoesNotExist",
			Parent:       "targets/prod",
		})
	if err == nil {
		t.Fatal("expected error for nonexistent resource type")
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_RejectsTypeWithoutInventoryMetadata(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)
	const managedOnly domain.ResourceType = "kind.fleetshift.io/Widget"
	seedManagedOnlyType(t, store, managedOnly)

	err := application.NewTargetInventoryCleanupService(store).DeleteOwnedInventorySubtree(
		context.Background(), domain.AddonID("kind.fleetshift.io"), domain.InventorySubtreeRef{
			ResourceType: managedOnly,
			Parent:       "targets/prod",
		})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("DeleteOwnedInventorySubtree error = %v, want ErrInvalidArgument", err)
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_RejectsManagedPlusInventoryType(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)
	const shared domain.ResourceType = "kind.fleetshift.io/Shared"
	seedManagedPlusInventoryType(t, store, shared)

	err := application.NewTargetInventoryCleanupService(store).DeleteOwnedInventorySubtree(
		context.Background(), domain.AddonID("kind.fleetshift.io"), domain.InventorySubtreeRef{
			ResourceType: shared,
			Parent:       "targets/prod",
		})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("DeleteOwnedInventorySubtree error = %v, want ErrInvalidArgument", err)
	}
}

func TestTargetInventoryCleanupService_DeleteOwnedInventorySubtree_NoMatchIsNoop(t *testing.T) {
	store := newTargetInventoryCleanupStore(t)
	seedInventoryType(t, store)

	err := application.NewTargetInventoryCleanupService(store).DeleteOwnedInventorySubtree(
		context.Background(), domain.AddonID("kind.fleetshift.io"), domain.InventorySubtreeRef{
			ResourceType: inventoryReportTestType,
			Parent:       "targets/never-existed",
		})
	if err != nil {
		t.Fatalf("DeleteOwnedInventorySubtree on empty subtree: %v", err)
	}
}
