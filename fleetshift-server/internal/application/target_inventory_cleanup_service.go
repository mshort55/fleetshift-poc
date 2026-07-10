package application

import (
	"context"
	"fmt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// TargetInventoryCleanupService exposes owner-validated,
// resource-type-scoped subtree cleanup for target-scoped indexed
// inventory. Concrete [TargetIndexedInventoryCleaner] implementations
// (e.g. a per-target-type addon cleaner) depend on this service
// instead of calling
// [domain.ExtensionResourceRepository.DeleteInventorySubtree]
// directly, so cleanup authority is always checked rather than
// assumed by the caller.
type TargetInventoryCleanupService struct {
	store domain.Store
}

// NewTargetInventoryCleanupService creates a service backed by store.
func NewTargetInventoryCleanupService(store domain.Store) *TargetInventoryCleanupService {
	return &TargetInventoryCleanupService{store: store}
}

// DeleteOwnedInventorySubtree deletes every extension resource under
// ref's subtree, after validating that ownerAddonID is authorized to
// perform destructive cleanup for ref.ResourceType. Required
// validation:
//
//   - ref.ResourceType exists;
//   - ref.ResourceType has inventory metadata;
//   - ref.ResourceType is owned by ownerAddonID, using the same
//     service-name ownership rule as [domain.InventoryResourceCapability]
//     (see [validateResourceTypeOwnership]);
//   - broad subtree cleanup is rejected for resource types that are
//     both managed and inventory-reporting, since resource-type
//     ownership alone is not enough to prove the subtree belongs
//     entirely to ownerAddonID's inventory reporting once a type is
//     shared with management.
//
// A subtree with no matching rows is a no-op, per
// [domain.ExtensionResourceRepository.DeleteInventorySubtree]'s doc.
// Ownership and metadata validation failures are always cleanup
// errors.
func (s *TargetInventoryCleanupService) DeleteOwnedInventorySubtree(ctx context.Context, ownerAddonID domain.AddonID, ref domain.InventorySubtreeRef) error {
	tx, err := s.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	typeDef, err := tx.ExtensionResources().GetType(ctx, ref.ResourceType)
	if err != nil {
		return fmt.Errorf("lookup resource type %q: %w", ref.ResourceType, err)
	}
	if typeDef.Inventory() == nil {
		return fmt.Errorf("%w: resource type %q has no inventory metadata", domain.ErrInvalidArgument, ref.ResourceType)
	}
	if typeDef.Management() != nil {
		return fmt.Errorf(
			"%w: resource type %q is both managed and inventory-reporting; broad subtree cleanup is not yet safe for shared types",
			domain.ErrInvalidArgument, ref.ResourceType)
	}
	if err := validateResourceTypeOwnership(ownerAddonID, ref.ResourceType); err != nil {
		return err
	}

	if err := tx.ExtensionResources().DeleteInventorySubtree(ctx, ref); err != nil {
		return fmt.Errorf("delete inventory subtree: %w", err)
	}
	return tx.Commit()
}
