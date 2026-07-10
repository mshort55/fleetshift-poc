package application

import (
	"context"
	"fmt"
	"maps"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// TargetRuntimeNotifier receives non-failing lifecycle hints for a
// target as it becomes ready or begins terminating. Implementations
// must not return an error to orchestration: if a hint cannot be
// acted on (e.g. a local runtime cannot be woken or stopped),
// implementations should log internally and return. The target store
// plus periodic reconcile in the interested runtime remains the
// correctness path; these hints only reduce latency and reduce a
// cleanup race.
type TargetRuntimeNotifier interface {
	// NotifyTargetReady is called after target registration commits. It
	// is a wake-up hint only.
	NotifyTargetReady(ctx context.Context, target domain.TargetInfo)

	// NotifyTargetTerminating is called before target-owned indexed
	// inventory cleanup for this target. The target row still exists
	// when this fires. It is a bounded, best-effort stop hint only.
	NotifyTargetTerminating(ctx context.Context, target domain.TargetInfo)
}

// NoOpTargetRuntimeNotifier is a [TargetRuntimeNotifier] that does
// nothing. It is the default when no notifier is configured.
type NoOpTargetRuntimeNotifier struct{}

func (NoOpTargetRuntimeNotifier) NotifyTargetReady(context.Context, domain.TargetInfo)       {}
func (NoOpTargetRuntimeNotifier) NotifyTargetTerminating(context.Context, domain.TargetInfo) {}

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
// composition of non-failing target runtime notifiers and
// platform-owned target indexed inventory cleaners, keeping that
// registry out of the orchestration workflow spec.
type TargetOutputHookService struct {
	notifier TargetRuntimeNotifier
	cleaners map[domain.TargetType]TargetIndexedInventoryCleaner
}

// TargetOutputHookServiceOption configures a
// [TargetOutputHookService].
type TargetOutputHookServiceOption func(*TargetOutputHookService)

// WithTargetRuntimeNotifier sets the non-failing notifier that
// receives target ready and terminating hints.
func WithTargetRuntimeNotifier(notifier TargetRuntimeNotifier) TargetOutputHookServiceOption {
	return func(s *TargetOutputHookService) {
		if notifier != nil {
			s.notifier = notifier
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
func NewTargetOutputHookService(opts ...TargetOutputHookServiceOption) *TargetOutputHookService {
	s := &TargetOutputHookService{
		notifier: NoOpTargetRuntimeNotifier{},
		cleaners: make(map[domain.TargetType]TargetIndexedInventoryCleaner),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// AfterTargetRegistered sends a non-failing target-ready hint.
func (s *TargetOutputHookService) AfterTargetRegistered(ctx context.Context, target domain.TargetInfo) {
	s.notifier.NotifyTargetReady(ctx, target)
}

// BeforeTargetDeleted sends a non-failing terminating hint, then
// runs the platform-owned indexed inventory cleaner registered for the
// target type. A target type with no cleaner registration is treated as
// having no indexed inventory. A target type registered with a nil
// cleaner is a wiring error and fails cleanup.
func (s *TargetOutputHookService) BeforeTargetDeleted(ctx context.Context, target domain.TargetInfo) error {
	s.notifier.NotifyTargetTerminating(ctx, target)

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
