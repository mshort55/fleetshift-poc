# fleetshift-poc

This repository represents both a **prototype** for a next generation k8s/OpenShift cluster management vision, alongside **individual POCs** for exploration of isolated concepts.

## Prerequisites

- **Go 1.22+**
- **[Task](https://taskfile.dev/)** — `go install github.com/go-task/task/v3/cmd/task@latest`
- **podman** — container runtime
- **[docker-compose](https://docs.docker.com/compose/install/)** — `podman-compose` is not compatible
- **jq** — JSON processing
- **kind** — for local cluster provisioning
- **buf** — for protobuf generation (`brew install bufbuild/buf/buf`)
- `.env` file — copy from `.env.template`

> **macOS:** Podman only forwards IPv6 loopback. Add this one-time `/etc/hosts` entry or Keycloak will be unreachable:
> ```bash
> echo "::1 keycloak" | sudo tee -a /etc/hosts
> ```

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
task deploy:up                        # start the stack (demo mode by default)
task deploy:dev                       # dev mode — source mounts + hot-reload
task deploy:down                      # stop containers, preserve data
task deploy:clean                     # stop + delete all data
task deploy:rebuild                   # stop → rebuild images → restart
task deploy:logs                      # follow logs from all containers
task deploy:logs:fleetshift-server    # tail specific service
task deploy:status                    # show running containers
task deploy:cli-setup                 # configure fleetctl for local auth
task deploy:reset-keycloak            # wipe keycloak state, re-import realm
```

Customize with `DEPLOY_MODE`, `DB`, `AUTH` variables (e.g. `task deploy:up DEPLOY_MODE=prod`). The `d:` alias works for all deploy tasks.

### Generate & Images

```bash
task generate           # regenerate protobuf and gRPC stubs
task image:build        # build server container images
task image:push         # push to DEV_REGISTRY
```

## Deploy Modes

| Mode | DB | Auth | Use Case |
|---|---|---|---|
| `demo` (default) | SQLite | Local Keycloak | Local dev, demos |
| `prod` | PostgreSQL | External OIDC | Production-like |

Override axes independently with `DB=sqlite|postgres` and `AUTH=local|external`.

## Dev Mode

`task deploy:dev` builds frontend assets in a container (using `Dockerfile.web` from the UI repo) and starts the Go backend serving everything on `:8085`. No host Node.js or npm required. Requires `UI_DIR` in `.env` pointing to the `fleetshift-user-interface` repo.

After changing Go code, run `task deploy:rebuild` to rebuild and restart. After changing frontend code, run `task deploy:clean` then `task deploy:dev` to rebuild the web assets.

## Configuration

Copy `.env.template` to `.env` and edit. All available settings are documented in the template. Command-line variables always override `.env`.

## OpenShift Keycloak

For deploying a production-like Keycloak on OpenShift for OIDC integration testing, see [deploy/keycloak/README.md](deploy/keycloak/README.md).
