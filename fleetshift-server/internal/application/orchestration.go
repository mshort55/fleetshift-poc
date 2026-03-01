package application

import (
	"context"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// OrchestrationService delivers events to running orchestration workflows.
type OrchestrationService struct {
	Workflow domain.OrchestrationRunner
}

// SignalDeploymentEvent delivers a [domain.DeploymentEvent] to the
// running orchestration workflow for the given deployment.
func (o *OrchestrationService) SignalDeploymentEvent(ctx context.Context, deploymentID domain.DeploymentID, event domain.DeploymentEvent) error {
	return o.Workflow.SignalDeploymentEvent(ctx, deploymentID, event)
}
