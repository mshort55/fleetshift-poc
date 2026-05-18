package gcphcp_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/gcphcp"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func boolPtr(b bool) *bool { return &b }

func fullSpecJSON() string {
	return `{
		"name": "test-cluster",
		"endpointAccess": "PublicAndPrivate",
		"releaseVersion": "4.18.0",
		"channelGroup": "stable",
		"nodepools": [{
			"name": "worker-pool",
			"replicas": 2,
			"instanceType": "n1-standard-4",
			"rootVolumeSize": 128,
			"rootVolumeType": "pd-standard",
			"autoRepair": true,
			"upgradeType": "Replace"
		}]
	}`
}

func TestParseClusterSpec_ValidFullSpec(t *testing.T) {
	raw := json.RawMessage(fullSpecJSON())

	spec, err := gcphcp.ParseClusterSpec(raw)
	if err != nil {
		t.Fatalf("ParseClusterSpec failed: %v", err)
	}

	if spec.Name != "test-cluster" {
		t.Errorf("expected Name=test-cluster, got %s", spec.Name)
	}
	if spec.EndpointAccess != "PublicAndPrivate" {
		t.Errorf("expected EndpointAccess=PublicAndPrivate, got %s", spec.EndpointAccess)
	}
	if spec.ReleaseVersion != "4.18.0" {
		t.Errorf("expected ReleaseVersion=4.18.0, got %s", spec.ReleaseVersion)
	}
	if spec.ChannelGroup != "stable" {
		t.Errorf("expected ChannelGroup=stable, got %s", spec.ChannelGroup)
	}

	if len(spec.Nodepools) != 1 {
		t.Fatalf("expected 1 nodepool, got %d", len(spec.Nodepools))
	}

	np := spec.Nodepools[0]
	if np.Name != "worker-pool" {
		t.Errorf("expected nodepool Name=worker-pool, got %s", np.Name)
	}
	if np.Replicas != 2 {
		t.Errorf("expected nodepool Replicas=2, got %d", np.Replicas)
	}
	if np.InstanceType != "n1-standard-4" {
		t.Errorf("expected nodepool InstanceType=n1-standard-4, got %s", np.InstanceType)
	}
	if np.RootVolumeSize != 128 {
		t.Errorf("expected nodepool RootVolumeSize=128, got %d", np.RootVolumeSize)
	}
	if np.RootVolumeType != "pd-standard" {
		t.Errorf("expected nodepool RootVolumeType=pd-standard, got %s", np.RootVolumeType)
	}
	if np.AutoRepair == nil || *np.AutoRepair != true {
		t.Errorf("expected nodepool AutoRepair=true, got %v", np.AutoRepair)
	}
	if np.UpgradeType != "Replace" {
		t.Errorf("expected nodepool UpgradeType=Replace, got %s", np.UpgradeType)
	}
}

func TestParseClusterSpec_AllFieldsExplicit(t *testing.T) {
	raw := json.RawMessage(`{
		"name": "prod-cluster",
		"endpointAccess": "Private",
		"releaseVersion": "4.14.0",
		"channelGroup": "fast",
		"nodepools": [
			{
				"name": "worker-pool",
				"replicas": 3,
				"instanceType": "n1-standard-8",
				"rootVolumeSize": 256,
				"rootVolumeType": "pd-ssd",
				"autoRepair": false,
				"upgradeType": "InPlace"
			}
		]
	}`)

	spec, err := gcphcp.ParseClusterSpec(raw)
	if err != nil {
		t.Fatalf("ParseClusterSpec failed: %v", err)
	}

	if spec.Name != "prod-cluster" {
		t.Errorf("expected Name=prod-cluster, got %s", spec.Name)
	}
	if spec.EndpointAccess != "Private" {
		t.Errorf("expected EndpointAccess=Private, got %s", spec.EndpointAccess)
	}
	if spec.ReleaseVersion != "4.14.0" {
		t.Errorf("expected ReleaseVersion=4.14.0, got %s", spec.ReleaseVersion)
	}
	if spec.ChannelGroup != "fast" {
		t.Errorf("expected ChannelGroup=fast, got %s", spec.ChannelGroup)
	}

	np := spec.Nodepools[0]
	if np.Name != "worker-pool" {
		t.Errorf("expected nodepool Name=worker-pool, got %s", np.Name)
	}
	if np.Replicas != 3 {
		t.Errorf("expected nodepool Replicas=3, got %d", np.Replicas)
	}
	if np.InstanceType != "n1-standard-8" {
		t.Errorf("expected nodepool InstanceType=n1-standard-8, got %s", np.InstanceType)
	}
	if np.RootVolumeSize != 256 {
		t.Errorf("expected nodepool RootVolumeSize=256, got %d", np.RootVolumeSize)
	}
	if np.RootVolumeType != "pd-ssd" {
		t.Errorf("expected nodepool RootVolumeType=pd-ssd, got %s", np.RootVolumeType)
	}
	if np.AutoRepair == nil || *np.AutoRepair != false {
		t.Errorf("expected nodepool AutoRepair=false, got %v", np.AutoRepair)
	}
	if np.UpgradeType != "InPlace" {
		t.Errorf("expected nodepool UpgradeType=InPlace, got %s", np.UpgradeType)
	}
}

func TestParseClusterSpec_MissingName(t *testing.T) {
	raw := json.RawMessage(`{
		"endpointAccess": "Public",
		"releaseVersion": "4.18.0",
		"channelGroup": "stable",
		"nodepools": [{"name":"w","replicas":1,"instanceType":"n1-standard-4","rootVolumeSize":128,"rootVolumeType":"pd-standard","autoRepair":true,"upgradeType":"Replace"}]
	}`)

	_, err := gcphcp.ParseClusterSpec(raw)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected error to wrap ErrInvalidArgument, got %v", err)
	}
}

