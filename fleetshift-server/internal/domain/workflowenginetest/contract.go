// Package workflowenginetest provides contract tests for [domain.Registry]
// implementations. The test owns all orchestration: it provides infra (repos,
// delivery), builds the domain workflow specs, and calls the registry to
// obtain [domain.OrchestrationWorkflow] and [domain.CreateDeploymentWorkflow].
// The registry implementation just provides [domain.Registry]; it is unaware
// of how the tests work.
package workflowenginetest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// Infra is the test-owned infrastructure: store and delivery.
// The same infra is used for all engines; implementations do not provide it.
type Infra struct {
	Store    domain.Store
	Delivery domain.DeliveryService
}

// InfraFactory creates infra for a test. Typically shared across engine tests
// (e.g. sqlite in-memory). Called once per subtest.
type InfraFactory func(t *testing.T) Infra

// RegistryFactory returns the [domain.Registry] under test. The registry
// may perform implementation-specific setup (e.g. launch DBOS, start worker)
// and register t.Cleanup for teardown. The registry is not given workflow
// specs; the contract builds them from infra and passes them to Register.
type RegistryFactory func(t *testing.T) domain.Registry

// workflows holds the registered workflow interfaces used by contract tests.
type workflows struct {
	Orchestration    domain.OrchestrationWorkflow
	CreateDeployment domain.CreateDeploymentWorkflow
}

