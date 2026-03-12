package kind_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/log"

	kindaddon "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kind"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/oidc/oidctest"
)

// TestKindAddon_OIDCAuth creates a kind cluster with OIDC authentication
// configured via the kind delivery agent, then verifies that JWTs from
// the fake OIDC provider are accepted by the K8s API server.
//
// Requires Docker or Podman (skipped when unavailable or -short).
// Requires host.docker.internal to resolve inside the kind container
// (Docker Desktop on macOS/Windows, or Podman with user-mode networking).
func TestKindAddon_OIDCAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real Docker test in short mode")
	}

	checker := cluster.NewProvider()
	if _, err := checker.List(); err != nil {
		t.Skipf("container runtime not available: %v", err)
	}

	const clusterName = "fleetshift-oidc-test"

	t.Cleanup(func() { _ = checker.Delete(clusterName, "") })
	_ = checker.Delete(clusterName, "")

	idp := oidctest.Start(t,
		oidctest.WithListenAddress("0.0.0.0:0"),
		oidctest.WithAudience("fleetshift"),
	)
	dockerIssuer := fmt.Sprintf("https://host.docker.internal:%s", idp.Port())
	idp.SetIssuerURL(dockerIssuer)

	kindAgent := kindaddon.NewAgent(func(logger log.Logger) kindaddon.ClusterProvider {
		return cluster.NewProvider(cluster.ProviderWithLogger(logger))
	})
	kindAgent.TempDir = t.TempDir()

	spec := kindaddon.ClusterSpec{
		Name: clusterName,
		OIDC: &kindaddon.OIDCSpec{
			IssuerURL: dockerIssuer,
			ClientID:  "fleetshift",
			CABundle:  idp.CACertPEM(),
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	obs := newChannelDeliveryObserver()
	signaler := newChannelSignaler(obs)

	target := domain.TargetInfo{ID: "oidc-kind", Type: kindaddon.TargetType, Name: "OIDC Kind"}
	manifests := []domain.Manifest{{
		ResourceType: kindaddon.ClusterResourceType,
		Raw:          json.RawMessage(specBytes),
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := kindAgent.Deliver(ctx, target, "d1:oidc-kind", manifests, signaler)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.State != domain.DeliveryStateAccepted {
		t.Fatalf("State = %q, want %q", result.State, domain.DeliveryStateAccepted)
	}

	select {
	case doneResult := <-obs.done:
		if doneResult.State != domain.DeliveryStateDelivered {
			t.Fatalf("delivery State = %q, want %q (message: %s)", doneResult.State, domain.DeliveryStateDelivered, doneResult.Message)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for delivery to complete")
	}

	kcStr, err := checker.KubeConfig(clusterName, false)
	if err != nil {
		t.Fatalf("KubeConfig: %v", err)
	}

	token := idp.IssueToken(t, oidctest.TokenClaims{
		Subject: "alice",
		Groups:  []string{"developers"},
	})

	restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kcStr))
	if err != nil {
		t.Fatalf("parse kubeconfig: %v", err)
	}
	restCfg.BearerToken = token
	restCfg.Username = ""
	restCfg.Password = ""
	restCfg.CertData = nil
	restCfg.KeyData = nil
	restCfg.CertFile = ""
	restCfg.KeyFile = ""

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}

	ssr, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("SelfSubjectReview: %v", err)
	}

	// K8s OIDC authentication prefixes the issuer URL to the subject
	// claim: "issuer#sub". See https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens
	wantUsername := dockerIssuer + "#alice"
	if ssr.Status.UserInfo.Username != wantUsername {
		t.Errorf("Username = %q, want %q", ssr.Status.UserInfo.Username, wantUsername)
	}

	foundGroup := false
	for _, g := range ssr.Status.UserInfo.Groups {
		if g == "developers" {
			foundGroup = true
			break
		}
	}
	if !foundGroup {
		t.Errorf("Groups = %v, expected to contain %q", ssr.Status.UserInfo.Groups, "developers")
	}

	expiredToken := idp.IssueToken(t, oidctest.TokenClaims{
		Subject: "alice",
		Expiry:  -time.Hour,
	})
	restCfg.BearerToken = expiredToken
	expiredClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("NewForConfig (expired): %v", err)
	}

	_, err = expiredClient.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}
