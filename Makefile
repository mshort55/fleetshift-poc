.PHONY: help build test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the server binary
	cd fleetshift-server && go build ./...

test: ## Run all tests
	cd fleetshift-server && go test ./...
