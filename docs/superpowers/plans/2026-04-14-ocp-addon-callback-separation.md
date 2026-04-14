# OCP Addon Callback Separation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the OCP callback gRPC server out of fleetshift-server core and into the addon's own infrastructure, so fleetshift-server has zero knowledge of the callback protocol.

**Architecture:** The OCP addon spins up its own `grpc.Server` on a dedicated port (`:50052`) with its own token-auth interceptor. The callback proto moves from `proto/fleetshift/v1/` to `proto/ocp/v1/` with shared generated code in `gen/ocp/v1/` referenced by both Go modules via `replace` directives. serve.go loses all callback-related code and gains only `ocpAgent.Start(addr)` / `defer ocpAgent.Shutdown(ctx)`.

**Tech Stack:** Go, gRPC, protobuf, buf

---

## File Structure

### New files
- `proto/ocp/v1/callback_service.proto` — callback proto in addon namespace
- `buf.gen.ocp.yaml` — buf generation config for ocp proto only
- `gen/ocp/v1/*.pb.go` — shared generated code (output of buf generate)
- `gen/go.mod`, `gen/go.sum` — Go module for shared gen code
- `fleetshift-server/internal/addon/ocp/server.go` — addon-owned gRPC server lifecycle
- `fleetshift-server/internal/addon/ocp/server_test.go` — tests for the server

### Modified files
- `fleetshift-server/internal/addon/ocp/agent.go` — add Start/Shutdown, remove CallbackServer()
- `fleetshift-server/internal/addon/ocp/callback_server.go` — update imports from fleetshiftv1 to ocpv1
- `fleetshift-server/internal/addon/ocp/callback_server_test.go` — update imports
- `fleetshift-server/internal/addon/ocp/callbacktoken_test.go` — update imports (if needed)
- `fleetshift-server/internal/cli/serve.go` — remove callback registration, add agent lifecycle
- `fleetshift-server/go.mod` — add replace directive for gen module
- `ocp-engine/internal/callback/client.go` — update imports to use shared gen
- `ocp-engine/internal/callback/client_test.go` — update imports to use shared gen
- `ocp-engine/go.mod` — add replace directive for gen module
- `Makefile` — update generate target
- `docs/openapi/fleetshift.swagger.yaml` — regenerated without callback service

### Deleted files
- `proto/fleetshift/v1/ocp_engine_callback_service.proto` — moved to ocp namespace
- `fleetshift-server/gen/fleetshift/v1/ocp_engine_callback_service.pb.go` — regenerated without callback
- `fleetshift-server/gen/fleetshift/v1/ocp_engine_callback_service_grpc.pb.go` — regenerated without callback
- `ocp-engine/internal/callback/ocp_engine_callback_service.pb.go` — replaced by shared gen
- `ocp-engine/internal/callback/ocp_engine_callback_service_grpc.pb.go` — replaced by shared gen

---

### Task 1: Create callback proto in addon namespace

**Files:**
- Create: `proto/ocp/v1/callback_service.proto`
- Create: `buf.gen.ocp.yaml`
- Create: `gen/go.mod`
- Delete: `proto/fleetshift/v1/ocp_engine_callback_service.proto`

- [ ] **Step 1: Create the ocp proto directory**

```bash
mkdir -p proto/ocp/v1
```

- [ ] **Step 2: Create `proto/ocp/v1/callback_service.proto`**

```protobuf
syntax = "proto3";

package ocp.v1;

option go_package = "github.com/fleetshift/fleetshift-poc/gen/ocp/v1;ocpv1";

service CallbackService {
  rpc ReportPhaseResult(PhaseResultRequest) returns (Ack);
  rpc ReportMilestone(MilestoneRequest) returns (Ack);
  rpc ReportCompletion(CompletionRequest) returns (Ack);
  rpc ReportFailure(FailureRequest) returns (Ack);
}

message PhaseResultRequest {
  string cluster_id = 1;
  string phase = 2;
  string status = 3;
  int32 elapsed_seconds = 4;
  string error = 5;
  int32 attempt = 6;
}

message MilestoneRequest {
  string cluster_id = 1;
  string event = 2;
  int32 elapsed_seconds = 3;
  int32 attempt = 4;
}

message CompletionRequest {
  string cluster_id = 1;
  string infra_id = 2;
  string cluster_uuid = 3;
  string api_server = 4;
  bytes kubeconfig = 5;
  bytes ca_cert = 6;
  bytes ssh_private_key = 7;
  bytes ssh_public_key = 8;
  string region = 9;
  bytes metadata_json = 10;
  bool recovery_attempted = 11;
  int32 elapsed_seconds = 12;
  int32 attempt = 13;
}

message FailureRequest {
  string cluster_id = 1;
  string phase = 2;
  string failure_reason = 3;
  string failure_message = 4;
  string log_tail = 5;
  bool requires_destroy = 6;
  bool recovery_attempted = 7;
  int32 attempt = 8;
}

message Ack {}
```

