package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// Property keys for target properties.
const (
	PropAPIServer              = "api_server"
	PropCACert                 = "ca_cert"
	PropServiceAccountToken    = "service_account_token"
	PropServiceAccountTokenRef = "service_account_token_ref"
)

// Manager manages the lifecycle of TargetAgents — one per ready target.
// It builds K8s clients from target properties + vault, creates
// TargetAgents with delivery and indexer components, and implements
// [domain.DeliveryAgent] by routing to the appropriate TargetAgent.
type Manager struct {
	store            domain.Store
	vault            domain.Vault
	inventoryWriter  domain.InventoryWriter
	deliveryReporter domain.DeliveryReporter
	keyResolver      *domain.KeyResolver
	httpClient       *http.Client
	logger           *slog.Logger

	mu     sync.Mutex
	agents map[domain.TargetID]*TargetAgent
}

// NewManager creates a Manager.
func NewManager(
	store domain.Store,
	vault domain.Vault,
	inventoryWriter domain.InventoryWriter,
	deliveryReporter domain.DeliveryReporter,
	keyResolver *domain.KeyResolver,
	httpClient *http.Client,
	logger *slog.Logger,
) *Manager {
	return &Manager{
		store:            store,
		vault:            vault,
		inventoryWriter:  inventoryWriter,
		deliveryReporter: deliveryReporter,
		keyResolver:      keyResolver,
		httpClient:       httpClient,
		logger:           logger,
		agents:           make(map[domain.TargetID]*TargetAgent),
	}
}

// HandleTargetReady builds K8s clients, creates a TargetAgent with both
// delivery and indexer components, and starts it in a goroutine. It is
// idempotent: if an agent for the given target is already running, it
// returns nil without starting a duplicate.
func (m *Manager) HandleTargetReady(ctx context.Context, target domain.TargetInfo) error {
	id := target.ID()

	m.mu.Lock()
	if _, ok := m.agents[id]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	cfg, err := m.buildRESTConfig(ctx, target)
	if err != nil {
		return fmt.Errorf("build rest config for %s: %w", id, err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client for %s: %w", id, err)
	}

	discClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create discovery client for %s: %w", id, err)
	}

	logger := m.logger.With("target", string(id))
	agentCtx, cancel := context.WithCancel(ctx)

	dc := newDeliveryComponent(m.deliveryReporter, m.keyResolver, m.httpClient)
	ic := newIndexerComponent(string(id), dynClient, discClient, m.inventoryWriter, DefaultKubernetesSchema(), 0, logger)

	ta := &TargetAgent{
		targetID:   id,
		restConfig: cfg,
		dynClient:  dynClient,
		discClient: discClient,
		logger:     logger,
		delivery:   dc,
		indexer:    ic,
		ctx:        agentCtx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}

	// Re-check under lock to handle races.
	m.mu.Lock()
	if _, ok := m.agents[id]; ok {
		m.mu.Unlock()
		cancel()
		return nil
	}
	m.agents[id] = ta
	m.mu.Unlock()

	go ta.start()
	return nil
}

// HandleTargetTerminated stops the agent for the given target, removes it
// from tracking, and deletes inventory for the target.
func (m *Manager) HandleTargetTerminated(ctx context.Context, targetID domain.TargetID) error {
	m.mu.Lock()
	ta, ok := m.agents[targetID]
	delete(m.agents, targetID)
	m.mu.Unlock()

	if ok {
		ta.Stop()
	}

	// Clean up inventory for this target.
	tx, err := m.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for inventory cleanup: %w", err)
	}
	defer tx.Rollback()

	if err := tx.Inventory().DeleteByTarget(ctx, targetID); err != nil {
		return fmt.Errorf("delete inventory for target %s: %w", targetID, err)
	}
	return tx.Commit()
}

// GetTarget returns the running TargetAgent for the given ID, or nil if
// no agent is running.
func (m *Manager) GetTarget(id domain.TargetID) *TargetAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.agents[id]
}

// StopAll stops all running agents.
func (m *Manager) StopAll() {
	m.mu.Lock()
	agents := make(map[domain.TargetID]*TargetAgent, len(m.agents))
	for id, ta := range m.agents {
		agents[id] = ta
	}
	m.agents = make(map[domain.TargetID]*TargetAgent)
	m.mu.Unlock()

	for _, ta := range agents {
		ta.Stop()
	}
}

// Deliver implements [domain.DeliveryAgent] by routing to the
// appropriate TargetAgent.
func (m *Manager) Deliver(ctx context.Context, target domain.TargetInfo, deliveryID domain.DeliveryID, manifests []domain.Manifest, auth domain.DeliveryAuth, att *domain.Attestation, generation domain.Generation) error {
	ta := m.GetTarget(target.ID())
	if ta == nil {
		return fmt.Errorf("no agent for target %s", target.ID())
	}
	return ta.Deliver(ctx, target, deliveryID, manifests, auth, att, generation)
}

// Remove implements [domain.DeliveryAgent] by routing to the
// appropriate TargetAgent.
func (m *Manager) Remove(ctx context.Context, target domain.TargetInfo, deliveryID domain.DeliveryID, manifests []domain.Manifest, auth domain.DeliveryAuth, att *domain.Attestation, generation domain.Generation) error {
	ta := m.GetTarget(target.ID())
	if ta == nil {
		return fmt.Errorf("no agent for target %s", target.ID())
	}
	return ta.Remove(ctx, target, deliveryID, manifests, auth, att, generation)
}

// buildRESTConfig constructs a [rest.Config] from the target's properties
// and optional vault-backed service account token.
func (m *Manager) buildRESTConfig(ctx context.Context, target domain.TargetInfo) (*rest.Config, error) {
	props := target.Properties()
	host := props[PropAPIServer]
	if host == "" {
		return nil, fmt.Errorf("missing property %q", PropAPIServer)
	}

	cfg := &rest.Config{
		Host: host,
	}

	if ca := props[PropCACert]; ca != "" {
		cfg.TLSClientConfig = rest.TLSClientConfig{
			CAData: []byte(ca),
		}
	}

	// Resolve SA token: direct value first, then vault reference.
	if tok := props[PropServiceAccountToken]; tok != "" {
		cfg.BearerToken = tok
	} else if ref := props[PropServiceAccountTokenRef]; ref != "" {
		if m.vault == nil {
			return nil, fmt.Errorf("vault required for %q but not configured", PropServiceAccountTokenRef)
		}
		val, err := m.vault.Get(ctx, domain.SecretRef(ref))
		if err != nil {
			return nil, fmt.Errorf("resolve vault ref %q: %w", ref, err)
		}
		cfg.BearerToken = string(val)
	}

	return cfg, nil
}
