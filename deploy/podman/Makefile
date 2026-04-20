.PHONY: help up down clean restart-% logs logs-% status reset-keycloak

SCRIPTS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))scripts

help: ## Show this help
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

up: ## Start the full FleetShift stack
	$(SCRIPTS_DIR)/start.sh

down: ## Stop all containers, preserve data
	$(SCRIPTS_DIR)/stop.sh

clean: ## Stop all containers and remove ALL data (volumes, network)
	$(SCRIPTS_DIR)/stop.sh --clean

restart-%: ## Restart a single container (e.g., make restart-fleetshift-mock-servers)
	podman restart $*

logs: ## Tail logs from all FleetShift containers
	podman logs -f fleetshift-server &
	podman logs -f fleetshift-mock-servers &
	podman logs -f fleetshift-mock-ui-plugins &
	podman logs -f fleetshift-gui &
	wait

logs-%: ## Tail logs from a single container (e.g., make logs-fleetshift-server)
	podman logs -f $*

status: ## Show running FleetShift containers
	@podman ps --filter network=fleetshift --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

reset-keycloak: ## Wipe Keycloak state and re-import realm (generates new passwords)
	podman stop keycloak 2>/dev/null || true
	podman rm keycloak 2>/dev/null || true
	podman volume rm keycloak-certs 2>/dev/null || true
	@echo "Keycloak state wiped. Run 'make up' to restart with fresh realm and passwords."
