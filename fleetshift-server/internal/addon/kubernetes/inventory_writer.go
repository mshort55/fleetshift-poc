package kubernetes

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// edgeKey identifies a unique edge in the diff map.
// No current code path needs O(1) lookup by source UID alone (e.g. "delete all
// edges from source X"). If that changes, switch to map[string]map[edgeKey]Edge
// keyed by sourceUID for the outer map, or add a secondary index.
type edgeKey struct {
	SourceUID string
	DestUID   string
	EdgeType  EdgeType
}

// Writer batches informer events and reports them through an
// [InventoryReporter]. Topology edge deltas are computed in memory and
// delivered to an [EdgeSink] (typically [NoopEdgeSink]); they never
// flow through the inventory reporter.
type Writer struct {
	targetID      string
	reporter      InventoryReporter
	edgeSink      EdgeSink
	schema        map[schema.GroupVersionResource]SchemaEntry
	eventCh       chan ResourceEvent
	resyncCh      chan ResyncEvent
	removeCh      chan RemoveGVREvent
	batchInterval time.Duration
	currentNodes  map[string]inventoryNode
	edgeFuncs     map[string]func(NodeStore) []Edge
	previousEdges map[edgeKey]Edge
	logger        *slog.Logger

	// stopCh requests a shutdown flush under a caller-provided context.
	// Buffered so Stop does not block if Run has already exited.
	stopCh chan context.Context
}

// defaultWriterShutdownFlushTimeout is used when Run's context is canceled
// without an explicit [Writer.Stop] flush context (and without a deadline).
const defaultWriterShutdownFlushTimeout = 5 * time.Second

