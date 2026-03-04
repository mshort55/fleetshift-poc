package kind

import "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"

// KubernetesTargetType is the [domain.TargetType] for Kubernetes
// clusters provisioned by the kind addon. The kubernetes-direct
// delivery agent handles delivery to these targets.
const KubernetesTargetType domain.TargetType = "kubernetes"

// ClusterOutput captures what a successful kind cluster creation
// produces. It constructs the [domain.ProvisionedTarget] and
// [domain.ProducedSecret] that the platform processes after delivery.
type ClusterOutput struct {
	TargetID   domain.TargetID
	Name       string
	KubeConfig []byte
}

// Target returns a [domain.ProvisionedTarget] for the provisioned
// cluster. Properties include a vault ref for the kubeconfig.
func (o *ClusterOutput) Target() domain.ProvisionedTarget {
	return domain.ProvisionedTarget{
		ID:   o.TargetID,
		Type: KubernetesTargetType,
		Name: o.Name,
		Properties: map[string]string{
			"kubeconfig_ref": string(o.secretRef()),
		},
	}
}

// Secret returns a [domain.ProducedSecret] containing the kubeconfig.
func (o *ClusterOutput) Secret() domain.ProducedSecret {
	return domain.ProducedSecret{
		Ref:   o.secretRef(),
		Value: o.KubeConfig,
	}
}

func (o *ClusterOutput) secretRef() domain.SecretRef {
	return domain.SecretRef("targets/" + string(o.TargetID) + "/kubeconfig")
}
