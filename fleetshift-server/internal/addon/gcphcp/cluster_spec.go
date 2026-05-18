package gcphcp

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// ClusterSpec defines the declarative specification for a GCP HCP cluster.
// All fields are required.
type ClusterSpec struct {
	Name           string         `json:"name"`
	EndpointAccess string         `json:"endpointAccess"`
	ReleaseVersion string         `json:"releaseVersion"`
	ChannelGroup   string         `json:"channelGroup"`
	Nodepools      []NodepoolSpec `json:"nodepools"`
}

// NodepoolSpec defines the specification for a GCP HCP cluster nodepool.
// All fields are required.
type NodepoolSpec struct {
	Name           string `json:"name"`
	Replicas       int    `json:"replicas"`
	InstanceType   string `json:"instanceType"`
	RootVolumeSize int    `json:"rootVolumeSize"`
	RootVolumeType string `json:"rootVolumeType"`
	AutoRepair     *bool  `json:"autoRepair"`
	UpgradeType    string `json:"upgradeType"`
}

var clusterNamePattern = regexp.MustCompile(`^[a-z][-a-z0-9]*$`)

// ParseClusterSpec unmarshals and validates a ClusterSpec from JSON.
func ParseClusterSpec(raw json.RawMessage) (ClusterSpec, error) {
	var spec ClusterSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return ClusterSpec{}, fmt.Errorf("failed to unmarshal cluster spec: %w", err)
	}

	if err := validateClusterSpec(&spec); err != nil {
		return ClusterSpec{}, err
	}

	return spec, nil
}

func validateClusterSpec(spec *ClusterSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("%w: cluster name is required", domain.ErrInvalidArgument)
	}

	if len(spec.Name) > 15 {
		return fmt.Errorf("%w: cluster name must be 15 characters or less (got %d)",
			domain.ErrInvalidArgument, len(spec.Name))
	}

	if !clusterNamePattern.MatchString(spec.Name) {
		return fmt.Errorf("%w: cluster name must match pattern ^[a-z][-a-z0-9]*$ (got %q)",
			domain.ErrInvalidArgument, spec.Name)
	}

	if spec.EndpointAccess == "" {
		return fmt.Errorf("%w: endpointAccess is required", domain.ErrInvalidArgument)
	}
	if spec.ReleaseVersion == "" {
		return fmt.Errorf("%w: releaseVersion is required", domain.ErrInvalidArgument)
	}
	if spec.ChannelGroup == "" {
		return fmt.Errorf("%w: channelGroup is required", domain.ErrInvalidArgument)
	}

	if len(spec.Nodepools) == 0 {
		return fmt.Errorf("%w: at least one nodepool is required", domain.ErrInvalidArgument)
	}

	for i, np := range spec.Nodepools {
		if np.Name == "" {
			return fmt.Errorf("%w: nodepools[%d].name is required",
				domain.ErrInvalidArgument, i)
		}
		if np.Replicas <= 0 {
			return fmt.Errorf("%w: nodepools[%d].replicas must be > 0 (got %d)",
				domain.ErrInvalidArgument, i, np.Replicas)
		}
		if np.InstanceType == "" {
			return fmt.Errorf("%w: nodepools[%d].instanceType is required",
				domain.ErrInvalidArgument, i)
		}
		if np.RootVolumeSize <= 0 {
			return fmt.Errorf("%w: nodepools[%d].rootVolumeSize must be > 0 (got %d)",
				domain.ErrInvalidArgument, i, np.RootVolumeSize)
		}
		if np.RootVolumeType == "" {
			return fmt.Errorf("%w: nodepools[%d].rootVolumeType is required",
				domain.ErrInvalidArgument, i)
		}
		if np.AutoRepair == nil {
			return fmt.Errorf("%w: nodepools[%d].autoRepair is required",
				domain.ErrInvalidArgument, i)
		}
		if np.UpgradeType == "" {
			return fmt.Errorf("%w: nodepools[%d].upgradeType is required",
				domain.ErrInvalidArgument, i)
		}
	}

	return nil
}
