package kubernetes_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// recordingInventoryReportBackend records every InventoryReportBackend
// call so LocalInventoryReporter mapping can be asserted without a
// real InventoryReportService, informer, or store.
type recordingInventoryReportBackend struct {
	replaceBatchCalls      []replaceBatchCall
	deleteBatchCalls       [][]domain.InventoryResourceRef
	replaceCollectionCalls []replaceCollectionCall
	deleteCollectionCalls  []deleteCollectionCall
	deleteSubtreeCalls     []domain.InventorySubtreeRef

	replaceBatchErr      error
	deleteBatchErr       error
	replaceCollectionErr error
	deleteCollectionErr  error
	deleteSubtreeErr     error
}

type replaceBatchCall struct {
	resourceType domain.ResourceType
	reports      []kubernetes.InventoryObjectReport
}

type replaceCollectionCall struct {
	resourceType domain.ResourceType
	collection   domain.CollectionName
	reports      []kubernetes.InventoryObjectReport
}

type deleteCollectionCall struct {
	resourceType domain.ResourceType
	collection   domain.CollectionName
}

func (r *recordingInventoryReportBackend) ReplaceBatch(_ context.Context, resourceType domain.ResourceType, reports []kubernetes.InventoryObjectReport) error {
	r.replaceBatchCalls = append(r.replaceBatchCalls, replaceBatchCall{resourceType: resourceType, reports: reports})
	return r.replaceBatchErr
}

func (r *recordingInventoryReportBackend) DeleteBatch(_ context.Context, resources []domain.InventoryResourceRef) error {
	r.deleteBatchCalls = append(r.deleteBatchCalls, resources)
	return r.deleteBatchErr
}

func (r *recordingInventoryReportBackend) ReplaceCollection(_ context.Context, resourceType domain.ResourceType, collection domain.CollectionName, reports []kubernetes.InventoryObjectReport) error {
	r.replaceCollectionCalls = append(r.replaceCollectionCalls, replaceCollectionCall{
		resourceType: resourceType,
		collection:   collection,
		reports:      reports,
	})
	return r.replaceCollectionErr
}

func (r *recordingInventoryReportBackend) DeleteCollection(_ context.Context, resourceType domain.ResourceType, collection domain.CollectionName) error {
	r.deleteCollectionCalls = append(r.deleteCollectionCalls, deleteCollectionCall{
		resourceType: resourceType,
		collection:   collection,
	})
	return r.deleteCollectionErr
}

func (r *recordingInventoryReportBackend) DeleteSubtree(_ context.Context, ref domain.InventorySubtreeRef) error {
	r.deleteSubtreeCalls = append(r.deleteSubtreeCalls, ref)
	return r.deleteSubtreeErr
}

func TestLocalInventoryReporter_ApplyDelta_EmptyIsNoop(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	if err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{}); err != nil {
		t.Fatalf("ApplyDelta(empty): %v", err)
	}
	if len(fake.replaceBatchCalls) != 0 || len(fake.deleteBatchCalls) != 0 {
		t.Fatalf("empty ApplyDelta must not call backend; got replace=%d delete=%d",
			len(fake.replaceBatchCalls), len(fake.deleteBatchCalls))
	}
}