// Run exercises the [domain.Registry] contract. It uses infraFactory
// to get repos and delivery, builds [OrchestrationWorkflowSpec] and
// [CreateDeploymentWorkflowSpec], calls registry.RegisterOrchestration
// and registry.RegisterCreateDeployment, then runs the same scenarios
// against the returned workflow interfaces and infra.
func Run(t *testing.T, infraFactory InfraFactory, registryFactory RegistryFactory) {
	t.Helper()

	t.Run("CreateDeployment_StaticPlacement", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1", "t2", "t3")

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{"kind":"ConfigMap"}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"t1", "t3"},
			},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		dep := awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateActive)
		assertResolvedTargets(t, dep, "t1", "t3")

		records := queryDeliveries(ctx, t, infra, "d1")
		if len(records) != 2 {
			t.Fatalf("expected 2 delivery records, got %d", len(records))
		}
		for _, rec := range records {
			if rec.State != domain.DeliveryStateDelivered {
				t.Errorf("record for %s: State = %q, want %q", rec.TargetID, rec.State, domain.DeliveryStateDelivered)
			}
			if len(rec.Manifests) != 1 {
				t.Errorf("record for %s: Manifests len = %d, want 1", rec.TargetID, len(rec.Manifests))
			}
		}
	})

	t.Run("CreateDeployment_AllPlacement", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1", "t2", "t3")

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{Type: domain.PlacementStrategyAll},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		dep := awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateActive)
		assertResolvedTargets(t, dep, "t1", "t2", "t3")

		records := queryDeliveries(ctx, t, infra, "d1")
		if len(records) != 3 {
			t.Fatalf("expected 3 delivery records, got %d", len(records))
		}
	})

	t.Run("CreateDeployment_SelectorPlacement", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		createTargets(ctx, t, infra,
			domain.TargetInfo{ID: "t1", Type: TestTargetType, Name: "cluster-prod", Labels: map[string]string{"env": "prod"}},
			domain.TargetInfo{ID: "t2", Type: TestTargetType, Name: "cluster-staging", Labels: map[string]string{"env": "staging"}},
			domain.TargetInfo{ID: "t3", Type: TestTargetType, Name: "cluster-prod-eu", Labels: map[string]string{"env": "prod"}},
		)

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:           domain.PlacementStrategySelector,
				TargetSelector: &domain.TargetSelector{MatchLabels: map[string]string{"env": "prod"}},
			},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		dep := awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateActive)
		assertResolvedTargets(t, dep, "t1", "t3")
	})

	t.Run("CreateDeployment_StaticPlacement_UnknownTarget", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1")

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"t1", "missing"},
			},
		})
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound or nil, got: %v", err)
		}
	})

	t.Run("DeleteDeployment_RemovesRecords", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1", "t2")

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{Type: domain.PlacementStrategyAll},
		})
		if err != nil {
			t.Fatal(err)
		}

		awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateActive)

		deleteDeploymentAndDeliveries(ctx, t, infra, "d1")

		records := queryDeliveries(ctx, t, infra, "d1")
		if len(records) != 0 {
			t.Fatalf("expected 0 delivery records after delete, got %d", len(records))
		}

		_, err = queryDeployment(ctx, t, infra, "d1")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got: %v", err)
		}
	})

	t.Run("CreateDeployment_DuplicateID", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1", "t2")

		input := domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{Type: domain.PlacementStrategyAll},
		}
		_, err := runCreateDeployment(ctx, t, wfs, input)
		if err != nil {
			t.Fatalf("first Create: %v", err)
		}

		_, err = runCreateDeployment(ctx, t, wfs, input)
		if err != nil {
			// Engine rejected duplicate: error should be ErrAlreadyExists (or wrapped).
			if !errors.Is(err, domain.ErrAlreadyExists) {
				t.Logf("second Create returned error (acceptable): %v", err)
			}
			return
		}
		// Engine may be idempotent (same workflow instance ID) and return success.
		dep, err := queryDeployment(ctx, t, infra, "d1")
		if err != nil || dep.ID != "d1" {
			t.Fatalf("second Create succeeded but deployment d1 missing or wrong: %v", err)
		}
	})

	t.Run("CreateDeployment_SelectorPlacement_ZeroMatches", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		createTargets(ctx, t, infra,
			domain.TargetInfo{ID: "t1", Type: TestTargetType, Name: "a", Labels: map[string]string{"env": "prod"}},
			domain.TargetInfo{ID: "t2", Type: TestTargetType, Name: "b", Labels: map[string]string{"env": "staging"}},
		)

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:           domain.PlacementStrategySelector,
				TargetSelector: &domain.TargetSelector{MatchLabels: map[string]string{"env": "dev"}},
			},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		dep := awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateFailed)
		if len(dep.ResolvedTargets) != 0 {
			t.Fatalf("selector matched no targets: ResolvedTargets = %v, want []", dep.ResolvedTargets)
		}

		records := queryDeliveries(ctx, t, infra, "d1")
		if len(records) != 0 {
			t.Fatalf("expected 0 delivery records, got %d", len(records))
		}
	})

	t.Run("TwoDeployments_Isolation", func(t *testing.T) {
		infra := infraFactory(t)
		wfs := registerWorkflows(t, infra, registryFactory)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		registerTargets(ctx, t, infra, "t1", "t2", "t3")

		_, err := runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d1",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"t1", "t3"},
			},
		})
		if err != nil {
			t.Fatalf("Create d1: %v", err)
		}

		_, err = runCreateDeployment(ctx, t, wfs, domain.CreateDeploymentInput{
			ID: "d2",
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"t2"},
			},
		})
		if err != nil {
			t.Fatalf("Create d2: %v", err)
		}

		dep1 := awaitDeploymentState(ctx, t, infra, "d1", domain.DeploymentStateActive)
		dep2 := awaitDeploymentState(ctx, t, infra, "d2", domain.DeploymentStateActive)

		assertResolvedTargets(t, dep1, "t1", "t3")
		assertResolvedTargets(t, dep2, "t2")

		records1 := queryDeliveries(ctx, t, infra, "d1")
		records2 := queryDeliveries(ctx, t, infra, "d2")
		if len(records1) != 2 {
			t.Fatalf("d1: expected 2 delivery records, got %d", len(records1))
		}
		if len(records2) != 1 {
			t.Fatalf("d2: expected 1 delivery record, got %d", len(records2))
		}
	})

}