Note: Message names drop the `OCPEngine` prefix since they're now in the `ocp.v1` package — `ocp.v1.CompletionRequest` is self-explanatory.

- [ ] **Step 3: Create `buf.gen.ocp.yaml`**

This is a separate buf generation config for the ocp proto only. It generates protobuf + gRPC code (no gateway, no openapi — this is an internal protocol).

```yaml
version: v2
clean: true
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt:
      - paths=source_relative
  - remote: buf.build/grpc/go
    out: gen
    opt:
      - paths=source_relative
```

- [ ] **Step 4: Create `gen/go.mod`**

```bash
cd gen && go mod init github.com/fleetshift/fleetshift-poc/gen
```

Then tidy after generation (step 6) to pick up protobuf/gRPC dependencies.

- [ ] **Step 5: Delete the old proto file**

```bash
rm proto/fleetshift/v1/ocp_engine_callback_service.proto
```

- [ ] **Step 6: Generate the shared code**

```bash
buf generate --template buf.gen.ocp.yaml --path proto/ocp/v1
```

Verify output exists:

```bash
ls gen/ocp/v1/
# Expected: callback_service.pb.go  callback_service_grpc.pb.go
```

Then tidy the gen module:

```bash
cd gen && go mod tidy
```

- [ ] **Step 7: Update `Makefile` generate target**

```makefile
generate: ## Generate protobuf and gRPC code
	buf generate
	buf generate --template buf.gen.ocp.yaml --path proto/ocp/v1
```

- [ ] **Step 8: Regenerate platform proto (without callback)**

```bash
buf generate
```

Verify the callback generated files are gone from fleetshift-server/gen:

```bash
ls fleetshift-server/gen/fleetshift/v1/ocp_engine_callback*
# Expected: No such file or directory
```

The `docs/openapi/fleetshift.swagger.yaml` will also regenerate without the `OCPEngineCallbackService` tag.

- [ ] **Step 9: Delete old ocp-engine generated code**

```bash
rm ocp-engine/internal/callback/ocp_engine_callback_service.pb.go
rm ocp-engine/internal/callback/ocp_engine_callback_service_grpc.pb.go
```

- [ ] **Step 10: Commit**

```bash
git add -A proto/ocp/ buf.gen.ocp.yaml gen/ Makefile
git add proto/fleetshift/v1/  # tracks deletion
git add fleetshift-server/gen/fleetshift/v1/ocp_engine_callback*  # tracks deletion
git add ocp-engine/internal/callback/ocp_engine_callback*  # tracks deletion
git add docs/openapi/fleetshift.swagger.yaml
git commit -m "chore: move callback proto from fleetshift/v1 to ocp/v1 namespace"
```

---

### Task 2: Update go.mod files with replace directives

**Files:**
- Modify: `fleetshift-server/go.mod`
- Modify: `ocp-engine/go.mod`

- [ ] **Step 1: Add replace directive to fleetshift-server/go.mod**

Add this line after the `go 1.25.0` directive:

```
replace github.com/fleetshift/fleetshift-poc/gen => ../gen
```

Then add the require:

```
require github.com/fleetshift/fleetshift-poc/gen v0.0.0
```

Run:

```bash
cd fleetshift-server && go mod tidy
```

- [ ] **Step 2: Add replace directive to ocp-engine/go.mod**

Add this line after the `go 1.25.0` directive:

```
replace github.com/fleetshift/fleetshift-poc/gen => ../gen
```

Then add the require:

```
require github.com/fleetshift/fleetshift-poc/gen v0.0.0
```

Run:

