package kubernetes

import (
	"context"
	"log/slog"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// indexerComponent holds indexing-specific state for a TargetAgent.
// It manages the informer-to-writer pipeline that watches Kubernetes
// resources and writes inventory items via an InventoryWriter.
type indexerComponent struct {
	targetID      string
	dynClient     dynamic.Interface
	discClient    discovery.DiscoveryInterface
	writer        domain.InventoryWriter
	schema        IndexSchema
	batchInterval time.Duration
	logger        *slog.Logger
	done          chan struct{}
}

// newIndexerComponent creates an indexerComponent. A zero batchInterval
// defaults to 5 seconds.
func newIndexerComponent(
	targetID string,
	dynClient dynamic.Interface,
	discClient discovery.DiscoveryInterface,
	writer domain.InventoryWriter,
	schema IndexSchema,
	batchInterval time.Duration,
	logger *slog.Logger,
) *indexerComponent {
	if batchInterval == 0 {
		batchInterval = 5 * time.Second
	}
	return &indexerComponent{
		targetID:      targetID,
		dynClient:     dynClient,
		discClient:    discClient,
		writer:        writer,
		schema:        schema,
		batchInterval: batchInterval,
		logger:        logger,
		done:          make(chan struct{}),
	}
}

// start runs the informer manager and writer until ctx is cancelled.
func (ic *indexerComponent) start(ctx context.Context) {
	defer close(ic.done)

	schemaMap := ic.schema.Entries
	desiredGVRs := ic.schema.GVRs()

	w := NewWriter(ic.targetID, ic.writer, schemaMap, ic.batchInterval)

	mgr := NewInformerManager(
		ic.dynClient,
		ic.discClient,
		w.EventCh(),
		w.ResyncCh(),
		ic.logger,
	)

	writerCtx, writerCancel := context.WithCancel(ctx)
	defer writerCancel()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		w.Run(writerCtx)
	}()

	mgr.Reconcile(ctx, desiredGVRs)
	defer mgr.StopAll()

	// Block until context is cancelled.
	<-ctx.Done()
	writerCancel()
	<-writerDone
}
