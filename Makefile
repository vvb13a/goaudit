# GoAudit developer tasks. Run `make` for the list of targets.
#
# The Vue app lives in ./frontend; npm's --prefix flag runs its scripts from
# the repo root, so no `cd` is needed.

.DEFAULT_GOAL := help

API_ADDR ?= :8080
DEV_PORT ?= 5173

NPM := npm --prefix frontend

.PHONY: help install api web dev build build-web build-api tidy clean

help: ## List available targets
	@echo "GoAudit targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

install: ## Install frontend dependencies
	$(NPM) ci

api: ## Run the Go JSON API (override with API_ADDR=...)
	GOAUDIT_ADDR=$(API_ADDR) go run . serve

web: ## Run the Vite dev server (override with DEV_PORT=...)
	$(NPM) run dev -- --port $(DEV_PORT)

dev: ## Run the API and web dev server together (Ctrl+C stops both)
	@test -d frontend/node_modules || { echo "frontend deps missing: run 'make install'"; exit 1; }
	@echo "GoAudit API on $(API_ADDR) + web on http://localhost:$(DEV_PORT)  (Ctrl+C to stop)"
	@trap 'kill 0' INT TERM; \
		GOAUDIT_ADDR=$(API_ADDR) go run . serve & \
		$(NPM) run dev -- --port $(DEV_PORT) & \
		wait

build: build-web build-api ## Build the web assets and the Go binary

build-web: ## Build the Vue app into frontend/dist
	$(NPM) run build

build-api: ## Build the Go binary into bin/goaudit
	go build -o bin/goaudit .

tidy: ## Tidy Go modules
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin frontend/dist