```bash
cd ocp-engine && go mod tidy
```

- [ ] **Step 3: Commit**

```bash
git add fleetshift-server/go.mod fleetshift-server/go.sum ocp-engine/go.mod ocp-engine/go.sum
git commit -m "chore: add replace directives for shared ocp gen module"
```

---

### Task 3: Update ocp-engine callback client to use shared gen

**Files:**
- Modify: `ocp-engine/internal/callback/client.go`
- Modify: `ocp-engine/internal/callback/client_test.go`

- [ ] **Step 1: Update client.go imports and type references**

Replace the unqualified type references with the `ocpv1` package. The key changes:

```go
package callback

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
)

type Client struct {
	conn      *grpc.ClientConn
	client    ocpv1.CallbackServiceClient
	clusterID string
	token     string
}

func New(addr, clusterID, token string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("callback: dial %s: %w", addr, err)
	}
	return &Client{
		conn:      conn,
		client:    ocpv1.NewCallbackServiceClient(conn),
		clusterID: clusterID,
		token:     token,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) withAuth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

func (c *Client) ReportPhaseResult(ctx context.Context, phase, status string, elapsed int32, errMsg string, attempt int32) error {
	_, err := c.client.ReportPhaseResult(c.withAuth(ctx), &ocpv1.PhaseResultRequest{
		ClusterId:      c.clusterID,
		Phase:          phase,
		Status:         status,
		ElapsedSeconds: elapsed,
		Error:          errMsg,
		Attempt:        attempt,
	})
	return err
}

func (c *Client) ReportMilestone(ctx context.Context, event string, elapsed, attempt int32) error {
	_, err := c.client.ReportMilestone(c.withAuth(ctx), &ocpv1.MilestoneRequest{
		ClusterId:      c.clusterID,
		Event:          event,
		ElapsedSeconds: elapsed,
		Attempt:        attempt,
	})
	return err
}

func (c *Client) ReportCompletion(ctx context.Context, data CompletionData) error {
	_, err := c.client.ReportCompletion(c.withAuth(ctx), &ocpv1.CompletionRequest{
		ClusterId:         c.clusterID,
		InfraId:           data.InfraID,
		ClusterUuid:       data.ClusterUUID,
		ApiServer:         data.APIServer,
		Kubeconfig:        data.Kubeconfig,
		CaCert:            data.CACert,
		SshPrivateKey:     data.SSHPrivateKey,
		SshPublicKey:      data.SSHPublicKey,
		Region:            data.Region,
		MetadataJson:      data.MetadataJSON,
		RecoveryAttempted: data.RecoveryAttempted,
		ElapsedSeconds:    data.ElapsedSeconds,
		Attempt:           data.Attempt,
	})
	return err
}

func (c *Client) ReportFailure(ctx context.Context, data FailureData) error {
	_, err := c.client.ReportFailure(c.withAuth(ctx), &ocpv1.FailureRequest{
		ClusterId:         c.clusterID,
		Phase:             data.Phase,
		FailureReason:     data.FailureReason,
		FailureMessage:    data.FailureMessage,
		LogTail:           data.LogTail,
		RequiresDestroy:   data.RequiresDestroy,
		RecoveryAttempted: data.RecoveryAttempted,
		Attempt:           data.Attempt,
	})
	return err
}
```

`CompletionData` and `FailureData` structs stay in `client.go` unchanged — they're the client's own types, not proto types.

- [ ] **Step 2: Update client_test.go imports and type references**

