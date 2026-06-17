# Kubernetes Addon

## What this doc covers

The built-in addon for direct Kubernetes cluster management:

- addon descriptor and capabilities (`DeliveryCapability`, `IndexCapability`)
- the Agent as the per-target unit of work
- deployment models: in-process and external agent
- the delivery pipeline: attested and passthrough modes, server-side apply, removal
- the indexing pipeline: LIST+WATCH informers, event batching, inventory writes
- schema-driven field extraction and the default schema

## When to read this

Read this when you are working on the kubernetes addon, extending its delivery or indexing behavior, adding indexed resource types, or understanding how a Kubernetes cluster becomes a managed target.

## What is intentionally elsewhere

- Core delivery contract and target model: [../architecture/target_delivery_contract.md](../architecture/target_delivery_contract.md)
- Delivery authorization, attestation, and the trust model: [../authentication.md](../authentication.md)
- Fleet-wide indexing model and the inventory item shape: [../architecture/resource_indexing.md](../architecture/resource_indexing.md)
- Addon lifecycle and capability model: [../architecture/addon_integration.md](../architecture/addon_integration.md)
- Core vocabulary (fulfillment, strategies, delivery agents): [../architecture/core_model.md](../architecture/core_model.md)

## Related docs

- [../architecture.md](../architecture.md)
- [../architecture/addon_integration.md](../architecture/addon_integration.md)
- [../architecture/resource_indexing.md](../architecture/resource_indexing.md)

## Overview

The kubernetes addon is a delivery agent and indexer for Kubernetes clusters. It implements the delivery contract via server-side apply and the indexing contract via LIST+WATCH against the cluster's API server.

The core invariant is one Agent per target cluster. Each Agent holds shared Kubernetes clients and delegates to two independent delegates: a delivery delegate that applies and removes manifests, and an indexer delegate that watches resources and writes inventory. The two delegates operate independently.

The addon supports two deployment models: in-process within the platform server and as an external agent attached to a fleetlet. The Agent and its delegates are the reusable core across both models. What changes between models is the wiring — how the Agent is created, how delivery requests reach it, and how inventory writes reach the platform.

## Addon descriptor and capabilities

The addon declares two capabilities for the `kubernetes` target type: delivery and indexing.

```go
AddonDescriptor{
    ID:   "kubernetes",
    Name: "Kubernetes Agent",
    Capabilities: []Capability{
        DeliveryCapability{TargetType: "kubernetes"},
        IndexCapability{TargetType: "kubernetes"},
    },
}
```

`DeliveryCapability` declares that the addon provides a delivery agent for Kubernetes targets — it can apply and remove manifests via server-side apply. `IndexCapability` declares that the addon provides an indexer agent — it watches cluster resources and writes observed inventory to the platform. These are independent capabilities: an addon could declare one without the other. For example, an observation-only addon could declare only `IndexCapability`, while a simple apply-only addon could declare only `DeliveryCapability`.

In the current POC, the descriptor is compiled into the server binary. Enable and Connect happen at startup. In a future external agent model, the descriptor would be provided dynamically at addon registration time, and the Agent would connect through a fleetlet channel rather than being wired in-process.

The addon does not declare a managed resource capability. Kubernetes targets are registered through the platform's target management API, not through addon-owned managed resources.

## Agent

The Agent is the primary abstraction — the per-target unit that does all delivery and indexing work for a single Kubernetes cluster. It holds shared Kubernetes clients and two delegates:

- a **delivery delegate** for manifest apply and removal
- an **indexer delegate** for resource watching and inventory writes

When started, the Agent runs the indexer delegate continuously. Delivery operations are dispatched to the delivery delegate when delivery requests arrive. The Agent stops when its context is cancelled.

### Target properties

A kubernetes target provides its connection details through properties:

| Property | Required | Description |
| --- | --- | --- |
| `api_server` | yes | Kubernetes API server URL |
| `ca_cert` | no | PEM-encoded CA certificate for TLS verification |
| `service_account_token` | no | Direct bearer token for authentication |
| `service_account_token_ref` | no | Vault secret reference resolved at connect time |

One of `service_account_token` or `service_account_token_ref` must be set. Direct tokens take precedence. In the external agent model, the Agent may use in-cluster credentials instead, making these properties unnecessary.

## Deployment models

The Agent is the reusable core. How it gets created and wired depends on the deployment model.

### In-process

