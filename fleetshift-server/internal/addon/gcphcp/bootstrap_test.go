package gcphcp_test

import (
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/gcphcp"
)

func TestDeliverySecretRef(t *testing.T) {
	ref := gcphcp.DeliverySecretRef("k8s-test-cluster")
	if ref != "targets/k8s-test-cluster/sa-token" {
		t.Errorf("ref = %q", ref)
	}
}
