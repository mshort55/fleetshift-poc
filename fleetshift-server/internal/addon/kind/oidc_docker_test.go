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

// oidcClusterResult holds the outputs of createOIDCCluster.
type oidcClusterResult struct {
	ClusterName string
	IDP         *oidctest.Provider
	IssuerURL   domain.IssuerURL
	Kubeconfig  string
}

// createOIDCCluster creates a kind cluster with OIDC trust derived from
// the caller's identity. It starts a fake OIDC provider, delivers the
// cluster via the kind agent, and returns the kubeconfig and provider.
func createOIDCCluster(t *testing.T, clusterName string, auth domain.DeliveryAuth) oidcClusterResult {
	t.Helper()

	checker := cluster.NewProvider()
	if _, err := checker.List(); err != nil {
		t.Skipf("container runtime not available: %v", err)
	}

	t.Cleanup(func() { _ = checker.Delete(clusterName, "") })
	_ = checker.Delete(clusterName, "")

	idp := oidctest.Start(t,
		oidctest.WithListenAddress("0.0.0.0:0"),
		oidctest.WithAudience(string(auth.Audience[0])),
	)
	dockerIssuer := domain.IssuerURL(fmt.Sprintf("https://host.docker.internal:%s", idp.Port()))
	idp.SetIssuerURL(dockerIssuer)

	auth.Caller.Issuer = dockerIssuer

	kindAgent := kindaddon.NewAgent(func(logger log.Logger) kindaddon.ClusterProvider {
		return cluster.NewProvider(cluster.ProviderWithLogger(logger))
	},
		kindaddon.WithTempDir(t.TempDir()),
		kindaddon.WithOIDCCABundle(idp.CACertPEM()),
	)

	spec := kindaddon.ClusterSpec{
		Name: clusterName,
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

	result, err := kindAgent.Deliver(ctx, target, "d1:oidc-kind", manifests, auth, signaler)
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

	return oidcClusterResult{
		ClusterName: clusterName,
		IDP:         idp,
		IssuerURL:   dockerIssuer,
		Kubeconfig:  kcStr,
	}
}

// oidcK8sClient builds a K8s client using the kubeconfig and a bearer token.
func oidcK8sClient(t *testing.T, kubeconfig, token string) *kubernetes.Clientset {
	t.Helper()
	restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
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
	return client
}

// TestKindAddon_OIDCAuth creates a kind cluster with OIDC authentication
// derived from the caller's identity, then verifies that JWTs from the
// fake OIDC provider are accepted by the K8s API server.
//
// Requires Docker or Podman (skipped when unavailable or -short).
func TestKindAddon_OIDCAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real Docker test in short mode")
	}

	auth := domain.DeliveryAuth{
		Caller:   &domain.SubjectClaims{ID: "alice"},
		Audience: []domain.Audience{"fleetshift"},
	}
	res := createOIDCCluster(t, "fleetshift-oidc-test", auth)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token := res.IDP.IssueToken(t, oidctest.TokenClaims{
		Subject: "alice",
		Groups:  []string{"developers"},
	})

	client := oidcK8sClient(t, res.Kubeconfig, token)

	ssr, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("SelfSubjectReview: %v", err)
	}

	wantUsername := string(res.IssuerURL) + "#alice"
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

	expiredToken := res.IDP.IssueToken(t, oidctest.TokenClaims{
		Subject: "alice",
		Expiry:  -time.Hour,
	})
	expiredClient := oidcK8sClient(t, res.Kubeconfig, expiredToken)
	_, err = expiredClient.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

// TestKindAddon_OIDCAuthWithRBAC verifies that the RBAC bootstrap
// grants the caller cluster-admin. Alice (the caller) can list
// namespaces; bob (not bootstrapped) gets 403.
//
// Requires Docker or Podman (skipped when unavailable or -short).
func TestKindAddon_OIDCAuthWithRBAC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real Docker test in short mode")
	}

	auth := domain.DeliveryAuth{
		Caller:   &domain.SubjectClaims{ID: "alice"},
		Audience: []domain.Audience{"fleetshift"},
	}
	res := createOIDCCluster(t, "fleetshift-rbac-test", auth)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	aliceToken := res.IDP.IssueToken(t, oidctest.TokenClaims{Subject: "alice"})
	aliceClient := oidcK8sClient(t, res.Kubeconfig, aliceToken)

	nsList, err := aliceClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("alice Namespaces().List: %v", err)
	}
	if len(nsList.Items) == 0 {
		t.Error("expected at least one namespace")
	}

	bobToken := res.IDP.IssueToken(t, oidctest.TokenClaims{Subject: "bob"})
	bobClient := oidcK8sClient(t, res.Kubeconfig, bobToken)

	_, err = bobClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		t.Error("expected bob to be forbidden, got nil error")
	}
}
