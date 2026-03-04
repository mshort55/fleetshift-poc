package domain_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// recordingRecord wraps a [domain.Record] and records activity names
// and target-related inputs so tests can assert execution sequence.
type recordingRecord struct {
	ctx      context.Context
	records  []activityRecord
	delegate domain.Record
}

type activityRecord struct {
	Name string
	// TargetID is set for remove-from-target, generate-manifests, deliver-to-target.
	TargetID domain.TargetID
}

func (r *recordingRecord) ID() string              { return r.delegate.ID() }
func (r *recordingRecord) Context() context.Context { return r.ctx }

func (r *recordingRecord) Run(activity domain.Activity[any, any], in any) (any, error) {
	name := activity.Name()
	var targetID domain.TargetID
	switch v := in.(type) {
	case domain.RemoveInput:
		targetID = v.Target.ID
	case domain.GenerateManifestsInput:
		targetID = v.Target.ID
	case domain.DeliverInput:
		targetID = v.Target.ID
	}
	r.records = append(r.records, activityRecord{Name: name, TargetID: targetID})
	return r.delegate.Run(activity, in)
}

func (r *recordingRecord) Await(signalName string) (any, error) {
	return r.delegate.Await(signalName)
}

func (r *recordingRecord) activityNames() []string {
	names := make([]string, len(r.records))
	for i, rec := range r.records {
		names[i] = rec.Name
	}
	return names
}

// stubDeploymentRepo returns a fixed deployment for Get and accepts Update.
// It tracks the full update history for tests that need to assert
// intermediate states (e.g. Active before Deleting).
type stubDeploymentRepo struct {
	deployment domain.Deployment
	updated    *domain.Deployment
	updates    []domain.Deployment
}

func (s *stubDeploymentRepo) Create(_ context.Context, d domain.Deployment) error {
	s.deployment = d
	return nil
}

func (s *stubDeploymentRepo) Get(_ context.Context, id domain.DeploymentID) (domain.Deployment, error) {
	if id != s.deployment.ID {
		return domain.Deployment{}, domain.ErrNotFound
	}
	if s.updated != nil {
		return *s.updated, nil
	}
	return s.deployment, nil
}

func (s *stubDeploymentRepo) List(_ context.Context) ([]domain.Deployment, error) {
	return []domain.Deployment{s.deployment}, nil
}

func (s *stubDeploymentRepo) Update(_ context.Context, d domain.Deployment) error {
	s.updated = &d
	s.updates = append(s.updates, d)
	return nil
}

func (s *stubDeploymentRepo) Delete(_ context.Context, _ domain.DeploymentID) error { return nil }

// stubTargetRepo returns a fixed list for List.
type stubTargetRepo struct {
	targets []domain.TargetInfo
}

func (s *stubTargetRepo) Create(_ context.Context, t domain.TargetInfo) error {
	s.targets = append(s.targets, t)
	return nil
}

