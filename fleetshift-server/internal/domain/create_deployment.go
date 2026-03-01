package domain

import (
	"context"
	"fmt"
)

// CreateDeploymentInput is the specification for creating a new deployment.
type CreateDeploymentInput struct {
	ID                DeploymentID
	ManifestStrategy  ManifestStrategySpec
	PlacementStrategy PlacementStrategySpec
	RolloutStrategy   *RolloutStrategySpec
}

// CreateDeploymentWorkflowRunner is the execution-time capability passed
// into [CreateDeploymentWorkflow.Run]. It extends [DurableRunner] with the
// ability to start the orchestration child workflow. The child performs
// initial placement without awaiting an event; [OrchestrationRunner.SignalDeploymentEvent]
// is used from the application when invalidation or other events occur.
type CreateDeploymentWorkflowRunner interface {
	DurableRunner

	// StartOrchestration durably starts the orchestration child
	// workflow for the given deployment. The child runs independently
	// and performs initial placement; this method returns once the start is recorded.
	StartOrchestration(deploymentID DeploymentID) error
}

// CreateDeploymentRunner starts create-deployment workflows (app-facing API).
type CreateDeploymentRunner interface {
	Run(ctx context.Context, input CreateDeploymentInput) (WorkflowHandle[Deployment], error)
}

// CreateDeploymentWorkflow is a short-lived parent workflow that
// persists a new deployment and starts the orchestration child
// workflow. Both steps are durable: on crash the engine replays
// from the last completed step.
type CreateDeploymentWorkflow struct {
	Deployments DeploymentRepository
}

func (w *CreateDeploymentWorkflow) Name() string { return "create-deployment" }

// PersistDeployment creates a pending deployment record.
func (w *CreateDeploymentWorkflow) PersistDeployment() Activity[CreateDeploymentInput, Deployment] {
	return NewActivity("persist-deployment", func(ctx context.Context, in CreateDeploymentInput) (Deployment, error) {
		dep := Deployment{
			ID:                in.ID,
			ManifestStrategy:  in.ManifestStrategy,
			PlacementStrategy: in.PlacementStrategy,
			RolloutStrategy:   in.RolloutStrategy,
			State:             DeploymentStatePending,
		}
		if err := w.Deployments.Create(ctx, dep); err != nil {
			return Deployment{}, err
		}
		return dep, nil
	})
}

// Run is the workflow body: persist the deployment, then start
// orchestration as a durable child workflow.
func (w *CreateDeploymentWorkflow) Run(runner CreateDeploymentWorkflowRunner, input CreateDeploymentInput) (Deployment, error) {
	dep, err := RunActivity(runner, w.PersistDeployment(), input)
	if err != nil {
		return Deployment{}, fmt.Errorf("persist deployment: %w", err)
	}

	if err := runner.StartOrchestration(dep.ID); err != nil {
		return Deployment{}, fmt.Errorf("start orchestration: %w", err)
	}

	return dep, nil
}