```go
package callback

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
)

type mockServer struct {
	ocpv1.UnimplementedCallbackServiceServer

	mu           sync.Mutex
	phaseResults []*ocpv1.PhaseResultRequest
	milestones   []*ocpv1.MilestoneRequest
	completions  []*ocpv1.CompletionRequest
	failures     []*ocpv1.FailureRequest
	authTokens   []string
}

func (m *mockServer) extractToken(ctx context.Context) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			m.authTokens = append(m.authTokens, vals[0])
		}
	}
}

func (m *mockServer) ReportPhaseResult(ctx context.Context, req *ocpv1.PhaseResultRequest) (*ocpv1.Ack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extractToken(ctx)
	m.phaseResults = append(m.phaseResults, req)
	return &ocpv1.Ack{}, nil
}

func (m *mockServer) ReportMilestone(ctx context.Context, req *ocpv1.MilestoneRequest) (*ocpv1.Ack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extractToken(ctx)
	m.milestones = append(m.milestones, req)
	return &ocpv1.Ack{}, nil
}

func (m *mockServer) ReportCompletion(ctx context.Context, req *ocpv1.CompletionRequest) (*ocpv1.Ack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extractToken(ctx)
	m.completions = append(m.completions, req)
	return &ocpv1.Ack{}, nil
}

func (m *mockServer) ReportFailure(ctx context.Context, req *ocpv1.FailureRequest) (*ocpv1.Ack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extractToken(ctx)
	m.failures = append(m.failures, req)
	return &ocpv1.Ack{}, nil
}

func startMockServer(t *testing.T) (*mockServer, string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	mock := &mockServer{}
	ocpv1.RegisterCallbackServiceServer(srv, mock)
	go func() { _ = srv.Serve(lis) }()
	return mock, lis.Addr().String(), func() { srv.Stop() }
}

func newTestClient(t *testing.T, addr, clusterID, token string) *Client {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return &Client{
		conn:      conn,
		client:    ocpv1.NewCallbackServiceClient(conn),
		clusterID: clusterID,
		token:     token,
	}
}

// Test functions (TestClient_ReportPhaseResult, etc.) remain identical
// except proto type references change:
//   OCPEnginePhaseResultRequest → ocpv1.PhaseResultRequest
//   OCPEngineMilestoneRequest   → ocpv1.MilestoneRequest
//   OCPEngineCompletionRequest  → ocpv1.CompletionRequest
//   OCPEngineFailureRequest     → ocpv1.FailureRequest
//   OCPEngineAck                → ocpv1.Ack
// The test logic and assertions are unchanged.
```

- [ ] **Step 3: Run ocp-engine tests**

```bash
cd ocp-engine && go test ./internal/callback/ -v -count=1
```

Expected: All 4 client tests pass.

- [ ] **Step 4: Commit**

```bash
git add ocp-engine/internal/callback/
git commit -m "refactor: update ocp-engine callback client to use shared gen module"
```

---

### Task 4: Update addon callback_server.go to use shared gen

**Files:**
- Modify: `fleetshift-server/internal/addon/ocp/callback_server.go`
- Modify: `fleetshift-server/internal/addon/ocp/callback_server_test.go`

- [ ] **Step 1: Update callback_server.go imports**

Replace:
```go
fleetshiftv1 "github.com/fleetshift/fleetshift-poc/fleetshift-server/gen/fleetshift/v1"
```

With:
```go
ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
```

Update all type references throughout the file:
- `fleetshiftv1.UnimplementedOCPEngineCallbackServiceServer` → `ocpv1.UnimplementedCallbackServiceServer`
- `fleetshiftv1.OCPEngineCompletionRequest` → `ocpv1.CompletionRequest`
- `fleetshiftv1.OCPEngineFailureRequest` → `ocpv1.FailureRequest`
- `fleetshiftv1.OCPEnginePhaseResultRequest` → `ocpv1.PhaseResultRequest`
- `fleetshiftv1.OCPEngineMilestoneRequest` → `ocpv1.MilestoneRequest`
- `fleetshiftv1.OCPEngineAck` → `ocpv1.Ack`
- `fleetshiftv1.OCPEngineCallbackServiceServer` → `ocpv1.CallbackServiceServer`

- [ ] **Step 2: Update callback_server_test.go imports**

Same import change: `fleetshiftv1` → `ocpv1`. Update all type references in test code to match:
- `fleetshiftv1.OCPEngineCompletionRequest` → `ocpv1.CompletionRequest`
- `fleetshiftv1.OCPEngineFailureRequest` → `ocpv1.FailureRequest`
- `fleetshiftv1.OCPEnginePhaseResultRequest` → `ocpv1.PhaseResultRequest`
- `fleetshiftv1.OCPEngineMilestoneRequest` → `ocpv1.MilestoneRequest`
- `fleetshiftv1.OCPEngineAck` → `ocpv1.Ack`

- [ ] **Step 3: Update agent.go import and type references**

