package cli

import (
	"context"
	"fmt"
	"log/slog"

	kubernetesaddon "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// startKubernetesIndexController runs the controller and returns a channel
// that closes after the controller loop returns.
func startKubernetesIndexController(ctx context.Context, run func(context.Context)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx)
	}()
	return done
}

// stopKubernetesIndexController cancels the controller and waits for its loop
// to return after its bounded hosted-indexer shutdown attempt.
func stopKubernetesIndexController(cancel context.CancelFunc, done <-chan struct{}) {
	cancel()
	<-done
}

// newKubernetesInProcessIndexing wires the Kubernetes in-process index
// controller and inventory reporter for server composition. The
// returned hooks implement [domain.TargetOutputHooks]; the controller
// is started separately by the caller once the process is ready to
// serve.
func newKubernetesInProcessIndexing(
	ctx context.Context,
	store domain.Store,
	vault domain.Vault,
	logger *slog.Logger,
) (domain.TargetOutputHooks, *kubernetesaddon.InProcessIndexController) {
	inventoryReportSvc := application.NewInventoryReportService(store)
	reporter := kubernetesaddon.NewDirectInventoryReporter(
		newDirectInventoryReportBackend(inventoryReportSvc),
	)
	indexHost := kubernetesaddon.NewKubernetesInProcessIndexHost(ctx, vault, reporter, logger)
	controller := kubernetesaddon.NewInProcessIndexController(
		storeTargetLister{store: store},
		indexHost,
		kubernetesaddon.DefaultInProcessIndexPolicy{},
		logger,
	)
	hooks := application.NewTargetOutputHookService(
		store,
		application.WithTargetRuntimeHooks(controller),
	)
	return hooks, controller
}

// storeTargetLister adapts FleetShift's target store onto the Kubernetes
// in-process controller's TargetLister port at the server composition boundary.
type storeTargetLister struct {
	store domain.Store
}

// ListTargets implements [kubernetesaddon.TargetLister].
func (l storeTargetLister) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	tx, err := l.store.BeginReadOnly(ctx)
	if err != nil {
		return nil, fmt.Errorf("list targets: begin read-only tx: %w", err)
	}
	defer tx.Rollback()
	targets, err := tx.Targets().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	return targets, nil
}

// directInventoryReportBackend adapts InventoryReportService onto the
// Kubernetes addon's InventoryReportBackend at the server composition
// boundary.
type directInventoryReportBackend struct {
	reports *application.InventoryReportService
}

// newDirectInventoryReportBackend adapts InventoryReportService onto
// [kubernetesaddon.InventoryReportBackend].
func newDirectInventoryReportBackend(
	reports *application.InventoryReportService,
) *directInventoryReportBackend {
	return &directInventoryReportBackend{reports: reports}
}

// Compile-time check that the addon controller satisfies
// application.TargetRuntimeHooks (ready hint + OnTargetDraining). Kept here
// (composition) because the implementer and the interface live in
// different packages; the two local adapters omit this pattern since
// New* already type-checks them.
var _ application.TargetRuntimeHooks = (*kubernetesaddon.InProcessIndexController)(nil)

// ReplaceBatch implements [kubernetesaddon.InventoryReportBackend].
func (b *directInventoryReportBackend) ReplaceBatch(ctx context.Context, resourceType domain.ResourceType, reports []kubernetesaddon.InventoryObjectReport) error {
	in := application.InventoryReplacementBatchInput{
		Reports: make([]application.InventoryReplacementInput, len(reports)),
	}
	for i, report := range reports {
		name := report.Name
		in.Reports[i] = application.InventoryReplacementInput{
			ResourceType: resourceType,
			Name:         &name,
			IsDelete:     report.IsDelete,
			Labels:       report.Labels,
			Observation:  report.Observation,
			Conditions:   report.Conditions,
			ObservedAt:   report.ObservedAt,
		}
	}
	if err := b.reports.ReplaceBatch(ctx, in); err != nil {
		return fmt.Errorf("kubernetes inventory report adapter replace batch: %w", err)
	}
	return nil
}
