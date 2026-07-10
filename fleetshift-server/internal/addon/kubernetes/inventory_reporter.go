package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// InventoryReporter is the reusable inventory report boundary for the
// Kubernetes indexing core. Local in-process indexing and a future
// external agent both emit these commands; only the transport behind
// the reporter changes. Commands map onto InventoryReportService's
// delete/resync/replace surface and use the Kubernetes object identity
// helpers (ResourceType ObjectResourceType, names under
// {TargetCollectionID}/{target}/{APIResourceCollectionID}/{gvr}/{ObjectCollectionID}/{uid}).
type InventoryReporter interface {
	ApplyDelta(ctx context.Context, delta InventoryDeltaReport) error
	ReplaceCollection(ctx context.Context, snapshot InventoryCollectionSnapshot) error
	DeleteResources(ctx context.Context, refs []domain.InventoryResourceRef) error
	DeleteCollection(ctx context.Context, ref domain.InventoryCollectionRef) error
	DeleteSubtree(ctx context.Context, ref domain.InventorySubtreeRef) error
}

// InventoryObjectReport is the complete latest inventory state for one
// Kubernetes object. Name must already be an [ObjectResourceName]; the
// reporter does not re-derive it. Observation follows
// InventoryReportService replacement semantics: a nil or JSON-null
// value leaves the latest observation untouched; any other value
// replaces it.
type InventoryObjectReport struct {
	Name        domain.ResourceName
	Labels      map[string]string
	Observation *json.RawMessage
	Conditions  []domain.Condition
	ObservedAt  time.Time
}

// InventoryDeltaReport is one incremental flush of object upserts and
// deletes. Upserts are complete latest-state replacements (not
// field-level deltas). An empty report is a no-op: the reporter must
// not turn idle flushes into InventoryReportService heartbeats.
type InventoryDeltaReport struct {
	Upserts []InventoryObjectReport
	Deletes []domain.InventoryResourceRef
}

// InventoryCollectionSnapshot is the complete latest contents of one
// exact target+GVR collection ([ObjectCollectionName]). An empty
// Reports slice prunes the whole collection.
type InventoryCollectionSnapshot struct {
	Collection domain.CollectionName
	Reports    []InventoryObjectReport
}

// InventoryReportBackend is the application call surface the in-process
// [LocalInventoryReporter] adapts onto. It is declared here, at the
// point of use, rather than imported from the application layer, so
// this addon package never depends on internal/application. A thin
// bridge in server composition adapts
// *application.InventoryReportService (and owner-validated subtree
// cleanup) to this interface. Method shapes stay close to the
// application methods so that bridge is mechanical.
type InventoryReportBackend interface {
	ReplaceBatch(ctx context.Context, resourceType domain.ResourceType, reports []InventoryObjectReport) error
	DeleteBatch(ctx context.Context, resources []domain.InventoryResourceRef) error
	ReplaceCollection(ctx context.Context, resourceType domain.ResourceType, collection domain.CollectionName, reports []InventoryObjectReport) error
	DeleteCollection(ctx context.Context, resourceType domain.ResourceType, collection domain.CollectionName) error
	DeleteSubtree(ctx context.Context, ref domain.InventorySubtreeRef) error
}

// LocalInventoryReporter is the in-process [InventoryReporter]. It
// always reports under [ObjectResourceType] and forwards to an
// [InventoryReportBackend].
type LocalInventoryReporter struct {
	backend InventoryReportBackend
}

// NewLocalInventoryReporter creates a reporter backed by backend.
func NewLocalInventoryReporter(backend InventoryReportBackend) *LocalInventoryReporter {
	return &LocalInventoryReporter{backend: backend}
}

// ApplyDelta writes upserts via ReplaceBatch and deletes via
// DeleteBatch. An empty delta returns nil without calling the backend
// -- idle empty flushes are not heartbeats.
func (r *LocalInventoryReporter) ApplyDelta(ctx context.Context, delta InventoryDeltaReport) error {
	if len(delta.Upserts) == 0 && len(delta.Deletes) == 0 {
		return nil
	}
	if len(delta.Upserts) > 0 {
		if err := r.backend.ReplaceBatch(ctx, ObjectResourceType, delta.Upserts); err != nil {
			return fmt.Errorf("local inventory reporter apply delta upserts: %w", err)
		}
	}
	if len(delta.Deletes) > 0 {
		if err := r.backend.DeleteBatch(ctx, delta.Deletes); err != nil {
			return fmt.Errorf("local inventory reporter apply delta deletes: %w", err)
		}
	}
	return nil
}

// ReplaceCollection replaces one exact target+GVR object collection.
func (r *LocalInventoryReporter) ReplaceCollection(ctx context.Context, snapshot InventoryCollectionSnapshot) error {
	if err := r.backend.ReplaceCollection(ctx, ObjectResourceType, snapshot.Collection, snapshot.Reports); err != nil {
		return fmt.Errorf("local inventory reporter replace collection %q: %w", snapshot.Collection, err)
	}
	return nil
}

// DeleteResources hard-deletes the named inventory resources.
func (r *LocalInventoryReporter) DeleteResources(ctx context.Context, refs []domain.InventoryResourceRef) error {
	if len(refs) == 0 {
		return nil
	}
	if err := r.backend.DeleteBatch(ctx, refs); err != nil {
		return fmt.Errorf("local inventory reporter delete resources: %w", err)
	}
	return nil
}

// DeleteCollection removes every resource in the exact named
// collection (for example when a GVR is removed from indexing).
func (r *LocalInventoryReporter) DeleteCollection(ctx context.Context, ref domain.InventoryCollectionRef) error {
	if err := r.backend.DeleteCollection(ctx, ref.ResourceType, ref.Collection); err != nil {
		return fmt.Errorf("local inventory reporter delete collection %q: %w", ref.Collection, err)
	}
	return nil
}

// DeleteSubtree deletes every inventory resource under ref.Parent for
// ref.ResourceType. Target termination cleanup normally goes through
// KubernetesTargetIndexedInventoryCleaner rather than the writer; this
// method exists so the same reporter contract can express subtree
// deletes for external reporters.
func (r *LocalInventoryReporter) DeleteSubtree(ctx context.Context, ref domain.InventorySubtreeRef) error {
	if err := r.backend.DeleteSubtree(ctx, ref); err != nil {
		return fmt.Errorf("local inventory reporter delete subtree %q: %w", ref.Parent, err)
	}
	return nil
}
