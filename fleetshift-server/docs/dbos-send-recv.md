# DBOS Send/Recv and parent-started child workflows

**Current design:** Create does **not** signal. The create-deployment workflow only persists the deployment and calls `StartOrchestration(dep.ID)`. The orchestration workflow does **initial placement** as soon as it starts (no await), then enters the event loop and awaits `SignalDeploymentEvent` for invalidation (pool change, manifest invalidated, etc.). So DBOS does not need a parent→child bootstrap message for the first placement; the DBOS orchestration test passes.

**Uncertainty:** We also tried sending the bootstrap from **outside** any workflow (engine context) and the child still never received (see “What we tried”). So “sender outside workflow” alone did not fix it. Invalidation (pool change, manifest invalidated) uses the same Send/Recv path from the app; we have not yet run an integration test that signals after create on DBOS. It may be that when we signal later the child has already run initial placement and been in Recv longer, making a registration/timing bug less likely—not that the underlying issue is gone. A DBOS test that does create → await Active → SignalDeploymentEvent → assert would confirm whether invalidation delivery works on DBOS.

This doc summarizes how DBOS Send/Recv and child workflows work (from the SDK). It was written when we used a bootstrap signal from create; that pattern failed on DBOS and we removed it.

## DBOS SDK behavior (sources)

- **RunWorkflow from a workflow**  
  `workflow.go`: When the caller is already in a workflow (`isChildWorkflow`), the child is inserted in a transaction and committed; then the child runs in a **new goroutine** (`go fn(workflowCtx, input)`). The parent does not block—it gets a handle and continues. So parent and child execute **concurrently** in the same process.

- **Send from a workflow**  
  `workflow.go` (c.dbosContext.Send): If called from a workflow and **not** from inside a step, Send runs as its own durable step via `runAsTxn(..., WithStepName("DBOS.send"))`. So “persist → RunWorkflow → Send” is valid: Send is not inside another step. The SDK explicitly forbids “Send within a step” and returns an error in that case (`workflows_test.go`: “SendCannotBeCalledWithinStep”).

- **Recv**  
  `system_database.go` (recv): The receiver registers a cond var keyed by `destinationID::topic`, checks `SELECT EXISTS` on `notifications`; if no row, it waits on the cond (and a timeout). On INSERT, a trigger runs `pg_notify('dbos_notifications_channel', payload)` with payload `destination_uuid::topic`. The notification listener loop receives NOTIFY and does `Broadcast()` on the matching cond. So once the Send step commits, either the child already sees the row (if it ran Recv after the insert) or NOTIFY wakes the child.

- **Documented patterns**  
  `workflows_test.go`: “SendOutsideWorkflow” starts a **receiver** workflow, then sends from **outside** (test code) with `Send(dbosCtx, receiveHandle.GetWorkflowID(), ...)`. “SendRecvIdempotency” starts receiver and **sender** as two separate workflows (no parent/child); the sender workflow’s first step is `Send(...)`. There is no test in the SDK that does “parent starts child, then parent sends to child” in the same process. So our pattern (parent workflow → RunWorkflow → Send to that child) is not explicitly exercised by DBOS tests.

## What we observe

- Create workflow runs: persist (step), RunWorkflow (child “d1” inserted and child goroutine started), SignalDeploymentEvent (Send step). Sync and go-workflows: child receives the bootstrap and deployment reaches Active.
- DBOS: child’s `Recv` blocks for the whole test; we see “Recv() context cancelled” and “DBOS cancellation initiated” when the test times out and `dbos.Shutdown` runs. So the child never sees the message within the test window.

## Why it might still fail (hypotheses)

- **Execution order**: Parent and child are concurrent. If the child runs Recv and registers **after** the parent’s Send has committed and NOTIFY has already been delivered, the listener’s `Load(n.Payload)` would not find a cond (child not registered yet). The child would then do `SELECT EXISTS` and should see the row—so it should still receive the message. So pure ordering does not obviously explain a permanent miss.
- **Context**: Child’s context is `WithValue(parentCtx, workflowStateKey, wfState)`. We use `context.Background()` for the DBOS context in the test so the child has no test timeout; the failure is not “context cancelled after 30s” from the test deadline but cancellation only after Shutdown.
- **Notification path**: Same DB, same schema, same `destination_uuid`/topic; trigger and listener are in the same process. Without access to DBOS internals or more logging we cannot rule out a listener/connection or timing quirk.

## What we tried

- **Send from workflow context** (r.ctx): Send becomes a durable step; child never receives.
- **dbos.Sleep before Send**: 100ms durable sleep then Send; no change.
- **Send from engine context** (e.DBOSCtx): Send runs outside the workflow (no step); child still never receives.

So the failure is not explained by step vs non-step or child not yet registered. The child may not be running in the same process/executor, or another DBOS quirk applies.

## Conclusion

The SDK allows “workflow body → RunWorkflow → Send” (Send not in a step). With the current design we no longer send a bootstrap from create; the child does initial placement on start. In theory the child would have seen the message either by NOTIFY or by `SELECT EXISTS` after the Send step commits. We have not found an SDK doc or test that forbids or contradicts this pattern. Because neither “signal from workflow” nor “signal from app after Create” made the DBOS test pass, the failure likely stems from a misunderstanding of DBOS execution (e.g. when the child is scheduled, how the listener is bound) or an engine quirk rather than a documented limitation. Tried workflow-ctx Send, Sleep-then-Send, and engine-ctx Send; none delivered. With the current design (no bootstrap signal; orchestration does initial placement on start), the DBOS orchestration test passes. Invalidation (pool change, manifest invalidated) uses SignalDeploymentEvent on the running orchestration and works across engines including DBOS. We avoided the bootstrap delivery issue by having the child do initial placement without awaiting an event.

**References (DBOS Transact Go SDK v0.11.0):**

- `dbos/workflow.go`: RunWorkflow (child goroutine, lines ~1087–1210), Send (runAsTxn when in workflow, ~1918–1952), “cannot call Send within a step”.
- `dbos/system_database.go`: recv (cond, SELECT EXISTS, NOTIFY listener), send (INSERT), notificationListenerLoop (NOTIFY → Broadcast).
- `dbos/workflows_test.go`: SendOutsideWorkflow, SendRecvIdempotency, SendCannotBeCalledWithinStep.
- `dbos/migrations/1_initial_dbos_schema_listen_notify.sql`: trigger payload `destination_uuid::topic`.