func TestLocalInventoryReporter_ApplyDelta_MapsUpsertsAndDeletes(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	now := time.Unix(1700000000, 0).UTC()
	obs := json.RawMessage(`{"kind":"Pod"}`)
	name := domain.ResourceName("clusters/prod/apiResources/core~v1~pods/objects/uid-1")
	delName := domain.ResourceName("clusters/prod/apiResources/core~v1~pods/objects/uid-gone")

	err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{
		Upserts: []kubernetes.InventoryObjectReport{{
			Name:        name,
			Labels:      map[string]string{"k8s.uid": "uid-1"},
			Observation: &obs,
			ObservedAt:  now,
		}},
		Deletes: []domain.InventoryResourceRef{{
			ResourceType: kubernetes.ObjectResourceType,
			Name:         delName,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyDelta: %v", err)
	}

	if len(fake.replaceBatchCalls) != 1 {
		t.Fatalf("ReplaceBatch calls = %d, want 1", len(fake.replaceBatchCalls))
	}
	gotBatch := fake.replaceBatchCalls[0]
	if gotBatch.resourceType != kubernetes.ObjectResourceType {
		t.Errorf("ReplaceBatch resourceType = %q, want %q", gotBatch.resourceType, kubernetes.ObjectResourceType)
	}
	if len(gotBatch.reports) != 1 || gotBatch.reports[0].Name != name {
		t.Errorf("ReplaceBatch reports = %+v, want name %q", gotBatch.reports, name)
	}
	if !reflect.DeepEqual(gotBatch.reports[0].Labels, map[string]string{"k8s.uid": "uid-1"}) {
		t.Errorf("ReplaceBatch labels = %#v", gotBatch.reports[0].Labels)
	}
	if gotBatch.reports[0].Observation == nil || string(*gotBatch.reports[0].Observation) != string(obs) {
		t.Errorf("ReplaceBatch observation = %v, want %s", gotBatch.reports[0].Observation, obs)
	}
	if !gotBatch.reports[0].ObservedAt.Equal(now) {
		t.Errorf("ReplaceBatch ObservedAt = %v, want %v", gotBatch.reports[0].ObservedAt, now)
	}

	if len(fake.deleteBatchCalls) != 1 {
		t.Fatalf("DeleteBatch calls = %d, want 1", len(fake.deleteBatchCalls))
	}
	wantDel := []domain.InventoryResourceRef{{
		ResourceType: kubernetes.ObjectResourceType,
		Name:         delName,
	}}
	if !reflect.DeepEqual(fake.deleteBatchCalls[0], wantDel) {
		t.Errorf("DeleteBatch = %#v, want %#v", fake.deleteBatchCalls[0], wantDel)
	}
}

func TestLocalInventoryReporter_ApplyDelta_UpsertsOnly(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	name := domain.ResourceName("clusters/prod/apiResources/apps~v1~deployments/objects/uid-d1")

	err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{
		Upserts: []kubernetes.InventoryObjectReport{{Name: name}},
	})
	if err != nil {
		t.Fatalf("ApplyDelta: %v", err)
	}
	if len(fake.replaceBatchCalls) != 1 {
		t.Fatalf("ReplaceBatch calls = %d, want 1", len(fake.replaceBatchCalls))
	}
	if len(fake.deleteBatchCalls) != 0 {
		t.Fatalf("DeleteBatch calls = %d, want 0", len(fake.deleteBatchCalls))
	}
}

func TestLocalInventoryReporter_ApplyDelta_DeletesOnly(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	refs := []domain.InventoryResourceRef{{
		ResourceType: kubernetes.ObjectResourceType,
		Name:         "clusters/prod/apiResources/core~v1~pods/objects/uid-x",
	}}

	err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{Deletes: refs})
	if err != nil {
		t.Fatalf("ApplyDelta: %v", err)
	}
	if len(fake.replaceBatchCalls) != 0 {
		t.Fatalf("ReplaceBatch calls = %d, want 0", len(fake.replaceBatchCalls))
	}
	if len(fake.deleteBatchCalls) != 1 || !reflect.DeepEqual(fake.deleteBatchCalls[0], refs) {
		t.Fatalf("DeleteBatch = %#v, want %#v", fake.deleteBatchCalls, refs)
	}
}