func (s *stubTargetRepo) Get(_ context.Context, id domain.TargetID) (domain.TargetInfo, error) {
	for _, t := range s.targets {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.TargetInfo{}, domain.ErrNotFound
}

func (s *stubTargetRepo) List(_ context.Context) ([]domain.TargetInfo, error) {
	return s.targets, nil
}

func (s *stubTargetRepo) Delete(_ context.Context, _ domain.TargetID) error { return nil }

// stubStore implements domain.Store backed by in-memory stub repos.
type stubStore struct {
	deployments *stubDeploymentRepo
	targets     *stubTargetRepo
	deliveries  *stubDeliveryRepo
}

func (s *stubStore) Begin(_ context.Context) (domain.Tx, error) {
	return &stubTx{store: s}, nil
}

type stubTx struct {
	store *stubStore
}

func (t *stubTx) Targets() domain.TargetRepository        { return t.store.targets }
func (t *stubTx) Deployments() domain.DeploymentRepository { return t.store.deployments }
func (t *stubTx) Deliveries() domain.DeliveryRepository    { return t.store.deliveries }
func (t *stubTx) Inventory() domain.InventoryRepository    { return nil }
func (t *stubTx) Commit() error                            { return nil }
func (t *stubTx) Rollback() error                          { return nil }

// stubDeliveryRepo implements DeliveryRepository with an in-memory map.
type stubDeliveryRepo struct {
	deliveries map[domain.DeliveryID]domain.Delivery
}

func newStubDeliveryRepo() *stubDeliveryRepo {
	return &stubDeliveryRepo{deliveries: make(map[domain.DeliveryID]domain.Delivery)}
}

func (s *stubDeliveryRepo) Put(_ context.Context, d domain.Delivery) error {
	s.deliveries[d.ID] = d
	return nil
}

func (s *stubDeliveryRepo) Get(_ context.Context, id domain.DeliveryID) (domain.Delivery, error) {
	d, ok := s.deliveries[id]
	if !ok {
		return domain.Delivery{}, domain.ErrNotFound
	}
	return d, nil
}

func (s *stubDeliveryRepo) GetByDeploymentTarget(_ context.Context, depID domain.DeploymentID, tgtID domain.TargetID) (domain.Delivery, error) {
	for _, d := range s.deliveries {
		if d.DeploymentID == depID && d.TargetID == tgtID {
			return d, nil
		}
	}
	return domain.Delivery{}, domain.ErrNotFound
}

func (s *stubDeliveryRepo) ListByDeployment(_ context.Context, depID domain.DeploymentID) ([]domain.Delivery, error) {
	var result []domain.Delivery
	for _, d := range s.deliveries {
		if d.DeploymentID == depID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s *stubDeliveryRepo) DeleteByDeployment(_ context.Context, depID domain.DeploymentID) error {
	for id, d := range s.deliveries {
		if d.DeploymentID == depID {
			delete(s.deliveries, id)
		}
	}
	return nil
}

// stubRegistry implements [domain.Registry] for domain unit tests.
// It routes SignalDeploymentEvent to a shared events channel that test
// records read from via Await.
type stubRegistry struct {
	events chan domain.DeploymentEvent
}

func (r *stubRegistry) SignalDeploymentEvent(_ context.Context, _ domain.DeploymentID, event domain.DeploymentEvent) error {
	r.events <- event
	return nil
}

func (r *stubRegistry) RegisterOrchestration(_ *domain.OrchestrationWorkflowSpec) (domain.OrchestrationWorkflow, error) {
	return nil, nil
}

func (r *stubRegistry) RegisterCreateDeployment(_ *domain.CreateDeploymentWorkflowSpec) (domain.CreateDeploymentWorkflow, error) {
	return nil, nil
}

// noopDelivery implements DeliveryService with no-op Deliver and Remove.
// It calls signaler.Done synchronously, which is safe in domain unit
// tests where workflows execute in a single goroutine without locks.
type noopDelivery struct{}

func (noopDelivery) Deliver(ctx context.Context, _ domain.TargetInfo, _ domain.DeliveryID, _ []domain.Manifest, signaler *domain.DeliverySignaler) (domain.DeliveryResult, error) {
	result := domain.DeliveryResult{State: domain.DeliveryStateDelivered}
	signaler.Done(ctx, result)
	return result, nil
}

func (noopDelivery) Remove(_ context.Context, _ domain.TargetInfo, _ domain.DeliveryID, _ *domain.DeliverySignaler) error {
	return nil
}

// singleEventRecord is a minimal Record that runs activities
// synchronously. Await delivers one scripted event and then signals
// delete. Delivery-completion events injected via the shared events
// channel are drained before the scripted event sequence.
type singleEventRecord struct {
	ctx       context.Context
	event     domain.DeploymentEvent
	delivered bool
	events    <-chan domain.DeploymentEvent
}

func (r *singleEventRecord) ID() string              { return "test-single" }
func (r *singleEventRecord) Context() context.Context { return r.ctx }
func (r *singleEventRecord) Run(activity domain.Activity[any, any], in any) (any, error) {
	return activity.Run(r.ctx, in)
}

func (r *singleEventRecord) Await(_ string) (any, error) {
	select {
	case e := <-r.events:
		return e, nil
	default:
	}
	if !r.delivered {
		r.delivered = true
		return r.event, nil
	}
	return domain.DeploymentEvent{Delete: true}, nil
}

func TestOrchestration_RemoveStepsRunBeforeDeliverSteps(t *testing.T) {
	deploymentID := domain.DeploymentID("d1")
	depRepo := &stubDeploymentRepo{
		deployment: domain.Deployment{
			ID:              deploymentID,
			ResolvedTargets: []domain.TargetID{"old1"},
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"new1", "new2"},
			},
			RolloutStrategy: nil,
			State:           domain.DeploymentStateCreating,
		},
	}
	pool := []domain.TargetInfo{
		{ID: "old1"},
		{ID: "new1"},
		{ID: "new2"},
	}

	targetRepo := &stubTargetRepo{targets: pool}
	events := make(chan domain.DeploymentEvent, 16)
	reg := &stubRegistry{events: events}

	rec := &singleEventRecord{
		ctx:    context.Background(),
		event:  domain.DeploymentEvent{PoolChange: &domain.PoolChange{Set: pool}},
		events: events,
	}

	wf := &domain.OrchestrationWorkflowSpec{
		Store:      &stubStore{deployments: depRepo, targets: targetRepo, deliveries: newStubDeliveryRepo()},
		Delivery:   noopDelivery{},
		Strategies: domain.DefaultStrategyFactory{},
		Registry:   reg,
	}

	recorder := &recordingRecord{ctx: rec.ctx, delegate: rec}

	_, err := wf.Run(recorder, deploymentID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var removeOld1At, generateNew1At int
	removeOld1At = -1
	generateNew1At = -1
	for i, rec := range recorder.records {
		if rec.Name == "remove-from-target" && rec.TargetID == "old1" {
			removeOld1At = i
			break
		}
	}
	for i, rec := range recorder.records {
		if rec.Name == "generate-manifests" && rec.TargetID == "new1" {
			generateNew1At = i
			break
		}
	}
	if removeOld1At < 0 {
		t.Fatal("remove-from-target for old1 never recorded")
	}
	if generateNew1At < 0 {
		t.Fatal("generate-manifests for new1 never recorded")
	}
	if removeOld1At >= generateNew1At {
		t.Errorf("removals must run before delivery: remove(old1) at %d, generate(new1) at %d",
			removeOld1At, generateNew1At)
	}
}

func TestOrchestration_PlacementAndRolloutRunAsActivities(t *testing.T) {
	deploymentID := domain.DeploymentID("d1")
	depRepo := &stubDeploymentRepo{
		deployment: domain.Deployment{
			ID:                deploymentID,
			ResolvedTargets:   nil,
			ManifestStrategy:  domain.ManifestStrategySpec{Type: domain.ManifestStrategyInline, Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}}},
			PlacementStrategy: domain.PlacementStrategySpec{Type: domain.PlacementStrategyStatic, Targets: []domain.TargetID{"t1"}},
			RolloutStrategy:   nil,
			State:             domain.DeploymentStateCreating,
		},
	}
	pool := []domain.TargetInfo{{ID: "t1"}}

	targetRepo := &stubTargetRepo{targets: pool}
	events := make(chan domain.DeploymentEvent, 16)
	reg := &stubRegistry{events: events}

	rec := &singleEventRecord{
		ctx:    context.Background(),
		event:  domain.DeploymentEvent{PoolChange: &domain.PoolChange{Set: pool}},
		events: events,
	}

	wf := &domain.OrchestrationWorkflowSpec{
		Store:      &stubStore{deployments: depRepo, targets: targetRepo, deliveries: newStubDeliveryRepo()},
		Delivery:   noopDelivery{},
		Strategies: domain.DefaultStrategyFactory{},
		Registry:   reg,
	}

	recorder := &recordingRecord{ctx: rec.ctx, delegate: rec}

	_, err := wf.Run(recorder, deploymentID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := recorder.activityNames()
	hasResolvePlacement := false
	hasPlanRollout := false
	for _, n := range names {
		if n == "resolve-placement" {
			hasResolvePlacement = true
		}
		if n == "plan-rollout" {
			hasPlanRollout = true
		}
	}
	if !hasResolvePlacement {
		t.Errorf("workflow must invoke resolve-placement activity; got names: %v", names)
	}
	if !hasPlanRollout {
		t.Errorf("workflow must invoke plan-rollout activity; got names: %v", names)
	}
}

