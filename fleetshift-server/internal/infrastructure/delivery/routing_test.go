package delivery_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/delivery"
)

type spyAgent struct {
	delivered []domain.DeliverInput
	removed   []domain.RemoveInput
}

func (s *spyAgent) Deliver(_ context.Context, target domain.TargetInfo, deploymentID domain.DeploymentID, manifests []domain.Manifest) (domain.DeliveryResult, error) {
	s.delivered = append(s.delivered, domain.DeliverInput{
		Target:       target,
		DeploymentID: deploymentID,
		Manifests:    manifests,
	})
	return domain.DeliveryResult{State: domain.DeliveryStateDelivered}, nil
}

func (s *spyAgent) Remove(_ context.Context, target domain.TargetInfo, deploymentID domain.DeploymentID) error {
	s.removed = append(s.removed, domain.RemoveInput{
		Target:       target,
		DeploymentID: deploymentID,
	})
	return nil
}

func TestRoutingDeliveryService_RoutesToCorrectAgent(t *testing.T) {
	router := delivery.NewRoutingDeliveryService()

	kindAgent := &spyAgent{}
	k8sAgent := &spyAgent{}
	router.Register("kind", kindAgent)
	router.Register("kubernetes", k8sAgent)

	ctx := context.Background()
	kindTarget := domain.TargetInfo{ID: "k1", Type: "kind", Name: "local-kind"}
	k8sTarget := domain.TargetInfo{ID: "c1", Type: "kubernetes", Name: "prod-cluster"}

	manifests := []domain.Manifest{{Raw: json.RawMessage(`{}`)}}

	if _, err := router.Deliver(ctx, kindTarget, "d1", manifests); err != nil {
		t.Fatalf("Deliver to kind: %v", err)
	}
	if _, err := router.Deliver(ctx, k8sTarget, "d2", manifests); err != nil {
		t.Fatalf("Deliver to kubernetes: %v", err)
	}

	if len(kindAgent.delivered) != 1 {
		t.Fatalf("kindAgent: got %d deliveries, want 1", len(kindAgent.delivered))
	}
	if kindAgent.delivered[0].DeploymentID != "d1" {
		t.Errorf("kindAgent: DeploymentID = %q, want %q", kindAgent.delivered[0].DeploymentID, "d1")
	}

	if len(k8sAgent.delivered) != 1 {
		t.Fatalf("k8sAgent: got %d deliveries, want 1", len(k8sAgent.delivered))
	}
	if k8sAgent.delivered[0].DeploymentID != "d2" {
		t.Errorf("k8sAgent: DeploymentID = %q, want %q", k8sAgent.delivered[0].DeploymentID, "d2")
	}
}

func TestRoutingDeliveryService_RemoveRoutesToCorrectAgent(t *testing.T) {
	router := delivery.NewRoutingDeliveryService()

	agent := &spyAgent{}
	router.Register("kind", agent)

	ctx := context.Background()
	target := domain.TargetInfo{ID: "k1", Type: "kind", Name: "local-kind"}

	if err := router.Remove(ctx, target, "d1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if len(agent.removed) != 1 {
		t.Fatalf("got %d removes, want 1", len(agent.removed))
	}
	if agent.removed[0].DeploymentID != "d1" {
		t.Errorf("DeploymentID = %q, want %q", agent.removed[0].DeploymentID, "d1")
	}
}

func TestRoutingDeliveryService_UnregisteredTypeReturnsError(t *testing.T) {
	router := delivery.NewRoutingDeliveryService()

	ctx := context.Background()
	target := domain.TargetInfo{ID: "k1", Type: "unknown", Name: "target"}

	_, err := router.Deliver(ctx, target, "d1", nil)
	if err == nil {
		t.Fatal("expected error for unregistered target type")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}

	err = router.Remove(ctx, target, "d1")
	if err == nil {
		t.Fatal("expected error for unregistered target type")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}
}

func TestRoutingDeliveryService_RegisterReplacesPrevious(t *testing.T) {
	router := delivery.NewRoutingDeliveryService()

	first := &spyAgent{}
	second := &spyAgent{}
	router.Register("kind", first)
	router.Register("kind", second)

	ctx := context.Background()
	target := domain.TargetInfo{ID: "k1", Type: "kind", Name: "target"}
	manifests := []domain.Manifest{{Raw: json.RawMessage(`{}`)}}

	if _, err := router.Deliver(ctx, target, "d1", manifests); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if len(first.delivered) != 0 {
		t.Errorf("first agent received %d deliveries, want 0", len(first.delivered))
	}
	if len(second.delivered) != 1 {
		t.Errorf("second agent received %d deliveries, want 1", len(second.delivered))
	}
}
