//go:build integration

package kubernetes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	kindlog "sigs.k8s.io/kind/pkg/log"

	kindaddon "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kind"
	kubeaddon "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/testharness"
)

const clusterName = "fleetshift-k8s-e2e"

var fixture *clusterFixture

type clusterFixture struct {
	apiServer      string
	caCert         string
	saToken        string
	adminRestCfg   *rest.Config
	adminDynClient dynamic.Interface
	adminK8s       *kubernetes.Clientset
}

func TestMain(m *testing.M) {
	provider := cluster.NewProvider()
	if _, err := provider.List(); err != nil {
		fmt.Fprintf(os.Stderr, "container runtime not available, skipping: %v\n", err)
		os.Exit(0)
	}

	// Clean up stale cluster from previous runs.
	_ = provider.Delete(clusterName, "")

	// Create cluster via the kind addon's delivery pipeline.
	reporter := &channelReporter{done: make(chan domain.DeliveryResult, 1)}
	kindAgent := kindaddon.NewAgent(reporter, func(logger kindlog.Logger) kindaddon.ClusterProvider {
		return cluster.NewProvider(cluster.ProviderWithLogger(logger))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:   "kind-e2e-setup",
		Type: kindaddon.TargetType,
	})
	manifests := []domain.Manifest{{
		ResourceType: kindaddon.ClusterResourceType,
		Raw:          json.RawMessage(`{"name":"` + clusterName + `"}`),
	}}

	if err := kindAgent.Deliver(ctx, target, "e2e-setup", manifests, domain.DeliveryAuth{}, nil, 1); err != nil {
		fmt.Fprintf(os.Stderr, "kind Deliver: %v\n", err)
		os.Exit(1)
	}

	var result domain.DeliveryResult
	select {
	case result = <-reporter.done:
		if result.State != domain.DeliveryStateDelivered {
			fmt.Fprintf(os.Stderr, "kind delivery state = %q: %s\n", result.State, result.Message)
			_ = provider.Delete(clusterName, "")
			os.Exit(1)
		}
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "timed out waiting for kind cluster")
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}

	// Extract target properties from provisioned target.
	if len(result.ProvisionedTargets) == 0 || len(result.ProducedSecrets) == 0 {
		fmt.Fprintln(os.Stderr, "kind delivery missing provisioned targets or secrets")
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}
	pt := result.ProvisionedTargets[0]

	// Get admin kubeconfig for direct cluster access.
	kcStr, err := provider.KubeConfig(clusterName, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "KubeConfig: %v\n", err)
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}
	adminCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kcStr))
	if err != nil {
		fmt.Fprintf(os.Stderr, "RESTConfigFromKubeConfig: %v\n", err)
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}

	dynClient, err := dynamic.NewForConfig(adminCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dynamic client: %v\n", err)
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}

	adminK8s, err := kubernetes.NewForConfig(adminCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubernetes client: %v\n", err)
		_ = provider.Delete(clusterName, "")
		os.Exit(1)
	}

	fixture = &clusterFixture{
		apiServer:      pt.Properties["api_server"],
		caCert:         pt.Properties["ca_cert"],
		saToken:        string(result.ProducedSecrets[0].Value),
		adminRestCfg:   adminCfg,
		adminDynClient: dynClient,
		adminK8s:       adminK8s,
	}

	code := m.Run()
	_ = provider.Delete(clusterName, "")
	os.Exit(code)
}

// channelReporter implements [domain.DeliveryReporter] for TestMain.
type channelReporter struct {
	done chan domain.DeliveryResult
}

func (r *channelReporter) ReportEvent(_ context.Context, _ domain.DeliveryID, _ domain.Generation, _ domain.DeliveryEvent) error {
	return nil
}

func (r *channelReporter) ReportResult(_ context.Context, _ domain.DeliveryID, _ domain.Generation, result domain.DeliveryResult) error {
	r.done <- result
	return nil
}

func (r *channelReporter) ListActiveDeliveries(_ context.Context, _ []domain.TargetID) ([]domain.ActiveDelivery, error) {
	return nil, nil
}

type e2eFixture struct {
	harness   *testharness.Harness
	k8sMgr    *kubeaddon.Manager
	dynClient dynamic.Interface
	typedK8s  *kubernetes.Clientset
	namespace string
	targetID  domain.TargetID
}