// NewWriter creates a Writer that batches events over batchInterval and
// reports them via reporter. If edgeSink is nil, [NoopEdgeSink] is used.
func NewWriter(
	targetID string,
	reporter InventoryReporter,
	edgeSink EdgeSink,
	schema map[schema.GroupVersionResource]SchemaEntry,
	batchInterval time.Duration,
	logger *slog.Logger,
) *Writer {
	if edgeSink == nil {
		edgeSink = NoopEdgeSink{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Writer{
		targetID:      targetID,
		reporter:      reporter,
		edgeSink:      edgeSink,
		schema:        schema,
		eventCh:       make(chan ResourceEvent, 256),
		resyncCh:      make(chan ResyncEvent, 16),
		removeCh:      make(chan RemoveGVREvent, 16),
		batchInterval: batchInterval,
		currentNodes:  make(map[string]inventoryNode),
		edgeFuncs:     make(map[string]func(NodeStore) []Edge),
		previousEdges: make(map[edgeKey]Edge),
		logger:        logger,
		stopCh:        make(chan context.Context, 1),
	}
}

// EventCh returns the channel callers use to submit resource events.
func (w *Writer) EventCh() chan<- ResourceEvent { return w.eventCh }

// ResyncCh returns the channel callers use to submit resync events.
func (w *Writer) ResyncCh() chan<- ResyncEvent { return w.resyncCh }

// RemoveCh returns the channel callers use to signal that a GVR is no
// longer being indexed. Removal is non-destructive to persisted
// inventory: the writer drops in-memory state for that GVR only.
func (w *Writer) RemoveCh() chan<- RemoveGVREvent { return w.removeCh }

// Stop requests that Run perform a final flush under flushCtx and then
// return. It is safe to call Stop more than once; only the first request
// is delivered. Prefer Stop over canceling Run's context when a shared
// shutdown deadline must govern the final flush.
func (w *Writer) Stop(flushCtx context.Context) {
	if flushCtx == nil {
		flushCtx = context.Background()
	}
	select {
	case w.stopCh <- flushCtx:
	default:
	}
}

// Run starts the event loop. It blocks until Stop is called or ctx is
// cancelled, flushing any remaining batch before returning. Informer
// shutdown flushes real pending object events only; it does not turn
// local cache eviction into persisted deletes. RemoveGVR also leaves
// persisted inventory unchanged (in-memory drop only).
func (w *Writer) Run(ctx context.Context) {
	batchTicker := time.NewTicker(w.batchInterval)
	defer batchTicker.Stop()

	// Pending batch state.
	pendingUpserts := make(map[string]*unstructured.Unstructured) // UID -> resource
	pendingUpsertGVR := make(map[string]schema.GroupVersionResource)
	pendingDeletes := make(map[string]schema.GroupVersionResource) // UID -> GVR

	// Dedup: tracks UID -> last-sent resourceVersion.
	sentVersions := make(map[string]string)

	flushAndReturn := func(flushCtx context.Context) {
		if flushCtx == nil {
			flushCtx = context.Background()
		}
		w.flush(flushCtx, pendingUpserts, pendingUpsertGVR, pendingDeletes, sentVersions)
	}

	for {
		select {
		case flushCtx := <-w.stopCh:
			flushAndReturn(flushCtx)
			return

		case <-ctx.Done():
			flushCtx, flushCancel := writerShutdownFlushContext(ctx)
			flushAndReturn(flushCtx)
			flushCancel()
			return

		case ev := <-w.eventCh:
			uid := string(ev.Resource.GetUID())

			switch ev.Op {
			case EventAdd, EventUpdate:
				// Late-delete protection: if this UID was deleted in the
				// current batch, drop the add/update.
				if _, deleted := pendingDeletes[uid]; deleted {
					continue
				}
				pendingUpserts[uid] = ev.Resource
				pendingUpsertGVR[uid] = ev.GVR

			case EventDelete:
				pendingDeletes[uid] = ev.GVR
				// Remove any pending upsert for this UID — the delete wins.
				delete(pendingUpserts, uid)
				delete(pendingUpsertGVR, uid)
				// Clear sent version so a future add for this UID is not
				// deduped against the deleted resource.
				delete(sentVersions, uid)
				// Remove from edge state.
				delete(w.currentNodes, uid)
				delete(w.edgeFuncs, uid)
			}

		case rs := <-w.resyncCh:
			w.sendResync(ctx, rs)

		case rm := <-w.removeCh:
			// Drop any pending work for this GVR so a later flush cannot
			// resurrect objects after the GVR generation is closed.
			for uid, gvr := range pendingUpsertGVR {
				if gvr == rm.GVR {
					delete(pendingUpserts, uid)
					delete(pendingUpsertGVR, uid)
				}
			}
			for uid, gvr := range pendingDeletes {
				if gvr == rm.GVR {
					delete(pendingDeletes, uid)
				}
			}
			w.removeGVR(rm.GVR)

		case <-batchTicker.C:
			if err := w.flush(ctx, pendingUpserts, pendingUpsertGVR, pendingDeletes, sentVersions); err == nil {
				pendingUpserts = make(map[string]*unstructured.Unstructured)
				pendingUpsertGVR = make(map[string]schema.GroupVersionResource)
				pendingDeletes = make(map[string]schema.GroupVersionResource)
			}
		}
	}
}

// writerShutdownFlushContext derives a flush context after Run's context is
// canceled. Prefer any remaining deadline on ctx; otherwise use the default
// shutdown flush timeout.
func writerShutdownFlushContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if dl, ok := ctx.Deadline(); ok {
		return context.WithDeadline(context.Background(), dl)
	}
	return context.WithTimeout(context.Background(), defaultWriterShutdownFlushTimeout)
}

// flush sends the accumulated batch as a single ApplyDelta call. It applies
// dedup by skipping upserts whose resourceVersion has not changed.
// Returns error if the write fails; state is only advanced on success.
func (w *Writer) flush(
	ctx context.Context,
	upserts map[string]*unstructured.Unstructured,
	upsertGVR map[string]schema.GroupVersionResource,
	deletes map[string]schema.GroupVersionResource,
	sentVersions map[string]string,
) error {
	if len(upserts) == 0 && len(deletes) == 0 {
		return nil
	}

	var reports []InventoryObjectReport
	newSentVersions := make(map[string]string)

	for uid, r := range upserts {
		rv := r.GetResourceVersion()
		// Dedup: skip if we already sent this exact version.
		if lastRV, ok := sentVersions[uid]; ok && lastRV == rv {
			continue
		}

		gvr := upsertGVR[uid]
		entry := w.schemaEntry(gvr)
		report, node, err := ExtractObservedResource(r, entry, w.targetID)
		if err != nil {
			w.logger.Warn("skipping upsert; extraction failed",
				"uid", uid,
				"gvr", gvr.String(),
				"error", err)
			continue
		}
		node.GVR = gvr
		reports = append(reports, report)
		newSentVersions[uid] = rv

		// Track inventory node for edge computation.
		w.currentNodes[uid] = node

		if entry.BuildEdges != nil {
			w.edgeFuncs[uid] = entry.BuildEdges(r, uid)
		}
	}

	var deletedRefs []domain.InventoryResourceRef
	for uid, gvr := range deletes {
		ref, err := w.resourceRef(gvr, uid)
		if err != nil {
			w.logger.Warn("skipping delete; resource name construction failed",
				"uid", uid,
				"gvr", gvr.String(),
				"error", err)
			continue
		}
		deletedRefs = append(deletedRefs, ref)
	}

	if len(reports) == 0 && len(deletedRefs) == 0 {
		return nil
	}

	edgeAdds, edgeDels, newEdges := w.diffEdges()

	if w.reporter == nil {
		return nil
	}

	delta := InventoryDeltaReport{
		Upserts: reports,
		Deletes: deletedRefs,
	}
	if err := w.applyDeltaWithRetry(ctx, delta); err != nil {
		return err
	}

	if len(edgeAdds) > 0 || len(edgeDels) > 0 {
		if err := w.edgeSink.ApplyEdgeDelta(ctx, domain.TargetID(w.targetID), EdgeDelta{
			Adds:    edgeAdds,
			Deletes: edgeDels,
		}); err != nil {
			w.logger.Warn("edge sink ApplyEdgeDelta failed", "error", err)
			return err
		}
	}

	maps.Copy(sentVersions, newSentVersions)
	w.previousEdges = newEdges
	return nil
}

// applyDeltaWithRetry applies a delta with exponential backoff retry.
// It retries up to 3 times with 1s, 2s, 4s backoff. Returns the error
// if all retries fail. Empty deltas are not sent — idle flushes are not
// heartbeats.
func (w *Writer) applyDeltaWithRetry(ctx context.Context, delta InventoryDeltaReport) error {
	if w.reporter == nil {
		return nil
	}
	if len(delta.Upserts) == 0 && len(delta.Deletes) == 0 {
		return nil
	}

	var err error
	for attempt := range 3 {
		err = w.reporter.ApplyDelta(ctx, delta)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}

		backoff := time.Duration(1<<attempt) * time.Second
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}

		w.logger.Warn("ApplyDelta failed, retrying",
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err)

		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff):
		}
	}

	w.logger.Error("ApplyDelta failed after retries", "error", err)
	return err
}