func TestParseClusterSpec_NameTooLong(t *testing.T) {
	raw := json.RawMessage(`{"name": "this-cluster-name-is-way-too-long","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`)

	_, err := gcphcp.ParseClusterSpec(raw)
	if err == nil {
		t.Fatal("expected error for name too long, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected error to wrap ErrInvalidArgument, got %v", err)
	}
}

func TestParseClusterSpec_InvalidNamePattern(t *testing.T) {
	tests := []struct {
		name    string
		rawName string
	}{
		{"uppercase", "TestCluster"},
		{"starts with number", "1cluster"},
		{"has underscore", "test_cluster"},
		{"has dots", "test.cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(`{"name":"` + tt.rawName + `","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`)

			_, err := gcphcp.ParseClusterSpec(raw)
			if err == nil {
				t.Fatal("expected error for invalid name pattern, got nil")
			}
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("expected error to wrap ErrInvalidArgument, got %v", err)
			}
		})
	}
}

func TestParseClusterSpec_ValidNamePatterns(t *testing.T) {
	tests := []string{
		"a",
		"z",
		"test",
		"test-cluster",
		"test-123",
		"a-b-c-d-e-f-g",
		"test-cluster-99",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			raw := json.RawMessage(`{"name":"` + name + `","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`)

			spec, err := gcphcp.ParseClusterSpec(raw)
			if err != nil {
				t.Fatalf("ParseClusterSpec failed for valid name %q: %v", name, err)
			}
			if spec.Name != name {
				t.Errorf("expected Name=%s, got %s", name, spec.Name)
			}
		})
	}
}

func TestParseClusterSpec_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		rawJSON string
		want    string
	}{
		{
			name:    "missing endpointAccess",
			rawJSON: `{"name":"test","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`,
			want:    "endpointAccess is required",
		},
		{
			name:    "missing releaseVersion",
			rawJSON: `{"name":"test","endpointAccess":"Public","channelGroup":"stable","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`,
			want:    "releaseVersion is required",
		},
		{
			name:    "missing channelGroup",
			rawJSON: `{"name":"test","endpointAccess":"Public","releaseVersion":"4.18.0","nodepools":[{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}]}`,
			want:    "channelGroup is required",
		},
		{
			name:    "missing nodepools",
			rawJSON: `{"name":"test","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable"}`,
			want:    "at least one nodepool is required",
		},
		{
			name:    "empty nodepools",
			rawJSON: `{"name":"test","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[]}`,
			want:    "at least one nodepool is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gcphcp.ParseClusterSpec(json.RawMessage(tt.rawJSON))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestParseClusterSpec_MissingNodepoolFields(t *testing.T) {
	base := `{"name":"test","endpointAccess":"Public","releaseVersion":"4.18.0","channelGroup":"stable","nodepools":[`

	tests := []struct {
		name     string
		nodepool string
		want     string
	}{
		{
			name:     "missing name",
			nodepool: `{"replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].name is required",
		},
		{
			name:     "zero replicas",
			nodepool: `{"name":"w","replicas":0,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].replicas must be > 0",
		},
		{
			name:     "negative replicas",
			nodepool: `{"name":"w","replicas":-1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].replicas must be > 0",
		},
		{
			name:     "missing instanceType",
			nodepool: `{"name":"w","replicas":1,"rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].instanceType is required",
		},
		{
			name:     "zero rootVolumeSize",
			nodepool: `{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":0,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].rootVolumeSize must be > 0",
		},
		{
			name:     "negative rootVolumeSize",
			nodepool: `{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":-10,"rootVolumeType":"t","autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].rootVolumeSize must be > 0",
		},
		{
			name:     "missing rootVolumeType",
			nodepool: `{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"autoRepair":true,"upgradeType":"Replace"}`,
			want:     "nodepools[0].rootVolumeType is required",
		},
		{
			name:     "missing autoRepair",
			nodepool: `{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","upgradeType":"Replace"}`,
			want:     "nodepools[0].autoRepair is required",
		},
		{
			name:     "missing upgradeType",
			nodepool: `{"name":"w","replicas":1,"instanceType":"t","rootVolumeSize":1,"rootVolumeType":"t","autoRepair":true}`,
			want:     "nodepools[0].upgradeType is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawJSON := base + tt.nodepool + `]}`
			_, err := gcphcp.ParseClusterSpec(json.RawMessage(rawJSON))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestParseClusterSpec_MultipleNodepools(t *testing.T) {
	raw := json.RawMessage(`{
		"name": "test",
		"endpointAccess": "Public",
		"releaseVersion": "4.18.0",
		"channelGroup": "stable",
		"nodepools": [
			{"name":"pool-a","replicas":3,"instanceType":"n1-standard-8","rootVolumeSize":256,"rootVolumeType":"pd-ssd","autoRepair":true,"upgradeType":"Replace"},
			{"name":"pool-b","replicas":1,"instanceType":"n1-standard-4","rootVolumeSize":128,"rootVolumeType":"pd-standard","autoRepair":false,"upgradeType":"InPlace"}
		]
	}`)

	spec, err := gcphcp.ParseClusterSpec(raw)
	if err != nil {
		t.Fatalf("ParseClusterSpec failed: %v", err)
	}

	if len(spec.Nodepools) != 2 {
		t.Fatalf("expected 2 nodepools, got %d", len(spec.Nodepools))
	}
	if spec.Nodepools[0].Name != "pool-a" {
		t.Errorf("expected first nodepool Name=pool-a, got %s", spec.Nodepools[0].Name)
	}
	if spec.Nodepools[1].Name != "pool-b" {
		t.Errorf("expected second nodepool Name=pool-b, got %s", spec.Nodepools[1].Name)
	}
}
