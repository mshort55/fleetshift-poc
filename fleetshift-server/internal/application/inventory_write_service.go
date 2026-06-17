package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// InventoryWriteService implements [domain.InventoryWriter] as an
// application-layer service. It handles addon-to-platform inventory
// writes: upserts, deletes, and full resyncs.
type InventoryWriteService struct {
	store domain.Store
}

// NewInventoryWriteService creates an InventoryWriteService.
func NewInventoryWriteService(store domain.Store) *InventoryWriteService {
	return &InventoryWriteService{store: store}
}

// ApplyDelta upserts and deletes inventory items in a single transaction.
func (s *InventoryWriteService) ApplyDelta(ctx context.Context, targetID domain.TargetID, upserts []domain.InventoryItem, deletedIDs []domain.InventoryItemID) error {
	tx, err := s.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	repo := tx.Inventory()

	for _, item := range upserts {
		if err := repo.CreateOrUpdate(ctx, item); err != nil {
			return fmt.Errorf("upsert %s: %w", item.ID(), err)
		}
	}

	for _, id := range deletedIDs {
		if err := repo.Delete(ctx, id); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("delete %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// Resync atomically replaces all items for a target+type.
func (s *InventoryWriteService) Resync(ctx context.Context, targetID domain.TargetID, inventoryType domain.InventoryType, items []domain.InventoryItem) error {
	tx, err := s.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := tx.Inventory().ReplaceByTargetAndType(ctx, targetID, inventoryType, items); err != nil {
		return fmt.Errorf("replace by target and type: %w", err)
	}

	return tx.Commit()
}
