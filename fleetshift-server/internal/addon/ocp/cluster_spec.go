package ocp

import (
	"encoding/json"
	"fmt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"gopkg.in/yaml.v3"
)

const TargetType domain.TargetType = "ocp"
const ClusterResourceType domain.ResourceType = "api.ocp.cluster"

// ClusterSpec defines the declarative specification for an OCP cluster.
type ClusterSpec struct {
	Name          string         `json:"name"`
	BaseDomain    string         `json:"base_domain"`
	ReleaseImage  string         `json:"release_image,omitempty"`
	InstallConfig map[string]any `json:"install_config,omitempty"`
}

// ParseClusterSpec extracts the ClusterSpec from deployment manifests.
// Returns an error if no OCP cluster manifest is found or if required
// fields (Name, BaseDomain) are missing.
func ParseClusterSpec(manifests []domain.Manifest) (*ClusterSpec, error) {
	// Find the first manifest with ClusterResourceType
	var clusterManifest *domain.Manifest
	for i := range manifests {
		if manifests[i].ResourceType == ClusterResourceType {
			clusterManifest = &manifests[i]
			break
		}
	}

	if clusterManifest == nil {
		return nil, fmt.Errorf("no OCP cluster manifest found (expected ResourceType=%s)", ClusterResourceType)
	}

	// Unmarshal the Raw JSON into ClusterSpec
	var spec ClusterSpec
	if err := json.Unmarshal(clusterManifest.Raw, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster spec: %w", err)
	}

	// Validate required fields
	if spec.Name == "" {
		return nil, fmt.Errorf("cluster spec missing required field: name")
	}
	if spec.BaseDomain == "" {
		return nil, fmt.Errorf("cluster spec missing required field: base_domain")
	}

	return &spec, nil
}

// BuildClusterYAML generates ocp-engine's cluster.yaml configuration.
// The output includes:
// - ocp_engine section with pull_secret_file and optional release_image
// - baseDomain from spec
// - credentialsMode: Manual (for CCO STS mode)
// - metadata.name from spec
// - platform.aws.region from the region parameter
// - sshKey from sshPublicKey parameter
// - All fields from spec.InstallConfig merged in (pass-through fields)
func BuildClusterYAML(spec *ClusterSpec, region, pullSecretFile, sshPublicKey string) ([]byte, error) {
	// Start with base structure
	config := map[string]any{
		"ocp_engine": map[string]any{
			"pull_secret_file": pullSecretFile,
		},
		"baseDomain":      spec.BaseDomain,
		"credentialsMode": "Manual",
		"metadata": map[string]any{
			"name": spec.Name,
		},
		"platform": map[string]any{
			"aws": map[string]any{
				"region": region,
			},
		},
		"sshKey": sshPublicKey,
	}

	// Add release_image to ocp_engine section if specified
	if spec.ReleaseImage != "" {
		ocpEngine := config["ocp_engine"].(map[string]any)
		ocpEngine["release_image"] = spec.ReleaseImage
	}

	// Merge in InstallConfig pass-through fields
	for key, value := range spec.InstallConfig {
		config[key] = value
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster config to YAML: %w", err)
	}

	return yamlBytes, nil
}