// sendResync upserts the LIST snapshot and deletes only resources this
// writer already knew for the GVR that are absent from the LIST. That
// is same-process omission reconciliation via in-memory membership; it
// does not discover database-only rows from an earlier process.
// In-memory membership for this GVR is updated only after the write
// succeeds (or when there is no reporter), matching flush's
// advance-on-success rule.
//
// Resync handles items only — edges are not written. Edge computation
// is deferred to the flush path, which runs ALL edge closures against
// the full w.currentNodes on every tick. This avoids a class of race
// where a GVR resync runs before cross-GVR dependencies are in
// w.currentNodes (e.g. Pod resync before ReplicaSets are known),
// which would delete correct edges and fail to re-create them.
func (w *Writer) sendResync(ctx context.Context, rs ResyncEvent) {
	entry := w.schemaEntry(rs.GVR)

	resyncUIDs := make(map[string]struct{})
	var upserts []InventoryObjectReport
	nextNodes := make(map[string]inventoryNode)
	nextEdgeFuncs := make(map[string]func(NodeStore) []Edge)

	for _, r := range rs.Resources {
		report, node, err := ExtractObservedResource(r, entry, w.targetID)
		if err != nil {
			w.logger.Warn("skipping resync item; extraction failed",
				"uid", string(r.GetUID()),
				"gvr", rs.GVR.String(),
				"error", err)
			continue
		}
		node.GVR = rs.GVR
		upserts = append(upserts, report)
		uid := string(r.GetUID())
		resyncUIDs[uid] = struct{}{}
		nextNodes[uid] = node
		if entry.BuildEdges != nil {
			nextEdgeFuncs[uid] = entry.BuildEdges(r, uid)
		}
	}

	var deletes []domain.InventoryResourceRef
	var staleUIDs []string
	for uid, node := range w.currentNodes {
		if node.GVR != rs.GVR {
			continue
		}
		if _, exists := resyncUIDs[uid]; exists {
			continue
		}
		ref, err := w.resourceRef(rs.GVR, uid)
		if err != nil {
			w.logger.Warn("skipping resync delete; resource name construction failed",
				"uid", uid,
				"gvr", rs.GVR.String(),
				"error", err)
			continue
		}
		deletes = append(deletes, ref)
		staleUIDs = append(staleUIDs, uid)
	}

	if err := w.applyDeltaWithRetry(ctx, InventoryDeltaReport{
		Upserts: upserts,
		Deletes: deletes,
	}); err != nil {
		// applyDeltaWithRetry already logged; keep prior membership so a
		// later resync can retry the same omission set.
		return
	}

	maps.Copy(w.currentNodes, nextNodes)
	maps.Copy(w.edgeFuncs, nextEdgeFuncs)
	for _, uid := range staleUIDs {
		delete(w.currentNodes, uid)
		delete(w.edgeFuncs, uid)
	}
}

