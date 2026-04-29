# fleetshift-poc

This repository represents both a **prototype** for a next generation k8s/OpenShift cluster management vision, alongside **individual POCs** for exploration of isolated concepts.

## Prerequisites

- **Go 1.22+**
- **[Task](https://taskfile.dev/)** — `go install github.com/go-task/task/v3/cmd/task@latest`
- **podman** — for container deployment
- **buf** — for protobuf generation (`brew install bufbuild/buf/buf`)

## Quick Start

```bash
cp .env.template .env         # configure (edit as needed)
task build                    # build all Go binaries
task deploy:up                # start the stack (demo mode)
task deploy:cli-setup         # configure fleetctl CLI
bin/fleetctl auth login       # log in (opens browser)
```

For development with hot-reload:

```bash
task deploy:dev               # builds from source, mounts source dirs
```

## Tasks

Run `task --list` for the full list. All tasks run from the project root.

### Build

```bash
task build              # build all Go binaries → bin/
task build:server       # fleetshift-server
task build:cli          # fleetctl CLI
task build:ocp-engine   # ocp-engine
```

Builds are incremental — only recompiles when source files change.

### Test

```bash
task test               # unit tests for all modules
task test:e2e           # end-to-end tests (requires .env + interactive auth)
task test:e2e-aws       # AWS provision/destroy end-to-end test
```

### Deploy

```bash
task deploy:up          # start the stack (demo mode by default)
task deploy:dev         # dev mode — source mounts + hot-reload
task deploy:down        # stop containers, preserve data
task deploy:clean       # stop + delete all data
task deploy:rebuild     # stop → rebuild images → restart
task deploy:logs        # follow logs from all containers
task deploy:status      # show running containers
task deploy:cli-setup   # configure fleetctl for local auth
```

Customize with `DEPLOY_MODE`, `DB`, `AUTH` variables (e.g. `task deploy:up DEPLOY_MODE=prod`). The `d:` alias works for all deploy tasks.

See [deploy/podman/README.md](deploy/podman/README.md) for modes, configuration, networking, and troubleshooting.

### Generate & Images

```bash
task generate           # regenerate protobuf and gRPC stubs
task image:build        # build server container image
task image:push         # push to DEV_REGISTRY
```

## Configuration

Copy `.env.template` to `.env` and edit. See the template for all available settings. Command-line variables always override `.env`.
