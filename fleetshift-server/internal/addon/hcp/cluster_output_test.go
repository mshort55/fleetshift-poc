package hcp

import (
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func TestClusterOutput_Target(t *testing.T) {
	out := ClusterOutput{
		TargetID:   "k8s-test",
		Name:       "test-cluster",
		APIServer:  "https://api.test.example.com:6443",
		CACert:     []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
		SATokenRef: "vault:sa-token-test",
	}

	target := out.Target()

	if target.ID != "k8s-test" {
		t.Errorf("ID = %q, want %q", target.ID, "k8s-test")
	}
	if target.Type != KubernetesTargetType {
		t.Errorf("Type = %q, want %q", target.Type, KubernetesTargetType)
	}
	if target.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", target.Name, "test-cluster")
	}
	if target.Properties["api_server"] != "https://api.test.example.com:6443" {
		t.Errorf("api_server = %q, want correct value", target.Properties["api_server"])
	}
	if target.Properties["ca_cert"] != string(out.CACert) {
		t.Errorf("ca_cert not set correctly")
	}
	if target.Properties["service_account_token_ref"] != "vault:sa-token-test" {
		t.Errorf("service_account_token_ref = %q, want %q", target.Properties["service_account_token_ref"], "vault:sa-token-test")
	}
	if len(target.AcceptedResourceTypes) != 1 || target.AcceptedResourceTypes[0] != kubernetes.ManifestResourceType {
		t.Errorf("AcceptedResourceTypes = %v, want [%q]", target.AcceptedResourceTypes, kubernetes.ManifestResourceType)
	}
}

func TestClusterOutput_Secrets_WithSA(t *testing.T) {
	out := ClusterOutput{
		TargetID:   "k8s-test",
		Name:       "test-cluster",
		SATokenRef: domain.SecretRef("vault:sa-token-test"),
		SAToken:    []byte("bearer-token-value"),
	}

	secrets := out.Secrets()

	if len(secrets) != 1 {
		t.Fatalf("got %d secrets, want 1", len(secrets))
	}
	if secrets[0].Ref != "vault:sa-token-test" {
		t.Errorf("Ref = %q, want %q", secrets[0].Ref, "vault:sa-token-test")
	}
	if string(secrets[0].Value) != "bearer-token-value" {
		t.Errorf("Value = %q, want %q", string(secrets[0].Value), "bearer-token-value")
	}
}

func TestClusterOutput_Secrets_NoSA(t *testing.T) {
	out := ClusterOutput{
		TargetID: "k8s-test",
		Name:     "test-cluster",
	}

	secrets := out.Secrets()

	if secrets != nil {
		t.Errorf("got %v, want nil", secrets)
	}
}
