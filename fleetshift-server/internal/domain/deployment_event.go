package domain

// DeliveryCompletionEvent records the terminal outcome of a single delivery.
type DeliveryCompletionEvent struct {
	DeliveryID DeliveryID
	Result     DeliveryResult
}

// DeploymentEvent is the signal delivered to a running reconciliation
// workflow. In the discrete-workflow model only delivery completion
// events are signaled; all other mutations bump the deployment's
// [Generation] and start a new reconciliation workflow.
type DeploymentEvent struct {
	DeliveryCompleted *DeliveryCompletionEvent
}
