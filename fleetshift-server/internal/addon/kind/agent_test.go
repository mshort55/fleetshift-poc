package kind_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"sigs.k8s.io/kind/pkg/cluster"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kind"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// fakeProvider is a reusable in-memory implementation of
// [kind.ClusterProvider] for tests.
type fakeProvider struct {
	clusters map[string][]byte // name → raw config
	createErr error
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{clusters: make(map[string][]byte)}
}

func (p *fakeProvider) Create(name string, opts ...cluster.CreateOption) error {
	if p.createErr != nil {
		return p.createErr
	}
	p.clusters[name] = nil
	return nil
}

func (p *fakeProvider) Delete(name, _ string) error {
	delete(p.clusters, name)
	return nil
}

func (p *fakeProvider) List() ([]string, error) {
	out := make([]string, 0, len(p.clusters))
	for n := range p.clusters {
		out = append(out, n)
	}
	return out, nil
}

func TestAgent_Deliver_CreatesCluster(t *testing.T) {
	provider := newFakeProvider()
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(`{"name": "dev-cluster"}`),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.State != domain.DeliveryStateDelivered {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateDelivered)
	}
	if _, ok := provider.clusters["dev-cluster"]; !ok {
		t.Error("expected cluster 'dev-cluster' to exist")
	}
}

func TestAgent_Deliver_RecreatesExistingCluster(t *testing.T) {
	provider := newFakeProvider()
	provider.clusters["dev-cluster"] = nil
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(`{"name": "dev-cluster"}`),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.State != domain.DeliveryStateDelivered {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateDelivered)
	}
	if _, ok := provider.clusters["dev-cluster"]; !ok {
		t.Error("expected cluster 'dev-cluster' to exist after recreate")
	}
}

func TestAgent_Deliver_MissingNameReturnsError(t *testing.T) {
	provider := newFakeProvider()
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(`{}`),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests)
	if err == nil {
		t.Fatal("expected error for missing cluster name")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Deliver_CreateFailureReturnsError(t *testing.T) {
	provider := newFakeProvider()
	provider.createErr = errors.New("docker not available")
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(`{"name": "dev-cluster"}`),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests)
	if err == nil {
		t.Fatal("expected error when provider.Create fails")
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Remove_IsNoopForNow(t *testing.T) {
	provider := newFakeProvider()
	provider.clusters["dev-cluster"] = nil
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	if err := agent.Remove(context.Background(), target, "d1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestAgent_Deliver_MultipleManifests(t *testing.T) {
	provider := newFakeProvider()
	agent := kind.NewAgent(provider)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{
		{ResourceType: kind.ClusterResourceType, Raw: json.RawMessage(`{"name": "cluster-a"}`)},
		{ResourceType: kind.ClusterResourceType, Raw: json.RawMessage(`{"name": "cluster-b"}`)},
	}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.State != domain.DeliveryStateDelivered {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateDelivered)
	}
	if len(provider.clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(provider.clusters))
	}
}