func TestLocalInventoryReporter_ApplyDelta_PropagatesReplaceBatchError(t *testing.T) {
	wantErr := errors.New("replace failed")
	fake := &recordingInventoryReportBackend{replaceBatchErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{
		Upserts: []kubernetes.InventoryObjectReport{{Name: "clusters/prod/apiResources/core~v1~pods/objects/uid-1"}},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ApplyDelta error = %v, want wrapped %v", err, wantErr)
	}
	if len(fake.deleteBatchCalls) != 0 {
		t.Fatalf("DeleteBatch must not run after ReplaceBatch failure; got %d calls", len(fake.deleteBatchCalls))
	}
}

func TestLocalInventoryReporter_ApplyDelta_PropagatesDeleteBatchError(t *testing.T) {
	wantErr := errors.New("delete failed")
	fake := &recordingInventoryReportBackend{deleteBatchErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.ApplyDelta(context.Background(), kubernetes.InventoryDeltaReport{
		Deletes: []domain.InventoryResourceRef{{
			ResourceType: kubernetes.ObjectResourceType,
			Name:         "clusters/prod/apiResources/core~v1~pods/objects/uid-1",
		}},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ApplyDelta error = %v, want wrapped %v", err, wantErr)
	}
}

func TestLocalInventoryReporter_ReplaceCollection_MapsToBackend(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	collection := domain.CollectionName("clusters/prod/apiResources/core~v1~pods/objects")
	reports := []kubernetes.InventoryObjectReport{
		{Name: "clusters/prod/apiResources/core~v1~pods/objects/uid-1"},
		{Name: "clusters/prod/apiResources/core~v1~pods/objects/uid-2"},
	}

	err := reporter.ReplaceCollection(context.Background(), kubernetes.InventoryCollectionSnapshot{
		Collection: collection,
		Reports:    reports,
	})
	if err != nil {
		t.Fatalf("ReplaceCollection: %v", err)
	}
	if len(fake.replaceCollectionCalls) != 1 {
		t.Fatalf("ReplaceCollection calls = %d, want 1", len(fake.replaceCollectionCalls))
	}
	got := fake.replaceCollectionCalls[0]
	if got.resourceType != kubernetes.ObjectResourceType {
		t.Errorf("resourceType = %q, want %q", got.resourceType, kubernetes.ObjectResourceType)
	}
	if got.collection != collection {
		t.Errorf("collection = %q, want %q", got.collection, collection)
	}
	if !reflect.DeepEqual(got.reports, reports) {
		t.Errorf("reports = %#v, want %#v", got.reports, reports)
	}
}

func TestLocalInventoryReporter_ReplaceCollection_EmptyReportsStillCallsBackend(t *testing.T) {
	// Empty snapshot is a valid prune-everything resync; the reporter
	// must still forward it so InventoryReportService can prune.
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	collection := domain.CollectionName("clusters/prod/apiResources/apps~v1~deployments/objects")

	err := reporter.ReplaceCollection(context.Background(), kubernetes.InventoryCollectionSnapshot{
		Collection: collection,
	})
	if err != nil {
		t.Fatalf("ReplaceCollection(empty): %v", err)
	}
	if len(fake.replaceCollectionCalls) != 1 {
		t.Fatalf("ReplaceCollection calls = %d, want 1", len(fake.replaceCollectionCalls))
	}
	if len(fake.replaceCollectionCalls[0].reports) != 0 {
		t.Errorf("reports = %#v, want empty", fake.replaceCollectionCalls[0].reports)
	}
}

func TestLocalInventoryReporter_ReplaceCollection_PropagatesError(t *testing.T) {
	wantErr := errors.New("replace collection failed")
	fake := &recordingInventoryReportBackend{replaceCollectionErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.ReplaceCollection(context.Background(), kubernetes.InventoryCollectionSnapshot{
		Collection: "clusters/prod/apiResources/core~v1~pods/objects",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceCollection error = %v, want wrapped %v", err, wantErr)
	}
}

func TestLocalInventoryReporter_DeleteResources_MapsToDeleteBatch(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	refs := []domain.InventoryResourceRef{
		{ResourceType: kubernetes.ObjectResourceType, Name: "clusters/prod/apiResources/core~v1~pods/objects/uid-1"},
		{ResourceType: kubernetes.ObjectResourceType, Name: "clusters/prod/apiResources/core~v1~pods/objects/uid-2"},
	}

	err := reporter.DeleteResources(context.Background(), refs)
	if err != nil {
		t.Fatalf("DeleteResources: %v", err)
	}
	if len(fake.deleteBatchCalls) != 1 || !reflect.DeepEqual(fake.deleteBatchCalls[0], refs) {
		t.Fatalf("DeleteBatch = %#v, want %#v", fake.deleteBatchCalls, refs)
	}
}

func TestLocalInventoryReporter_DeleteResources_EmptyIsNoop(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	if err := reporter.DeleteResources(context.Background(), nil); err != nil {
		t.Fatalf("DeleteResources(nil): %v", err)
	}
	if len(fake.deleteBatchCalls) != 0 {
		t.Fatalf("DeleteBatch calls = %d, want 0", len(fake.deleteBatchCalls))
	}
}

func TestLocalInventoryReporter_DeleteResources_PropagatesError(t *testing.T) {
	wantErr := errors.New("delete resources failed")
	fake := &recordingInventoryReportBackend{deleteBatchErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.DeleteResources(context.Background(), []domain.InventoryResourceRef{{
		ResourceType: kubernetes.ObjectResourceType,
		Name:         "clusters/prod/apiResources/core~v1~pods/objects/uid-1",
	}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("DeleteResources error = %v, want wrapped %v", err, wantErr)
	}
}

func TestLocalInventoryReporter_DeleteCollection_MapsToBackend(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	ref := domain.InventoryCollectionRef{
		ResourceType: kubernetes.ObjectResourceType,
		Collection:   "clusters/prod/apiResources/core~v1~services/objects",
	}

	err := reporter.DeleteCollection(context.Background(), ref)
	if err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	if len(fake.deleteCollectionCalls) != 1 {
		t.Fatalf("DeleteCollection calls = %d, want 1", len(fake.deleteCollectionCalls))
	}
	got := fake.deleteCollectionCalls[0]
	if got.resourceType != ref.ResourceType || got.collection != ref.Collection {
		t.Errorf("DeleteCollection = %+v, want %+v", got, ref)
	}
}

func TestLocalInventoryReporter_DeleteCollection_PropagatesError(t *testing.T) {
	wantErr := errors.New("delete collection failed")
	fake := &recordingInventoryReportBackend{deleteCollectionErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.DeleteCollection(context.Background(), domain.InventoryCollectionRef{
		ResourceType: kubernetes.ObjectResourceType,
		Collection:   "clusters/prod/apiResources/core~v1~services/objects",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("DeleteCollection error = %v, want wrapped %v", err, wantErr)
	}
}

func TestLocalInventoryReporter_DeleteSubtree_MapsToBackend(t *testing.T) {
	fake := &recordingInventoryReportBackend{}
	reporter := kubernetes.NewLocalInventoryReporter(fake)
	ref := domain.InventorySubtreeRef{
		ResourceType: kubernetes.ObjectResourceType,
		Parent:       "clusters/prod",
	}

	err := reporter.DeleteSubtree(context.Background(), ref)
	if err != nil {
		t.Fatalf("DeleteSubtree: %v", err)
	}
	if len(fake.deleteSubtreeCalls) != 1 || fake.deleteSubtreeCalls[0] != ref {
		t.Fatalf("DeleteSubtree = %#v, want %#v", fake.deleteSubtreeCalls, ref)
	}
}

func TestLocalInventoryReporter_DeleteSubtree_PropagatesError(t *testing.T) {
	wantErr := errors.New("subtree delete failed")
	fake := &recordingInventoryReportBackend{deleteSubtreeErr: wantErr}
	reporter := kubernetes.NewLocalInventoryReporter(fake)

	err := reporter.DeleteSubtree(context.Background(), domain.InventorySubtreeRef{
		ResourceType: kubernetes.ObjectResourceType,
		Parent:       "clusters/prod",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("DeleteSubtree error = %v, want wrapped %v", err, wantErr)
	}
}

func TestInventoryReporter_DTOsHaveNoEdgeFields(t *testing.T) {
	// Inventory reporter DTOs must not carry edge fields; edge output
	// is isolated behind EdgeSink.
	for _, typ := range []any{
		kubernetes.InventoryDeltaReport{},
		kubernetes.InventoryCollectionSnapshot{},
		kubernetes.InventoryObjectReport{},
	} {
		rt := reflect.TypeOf(typ)
		for i := 0; i < rt.NumField(); i++ {
			name := rt.Field(i).Name
			switch name {
			case "Adds", "Deletes", "Edges", "EdgeAdds", "EdgeDeletes", "EdgeDelta":
				// InventoryDeltaReport.Deletes is resource refs, not edges.
				if rt == reflect.TypeOf(kubernetes.InventoryDeltaReport{}) && name == "Deletes" {
					continue
				}
				t.Errorf("%s has edge-related field %q", rt.Name(), name)
			}
		}
	}
}

func TestNoopEdgeSink_ApplyEdgeDeltaIsNoop(t *testing.T) {
	var sink kubernetes.EdgeSink = kubernetes.NoopEdgeSink{}
	err := sink.ApplyEdgeDelta(context.Background(), "prod", kubernetes.EdgeDelta{
		Adds: []kubernetes.Edge{{
			EdgeType:   kubernetes.EdgeOwnedBy,
			SourceUID:  "pod-1",
			DestUID:    "rs-1",
			SourceKind: "Pod",
			DestKind:   "ReplicaSet",
		}},
		Deletes: []kubernetes.Edge{{
			EdgeType:  kubernetes.EdgeRunsOn,
			SourceUID: "pod-1",
			DestUID:   "node-1",
		}},
	})
	if err != nil {
		t.Fatalf("NoopEdgeSink.ApplyEdgeDelta: %v", err)
	}
}

func TestNoopEdgeSink_EmptyDeltaIsNoop(t *testing.T) {
	var sink kubernetes.NoopEdgeSink
	if err := sink.ApplyEdgeDelta(context.Background(), "prod", kubernetes.EdgeDelta{}); err != nil {
		t.Fatalf("NoopEdgeSink.ApplyEdgeDelta(empty): %v", err)
	}
}
