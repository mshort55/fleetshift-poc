package kind_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"sigs.k8s.io/kind/pkg/cluster"

	kindaddon "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kind"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/delivery"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/syncworkflow"
)

// TestKindAddon_RealDocker exercises the full addon lifecycle against
// a real Docker daemon. It creates a kind cluster, verifies it exists,
// then tears it down. Skipped when Docker is not available or when
// running with -short.
func TestKindAddon_RealDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real Docker test in short mode")
	}

	provider := cluster.NewProvider()

	if _, err := provider.List(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	const clusterName = "fleetshift-test"

	t.Cleanup(func() {
		_ = provider.Delete(clusterName, "")
	})
	// Pre-clean in case a previous run left a stale cluster.
	_ = provider.Delete(clusterName, "")

	kindAgent := kindaddon.NewAgent(provider)
	router := delivery.NewRoutingDeliveryService()
	router.Register(kindaddon.TargetType, kindAgent)

	db := sqlite.OpenTestDB(t)
	targetRepo := &sqlite.TargetRepo{DB: db}
	deploymentRepo := &sqlite.DeploymentRepo{DB: db}
	recordRepo := &sqlite.DeliveryRecordRepo{DB: db}

	owf := &domain.OrchestrationWorkflow{
		Deployments: deploymentRepo,
		Targets:     targetRepo,
		Delivery:    router,
		Strategies:  domain.DefaultStrategyFactory{},
	}
	cwf := &domain.CreateDeploymentWorkflow{Deployments: deploymentRepo}

	engine := &syncworkflow.Engine{}
	runners, err := engine.Register(owf, cwf)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	targetSvc := &application.TargetService{Targets: targetRepo}
	deploySvc := &application.DeploymentService{
		Deployments:   deploymentRepo,
		Records:       recordRepo,
		CreateWF:      runners.CreateDeployment,
		Orchestration: &application.OrchestrationService{Workflow: runners.Orchestration},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := targetSvc.Register(ctx, domain.TargetInfo{
		ID:   "kind-docker",
		Type: kindaddon.TargetType,
		Name: "Docker Kind Provider",
	}); err != nil {
		t.Fatalf("Register target: %v", err)
	}

	spec := kindaddon.ClusterSpec{Name: clusterName}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	_, err = deploySvc.Create(ctx, domain.CreateDeploymentInput{
		ID: "kind-docker-deploy",
		ManifestStrategy: domain.ManifestStrategySpec{
			Type: domain.ManifestStrategyInline,
			Manifests: []domain.Manifest{{
				ResourceType: kindaddon.ClusterResourceType,
				Raw:          json.RawMessage(specBytes),
			}},
		},
		PlacementStrategy: domain.PlacementStrategySpec{
			Type:    domain.PlacementStrategyStatic,
			Targets: []domain.TargetID{"kind-docker"},
		},
	})
	if err != nil {
		t.Fatalf("Create deployment: %v", err)
	}

	dep := awaitState(ctx, t, deploymentRepo, "kind-docker-deploy", domain.DeploymentStateActive)
	if len(dep.ResolvedTargets) != 1 || dep.ResolvedTargets[0] != "kind-docker" {
		t.Fatalf("unexpected ResolvedTargets: %v", dep.ResolvedTargets)
	}

	clusters, err := provider.List()
	if err != nil {
		t.Fatalf("provider.List: %v", err)
	}
	found := slices.Contains(clusters, clusterName)
	if !found {
		t.Fatalf("kind cluster %q not found after delivery; clusters: %v", clusterName, clusters)
	}
}