In `agent.go`, replace the `fleetshiftv1` import with `ocpv1` and update the type references in `handleCompletion`:
- `fleetshiftv1.OCPEngineCompletionRequest` → `ocpv1.CompletionRequest`
- `fleetshiftv1.OCPEngineCallbackServiceServer` → `ocpv1.CallbackServiceServer`

Remove the `CallbackServer()` exported method entirely — it will be replaced by internal wiring in `server.go` (Task 5).

- [ ] **Step 4: Run addon tests**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -v -count=1
```

Expected: All tests pass (the tests don't depend on serve.go wiring).

- [ ] **Step 5: Commit**

```bash
git add fleetshift-server/internal/addon/ocp/
git commit -m "refactor: update ocp addon to use shared gen module"
```

---

### Task 5: Create addon-owned gRPC server

**Files:**
- Create: `fleetshift-server/internal/addon/ocp/server.go`
- Create: `fleetshift-server/internal/addon/ocp/server_test.go`

- [ ] **Step 1: Write the failing test for server Start/Shutdown**

Create `server_test.go`:

```go
package ocp

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
)

func TestServer_StartAndShutdown(t *testing.T) {
	agent := NewAgent(
		WithTokenSigner(mustNewSigner(t)),
	)

	if err := agent.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	addr := agent.CallbackAddr()
	if addr == "" {
		t.Fatal("CallbackAddr() returned empty after Start()")
	}

	// Verify server is accepting connections
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	agent.Shutdown(ctx)
}

func TestServer_AuthenticatedCallback(t *testing.T) {
	signer := mustNewSigner(t)
	agent := NewAgent(WithTokenSigner(signer))

	if err := agent.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer agent.Shutdown(context.Background())

	clusterID := "test-server-cluster"
	state := &provisionState{done: make(chan struct{})}
	agent.provisions.Store(clusterID, state)

	token, err := signer.Sign(clusterID, 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	conn, err := grpc.NewClient(agent.CallbackAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := ocpv1.NewCallbackServiceClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)

	_, err = client.ReportCompletion(ctx, &ocpv1.CompletionRequest{
		ClusterId: clusterID,
		InfraId:   "infra-server-test",
		ApiServer: "https://api.test.example.com:6443",
	})
	if err != nil {
		t.Fatalf("ReportCompletion: %v", err)
	}

	select {
	case <-state.done:
	default:
		t.Error("expected done channel to be closed after completion")
	}
}

func TestServer_RejectsUnauthenticatedCallback(t *testing.T) {
	signer := mustNewSigner(t)
	agent := NewAgent(WithTokenSigner(signer))

	if err := agent.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer agent.Shutdown(context.Background())

	conn, err := grpc.NewClient(agent.CallbackAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := ocpv1.NewCallbackServiceClient(conn)

	// No auth token
	_, err = client.ReportCompletion(context.Background(), &ocpv1.CompletionRequest{
		ClusterId: "some-cluster",
	})
	if err == nil {
		t.Fatal("expected error for unauthenticated callback, got nil")
	}
}

func mustNewSigner(t *testing.T) *CallbackTokenSigner {
	t.Helper()
	s, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner: %v", err)
	}
	return s
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -run TestServer -v -count=1
```

Expected: Compilation failure — `agent.Start`, `agent.Shutdown`, `agent.CallbackAddr` do not exist.

- [ ] **Step 3: Implement server.go**

```go
package ocp

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
)

// Start launches the addon's internal callback gRPC server on the
// given address. The server uses its own token-auth interceptor —
// it does not share auth with the main fleetshift-server.
//
// Use "host:0" to bind to a random available port (useful for tests).
// After Start returns, CallbackAddr() returns the resolved address.
func (a *Agent) Start(callbackAddr string) error {
	lis, err := net.Listen("tcp", callbackAddr)
	if err != nil {
		return fmt.Errorf("ocp callback: listen %s: %w", callbackAddr, err)
	}

	a.callbackAddr = lis.Addr().String()

	srv := grpc.NewServer()
	ocpv1.RegisterCallbackServiceServer(srv, &callbackServer{
		provisions:    &a.provisions,
		tokenVerifier: a.tokenSigner,
	})

	a.grpcServer = srv

	go func() {
		_ = srv.Serve(lis)
	}()

	return nil
}

// Shutdown gracefully stops the callback gRPC server.
func (a *Agent) Shutdown(ctx context.Context) {
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}
}

