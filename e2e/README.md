# FleetShift E2E Tests

End-to-end tests that exercise the full FleetShift platform against real infrastructure.

## Available Tests

| Test | Command | Duration | What it tests |
|------|---------|----------|---------------|
| `TestAWSProvision` | `make test-e2e-aws` | ~60 min | Full OCP cluster lifecycle on AWS with CCO STS mode |

## Prerequisites

### Software

- Go 1.25+
- `oc` CLI (for extracting binaries from release images)
- `dbus-x11` + `gnome-keyring` (Linux headless environments only)
- A browser accessible from your machine (for SSO logins)

### Keycloak (one-time per OCP cluster)

Deploy Keycloak with the FleetShift realm. Scripts are in `e2e/setup/keycloak/`:

```bash
cd e2e/setup/keycloak
./deploy.sh                    # Deploy Keycloak + realm + test users
./add-user.sh \                # Add your personal user
  --username you@company.com \
  --password yourpass \
  --github your-github-username \
  --roles ops,dev
```

See `e2e/setup/keycloak/README.md` for details on clients, realm config, and troubleshooting.

### AWS OIDC Federation (one-time per AWS account)

Register Keycloak as an IAM OIDC identity provider and create the `OCP-Provisioner` role. Script is in `e2e/setup/aws-oidc-federation/`:

```bash
# Get your Keycloak route
KEYCLOAK_HOST=$(oc get route -n keycloak-prod -o jsonpath='{.items[0].spec.host}')

# Run setup
KEYCLOAK_HOST=$KEYCLOAK_HOST ./e2e/setup/aws-oidc-federation/setup-aws-oidc-federation.sh

# To tear down:
KEYCLOAK_HOST=$KEYCLOAK_HOST ./e2e/setup/aws-oidc-federation/setup-aws-oidc-federation.sh --teardown
```

### AWS

- Route53 hosted zone for your base domain

### GitHub

- Each test run generates a new signing key that must be added to GitHub

## Setup

```bash
cd e2e
cp .env.example .env
# Edit .env with your values
```

## Running

### Linux headless (containers, SSH)

Unlock the keyring first, then run in the same shell:

```bash
eval "$(dbus-launch --sh-syntax)" && echo "" | gnome-keyring-daemon --unlock --components=secrets
make test-e2e-aws
```

### Desktop (macOS, Linux with desktop)

```bash
make test-e2e-aws
```

### Run all E2E tests

```bash
make test-e2e
```

## Interactive Steps

The test pauses for three browser-based logins:

1. **Keycloak login** — device code flow, open the printed URL
2. **Red Hat SSO login** — device code flow for pull secret
3. **Signing key enrollment** — browser redirect flow, then add the SSH key to GitHub

After validation, the test pauses for **manual inspection** before destroying the cluster.

## File Locations

### During test run

| File | Purpose |
|------|---------|
| `/tmp/fleetshift-e2e-server.log` | FleetShift server log (persists across runs) |
| `/tmp/fleetshift-e2e-data/fleetshift-e2e.db` | SQLite database (cleaned at start of each run) |
| `/tmp/ocp-provision-<cluster-name>/` | ocp-engine work directory (deleted by agent after completion) |
| `/tmp/ocp-provision-<cluster-name>/.openshift_install.log` | Raw OpenShift installer log |
| `/tmp/ocp-provision-<cluster-name>/auth/kubeconfig` | Cluster kubeconfig (cert-based, survives OIDC switch) |
| `/tmp/ocp-provision-<cluster-name>/auth/kubeadmin-password` | Kubeadmin password |
| `/tmp/ocp-provision-<cluster-name>/extra-manifests/` | OIDC manifests written by the agent |

### After test completion

| File | Purpose |
|------|---------|
| `/tmp/fleetshift-e2e-server.log` | Server log (always preserved) |
| `/tmp/fleetshift-e2e-data/` | DB dir (preserved for post-mortem, cleaned next run) |
| `/tmp/fleetshift-e2e-workdir/` | Copy of work dir (best-effort, may not exist if agent cleaned up first) |

### Configuration

| File | Purpose |
|------|---------|
| `e2e/.env` | Your test configuration (gitignored) |
| `e2e/.env.example` | Template with all available variables |
| `~/.config/fleetshift/auth.json` | fleetctl auth config (written by `fleetctl auth setup`) |

## Logging Architecture

ocp-engine splits output across two channels:

- **stdout** — JSON structured events: phase results, milestones, final result. Machine-readable, parsed by the agent via gRPC callback.
- **stderr** — Human-readable installer progress. Scrubbed of credentials during the cluster phase by the log pipeline. Raw during earlier phases (extract, ccoctl, manifests).

When running via fleetshift-server, both channels go to `/tmp/fleetshift-e2e-server.log`.

### Useful log searches

