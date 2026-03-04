package kubernetes_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

type stubVault struct {
	secrets map[domain.SecretRef][]byte
}

func (v *stubVault) Put(_ context.Context, ref domain.SecretRef, value []byte) error {
	v.secrets[ref] = value
	return nil
}

func (v *stubVault) Get(_ context.Context, ref domain.SecretRef) ([]byte, error) {
	val, ok := v.secrets[ref]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return val, nil
}

func (v *stubVault) Delete(_ context.Context, ref domain.SecretRef) error {
	delete(v.secrets, ref)
	return nil
}

func TestAgent_Deliver_MissingKubeconfigRef(t *testing.T) {
	vault := &stubVault{secrets: make(map[domain.SecretRef][]byte)}
	agent := kubernetes.NewAgent(vault)

	target := domain.TargetInfo{
		ID:         "k8s-test",
		Type:       kubernetes.TargetType,
		Name:       "test-cluster",
		Properties: map[string]string{},
	}

	result, err := agent.Deliver(context.Background(), target, "d1", nil, &domain.DeliverySignaler{})
	if err == nil {
		t.Fatal("expected error for missing kubeconfig_ref")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Deliver_VaultNotFound(t *testing.T) {
	vault := &stubVault{secrets: make(map[domain.SecretRef][]byte)}
	agent := kubernetes.NewAgent(vault)

	target := domain.TargetInfo{
		ID:   "k8s-test",
		Type: kubernetes.TargetType,
		Name: "test-cluster",
		Properties: map[string]string{
			"kubeconfig_ref": "targets/k8s-test/kubeconfig",
		},
	}

	result, err := agent.Deliver(context.Background(), target, "d1", nil, &domain.DeliverySignaler{})
	if err == nil {
		t.Fatal("expected error for missing vault secret")
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Deliver_InvalidKubeconfig(t *testing.T) {
	vault := &stubVault{secrets: map[domain.SecretRef][]byte{
		"targets/k8s-test/kubeconfig": []byte("not a valid kubeconfig"),
	}}
	agent := kubernetes.NewAgent(vault)

	target := domain.TargetInfo{
		ID:   "k8s-test",
		Type: kubernetes.TargetType,
		Name: "test-cluster",
		Properties: map[string]string{
			"kubeconfig_ref": "targets/k8s-test/kubeconfig",
		},
	}

	manifests := []domain.Manifest{{
		ResourceType: "raw",
		Raw:          json.RawMessage(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test","namespace":"default"},"data":{"key":"value"}}`),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1", manifests, &domain.DeliverySignaler{})
	if err == nil {
		t.Fatal("expected error for invalid kubeconfig")
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Remove_IsNoop(t *testing.T) {
	vault := &stubVault{secrets: make(map[domain.SecretRef][]byte)}
	agent := kubernetes.NewAgent(vault)

	target := domain.TargetInfo{ID: "k8s-test", Type: kubernetes.TargetType, Name: "test-cluster"}
	if err := agent.Remove(context.Background(), target, "d1", &domain.DeliverySignaler{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