// registerWorkflows builds workflow specs from infra, calls
// registry.RegisterOrchestration and registry.RegisterCreateDeployment,
// returns the registered workflow interfaces.
func registerWorkflows(t *testing.T, infra Infra, registryFactory RegistryFactory) workflows {
	t.Helper()
	reg := registryFactory(t)

	orchSpec := &domain.OrchestrationWorkflowSpec{
		Store:      infra.Store,
		Delivery:   infra.Delivery,
		Strategies: domain.DefaultStrategyFactory{},
		Registry:   reg,
	}
	orchWf, err := reg.RegisterOrchestration(orchSpec)
	if err != nil {
		t.Fatalf("RegisterOrchestration: %v", err)
	}

	cwfSpec := &domain.CreateDeploymentWorkflowSpec{
		Store:         infra.Store,
		Orchestration: orchWf,
	}
	createWf, err := reg.RegisterCreateDeployment(cwfSpec)
	if err != nil {
		t.Fatalf("RegisterCreateDeployment: %v", err)
	}

	return workflows{
		Orchestration:    orchWf,
		CreateDeployment: createWf,
	}
}

func runCreateDeployment(ctx context.Context, t *testing.T, wfs workflows, in domain.CreateDeploymentInput) (domain.Deployment, error) {
	t.Helper()
	exec, err := wfs.CreateDeployment.Start(ctx, in)
	if err != nil {
		return domain.Deployment{}, err
	}
	return exec.AwaitResult(ctx)
}

// TestTargetType is the default target type used by contract tests.
const TestTargetType domain.TargetType = "test"

func registerTargets(ctx context.Context, t *testing.T, infra Infra, ids ...string) {
	t.Helper()
	tx, err := infra.Store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	for _, id := range ids {
		must(t, tx.Targets().Create(ctx, domain.TargetInfo{
			ID:   domain.TargetID(id),
			Type: TestTargetType,
			Name: "cluster-" + id,
		}))
	}
	must(t, tx.Commit())
}

func awaitDeploymentState(ctx context.Context, t *testing.T, infra Infra, id domain.DeploymentID, want domain.DeploymentState) domain.Deployment {
	t.Helper()
	for {
		tx, err := infra.Store.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		dep, err := tx.Deployments().Get(ctx, id)
		tx.Rollback()
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if err == nil && dep.State == want {
			return dep
		}
		select {
		case <-ctx.Done():
			last := domain.DeploymentState("")
			if err == nil {
				last = dep.State
			}
			t.Fatalf("timed out waiting for deployment %s to reach state %q (last: %q)", id, want, last)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func assertResolvedTargets(t *testing.T, dep domain.Deployment, expectedIDs ...string) {
	t.Helper()
	if len(dep.ResolvedTargets) != len(expectedIDs) {
		t.Fatalf("ResolvedTargets: got %d, want %d", len(dep.ResolvedTargets), len(expectedIDs))
	}
	got := make(map[domain.TargetID]bool)
	for _, id := range dep.ResolvedTargets {
		got[id] = true
	}
	for _, id := range expectedIDs {
		if !got[domain.TargetID(id)] {
			t.Errorf("expected target %q in ResolvedTargets", id)
		}
	}
}

func queryDeliveries(ctx context.Context, t *testing.T, infra Infra, depID domain.DeploymentID) []domain.Delivery {
	t.Helper()
	tx, err := infra.Store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	records, err := tx.Deliveries().ListByDeployment(ctx, depID)
	if err != nil {
		t.Fatalf("ListByDeployment: %v", err)
	}
	return records
}

func queryDeployment(ctx context.Context, t *testing.T, infra Infra, id domain.DeploymentID) (domain.Deployment, error) {
	t.Helper()
	tx, err := infra.Store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	return tx.Deployments().Get(ctx, id)
}

func deleteDeploymentAndDeliveries(ctx context.Context, t *testing.T, infra Infra, depID domain.DeploymentID) {
	t.Helper()
	tx, err := infra.Store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	must(t, tx.Deliveries().DeleteByDeployment(ctx, depID))
	must(t, tx.Deployments().Delete(ctx, depID))
	must(t, tx.Commit())
}

func createTargets(ctx context.Context, t *testing.T, infra Infra, targets ...domain.TargetInfo) {
	t.Helper()
	tx, err := infra.Store.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	for _, tgt := range targets {
		must(t, tx.Targets().Create(ctx, tgt))
	}
	must(t, tx.Commit())
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