// CallbackAddr returns the resolved address the callback server is
// listening on. Only valid after Start() returns.
func (a *Agent) CallbackAddr() string {
	return a.callbackAddr
}
```

- [ ] **Step 4: Add grpcServer field to Agent struct in agent.go**

Add to the Agent struct:

```go
grpcServer *grpc.Server
```

Add the `grpc` import:

```go
"google.golang.org/grpc"
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -run TestServer -v -count=1
```

Expected: All 3 server tests pass.

- [ ] **Step 6: Run full addon test suite**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -v -count=1
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add fleetshift-server/internal/addon/ocp/server.go fleetshift-server/internal/addon/ocp/server_test.go fleetshift-server/internal/addon/ocp/agent.go
git commit -m "feat: add addon-owned callback gRPC server with Start/Shutdown lifecycle"
```

---

### Task 6: Update serve.go — remove callback from main server

**Files:**
- Modify: `fleetshift-server/internal/cli/serve.go`

- [ ] **Step 1: Remove callback service registration**

Delete this line (currently line 344):

```go
pb.RegisterOCPEngineCallbackServiceServer(grpcServer, ocpAgent.CallbackServer())
```

- [ ] **Step 2: Remove auth skip list entries**

Remove the 4 callback methods from the `WithSkipMethods` call (currently lines 286-291). Change from:

```go
authnInterceptor := transportgrpc.NewAuthnInterceptor(authMethodSvc, tokenVerifier, observability.NewAuthnObserver(logger),
    transportgrpc.WithSkipMethods(
        "/fleetshift.v1.OCPEngineCallbackService/ReportPhaseResult",
        "/fleetshift.v1.OCPEngineCallbackService/ReportMilestone",
        "/fleetshift.v1.OCPEngineCallbackService/ReportCompletion",
        "/fleetshift.v1.OCPEngineCallbackService/ReportFailure",
    ),
)
```

To:

```go
authnInterceptor := transportgrpc.NewAuthnInterceptor(authMethodSvc, tokenVerifier, observability.NewAuthnObserver(logger))
```

- [ ] **Step 3: Add `--ocp-callback-addr` flag**

Add to `serveFlags` struct:

```go
ocpCallbackAddr string
```

Add to `newServeCmd()`:

```go
cmd.Flags().StringVar(&f.ocpCallbackAddr, "ocp-callback-addr", ":50052", "OCP addon callback listen address")
```

- [ ] **Step 4: Replace callback wiring with agent lifecycle**

After the OCP agent creation block (around line 155), change from:

```go
ocpAgent := ocpaddon.NewAgent(
    ocpaddon.WithCallbackAddr(f.grpcAddr),
    ocpaddon.WithVault(vault),
    ocpaddon.WithCredentialProvider(ocpCredProvider),
    ocpaddon.WithTokenSigner(callbackSigner),
    ocpaddon.WithObserver(ocpaddon.NewSlogAgentObserver(logger)),
)
```

To:

```go
ocpAgent := ocpaddon.NewAgent(
    ocpaddon.WithVault(vault),
    ocpaddon.WithCredentialProvider(ocpCredProvider),
    ocpaddon.WithTokenSigner(callbackSigner),
    ocpaddon.WithObserver(ocpaddon.NewSlogAgentObserver(logger)),
)
if err := ocpAgent.Start(f.ocpCallbackAddr); err != nil {
    return fmt.Errorf("start ocp agent: %w", err)
}
defer ocpAgent.Shutdown(ctx)

logger.Info("OCP addon callback server listening", "addr", ocpAgent.CallbackAddr())
```

Note: `WithCallbackAddr` is no longer passed at construction time — `Start()` sets it.

- [ ] **Step 5: Verify build**

```bash
cd fleetshift-server && go build ./cmd/fleetshift
```

Expected: Compiles cleanly.

- [ ] **Step 6: Run full test suite**

```bash
make test
```

Expected: All tests pass (kind addon Docker failures are pre-existing and unrelated).

- [ ] **Step 7: Commit**

```bash
git add fleetshift-server/internal/cli/serve.go
git commit -m "refactor: remove callback service from main gRPC server, use addon lifecycle"
```

---

### Task 7: Move region/role_arn from target properties to ClusterSpec

