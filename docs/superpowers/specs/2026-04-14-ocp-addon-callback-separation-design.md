# OCP Addon Callback Separation Design

**Date:** 2026-04-14
**Status:** Proposed
**Related:** Addon, Agent & Fleetlet Analysis (external doc)

---

## Problem

The OCP delivery agent's callback service is tightly integrated into fleetshift-server's core gRPC infrastructure. This creates coupling that conflicts with the addon model, where fleetshift-server should remain general-purpose and addons should own their specific concerns.

Specific problems:
1. **Callback proto in platform namespace** — `proto/fleetshift/v1/ocp_engine_callback_service.proto` sits alongside DeploymentService and AuthMethodService, appearing to be a platform API when it is an addon-internal protocol between ocp-engine and the OCP addon.
2. **Callback service on the main gRPC server** — `RegisterOCPEngineCallbackServiceServer` is called on the same `grpc.Server` as all platform services, and the platform's auth interceptor has 4 explicit skip-list entries for callback methods.
3. **In-process state sharing via sync.Map** — The callback server and agent share memory directly. While this works in-process, it establishes a pattern that cannot survive when addons move out-of-process.

These are symptoms of the same structural issue: the addon is modifying the platform's server configuration instead of owning its own infrastructure.

## Target Architecture

The addon model described in the fleetlet analysis envisions addons as separate services that fleetshift-server calls via a remote `DeliveryAgent` interface (gRPC). The addon owns everything internally — its own listener, its own auth, its own subprocess management — and reports final results back through `Deliver()`/`Remove()`.

```
fleetctl ──► fleetshift-server ──► ocp-addon (own gRPC server)
                                      │
                                      ├── callback listener
                                      │      ▲
                                      │      │
                                      └── ocp-engine subprocess
```

ocp-engine calls back to the addon directly. Fleetshift-server never sees callback traffic.

## Design: Addon-Owned Callback Server

### Approach

The OCP addon spins up its own `grpc.Server` on a dedicated port for callback traffic. The main fleetshift-server gRPC server no longer registers the callback service or maintains auth skip-list entries for it.

This structurally mirrors the target out-of-process architecture while remaining in-process today. When the addon eventually moves to its own binary, the `Start()`/`Shutdown()` calls move to the addon binary's `main()` and fleetshift-server replaces the in-process `DeliveryAgent` with a remote gRPC client.

### Agent Lifecycle Interface

The OCP agent gains `Start()` and `Shutdown()` methods:

```go
// Start launches the addon's internal callback gRPC server on the
// given address. The server uses its own token-auth interceptor
// (CallbackTokenSigner.Verify) — it does not share auth with the
// main fleetshift-server.
func (a *Agent) Start(callbackAddr string) error

// Shutdown gracefully stops the callback gRPC server.
func (a *Agent) Shutdown(ctx context.Context)
```

serve.go usage:

```go
if err := ocpAgent.Start(callbackAddr); err != nil {
    return fmt.Errorf("start ocp agent: %w", err)
}
defer ocpAgent.Shutdown(ctx)
```

### New File: `addon/ocp/server.go`

A new file in the addon package owns the callback gRPC server lifecycle:

1. Creates a `grpc.NewServer()` with a token-auth unary interceptor
2. Registers `OCPEngineCallbackServiceServer` on it (the existing `callbackServer` struct)
3. Listens on the callback address
4. Handles graceful shutdown

The token-auth interceptor calls `CallbackTokenSigner.Verify()` for every inbound RPC. No skip list is needed because every method on this server requires a callback token.

### Proto Relocation

The callback proto moves from the platform namespace to an addon-specific namespace:

```
# Before
proto/fleetshift/v1/ocp_engine_callback_service.proto
  → generates into fleetshift-server/gen/fleetshift/v1/

# After
proto/ocp/v1/callback_service.proto
  → generates into gen/ocp/v1/
```

Both `ocp-engine/go.mod` and `fleetshift-server/go.mod` reference the shared generated code via `replace` directives, matching the existing monorepo pattern. Neither module "owns" the generated code — it is a shared contract.

`buf.yaml` and `buf.gen.yaml` at the repo root need updating to include the new `proto/ocp/v1/` path and direct generated output to `gen/ocp/v1/`. The existing `fleetshift/v1/` generation config is unchanged.

The `fleetshift/v1/` proto package retains only platform services: DeploymentService, AuthMethodService, SignerEnrollmentService, FleetletService. The `ocp_engine_callback_service.proto` file is deleted from `proto/fleetshift/v1/` and its generated code removed from `fleetshift-server/gen/fleetshift/v1/`.

### Why gRPC (Not HTTP)

The callback protocol stays as gRPC rather than switching to HTTP. The protocol is expected to grow beyond simple ack responses to include:

- **Retry negotiation** — addon responds with retry parameters instead of a bare ack
- **Dynamic reconfiguration** — addon pushes updated config mid-provision
- **Bidirectional streaming** — log streaming, cancellation signals
- **New event types** — adding an RPC is a proto change + handler