```bash
# Phase results
grep '"phase"' /tmp/fleetshift-e2e-server.log

# Errors and failures
grep -E '"status":"failed"|level=fatal|level=error' /tmp/fleetshift-e2e-server.log

# Credential resolution
grep "credentials resolved" /tmp/fleetshift-e2e-server.log

# OIDC manifest merge
grep -E "Merged|Injected|authentication" /tmp/fleetshift-e2e-server.log

# Deployment state changes
grep "deployment state changed" /tmp/fleetshift-e2e-server.log

# Delivery completion
grep "delivery completed" /tmp/fleetshift-e2e-server.log

# Bootstrap progress (from installer log, if work dir exists)
grep "level=info" /tmp/ocp-provision-<cluster-name>/.openshift_install.log | tail -20
```

## Troubleshooting

### Keyring errors

```
failed to unlock correct collection '/org/freedesktop/secrets/collection/login'
```

Run the keyring unlock before the test:
```bash
eval "$(dbus-launch --sh-syntax)" && echo "" | gnome-keyring-daemon --unlock --components=secrets
```

### Test failed — server still running

The test prompts before shutting down on failure. Use `fleetctl` to inspect:
```bash
fleetctl deployment list
fleetctl deployment get <name> -o json
```

Press Enter to shut down when done.

### Test failed — cluster still running

The test prints a warning banner with the cluster name. To destroy:
```bash
# If fleetshift-server is still running:
fleetctl deployment delete <name>

# If server is down, use ocp-engine directly:
# 1. Create metadata.json with the infra_id
# 2. Extract openshift-install from the release image
# 3. Run: ocp-engine destroy --work-dir <path>
# 4. Run: ccoctl aws delete --name=<cluster-name> --region=<region>
```

### Check for orphaned AWS resources

```bash
# EC2 instances
aws ec2 describe-instances --region us-west-2 \
  --filters "Name=instance-state-name,Values=running,pending" \
  --query 'Reservations[].Instances[].[InstanceId,Tags[?Key==`Name`].Value|[0]]' --output table | grep fleet

# ccoctl resources (S3, OIDC provider, IAM roles)
aws s3 ls | grep fleet
aws iam list-open-id-connect-providers --output text | grep fleet
aws iam list-roles --query 'Roles[].RoleName' --output text | tr '\t' '\n' | grep fleet
```

### Restart server with existing DB

```bash
bin/fleetshift serve --db /tmp/fleetshift-e2e-data/fleetshift-e2e.db --log-level debug
```

### Query target properties directly

```bash
sqlite3 /tmp/fleetshift-e2e-data/fleetshift-e2e.db \
  "SELECT id, name, properties FROM targets WHERE id LIKE 'k8s-%';"
```

## Test Flow (TestAWSProvision)

```
01_Build              — Build fleetshift-server, fleetctl, ocp-engine
02_KeycloakLogin      — Device code flow → Keycloak JWT (browser)
03_RedHatSSOLogin     — Device code flow → pull secret (browser)
04_StartServer        — Start fleetshift-server + auth setup
05_EnrollSigningKey   — Enroll signing key (browser) + add to GitHub
06_CreateDeployment   — fleetctl deployment create --sign
07_WaitForProvision   — Poll every 30s for STATE_ACTIVE (~45-60 min)
08_ValidateDeployment — Check target properties in DB
09_ValidateClusterOIDC— Login as OIDC user, check STS mode, operators
10_ManualInspection   — Pause for manual cluster inspection
11_DestroyDeployment  — fleetctl deployment delete
12_ValidateCleanup    — Verify AWS resources cleaned up
```

## Setup Directory Structure

```
e2e/setup/
├── keycloak/                          # Keycloak deployment on OCP
│   ├── deploy.sh                      # Deploy Keycloak + realm + test users
│   ├── teardown.sh                    # Remove everything
│   ├── add-user.sh                    # Add personal user with GitHub username
│   ├── README.md                      # Detailed Keycloak setup docs
│   ├── realm/
│   │   └── fleetshift-realm.json      # Realm config (3 clients, 3 test users, 2 roles)
│   └── manifests/
│       ├── namespace.yaml             # keycloak-prod namespace
│       ├── cert-manager-sub.yaml      # cert-manager operator
│       ├── cluster-issuer.yaml        # Let's Encrypt ClusterIssuer
│       ├── certificate.yaml           # Keycloak TLS certificate
│       ├── rhbk-sub.yaml             # RHBK operator (Keycloak)
│       ├── postgres-statefulset.yaml  # PostgreSQL database
│       └── keycloak.yaml              # Keycloak CR
│
└── aws-oidc-federation/               # AWS IAM setup for STS
    └── setup-aws-oidc-federation.sh   # Register Keycloak as IAM OIDC provider,
                                       # create OCP-Provisioner IAM role
                                       # Requires: KEYCLOAK_HOST env var
                                       # Idempotent, supports --teardown
```