**Files:**
- Modify: `fleetshift-server/internal/addon/ocp/cluster_spec.go`
- Modify: `fleetshift-server/internal/addon/ocp/cluster_spec_test.go`
- Modify: `fleetshift-server/internal/addon/ocp/agent.go`
- Modify: `fleetshift-server/internal/addon/ocp/cluster_output.go`
- Modify: `fleetshift-server/internal/addon/ocp/cluster_output_test.go`
- Modify: `fleetshift-server/internal/cli/serve.go`

- [ ] **Step 1: Write failing test — ClusterSpec requires region and role_arn**

Add to `cluster_spec_test.go`:

```go
func TestParseClusterSpec_RequiresRegionAndRoleARN(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing region",
			raw:  `{"name":"c","base_domain":"d","role_arn":"arn:aws:iam::123:role/r"}`,
		},
		{
			name: "missing role_arn",
			raw:  `{"name":"c","base_domain":"d","region":"us-east-1"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifests := []domain.Manifest{{
				ResourceType: ClusterResourceType,
				Raw:          json.RawMessage(tt.raw),
			}}
			_, err := ParseClusterSpec(manifests)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParseClusterSpec_WithRegionAndRoleARN(t *testing.T) {
	manifests := []domain.Manifest{{
		ResourceType: ClusterResourceType,
		Raw: json.RawMessage(`{
			"name": "test-cluster",
			"base_domain": "example.com",
			"region": "us-west-2",
			"role_arn": "arn:aws:iam::123456789012:role/provision"
		}`),
	}}

	spec, err := ParseClusterSpec(manifests)
	if err != nil {
		t.Fatalf("ParseClusterSpec failed: %v", err)
	}
	if spec.Region != "us-west-2" {
		t.Errorf("Region = %q, want us-west-2", spec.Region)
	}
	if spec.RoleARN != "arn:aws:iam::123456789012:role/provision" {
		t.Errorf("RoleARN = %q, want arn:...", spec.RoleARN)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -run TestParseClusterSpec_Requires -v -count=1
```

Expected: FAIL — `ClusterSpec` has no `Region` or `RoleARN` fields, and `ParseClusterSpec` doesn't validate them.

- [ ] **Step 3: Add Region and RoleARN to ClusterSpec**

In `cluster_spec.go`, update the struct:

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

Add validation in `ParseClusterSpec()` after the existing `BaseDomain` check:

```go
if spec.Region == "" {
	return nil, fmt.Errorf("cluster spec missing required field: region")
}
if spec.RoleARN == "" {
	return nil, fmt.Errorf("cluster spec missing required field: role_arn")
}
```

- [ ] **Step 4: Fix existing tests that don't provide region/role_arn**

Update `TestParseClusterSpec` manifest JSON to include region and role_arn:

```json
{
    "name": "test-cluster",
    "base_domain": "example.com",
    "region": "us-east-1",
    "role_arn": "arn:aws:iam::123456789012:role/test",
    "release_image": "quay.io/openshift-release-dev/ocp-release:4.14.0-x86_64"
}
```

Update `TestParseClusterSpec_Errors` — the "missing name" and "missing base_domain" cases need region/role_arn added so they fail for the right reason:

```go
{
    name: "missing name",
    manifests: []domain.Manifest{
        {ResourceType: ClusterResourceType, Raw: json.RawMessage(`{"base_domain":"example.com","region":"us-east-1","role_arn":"arn:aws:iam::123:role/r"}`)},
    },
},
{
    name: "missing base_domain",
    manifests: []domain.Manifest{
        {ResourceType: ClusterResourceType, Raw: json.RawMessage(`{"name":"test-cluster","region":"us-east-1","role_arn":"arn:aws:iam::123:role/r"}`)},
    },
},
```

Update `TestBuildClusterYAML`, `TestBuildClusterYAML_WithInstallConfig`, `TestBuildClusterYAML_BaseKeysCannotBeOverridden`, and `TestBuildClusterYAML_PlatformDeepMerge` — all `ClusterSpec` literals need `Region` and `RoleARN` fields added (the values don't affect BuildClusterYAML behavior since region is passed as a separate parameter, but the struct now requires them).

- [ ] **Step 5: Update agent.go Deliver() to read from spec**

Replace:

```go
// 3. Read region and role_arn from target.Properties
region := target.Properties["region"]
roleARN := target.Properties["role_arn"]
if region == "" {
    return domain.DeliveryResult{
        State:   domain.DeliveryStateFailed,
        Message: "target property 'region' is required",
    }, nil
}
if roleARN == "" {
    return domain.DeliveryResult{
        State:   domain.DeliveryStateFailed,
        Message: "target property 'role_arn' is required",
    }, nil
}
```

With:

```go
// 3. Read region and role_arn from cluster spec
region := spec.Region
roleARN := spec.RoleARN
```

The validation already happened in `ParseClusterSpec()` at step 1 of `Deliver()`.

- [ ] **Step 6: Add role_arn to ClusterOutput provisioned target properties**

In `cluster_output.go`, in the `Target()` method, add after the existing region property:

```go
if o.RoleARN != "" {
    props["role_arn"] = o.RoleARN
}
```

Add `RoleARN string` field to the `ClusterOutput` struct.

In `agent.go` `handleCompletion()`, set `RoleARN` on the output:

```go
output := &ClusterOutput{
    // ... existing fields ...
    RoleARN:       spec.RoleARN,  // note: spec needs to be passed to handleCompletion
}
```

This requires threading `spec.RoleARN` through to `handleCompletion`. The simplest way: pass the whole `spec` or just `roleARN` as an additional parameter. Pass `roleARN`:

Update `handleCompletion` signature to accept `roleARN string` and set it on the output.

Update the call in `deliverAsync`:

```go
output, err := a.handleCompletion(ctx, clusterID, completion, sshPrivateKey, auth, roleARN)
```

Where `roleARN` is captured from `spec.RoleARN` at the top of `deliverAsync` (passed through from `Deliver`).

- [ ] **Step 7: Remove Properties from ocp-aws target seed in serve.go**

The target seed should already have no Properties (we edited this earlier). Verify:

```go
if err := targetSvc.Register(ctx, domain.TargetInfo{
    ID:                    "ocp-aws",
    Type:                  ocpaddon.TargetType,
    Name:                  "OCP on AWS",
    AcceptedResourceTypes: []domain.ResourceType{ocpaddon.ClusterResourceType},
}); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
    return fmt.Errorf("seed ocp-aws target: %w", err)
}
```

- [ ] **Step 8: Run all tests**

```bash
cd fleetshift-server && go test ./internal/addon/ocp/ -v -count=1
```

Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add fleetshift-server/internal/addon/ocp/ fleetshift-server/internal/cli/serve.go
git commit -m "refactor: move region/role_arn from target properties to ClusterSpec"
```

---

### Task 8: Final verification

**Files:**
- Verify: all deleted files are gone
- Verify: all builds and tests pass

- [ ] **Step 1: Verify old generated callback code is gone from fleetshift-server**

```bash
ls fleetshift-server/gen/fleetshift/v1/ocp_engine_callback* 2>&1
# Expected: No such file or directory
```

- [ ] **Step 2: Verify old generated callback code is gone from ocp-engine**

```bash
ls ocp-engine/internal/callback/ocp_engine_callback* 2>&1
# Expected: No such file or directory
```

- [ ] **Step 3: Verify shared gen code exists**

```bash
ls gen/ocp/v1/callback_service.pb.go gen/ocp/v1/callback_service_grpc.pb.go
# Expected: both files exist
```

- [ ] **Step 4: Verify callback proto is not in platform namespace**

```bash
ls proto/fleetshift/v1/ocp_engine_callback* 2>&1
# Expected: No such file or directory
```

- [ ] **Step 5: Verify swagger doesn't reference callback service**

```bash
grep -c OCPEngineCallbackService docs/openapi/fleetshift.swagger.yaml
# Expected: 0
```

- [ ] **Step 6: Build all binaries**

```bash
make build
```

Expected: All 3 binaries (fleetshift, fleetctl, ocp-engine) compile cleanly.

- [ ] **Step 7: Run all tests**

```bash
make test
```

Expected: All ocp addon and ocp-engine tests pass. Kind addon Docker failures are pre-existing.

- [ ] **Step 8: Commit any remaining changes**

```bash
git add -A
git status  # verify only expected changes
git commit -m "chore: clean up callback separation — verify all builds and tests pass"
```
