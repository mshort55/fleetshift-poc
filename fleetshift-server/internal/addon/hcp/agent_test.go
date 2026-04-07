package hcp

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func makeManifest(t *testing.T, spec ClusterSpec) domain.Manifest {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Manifest{
		ResourceType: ClusterResourceType,
		Raw:          raw,
	}
}

func TestValidateManifests_ValidMinimalSpec(t *testing.T) {
	spec := ClusterSpec{
		Name:    "test-cluster",
		RoleARN: "arn:aws:iam::123456789012:role/test",
		NodePools: []NodePoolSpec{
			{Name: "default", Replicas: 2},
		},
	}
	manifests := []domain.Manifest{makeManifest(t, spec)}

	specs, err := validateManifests(manifests)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d specs, want 1", len(specs))
	}

	got := specs[0]
	if got.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", got.Name, "test-cluster")
	}
	if got.RoleARN != "arn:aws:iam::123456789012:role/test" {
		t.Errorf("RoleARN = %q, want correct ARN", got.RoleARN)
	}
}

func TestValidateManifests_DefaultsApplied(t *testing.T) {
	spec := ClusterSpec{
		Name:    "test-cluster",
		RoleARN: "arn:aws:iam::123456789012:role/test",
		NodePools: []NodePoolSpec{
			{Name: "default", Replicas: 2},
		},
	}
	manifests := []domain.Manifest{makeManifest(t, spec)}

	specs, err := validateManifests(manifests)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := specs[0]
	if got.NodePools[0].InstanceType != "m6i.xlarge" {
		t.Errorf("InstanceType = %q, want %q", got.NodePools[0].InstanceType, "m6i.xlarge")
	}
	if got.NodePools[0].Arch != "amd64" {
		t.Errorf("Arch = %q, want %q", got.NodePools[0].Arch, "amd64")
	}
	if got.ControlPlaneAvailability != "HighlyAvailable" {
		t.Errorf("ControlPlaneAvailability = %q, want %q", got.ControlPlaneAvailability, "HighlyAvailable")
	}
}

func TestValidateManifests_MissingName(t *testing.T) {
	spec := ClusterSpec{
		RoleARN: "arn:aws:iam::123456789012:role/test",
		NodePools: []NodePoolSpec{
			{Name: "default", Replicas: 2},
		},
	}
	manifests := []domain.Manifest{makeManifest(t, spec)}

	_, err := validateManifests(manifests)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("got %v, want ErrInvalidArgument", err)
	}
}

func TestValidateManifests_MissingRoleARN(t *testing.T) {
	spec := ClusterSpec{
		Name: "test-cluster",
		NodePools: []NodePoolSpec{
			{Name: "default", Replicas: 2},
		},
	}
	manifests := []domain.Manifest{makeManifest(t, spec)}

	_, err := validateManifests(manifests)
	if err == nil {
		t.Fatal("expected error for missing roleArn")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("got %v, want ErrInvalidArgument", err)
	}
}

func TestValidateManifests_EmptyNodePools(t *testing.T) {
	spec := ClusterSpec{
		Name:    "test-cluster",
		RoleARN: "arn:aws:iam::123456789012:role/test",
	}
	manifests := []domain.Manifest{makeManifest(t, spec)}

	_, err := validateManifests(manifests)
	if err == nil {
		t.Fatal("expected error for empty nodePools")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("got %v, want ErrInvalidArgument", err)
	}
}