In the in-process model, the platform server hosts multiple Agents — one per registered kubernetes target. A Manager handles this:

- **Agent registry**: tracks running Agents by target ID
- **Delivery routing**: implements `domain.DeliveryAgent` by dispatching to the correct Agent
- **Lifecycle handling**: creates Agents when targets become ready, destroys them when targets are terminated
- **Client construction**: builds Kubernetes client configuration from target properties and vault-backed service account tokens
- **Inventory cleanup**: deletes all inventory for a target on termination

Agent creation is idempotent — concurrent requests for the same target do not create duplicates.

The Manager is in-process infrastructure, not part of the addon's core model. The key interfaces it passes to each Agent — `InventoryWriter` and `DeliveryReporter` — are the modularity boundaries that enable the external deployment model.

### External agent

In the external model, the Agent runs as its own process on or near the target cluster — one Agent per cluster. There is no Manager. The process creates a single Agent directly, using in-cluster credentials or configuration flags for the Kubernetes client, and wiring the delegates to fleetlet channel adapters.

The `InventoryWriter` and `DeliveryReporter` interfaces are the key modularity boundaries. In-process, these are backed by application-layer services (direct function calls). In the external model, they would be backed by fleetlet channel adapters implementing the same interfaces. The Agent and its delegates are identical in both models — only the interface implementations change.

| Concern | In-process | External agent |
| --- | --- | --- |
| Agent creation | Manager builds from target properties + vault | Process builds from in-cluster config |
| Delivery requests | Direct calls from delivery router via Manager | Fleetlet delivery channel |
| Inventory writes | Direct InventoryWriteService | Fleetlet index channel adapter |
| Delivery reporting | Direct DeliveryReportService | Fleetlet delivery channel adapter |
| Lifecycle | Platform signals target ready/terminated | Process start/stop IS the lifecycle |

The actual fleetlet channel integration is not yet built, but no structural changes to the Agent or its delegates should be required.

## Delivery pipeline

The delivery delegate handles manifest apply and removal. It supports two delivery modes based on whether an attestation is present.

### Attested delivery

When an attestation is provided:

1. Verify the attestation against the target's trust bundle and the expected generation
2. On success: apply using the platform's service account
3. On failure: report `AuthFailed` to the delivery reporter

The trust bundle is a JSON array of trusted issuers stored as a target property. Each entry specifies an issuer URL, JWKS URI, audience, and key claim mappings. Verifiers are cached per target to avoid rebuilding on every delivery.

### Passthrough delivery

When no attestation is provided, the delivery falls through to passthrough mode. The caller must provide a bearer token. The delivery uses the caller's credentials directly, so the caller's own Kubernetes RBAC governs what gets applied. The platform's service account is never used.

### Server-side apply

Both modes use the same apply path. Manifests are applied sequentially via server-side apply with field manager `fleetshift`. Progress events are reported per manifest. A failure on any manifest stops the delivery. Kubernetes 401/403 errors map to `AuthFailed`; all other errors map to `Failed`.

### Removal

Removal follows the same attestation-or-passthrough flow as delivery, then deletes each resource. Resources that are already gone (404) are silently treated as success, making removal idempotent.

### Async dispatch

Validation and attestation verification happen synchronously. The actual apply or delete is dispatched asynchronously so that the operation completes even if the caller's context is cancelled.

## Indexing pipeline

The indexing pipeline watches Kubernetes resources on the target cluster and writes extracted inventory items to the platform. It is a three-stage system: informers produce events, a writer batches them, and the inventory writer persists them.

```text
InformerManager
  |
  +-- GenericInformer (pods)  --+
  +-- GenericInformer (nodes) --+--> eventCh / resyncCh --> Writer --> InventoryWriter
  +-- GenericInformer (...)   --+
```

### Informer manager

The `InformerManager` reconciles running informers against a desired set of GVRs:

1. Query the cluster's supported resources via discovery (filtered to WATCH-capable resources)
2. Intersect the desired set with the supported set to get the effective set
3. Stop informers for GVRs no longer in the effective set
4. Start informers for new GVRs, serialized with an initialization timeout to avoid memory spikes

The addon only watches resources the cluster actually supports. If a desired GVR (e.g. a CRD) is not present on the cluster, it is silently skipped.

### GenericInformer

Each `GenericInformer` implements LIST+WATCH for a single GVR with minimal memory overhead. It tracks only UID-to-resourceVersion mappings, not full objects.