func TestOrchestration_EmptyPool_FailsDeployment(t *testing.T) {
	deploymentID := domain.DeploymentID("empty-pool")
	depRepo := &stubDeploymentRepo{
		deployment: domain.Deployment{
			ID: deploymentID,
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type: domain.PlacementStrategyAll,
			},
			State: domain.DeploymentStateCreating,
		},
	}

	targetRepo := &stubTargetRepo{targets: nil}
	events := make(chan domain.DeploymentEvent, 16)
	reg := &stubRegistry{events: events}

	rec := &singleEventRecord{
		ctx:    context.Background(),
		events: events,
	}

	wf := &domain.OrchestrationWorkflowSpec{
		Store:      &stubStore{deployments: depRepo, targets: targetRepo, deliveries: newStubDeliveryRepo()},
		Delivery:   noopDelivery{},
		Strategies: domain.DefaultStrategyFactory{},
		Registry:   reg,
	}

	_, err := wf.Run(rec, deploymentID)
	if err == nil {
		t.Fatal("expected error from empty pool, got nil")
	}
	if !strings.Contains(err.Error(), "zero targets") {
		t.Errorf("error should mention zero targets, got: %v", err)
	}

	if depRepo.updated == nil {
		t.Fatal("deployment should have been updated to Failed state")
	}
	if depRepo.updated.State != domain.DeploymentStateFailed {
		t.Errorf("deployment state = %q, want %q", depRepo.updated.State, domain.DeploymentStateFailed)
	}
}