// removeGVR drops in-memory nodes/edge closures for that GVR. It does
// not persist inventory deletes; GVR removal from the desired set is
// non-destructive to stored rows.
func (w *Writer) removeGVR(gvr schema.GroupVersionResource) {
	for uid, node := range w.currentNodes {
		if node.GVR == gvr {
			delete(w.currentNodes, uid)
			delete(w.edgeFuncs, uid)
		}
	}
}

func (w *Writer) schemaEntry(gvr schema.GroupVersionResource) SchemaEntry {
	entry := w.schema[gvr]
	// Missing schema entries still need the watched GVR on the entry so
	// extraction can build ObjectResourceName / labels correctly.
	if entry.GVR.Empty() {
		entry.GVR = gvr
	}
	return entry
}

func (w *Writer) resourceRef(gvr schema.GroupVersionResource, uid string) (domain.InventoryResourceRef, error) {
	name, err := ObjectResourceName(KubernetesObjectIdentity{
		TargetID: domain.TargetID(w.targetID),
		GVR:      gvr,
		UID:      uid,
	})
	if err != nil {
		return domain.InventoryResourceRef{}, err
	}
	return domain.InventoryResourceRef{
		ResourceType: ObjectResourceType,
		Name:         name,
	}, nil
}

func (w *Writer) diffEdges() (adds, dels []Edge, newEdges map[edgeKey]Edge) {
	ns := buildNodeStore(w.currentNodes)
	newEdges = make(map[edgeKey]Edge)

	// Type-specific edges from BuildEdges closures.
	for _, edgeFn := range w.edgeFuncs {
		for _, e := range edgeFn(ns) {
			newEdges[edgeKey{e.SourceUID, e.DestUID, e.EdgeType}] = e
		}
	}

	// Common edges (ownedBy traversal) for ALL nodes.
	for uid := range w.currentNodes {
		for _, e := range commonEdges(uid, ns) {
			newEdges[edgeKey{e.SourceUID, e.DestUID, e.EdgeType}] = e
		}
	}

	for key, edge := range newEdges {
		if _, ok := w.previousEdges[key]; !ok {
			adds = append(adds, edge)
		}
	}
	for key, edge := range w.previousEdges {
		if _, ok := newEdges[key]; !ok {
			dels = append(dels, edge)
		}
	}
	return adds, dels, newEdges
}
