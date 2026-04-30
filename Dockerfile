FROM golang:1.25 AS builder

WORKDIR /src

# Copy go.mod/go.sum for both modules to cache deps
# CLI has a replace directive pointing to ../fleetshift-server
COPY fleetshift-server/go.mod fleetshift-server/go.sum ./fleetshift-server/
COPY fleetshift-cli/go.mod fleetshift-cli/go.sum ./fleetshift-cli/
COPY gen/go.mod gen/go.sum ./gen/
RUN cd fleetshift-server && go mod download && \
    cd ../fleetshift-cli && go mod download

# Copy all source (server, cli, gen, proto)
COPY fleetshift-server/ ./fleetshift-server/
COPY fleetshift-cli/ ./fleetshift-cli/
COPY gen/ ./gen/
COPY proto/ ./proto/

# Build both binaries
RUN cd fleetshift-server && CGO_ENABLED=0 go build -o /bin/fleetshift ./cmd/fleetshift
RUN cd fleetshift-cli && CGO_ENABLED=0 go build -o /bin/fleetctl ./cmd/fleetctl

# ── Shared runtime base ─────────────────────────────────────
FROM debian:bookworm-slim AS runtime-base

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /bin/fleetshift /usr/local/bin/fleetshift
COPY --from=builder /bin/fleetctl /usr/local/bin/fleetctl

EXPOSE 50051 8085

ENTRYPOINT ["fleetshift"]
CMD ["serve", "--http-addr", ":8085", "--grpc-addr", ":50051", "--db", "/data/fleetshift.db", "--log-level", "debug"]

# ── Production target (lean) ────────────────────────────────
FROM runtime-base AS production