// asyncDelivery returns Accepted immediately and calls signaler.Done
// in a background goroutine, simulating how real delivery agents
// (e.g. kind) operate.
type asyncDelivery struct {
	done chan struct{}
}

func (a *asyncDelivery) Deliver(ctx context.Context, _ domain.TargetInfo, _ domain.DeliveryID, _ []domain.Manifest, signaler *domain.DeliverySignaler) (domain.DeliveryResult, error) {
	go func() {
		signaler.Done(ctx, domain.DeliveryResult{State: domain.DeliveryStateDelivered})
		if a.done != nil {
			close(a.done)
		}
	}()
	return domain.DeliveryResult{State: domain.DeliveryStateAccepted}, nil
}

func (asyncDelivery) Remove(_ context.Context, _ domain.TargetInfo, _ domain.DeliveryID, _ *domain.DeliverySignaler) error {
	return nil
}

// asyncRecord is a Record for testing async delivery agents. Await
// blocks until a signal arrives on the events channel, then sends
// a Delete on the next call.
type asyncRecord struct {
	ctx    context.Context
	events chan domain.DeploymentEvent
	sawAll bool
}

func (r *asyncRecord) ID() string              { return "test-async" }
func (r *asyncRecord) Context() context.Context { return r.ctx }
func (r *asyncRecord) Run(activity domain.Activity[any, any], in any) (any, error) {
	return activity.Run(r.ctx, in)
}

func (r *asyncRecord) Await(_ string) (any, error) {
	if r.sawAll {
		return domain.DeploymentEvent{Delete: true}, nil
	}
	e := <-r.events
	return e, nil
}

func TestOrchestration_AsyncDelivery_ReachesActive(t *testing.T) {
	deploymentID := domain.DeploymentID("async-test")
	depRepo := &stubDeploymentRepo{
		deployment: domain.Deployment{
			ID: deploymentID,
			ManifestStrategy: domain.ManifestStrategySpec{
				Type:      domain.ManifestStrategyInline,
				Manifests: []domain.Manifest{{Raw: json.RawMessage(`{}`)}},
			},
			PlacementStrategy: domain.PlacementStrategySpec{
				Type:    domain.PlacementStrategyStatic,
				Targets: []domain.TargetID{"t1"},
			},
			State: domain.DeploymentStateCreating,
		},
	}
	pool := []domain.TargetInfo{{ID: "t1"}}
	targetRepo := &stubTargetRepo{targets: pool}
	deliveryRepo := newStubDeliveryRepo()
	store := &stubStore{deployments: depRepo, targets: targetRepo, deliveries: deliveryRepo}
	asyncDel := &asyncDelivery{done: make(chan struct{})}

	events := make(chan domain.DeploymentEvent, 16)
	reg := &stubRegistry{events: events}

	rec := &asyncRecord{
		ctx:    context.Background(),
		events: events,
	}

	wf := &domain.OrchestrationWorkflowSpec{
		Store:      store,
		Delivery:   asyncDel,
		Strategies: domain.DefaultStrategyFactory{},
		Registry:   reg,
	}

	// The async delivery agent signals completion, which the registry
	// routes to the record's events channel. After the delivery
	// completion arrives, the workflow reaches Active. We then need
	// to send a Delete to terminate the workflow.
	go func() {
		// Wait for delivery completion to be processed, then send Delete.
		<-asyncDel.done
		// Give the workflow a moment to process the delivery completion
		// and reach the event loop. Then send Delete.
		events <- domain.DeploymentEvent{Delete: true}
	}()

	_, err := wf.Run(rec, deploymentID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	sawActive := false
	for _, u := range depRepo.updates {
		if u.State == domain.DeploymentStateActive {
			sawActive = true
		}
	}
	if !sawActive {
		states := make([]string, len(depRepo.updates))
		for i, u := range depRepo.updates {
			states[i] = string(u.State)
		}
		t.Fatalf("workflow never reached Active; state transitions: %v", states)
	}
	if depRepo.updated.State != domain.DeploymentStateDeleting {
		t.Errorf("final deployment state = %q, want %q", depRepo.updated.State, domain.DeploymentStateDeleting)
	}

	did := domain.DeliveryID("async-test:t1")
	d, err := deliveryRepo.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("delivery record not found: %v", err)
	}
	if d.State != domain.DeliveryStateDelivered {
		t.Errorf("delivery state = %q, want %q", d.State, domain.DeliveryStateDelivered)
	}
}
