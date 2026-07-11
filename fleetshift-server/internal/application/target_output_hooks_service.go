package application

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// TargetRuntimeHooks receives target lifecycle signals for a
// server-hosted runtime that reacts to target readiness and draining.
//
// NotifyTargetReady is a non-failing wake-up hint: if the runtime cannot
// act on it, implementations log internally and return.
//
// OnTargetDraining is a failing barrier used after the target has been
// marked draining: implementations must stop local runtime work for that
// target and return any failure so callers can retry. It does not change
// target lifecycle state; durability of draining is owned by the caller.
type TargetRuntimeHooks interface {
	// NotifyTargetReady is called after target registration commits. It
	// is a wake-up hint only.
	NotifyTargetReady(ctx context.Context, target domain.TargetInfo)

	// OnTargetDraining is called after the target has been marked
	// draining and before platform cleanup that must not race still-
	// running local work. The target row still exists when this fires.
	// A failure to stop local runtime work for the target must be
	// returned.
	OnTargetDraining(ctx context.Context, target domain.TargetInfo) error
}

// NoOpTargetRuntimeHooks is a [TargetRuntimeHooks] that does nothing.
// It is the default when no runtime hooks are configured.
type NoOpTargetRuntimeHooks struct{}

func (NoOpTargetRuntimeHooks) NotifyTargetReady(context.Context, domain.TargetInfo) {}
func (NoOpTargetRuntimeHooks) OnTargetDraining(context.Context, domain.TargetInfo) error {
	return nil
}

// TargetIndexedInventoryCleaner performs platform-owned cleanup of
// source-owned indexed inventory for a single target when the target
// is terminating. It is not a callback into a live addon process,
// external agent, or local runtime -- concrete implementations delete
// platform-owned persisted inventory state directly; they do not ask
// the source to delete it.
type TargetIndexedInventoryCleaner interface {
	CleanupIndexedInventory(ctx context.Context, target domain.TargetInfo) error
}

// TargetOutputHookService implements [domain.TargetOutputHooks]
// for delivery-produced target outputs. It owns the application-level
// composition of [TargetRuntimeHooks] and
// [TargetIndexedInventoryCleaner] registrations, keeping that registry
// out of the orchestration workflow spec.
type TargetOutputHookService struct {
	store    domain.Store
	runtime  TargetRuntimeHooks
	cleaners map[domain.TargetType]TargetIndexedInventoryCleaner
}

// TargetOutputHookServiceOption configures a
// [TargetOutputHookService].
type TargetOutputHookServiceOption func(*TargetOutputHookService)

// WithTargetRuntimeHooks sets the hooks that receive target ready
// hints and draining barriers.
func WithTargetRuntimeHooks(hooks TargetRuntimeHooks) TargetOutputHookServiceOption {
	return func(s *TargetOutputHookService) {
		if hooks != nil {
			s.runtime = hooks
		}
	}
}

// WithTargetIndexedInventoryCleaner registers cleaner for targetType.
// Passing a nil cleaner deliberately declares targetType as an indexed
// target type without wiring its required cleanup implementation; this
// makes [TargetOutputHookService.BeforeTargetDeleted] fail
// instead of silently deleting the target row.
func WithTargetIndexedInventoryCleaner(targetType domain.TargetType, cleaner TargetIndexedInventoryCleaner) TargetOutputHookServiceOption {
	return func(s *TargetOutputHookService) {
		s.cleaners[targetType] = cleaner
	}
}

// WithTargetIndexedInventoryCleaners registers cleaners by target type.
// The map is copied so later caller-side mutations do not alter service
// behavior.
func WithTargetIndexedInventoryCleaners(cleaners map[domain.TargetType]TargetIndexedInventoryCleaner) TargetOutputHookServiceOption {
	return func(s *TargetOutputHookService) {
		maps.Copy(s.cleaners, cleaners)
	}
}

// NewTargetOutputHookService creates a target output hook service.
// store is required for the durable ready-to-draining compare-and-swap on delete.
func NewTargetOutputHookService(store domain.Store, opts ...TargetOutputHookServiceOption) *TargetOutputHookService {
	s := &TargetOutputHookService{
		store:    store,
		runtime:  NoOpTargetRuntimeHooks{},
		cleaners: make(map[domain.TargetType]TargetIndexedInventoryCleaner),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// AfterTargetRegistered sends a non-failing target-ready hint.
func (s *TargetOutputHookService) AfterTargetRegistered(ctx context.Context, target domain.TargetInfo) {
	s.runtime.NotifyTargetReady(ctx, target)
}

// BeforeTargetDeleted marks the target draining (compare-and-swap), asks the runtime
// to stop local work for it (failing on errors), then runs the
// platform-owned indexed inventory cleaner registered for the target
// type. A target type with no cleaner registration is treated as having
// no indexed inventory. A target type registered with a nil cleaner is
// a wiring error and fails cleanup.
func (s *TargetOutputHookService) BeforeTargetDeleted(ctx context.Context, target domain.TargetInfo) error {
	if err := s.markDraining(ctx, target.ID()); err != nil {
		return fmt.Errorf("mark target %s draining: %w", target.ID(), err)
	}

	if err := s.runtime.OnTargetDraining(ctx, target); err != nil {
		return fmt.Errorf("on target draining %s: %w", target.ID(), err)
	}

	cleaner, declared := s.cleaners[target.Type()]
	if !declared {
		return nil
	}
	if cleaner == nil {
		return fmt.Errorf(
			"%w: target type %q declares itself an indexed target type but has no registered TargetIndexedInventoryCleaner",
			domain.ErrInvalidArgument, target.Type())
	}
	if err := cleaner.CleanupIndexedInventory(ctx, target); err != nil {
		return fmt.Errorf("cleanup indexed inventory for target %s: %w", target.ID(), err)
	}
	return nil
}

// markDraining compare-and-swaps ready to draining without rewriting other
// target columns. Already-draining and already-deleted targets are
// treated as success so cleanup retries stay idempotent.
func (s *TargetOutputHookService) markDraining(ctx context.Context, id domain.TargetID) error {
	tx, err := s.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	err = tx.Targets().TransitionState(ctx, id, domain.TargetStateReady, domain.TargetStateDraining)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