func setupE2E(t *testing.T) *e2eFixture {
	t.Helper()

	h := testharness.New(t)
	inventoryWriter := application.NewInventoryWriteService(h.Store)

	k8sMgr := kubeaddon.NewManager(
		h.Store,
		nil, // vault — SA token is passed directly
		inventoryWriter,
		h.Reporter,
		nil, // keyResolver
		nil, // httpClient
		slog.Default(),
	)

	ctx := context.Background()
	targetID := domain.TargetID("k8s-e2e")

	if err := h.AddonMgr.Enable(ctx, kubeaddon.Descriptor()); err != nil {
		t.Fatalf("Enable kubernetes addon: %v", err)
	}
	if err := h.AddonMgr.Connect(ctx, "kubernetes", application.ConnectInput{
		Agent: k8sMgr,
	}); err != nil {
		t.Fatalf("Connect kubernetes addon: %v", err)
	}

	target := domain.TargetInfoFromSnapshot(domain.TargetInfoSnapshot{
		ID:   targetID,
		Type: kubeaddon.TargetType,
		Name: "E2E Kind Cluster",
		Properties: map[string]string{
			"api_server":            fixture.apiServer,
			"ca_cert":               fixture.caCert,
			"service_account_token": fixture.saToken,
		},
	})

	if err := h.Targets.Register(ctx, target); err != nil {
		t.Fatalf("Register target: %v", err)
	}
	if err := k8sMgr.HandleTargetReady(ctx, target); err != nil {
		t.Fatalf("HandleTargetReady: %v", err)
	}

	// Create a test-specific namespace.
	ns := "e2e-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	if len(ns) > 63 {
		ns = ns[:63]
	}
	nsObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]any{"name": ns},
		},
	}
	nsGVR := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	_, err := fixture.adminDynClient.Resource(nsGVR).Create(ctx, nsObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create namespace %s: %v", ns, err)
	}

	t.Cleanup(func() {
		k8sMgr.StopAll()
		_ = fixture.adminDynClient.Resource(nsGVR).Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	return &e2eFixture{
		harness:   h,
		k8sMgr:    k8sMgr,
		dynClient: fixture.adminDynClient,
		typedK8s:  fixture.adminK8s,
		namespace: ns,
		targetID:  targetID,
	}
}

// awaitInventoryMatch polls until the predicate returns true.
func awaitInventoryMatch(t *testing.T, store domain.Store, predicate func([]domain.InventoryItem) bool, timeout time.Duration) []domain.InventoryItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		tx, err := store.BeginReadOnly(context.Background())
		if err != nil {
			t.Fatalf("BeginReadOnly: %v", err)
		}
		items, err := tx.Inventory().List(context.Background())
		tx.Rollback()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if predicate(items) {
			return items
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for inventory match (%d items)", len(items))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// awaitNoInventoryItem polls until the item no longer exists.
func awaitNoInventoryItem(t *testing.T, store domain.Store, id domain.InventoryItemID, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		tx, err := store.BeginReadOnly(context.Background())
		if err != nil {
			t.Fatalf("BeginReadOnly: %v", err)
		}
		_, err = tx.Inventory().Get(context.Background(), id)
		tx.Rollback()
		if err != nil {
			return // item gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for item %s to disappear", id)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// queryEdgesFrom returns outgoing edges from a source UID.
func queryEdgesFrom(t *testing.T, store domain.Store, targetID domain.TargetID, sourceUID string) []domain.InventoryEdge {
	t.Helper()
	tx, err := store.BeginReadOnly(context.Background())
	if err != nil {
		t.Fatalf("BeginReadOnly: %v", err)
	}
	defer tx.Rollback()
	edges, err := tx.Edges().ListBySourceUID(context.Background(), targetID, sourceUID)
	if err != nil {
		t.Fatalf("ListBySourceUID: %v", err)
	}
	return edges
}

// queryEdgesTo returns incoming edges to a destination UID.
func queryEdgesTo(t *testing.T, store domain.Store, targetID domain.TargetID, destUID string) []domain.InventoryEdge {
	t.Helper()
	tx, err := store.BeginReadOnly(context.Background())
	if err != nil {
		t.Fatalf("BeginReadOnly: %v", err)
	}
	defer tx.Rollback()
	edges, err := tx.Edges().ListByDestUID(context.Background(), targetID, destUID)
	if err != nil {
		t.Fatalf("ListByDestUID: %v", err)
	}
	return edges
}
