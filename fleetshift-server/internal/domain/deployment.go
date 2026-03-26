package domain

import "time"

// DeploymentState indicates the lifecycle state of a deployment.
type DeploymentState string

const (
	DeploymentStateCreating    DeploymentState = "creating"
	DeploymentStateActive      DeploymentState = "active"
	DeploymentStateDeleting    DeploymentState = "deleting"
	DeploymentStateFailed      DeploymentState = "failed"
	DeploymentStatePausedAuth  DeploymentState = "paused_auth"
)

// Generation is a monotonically increasing counter on a [Deployment].
// It is bumped on every mutation (create, invalidation, resume, delete)
// and compared against [ObservedGeneration] to detect pending work.
type Generation int64

// Deployment is the composition of manifest, placement, and rollout strategies.
type Deployment struct {
	ID                DeploymentID
	UID               string
	ManifestStrategy  ManifestStrategySpec
	PlacementStrategy PlacementStrategySpec
	RolloutStrategy   *RolloutStrategySpec // nil means immediate
	ResolvedTargets   []TargetID
	State             DeploymentState
	Auth              DeliveryAuth // authorization context; may change over time (e.g. token refresh)
	Generation         Generation // incremented on every mutation; starts at 1
	ObservedGeneration Generation // last generation fully reconciled by a workflow
	Reconciling        bool       // true while a reconciliation workflow is running
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Etag              string
}

// BumpGeneration increments the deployment's generation counter.
func (d *Deployment) BumpGeneration() {
	d.Generation++
}

// TryAcquireReconciliation attempts to acquire the reconciliation lock.
// Returns true if the lock was acquired (was not already held).
func (d *Deployment) TryAcquireReconciliation() bool {
	if d.Reconciling {
		return false
	}
	d.Reconciling = true
	return true
}

// CompleteReconciliation releases the reconciliation lock and advances
// [ObservedGeneration] to reconciledGen. If [Generation] has advanced
// past reconciledGen, the lock is re-acquired and needsRestart is true,
// indicating the caller should start a new reconciliation workflow.
func (d *Deployment) CompleteReconciliation(reconciledGen Generation) (needsRestart bool) {
	d.ObservedGeneration = reconciledGen
	if d.Generation > reconciledGen {
		d.Reconciling = true
		return true
	}
	d.Reconciling = false
	return false
}
