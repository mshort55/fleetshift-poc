// Package kind implements a [domain.DeliveryAgent] for managing kind
// (Kubernetes-in-Docker) clusters. Manifests are interpreted as kind
// cluster specifications; delivery creates or updates clusters, and
// removal deletes them.
package kind

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/kind/pkg/cluster"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// TargetType is the [domain.TargetType] for kind-managed targets.
const TargetType domain.TargetType = "kind"

// ClusterResourceType is the [domain.ResourceType] for kind cluster
// specifications.
const ClusterResourceType domain.ResourceType = "api.kind.cluster"

// ClusterSpec is the manifest payload accepted by the kind delivery
// agent. Name identifies the kind cluster; Config holds the raw kind
// cluster configuration YAML/JSON (the same format accepted by
// kind create cluster --config).
type ClusterSpec struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config,omitempty"`
}

// ClusterProvider abstracts the kind cluster operations needed by the
// delivery agent. [cluster.Provider] satisfies this interface.
type ClusterProvider interface {
	Create(name string, options ...cluster.CreateOption) error
	Delete(name, kubeconfig string) error
	List() ([]string, error)
}

// Agent implements [domain.DeliveryAgent] for kind clusters.
type Agent struct {
	provider ClusterProvider
}

// NewAgent returns an Agent backed by the given [ClusterProvider].
func NewAgent(provider ClusterProvider) *Agent {
	return &Agent{provider: provider}
}

func (a *Agent) Deliver(_ context.Context, _ domain.TargetInfo, _ domain.DeploymentID, manifests []domain.Manifest) (domain.DeliveryResult, error) {
	for _, m := range manifests {
		var spec ClusterSpec
		if err := json.Unmarshal(m.Raw, &spec); err != nil {
			return domain.DeliveryResult{State: domain.DeliveryStateFailed},
				fmt.Errorf("unmarshal kind cluster spec: %w", err)
		}
		if spec.Name == "" {
			return domain.DeliveryResult{State: domain.DeliveryStateFailed},
				fmt.Errorf("%w: kind cluster spec requires a name", domain.ErrInvalidArgument)
		}

		if a.clusterExists(spec.Name) {
			if err := a.provider.Delete(spec.Name, ""); err != nil {
				return domain.DeliveryResult{State: domain.DeliveryStateFailed},
					fmt.Errorf("delete existing kind cluster %q for recreate: %w", spec.Name, err)
			}
		}

		var opts []cluster.CreateOption
		if len(spec.Config) > 0 {
			opts = append(opts, cluster.CreateWithRawConfig(spec.Config))
		}
		if err := a.provider.Create(spec.Name, opts...); err != nil {
			return domain.DeliveryResult{State: domain.DeliveryStateFailed},
				fmt.Errorf("create kind cluster %q: %w", spec.Name, err)
		}
	}
	return domain.DeliveryResult{State: domain.DeliveryStateDelivered}, nil
}

func (a *Agent) Remove(_ context.Context, _ domain.TargetInfo, _ domain.DeploymentID) error {
	// Removal requires knowing which clusters were created by this
	// deployment. For now, this is a no-op; a full implementation
	// would track the mapping from deployment to cluster name(s).
	return nil
}

func (a *Agent) clusterExists(name string) bool {
	clusters, err := a.provider.List()
	if err != nil {
		return false
	}
	for _, c := range clusters {
		if c == name {
			return true
		}
	}
	return false
}