**LIST phase**: paginated LIST, sending an `Add` event for each resource. Stale resources (UIDs in the previous index but absent from the LIST) produce `Delete` events. After the LIST completes, a `ResyncEvent` with the full resource set is sent, and the list's resourceVersion is saved for watch continuity.

**WATCH phase**: WATCH from the saved list resourceVersion. `Add`, `Update`, and `Delete` events are dispatched from the watch stream. On watch error or channel close, the informer returns to the LIST phase.

**Shutdown**: when the context is cancelled, the informer sends `Delete` events for all tracked resources, ensuring the inventory is cleaned up.

**Retry**: on LIST or WATCH errors, the informer retries with exponential backoff. Retries reset on a successful LIST+WATCH cycle.

### Writer

The Writer batches informer events and flushes them to the `InventoryWriter` at a configurable interval.

**Event handling**:

- `Add` / `Update`: buffer the resource in pending upserts keyed by UID
- `Delete`: add the UID to pending deletes and remove any pending upsert for that UID
- Late-delete protection: if a UID was deleted in the current batch, subsequent `Add`/`Update` events for that UID are dropped

**Flush** (on batch timer): for each pending upsert, skip if the resourceVersion is unchanged since the last flush (deduplication). Extract observed fields and call `InventoryWriter.ApplyDelta` with the upserts and deleted IDs.

**Resync** (on `ResyncEvent`): extract observed fields for all resources and call `InventoryWriter.Resync` to atomically replace all items for the target+type. The resync path bypasses the batch timer and writes immediately.

All event processing is serialized on a single goroutine for ordering safety. Late-delete protection and resourceVersion deduplication ensure correct behavior under interleaved events.

### InventoryWriter

The `InventoryWriter` interface models the addon-to-platform direction of the indexing protocol:

- **ApplyDelta**: upserts and deletes in a single transaction. This is the incremental update path.
- **Resync**: atomically replaces all items for a target+type. This is the full-sync path used on initial list and after errors.

In-process, the `InventoryWriteService` wraps each operation in a store transaction. In the external model, a fleetlet channel adapter would implement the same interface.

## Schema and field extraction

### Schema model

The indexing schema defines which resource types to watch and which fields to extract from each:

- **SchemaEntry**: a single resource type, identified by GVR and Kind. Contains a list of field extractions and a flag for condition extraction.
- **FieldExtraction**: a named field with a JSONPath expression and a data type for coercion.
- **IndexSchema**: the complete set of entries, keyed by GVR. Also provides the list of GVRs for the informer manager.

### Data types

| Type | Coercion |
| --- | --- |
| `string` | Default. Value returned as-is. |
| `number` | Coerced to float64. Accepts int, int64, float64, and string. |
| `bytes` | Parsed as a Kubernetes quantity (e.g. `1Gi`) and converted to bytes as float64. |
| `slice` | All JSONPath results collected into a flat list. |
| `mapString` | Map values coerced to strings via `fmt.Sprint`. |

### Extraction

`ExtractObservedResource` converts an unstructured Kubernetes resource and its schema entry into a domain `InventoryItem`:

1. Build the inventory type from `apiVersion` and `kind` (e.g. `apps/v1/Deployment`, `v1/Pod`)
2. Copy labels
3. Extract `.status.conditions` if configured
4. Evaluate JSONPath expressions for each schema field, coercing by data type
5. Build the item with ID `targetID/UID`, the computed inventory type, resource name, labels, observed fields, and conditions

### Default schema

The default schema indexes core Kubernetes resource types: workloads (Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, CronJobs), pods, services, nodes, namespaces, storage (PVCs, PVs), and configuration (ConfigMaps, Secrets). For most types, key status fields and replica counts are extracted. Nodes and pods include conditions. ConfigMaps and Secrets are indexed for identity and labels only — no fields are extracted.

## Open questions

### Schema extensibility

The default schema is compiled into the binary. There is no mechanism for users or addons to customize which resource types are indexed or which fields are extracted per target. A future iteration could support per-target schema overrides via target properties or a separate configuration API.

### Drift detection

The indexing pipeline observes cluster state but does not compare it against delivery intent. Drift detection — identifying resources that have diverged from their delivered manifests — is not yet implemented. See [../architecture/target_delivery_contract.md](../architecture/target_delivery_contract.md) for the design-level discussion.
