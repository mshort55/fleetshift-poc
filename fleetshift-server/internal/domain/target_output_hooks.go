package domain

import "context"

// TargetOutputHooks handles target output work that must run around
// delivery-produced target registration and deletion, but does not
// belong to the fulfillment pipeline itself. Orchestration owns the
// timing of these calls; implementations own any integration-specific
// composition behind those operations.
type TargetOutputHooks interface {
	// AfterTargetRegistered is called after target registration commits.
	// It must be non-failing from orchestration's point of view.
	AfterTargetRegistered(ctx context.Context, target TargetInfo)

	// BeforeTargetDeleted runs before the target row is deleted.
	// Returning an error fails that caller.
	BeforeTargetDeleted(ctx context.Context, target TargetInfo) error
}

// NoOpTargetOutputHooks is a [TargetOutputHooks] that does nothing. It
// is the default when no target output integration is configured.
type NoOpTargetOutputHooks struct{}

func (NoOpTargetOutputHooks) AfterTargetRegistered(context.Context, TargetInfo) {}

func (NoOpTargetOutputHooks) BeforeTargetDeleted(context.Context, TargetInfo) error {
	return nil
}
