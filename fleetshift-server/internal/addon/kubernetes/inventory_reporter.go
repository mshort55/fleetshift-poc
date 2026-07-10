package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// InventoryReporter is the reusable inventory report boundary for the
// Kubernetes indexing core. In-process indexing and a future
// external agent both emit these commands; only the transport behind
// the reporter changes. Commands map onto InventoryReportService's
// delete/resync/replace surface and use the Kubernetes object identity
// helpers (ResourceType ObjectResourceType, names under
// {TargetCollectionID}/{target}/{APIResourceCollectionID}/{gvr}/{ObjectCollectionID}/{uid}).
type InventoryReporter interface {
	// ApplyDelta writes incremental object upserts and deletes.
	ApplyDelta(ctx context.Context, delta InventoryDeltaReport) error
	// ReplaceCollection replaces one exact target+GVR object collection.
	ReplaceCollection(ctx context.Context, snapshot InventoryCollectionSnapshot) error
	// DeleteResources hard-deletes the named inventory resources.
	DeleteResources(ctx context.Context, refs []domain.InventoryResourceRef) error
	// DeleteCollection deletes every resource in the exact named collection.
	DeleteCollection(ctx context.Context, ref domain.InventoryCollectionRef) error
	// DeleteSubtree deletes every inventory resource under ref.Parent for
	// ref.ResourceType.
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

// InventoryReportBackend is the application call surface the direct
// [DirectInventoryReporter] adapts onto. It is declared here, at the
// point of use, rather than imported from the application layer, so
// this addon package never depends on internal/application. A thin
// adapter in server composition adapts
// *application.InventoryReportService (and owner-validated subtree
// cleanup) to this interface. Method shapes stay close to the
// application methods so that adapter is mechanical.
type InventoryReportBackend interface {
	// ReplaceBatch writes complete latest-state replacements for the
	// given resource type.
	ReplaceBatch(ctx context.Context, resourceType domain.ResourceType, reports []InventoryObjectReport) error
	// DeleteBatch hard-deletes the named inventory resources.
	DeleteBatch(ctx context.Context, resources []domain.InventoryResourceRef) error
	// ReplaceCollection replaces one exact collection's contents.
	ReplaceCollection(ctx context.Context, resourceType domain.ResourceType, collection domain.CollectionName, reports []InventoryObjectReport) error
	// DeleteCollection deletes every resource in the exact named collection.
	DeleteCollection(ctx context.Context, resourceType domain.ResourceType, collection domain.CollectionName) error
	// DeleteSubtree deletes every inventory resource under ref.Parent for
	// ref.ResourceType.
	DeleteSubtree(ctx context.Context, ref domain.InventorySubtreeRef) error
}

// DirectInventoryReporter is the no-transport [InventoryReporter] used by
// server-side indexing. It always reports under [ObjectResourceType] and
// forwards to an [InventoryReportBackend].
type DirectInventoryReporter struct {
	backend InventoryReportBackend
}

// NewDirectInventoryReporter creates a reporter backed by backend.
func NewDirectInventoryReporter(backend InventoryReportBackend) *DirectInventoryReporter {
	return &DirectInventoryReporter{backend: backend}
}

// ApplyDelta writes upserts via ReplaceBatch and deletes via
// DeleteBatch. An empty delta returns nil without calling the backend
// -- idle empty flushes are not heartbeats.
func (r *DirectInventoryReporter) ApplyDelta(ctx context.Context, delta InventoryDeltaReport) error {
	if len(delta.Upserts) == 0 && len(delta.Deletes) == 0 {
		return nil
	}
	if len(delta.Upserts) > 0 {
		if err := r.backend.ReplaceBatch(ctx, ObjectResourceType, delta.Upserts); err != nil {
			return fmt.Errorf("direct inventory reporter apply delta upserts: %w", err)
		}
	}
	if len(delta.Deletes) > 0 {
		if err := r.backend.DeleteBatch(ctx, delta.Deletes); err != nil {
			return fmt.Errorf("direct inventory reporter apply delta deletes: %w", err)
		}
	}
	return nil
}

// ReplaceCollection replaces one exact target+GVR object collection.
func (r *DirectInventoryReporter) ReplaceCollection(ctx context.Context, snapshot InventoryCollectionSnapshot) error {
	if err := r.backend.ReplaceCollection(ctx, ObjectResourceType, snapshot.Collection, snapshot.Reports); err != nil {
		return fmt.Errorf("direct inventory reporter replace collection %q: %w", snapshot.Collection, err)
	}
	return nil
}

// DeleteResources hard-deletes the named inventory resources.
func (r *DirectInventoryReporter) DeleteResources(ctx context.Context, refs []domain.InventoryResourceRef) error {
	if len(refs) == 0 {
		return nil
	}
	if err := r.backend.DeleteBatch(ctx, refs); err != nil {
		return fmt.Errorf("direct inventory reporter delete resources: %w", err)
	}
	return nil
}

// DeleteCollection removes every resource in the exact named
// collection (for example when a GVR is removed from indexing).
func (r *DirectInventoryReporter) DeleteCollection(ctx context.Context, ref domain.InventoryCollectionRef) error {
	if err := r.backend.DeleteCollection(ctx, ref.ResourceType, ref.Collection); err != nil {
		return fmt.Errorf("direct inventory reporter delete collection %q: %w", ref.Collection, err)
	}
	return nil
}

// DeleteSubtree deletes every inventory resource under ref.Parent for
// ref.ResourceType. Target termination cleanup normally goes through
// KubernetesTargetIndexedInventoryCleaner rather than the writer; this
// method exists so the same reporter contract can express subtree
// deletes for external reporters.
func (r *DirectInventoryReporter) DeleteSubtree(ctx context.Context, ref domain.InventorySubtreeRef) error {
	if err := r.backend.DeleteSubtree(ctx, ref); err != nil {
		return fmt.Errorf("direct inventory reporter delete subtree %q: %w", ref.Parent, err)
	}
	return nil
}
