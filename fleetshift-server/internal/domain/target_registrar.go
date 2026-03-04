package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TargetRegistrar creates a target and its corresponding inventory
// item. Callers provide the repository instances, which may come from
// a transaction or be used directly. Transaction handling is external.
type TargetRegistrar struct {
	Targets   TargetRepository
	Inventory InventoryRepository
	Now       func() time.Time
}

func (r *TargetRegistrar) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Register validates and persists a target along with a corresponding
// inventory item (when an [InventoryRepository] is configured). The
// target's [InventoryItemID] is set to "target:<TargetID>".
func (r *TargetRegistrar) Register(ctx context.Context, target TargetInfo) error {
	if target.ID == "" {
		return fmt.Errorf("%w: target ID is required", ErrInvalidArgument)
	}
	if target.Name == "" {
		return fmt.Errorf("%w: target name is required", ErrInvalidArgument)
	}

	if r.Inventory != nil {
		invID := InventoryItemID("target:" + string(target.ID))
		target.InventoryItemID = invID

		props, _ := json.Marshal(target.Properties)
		now := r.now()
		if err := r.Inventory.Create(ctx, InventoryItem{
			ID:         invID,
			Type:       InventoryType(target.Type),
			Name:       target.Name,
			Properties: props,
			Labels:     target.Labels,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			return fmt.Errorf("create inventory item for target: %w", err)
		}
	}

	return r.Targets.Create(ctx, target)
}
