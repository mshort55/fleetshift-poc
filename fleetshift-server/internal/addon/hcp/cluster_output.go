package hcp

import (
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/kubernetes"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// ClusterOutput captures what a successful HCP cluster creation
// produces: connection info for the provisioned cluster and,
// optionally, a platform ServiceAccount token for attested delivery.
type ClusterOutput struct {
	TargetID  domain.TargetID
	Name      string
	APIServer string
	CACert    []byte

	SATokenRef domain.SecretRef
	SAToken    []byte
}

// Target returns a [domain.ProvisionedTarget] with connection info
// stored as properties.
func (o *ClusterOutput) Target() domain.ProvisionedTarget {
	props := map[string]string{
		"api_server": o.APIServer,
	}
	if len(o.CACert) > 0 {
		props["ca_cert"] = string(o.CACert)
	}
	if o.SATokenRef != "" {
		props["service_account_token_ref"] = string(o.SATokenRef)
	}
	return domain.ProvisionedTarget{
		ID:                    o.TargetID,
		Type:                  KubernetesTargetType,
		Name:                  o.Name,
		Properties:            props,
		AcceptedResourceTypes: []domain.ResourceType{kubernetes.ManifestResourceType},
	}
}

// Secrets returns the [domain.ProducedSecret] entries that should be
// stored in the vault for this cluster. Returns nil when no platform
// SA was bootstrapped.
func (o *ClusterOutput) Secrets() []domain.ProducedSecret {
	if o.SATokenRef == "" {
		return nil
	}
	return []domain.ProducedSecret{{
		Ref:   o.SATokenRef,
		Value: o.SAToken,
	}}
}