Protobuf's field-level backward compatibility means new fields can be added to existing messages without breaking older ocp-engine binaries. gRPC is neutral once it's internal to the addon — nobody outside cares about the transport.

### Changes to fleetshift-server Core

All changes are removals:

1. **Remove** `pb.RegisterOCPEngineCallbackServiceServer(grpcServer, ocpAgent.CallbackServer())`
2. **Remove** the 4 callback method entries from `WithSkipMethods()`
3. **Remove** the callback proto import (if only used for registration)
4. **Add** `ocpAgent.Start(callbackAddr)` / `defer ocpAgent.Shutdown(ctx)`
5. **Add** a `--ocp-callback-addr` CLI flag (defaulting to `:50052`)

The `CallbackServer()` method on the agent is no longer exported.

### Changes to Existing Addon Files

- **`agent.go`**: `WithCallbackAddr` configures the addon's own listener address, not a reference to the main server. Remove the exported `CallbackServer()` method.
- **`callback_server.go`**: No changes. Handlers are identical, just registered on a different `grpc.Server`.
- **`callbacktoken.go`**: No changes.

### Changes to ocp-engine

- **`internal/callback/client.go`**: Update import path from `fleetshiftv1` to `ocpv1` (the new generated package). No behavioral changes.

### Data Flow After Change

```
fleetctl ──► :50051 fleetshift-server
                 │
                 │ router.Deliver("ocp", ...)
                 ▼
             ocp addon (in-process)
                 │
                 ├── Start(:50052) ← addon owns this listener
                 │       ▲
                 │       │ callback RPCs (token-authed)
                 │       │
                 └── ocp-engine subprocess
                         (--callback-url :50052)
```

### Future: Moving Out-of-Process

When the addon moves to its own binary:

1. `server.go` and `agent.go` move to the addon binary's `main()`
2. The addon binary listens on two ports: one for `DeliveryAgent` RPCs from fleetshift-server, one for callbacks from ocp-engine
3. fleetshift-server replaces the in-process agent with a remote gRPC client implementing `DeliveryAgent`
4. ocp-engine changes nothing — it still connects to the addon's callback port
5. The callback proto and shared gen code stay in the monorepo or move with the addon

### What the Addon Owns After This Change

- Its own `grpc.Server` on a dedicated port
- Its own auth interceptor (callback token verification)
- Its own callback proto definition (in `proto/ocp/v1/`)
- The full callback lifecycle: token minting, subprocess launch, callback receipt, result reporting via `signaler.Done()`

### What fleetshift-server Core Knows About the OCP Addon

- It implements `DeliveryAgent` (`Deliver`/`Remove`)
- It has `Start(addr)` and `Shutdown(ctx)` for lifecycle
- Nothing else. No callback proto, no auth skip list, no service registration.

---

## Design: Move region/role_arn from Target Properties to Cluster Spec

### Problem

The `ocp-aws` target is seeded in serve.go with `region` and `role_arn` read from environment variables at server startup. This couples target registration to environment configuration and locks a target into a single region/role. It also means the properties are empty strings if the env vars aren't set, causing silent failures at delivery time.

### Design

Move `region` and `role_arn` into the `ClusterSpec` — the per-deployment manifest that the user submits. The target represents "I can provision OCP clusters on AWS" without encoding where. Each deployment specifies its own region and role.

**ClusterSpec gains two fields:**

```go
type ClusterSpec struct {
    Name          string         `json:"name"`
    BaseDomain    string         `json:"base_domain"`
    Region        string         `json:"region"`
    RoleARN       string         `json:"role_arn"`
    ReleaseImage  string         `json:"release_image,omitempty"`
    InstallConfig map[string]any `json:"install_config,omitempty"`
}
```

**Validation moves from target property checks to ClusterSpec validation** in `ParseClusterSpec()`:

```go
if spec.Region == "" {
    return nil, fmt.Errorf("cluster spec missing required field: region")
}
if spec.RoleARN == "" {
    return nil, fmt.Errorf("cluster spec missing required field: role_arn")
}
```

**Deliver() reads from spec instead of target:**

```go
// Before
region := target.Properties["region"]
roleARN := target.Properties["role_arn"]

// After
region := spec.Region
roleARN := spec.RoleARN
```

**Remove() still reads from target properties** — but these are the *provisioned* target's properties (set by `ClusterOutput.Target()` when the cluster was created), not the seed target's properties. `ClusterOutput` already stores `region` in the provisioned target's properties (`cluster_output.go:63`), and `role_arn` should be added to it so Remove can retrieve it.

**ocp-aws target seed drops Properties entirely:**

```go
targetSvc.Register(ctx, domain.TargetInfo{
    ID:                    "ocp-aws",
    Type:                  ocpaddon.TargetType,
    Name:                  "OCP on AWS",
    AcceptedResourceTypes: []domain.ResourceType{ocpaddon.ClusterResourceType},
})
```

**Example deployment manifest after this change:**

```yaml
name: my-cluster
base_domain: example.com
region: us-east-1
role_arn: arn:aws:iam::123456789012:role/fleetshift-provision
install_config:
  controlPlane:
    replicas: 3
```
