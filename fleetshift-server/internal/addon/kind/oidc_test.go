package kind_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kind"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func TestBuildKindOIDCConfig_BasicFlags(t *testing.T) {
	spec := &kind.OIDCSpec{
		IssuerURL: "https://idp.example.com",
		ClientID:  "fleetshift",
	}

	cfg, err := kind.BuildKindOIDCConfig(spec, "")
	if err != nil {
		t.Fatalf("buildKindOIDCConfig: %v", err)
	}

	s := string(cfg)
	assertContains(t, s, "kind: Cluster")
	assertContains(t, s, "apiVersion: kind.x-k8s.io/v1alpha4")
	assertContains(t, s, `oidc-issuer-url: "https://idp.example.com"`)
	assertContains(t, s, `oidc-client-id: "fleetshift"`)
	assertContains(t, s, `oidc-username-claim: "sub"`)
	assertContains(t, s, `oidc-groups-claim: "groups"`)

	if strings.Contains(s, "oidc-ca-file") {
		t.Error("oidc-ca-file should not be present without CA bundle")
	}
	if strings.Contains(s, "extraMounts") {
		t.Error("extraMounts should not be present without CA bundle")
	}
}

func TestBuildKindOIDCConfig_WithCABundle(t *testing.T) {
	spec := &kind.OIDCSpec{
		IssuerURL: "https://host.docker.internal:9443",
		ClientID:  "fleetshift",
		CABundle:  []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----"),
	}

	cfg, err := kind.BuildKindOIDCConfig(spec, "/tmp/ca.pem")
	if err != nil {
		t.Fatalf("buildKindOIDCConfig: %v", err)
	}

	s := string(cfg)
	assertContains(t, s, `oidc-ca-file: "/etc/kubernetes/pki/oidc-ca.pem"`)
	assertContains(t, s, "extraMounts:")
	assertContains(t, s, "hostPath: /tmp/ca.pem")
	assertContains(t, s, "containerPath: /etc/kubernetes/pki/oidc-ca.pem")
}

func TestBuildKindOIDCConfig_CustomClaims(t *testing.T) {
	spec := &kind.OIDCSpec{
		IssuerURL:     "https://idp.example.com",
		ClientID:      "my-app",
		UsernameClaim: "email",
		GroupsClaim:   "roles",
	}

	cfg, err := kind.BuildKindOIDCConfig(spec, "")
	if err != nil {
		t.Fatalf("buildKindOIDCConfig: %v", err)
	}

	s := string(cfg)
	assertContains(t, s, `oidc-username-claim: "email"`)
	assertContains(t, s, `oidc-groups-claim: "roles"`)
}

func TestBuildKindOIDCConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		spec *kind.OIDCSpec
	}{
		{"missing issuer", &kind.OIDCSpec{ClientID: "x"}},
		{"missing clientID", &kind.OIDCSpec{IssuerURL: "https://x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kind.BuildKindOIDCConfig(tt.spec, "")
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestAgent_Deliver_OIDCSpec(t *testing.T) {
	provider := newFakeProvider()
	obs := newChannelDeliveryObserver()
	signaler := newChannelSignaler(obs)
	agent := kind.NewAgent(fakeFactory(provider))

	spec := kind.ClusterSpec{
		Name: "oidc-cluster",
		OIDC: &kind.OIDCSpec{
			IssuerURL: "https://host.docker.internal:9443",
			ClientID:  "fleetshift",
			CABundle:  []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----"),
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(specBytes),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1:k1", manifests, signaler)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.State != domain.DeliveryStateAccepted {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateAccepted)
	}

	doneResult := <-obs.done
	if doneResult.State != domain.DeliveryStateDelivered {
		t.Fatalf("done State = %q, want %q", doneResult.State, domain.DeliveryStateDelivered)
	}

	if !provider.hasCluster("oidc-cluster") {
		t.Error("expected cluster 'oidc-cluster' to be created")
	}
}

func TestAgent_Deliver_OIDCAndConfigMutuallyExclusive(t *testing.T) {
	provider := newFakeProvider()
	agent := kind.NewAgent(fakeFactory(provider))

	spec := kind.ClusterSpec{
		Name:   "bad-cluster",
		Config: json.RawMessage(`{"kind":"Cluster"}`),
		OIDC:   &kind.OIDCSpec{IssuerURL: "https://x", ClientID: "y"},
	}
	specBytes, _ := json.Marshal(spec)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(specBytes),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1:k1", manifests, nop)
	if err == nil {
		t.Fatal("expected error for config + oidc")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func TestAgent_Deliver_OIDCMissingIssuerURL(t *testing.T) {
	provider := newFakeProvider()
	agent := kind.NewAgent(fakeFactory(provider))

	spec := kind.ClusterSpec{
		Name: "bad-cluster",
		OIDC: &kind.OIDCSpec{ClientID: "y"},
	}
	specBytes, _ := json.Marshal(spec)

	target := domain.TargetInfo{ID: "k1", Type: kind.TargetType, Name: "local-kind"}
	manifests := []domain.Manifest{{
		ResourceType: kind.ClusterResourceType,
		Raw:          json.RawMessage(specBytes),
	}}

	result, err := agent.Deliver(context.Background(), target, "d1:k1", manifests, nop)
	if err == nil {
		t.Fatal("expected error for missing issuerURL")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got: %v", err)
	}
	if result.State != domain.DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, domain.DeliveryStateFailed)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to contain %q, got:\n%s", needle, haystack)
	}
}
