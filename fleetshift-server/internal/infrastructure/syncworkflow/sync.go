// Package syncworkflow provides a synchronous, in-process [domain.Registry].
// Activities execute inline with no persistence or replay. The workflow runs
// in a goroutine and receives [domain.DeploymentEvent] values through a
// buffered channel, so callers must coordinate start and signal.
package syncworkflow

import (
	"context"
	"sync"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// Registry implements [domain.Registry] with synchronous, in-process
// execution. No durable state is kept. Workflow instances are tracked so
// that event signals can be delivered to the correct goroutine.
type Registry struct {
	mu        sync.Mutex
	instances map[domain.DeploymentID]*instance
}

type instance struct {
	events chan domain.DeploymentEvent
}

func (r *Registry) getInstance(id domain.DeploymentID) *instance {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.instances == nil {
		r.instances = make(map[domain.DeploymentID]*instance)
	}
	inst, ok := r.instances[id]
	if !ok {
		inst = &instance{events: make(chan domain.DeploymentEvent, 16)}
		r.instances[id] = inst
	}
	return inst
}

func (r *Registry) removeInstance(id domain.DeploymentID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, id)
}

func (r *Registry) SignalDeploymentEvent(ctx context.Context, id domain.DeploymentID, event domain.DeploymentEvent) error {
	inst := r.getInstance(id)
	select {
	case inst.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Registry) RegisterOrchestration(spec *domain.OrchestrationWorkflowSpec) (domain.OrchestrationWorkflow, error) {
	return &orchestrationWorkflow{registry: r, spec: spec}, nil
}

func (r *Registry) RegisterCreateDeployment(spec *domain.CreateDeploymentWorkflowSpec) (domain.CreateDeploymentWorkflow, error) {
	return &createDeploymentWorkflow{spec: spec}, nil
}

// --- OrchestrationWorkflow ---

type orchestrationWorkflow struct {
	registry *Registry
	spec     *domain.OrchestrationWorkflowSpec
}

func (w *orchestrationWorkflow) Start(ctx context.Context, deploymentID domain.DeploymentID) (domain.Execution[struct{}], error) {
	inst := w.registry.getInstance(deploymentID)

	done := make(chan orchResult, 1)

	go func() {
		record := &baseRecord{
			id:     string(deploymentID),
			ctx:    ctx,
			events: inst.events,
		}
		val, err := w.spec.Run(record, deploymentID)
		w.registry.removeInstance(deploymentID)
		done <- orchResult{val: val, err: err}
	}()

	return &orchExecution{id: string(deploymentID), done: done}, nil
}

// --- CreateDeploymentWorkflow ---

type createDeploymentWorkflow struct {
	spec *domain.CreateDeploymentWorkflowSpec
}

func (w *createDeploymentWorkflow) Start(ctx context.Context, input domain.CreateDeploymentInput) (domain.Execution[domain.Deployment], error) {
	done := make(chan createResult, 1)

	go func() {
		record := &baseRecord{
			id:  "create-" + string(input.ID),
			ctx: ctx,
		}
		val, err := w.spec.Run(record, input)
		done <- createResult{val: val, err: err}
	}()

	return &createExecution{id: "create-" + string(input.ID), done: done}, nil
}

// --- shared base Record ---

type baseRecord struct {
	id     string
	ctx    context.Context
	events <-chan domain.DeploymentEvent
}

func (r *baseRecord) ID() string              { return r.id }
func (r *baseRecord) Context() context.Context { return r.ctx }
func (r *baseRecord) Run(activity domain.Activity[any, any], in any) (any, error) {
	return activity.Run(r.ctx, in)
}

func (r *baseRecord) Await(signalName string) (any, error) {
	select {
	case event := <-r.events:
		return event, nil
	case <-r.ctx.Done():
		return nil, r.ctx.Err()
	}
}

// --- Executions and result types ---

type orchResult struct {
	val struct{}
	err error
}

type orchExecution struct {
	id   string
	done <-chan orchResult
}

func (e *orchExecution) WorkflowID() string { return e.id }
func (e *orchExecution) AwaitResult(ctx context.Context) (struct{}, error) {
	select {
	case r := <-e.done:
		return r.val, r.err
	case <-ctx.Done():
		return struct{}{}, ctx.Err()
	}
}

type createResult struct {
	val domain.Deployment
	err error
}

type createExecution struct {
	id   string
	done <-chan createResult
}

func (e *createExecution) WorkflowID() string { return e.id }
func (e *createExecution) AwaitResult(ctx context.Context) (domain.Deployment, error) {
	select {
	case r := <-e.done:
		return r.val, r.err
	case <-ctx.Done():
		return domain.Deployment{}, ctx.Err()
	}
}

// Compile-time interface checks.
var (
	_ domain.Registry             = (*Registry)(nil)
	_ domain.OrchestrationWorkflow = (*orchestrationWorkflow)(nil)
	_ domain.CreateDeploymentWorkflow = (*createDeploymentWorkflow)(nil)
)
