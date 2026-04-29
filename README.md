# fleetshift-poc

This repository represents both a **prototype** for a next generation k8s/OpenShift cluster management vision, alongside **individual POCs** for exploration of isolated concepts.

## Full-stack local development

```bash
task deploy:dev    # builds from source, hot-reload for GUI and mock servers
task deploy:down   # stop (preserve data)
task deploy:logs   # tail all container logs
task deploy:status # show running containers
```

See [deploy/podman/README.md](deploy/podman/README.md) for full documentation (profiles, overrides, signing config, troubleshooting).
