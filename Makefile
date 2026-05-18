MIGRATIONS_DIR ?= migrations
MIGRATE ?= migrate
PANEL_API_DIR ?= services/panel-api
NODE_AGENT_DIR ?= services/node-agent
OPENAPI_SPEC ?= docs/openapi/panel-api.v1.yaml
DOCKER_COMPOSE ?= docker compose
DOCKER_COMPOSE_FILE ?= deploy/docker/docker-compose.local.yml

.PHONY: migrate-up migrate-down migrate-force bootstrap-admin run-panel-api run-node-agent test-panel-api test-node-agent openapi-lint validate-openapi test docker-build docker-up docker-down docker-logs docker-ps docker-bootstrap-admin docker-smoke docker-runtime-smoke docker-runtime-failure-smoke docker-runtime-restore-smoke docker-subscription-access-smoke docker-handoff-smoke docker-xray-dry-run-env

migrate-up:
	@if [ -z "$$LENKER_DATABASE_URL" ]; then echo "LENKER_DATABASE_URL is required"; exit 1; fi
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$LENKER_DATABASE_URL" up

migrate-down:
	@if [ -z "$$LENKER_DATABASE_URL" ]; then echo "LENKER_DATABASE_URL is required"; exit 1; fi
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$LENKER_DATABASE_URL" down 1

migrate-force:
	@if [ -z "$$LENKER_DATABASE_URL" ]; then echo "LENKER_DATABASE_URL is required"; exit 1; fi
	@if [ -z "$$VERSION" ]; then echo "VERSION is required"; exit 1; fi
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$LENKER_DATABASE_URL" force "$$VERSION"

bootstrap-admin:
	@if [ -z "$$LENKER_DATABASE_URL" ]; then echo "LENKER_DATABASE_URL is required"; exit 1; fi
	@if [ -z "$$ADMIN_EMAIL" ]; then echo "ADMIN_EMAIL is required"; exit 1; fi
	@if [ -z "$$ADMIN_PASSWORD" ]; then echo "ADMIN_PASSWORD is required"; exit 1; fi
	cd $(PANEL_API_DIR) && go run ./cmd/bootstrap-admin -email "$$ADMIN_EMAIL" -password "$$ADMIN_PASSWORD"

run-panel-api:
	cd $(PANEL_API_DIR) && go run ./cmd/panel-api

run-node-agent:
	cd $(NODE_AGENT_DIR) && go run ./cmd/node-agent

test-panel-api:
	cd $(PANEL_API_DIR) && go test ./...

test-node-agent:
	cd $(NODE_AGENT_DIR) && go test ./...

openapi-lint validate-openapi:
	@if command -v ruby >/dev/null 2>&1; then ruby scripts/validate-openapi.rb $(OPENAPI_SPEC); else echo "ruby not found; skipping OpenAPI validation"; fi

test: test-panel-api test-node-agent openapi-lint

docker-build:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) build

docker-up:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) up -d postgres migrate panel-api node-agent

docker-down:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) down

docker-logs:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) logs -f --tail=200

docker-ps:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) ps

docker-bootstrap-admin:
	$(DOCKER_COMPOSE) -f $(DOCKER_COMPOSE_FILE) --profile setup run --rm bootstrap-admin

docker-smoke:
	curl -fsS http://localhost:8080/healthz
	curl -fsS http://localhost:8090/healthz

docker-runtime-smoke:
	scripts/docker-runtime-smoke.sh

docker-runtime-failure-smoke:
	scripts/docker-runtime-failure-smoke.sh

docker-runtime-restore-smoke:
	scripts/docker-runtime-restore-smoke.sh

docker-subscription-access-smoke:
	scripts/docker-subscription-access-smoke.sh

docker-handoff-smoke:
	scripts/docker-subscription-access-smoke.sh

docker-xray-dry-run-env:
	@xray_bin="$${XRAY_BIN:-$$(command -v xray 2>/dev/null || true)}"; \
	if [ -z "$$xray_bin" ]; then \
		echo "xray binary not found. Install it locally or run: XRAY_BIN=/absolute/path/to/xray make docker-xray-dry-run-env"; \
		exit 1; \
	fi; \
	if [ ! -x "$$xray_bin" ]; then \
		echo "xray binary is not executable: $$xray_bin"; \
		exit 1; \
	fi; \
	xray_dir="$$(dirname "$$xray_bin")"; \
	xray_name="$$(basename "$$xray_bin")"; \
	echo "export LENKER_LOCAL_XRAY_DIR=$$xray_dir"; \
	echo "export LENKER_AGENT_XRAY_BIN=/opt/lenker/xray/$$xray_name"; \
	echo "make docker-up"
