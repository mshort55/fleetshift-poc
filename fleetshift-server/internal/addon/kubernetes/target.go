package kubernetes

import (
	"context"
	"log/slog"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// TargetAgent is the unified per-target agent that holds shared K8s
// clients and delegates to a delivery component and an indexer component.
type TargetAgent struct {
	targetID   domain.TargetID
	restConfig *rest.Config
	dynClient  dynamic.Interface
	discClient discovery.DiscoveryInterface
	logger     *slog.Logger

	delivery *deliveryComponent
	indexer  *indexerComponent

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// TargetID returns the agent's target identifier.
func (ta *TargetAgent) TargetID() domain.TargetID { return ta.targetID }

// DynClient returns the dynamic Kubernetes client.
func (ta *TargetAgent) DynClient() dynamic.Interface { return ta.dynClient }

// DiscClient returns the discovery client.
func (ta *TargetAgent) DiscClient() discovery.DiscoveryInterface { return ta.discClient }

// APIServer returns the API server URL from the rest config.
func (ta *TargetAgent) APIServer() string { return ta.restConfig.Host }

// CAData returns the CA certificate data from the rest config.
func (ta *TargetAgent) CAData() []byte { return ta.restConfig.TLSClientConfig.CAData }

// Done returns a channel that is closed when the agent has stopped.
func (ta *TargetAgent) Done() <-chan struct{} { return ta.done }

// Deliver forwards to the delivery component.
func (ta *TargetAgent) Deliver(ctx context.Context, target domain.TargetInfo, deliveryID domain.DeliveryID, manifests []domain.Manifest, auth domain.DeliveryAuth, att *domain.Attestation, generation domain.Generation) error {
	return ta.delivery.deliver(ctx, ta.restConfig, target, deliveryID, manifests, auth, att, generation)
}

// Remove forwards to the delivery component.
func (ta *TargetAgent) Remove(ctx context.Context, target domain.TargetInfo, deliveryID domain.DeliveryID, manifests []domain.Manifest, auth domain.DeliveryAuth, att *domain.Attestation, generation domain.Generation) error {
	return ta.delivery.remove(ctx, ta.restConfig, target, deliveryID, manifests, auth, att, generation)
}

// start runs the indexer component until the agent's context is cancelled.
func (ta *TargetAgent) start() {
	defer close(ta.done)
	ta.indexer.start(ta.ctx)
}

// Stop cancels the agent's context and waits for it to finish.
func (ta *TargetAgent) Stop() {
	ta.cancel()
	<-ta.done
}
