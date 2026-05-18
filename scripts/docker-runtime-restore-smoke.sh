#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${DOCKER_COMPOSE_FILE:-deploy/docker/docker-compose.local.yml}"
PANEL_URL="${LENKER_PANEL_URL:-http://localhost:8080}"
AGENT_URL="${LENKER_AGENT_URL:-http://localhost:8090}"
ADMIN_EMAIL="${ADMIN_EMAIL:-owner@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-change-me-now}"
POLL_INTERVAL="${LENKER_AGENT_CONFIG_POLL_INTERVAL:-2s}"
HEARTBEAT_INTERVAL="${LENKER_AGENT_HEARTBEAT_INTERVAL:-2s}"

log() {
  printf '[runtime-restore-smoke] %s\n' "$*"
}

fail() {
  printf '[runtime-restore-smoke] ERROR: %s\n' "$*" >&2
  exit 1
}

need_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

json_get() {
  ruby -rjson -e '
    value = JSON.parse(STDIN.read)
    ARGV.fetch(0).split(".").each { |key| value = value.fetch(key) }
    puts value
  ' "$1"
}

wait_for_url() {
  url="$1"
  label="$2"
  tries="${3:-30}"
  i=1
  while [ "$i" -le "$tries" ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  fail "$label did not become ready at $url"
}

wait_for_apply() {
  node_id="$1"
  revision_id="$2"
  admin_token="$3"
  tries="${4:-45}"
  i=1
  while [ "$i" -le "$tries" ]; do
    detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id" -H "Authorization: Bearer $admin_token")"
    revision_detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id/config-revisions/$revision_id" -H "Authorization: Bearer $admin_token")"
    if DETAIL="$detail_json" REVISION="$revision_detail_json" ruby -rjson -e '
        detail = JSON.parse(ENV.fetch("DETAIL")).fetch("data")
        revision = JSON.parse(ENV.fetch("REVISION")).fetch("data")
        events = detail.fetch("runtime_events", [])
        ok = revision["status"] == "applied" &&
          detail["active_revision_id"].to_i == revision["revision_number"].to_i &&
          detail["last_applied_revision"].to_i == revision["revision_number"].to_i &&
          detail["last_validation_status"] == "applied" &&
          detail["runtime_state"] == "active_config_ready" &&
          detail["active_config_path"].to_s != "" &&
          events.any? { |event| event["type"] == "apply_success" && event["status"] == "applied" }
        exit(ok ? 0 : 1)
      '; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

wait_for_restore_heartbeat() {
  node_id="$1"
  revision_id="$2"
  admin_token="$3"
  applied_at="$4"
  tries="${5:-60}"
  i=1
  while [ "$i" -le "$tries" ]; do
    detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id" -H "Authorization: Bearer $admin_token")"
    revision_detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id/config-revisions/$revision_id" -H "Authorization: Bearer $admin_token")"
    if DETAIL="$detail_json" REVISION="$revision_detail_json" APPLIED_AT="$applied_at" ruby -rjson -e '
        detail = JSON.parse(ENV.fetch("DETAIL")).fetch("data")
        revision = JSON.parse(ENV.fetch("REVISION")).fetch("data")
        events = detail.fetch("runtime_events", [])
        apply_success_count = events.count { |event| event["type"] == "apply_success" && event["status"] == "applied" }
        ok = revision["status"] == "applied" &&
          revision["applied_at"].to_s == ENV.fetch("APPLIED_AT") &&
          detail["active_revision_id"].to_i == revision["revision_number"].to_i &&
          detail["last_applied_revision"].to_i == revision["revision_number"].to_i &&
          detail["last_validation_status"] == "applied" &&
          detail["runtime_state"] == "active_config_ready" &&
          detail["active_config_path"].to_s != "" &&
          events.any? { |event| event["type"] == "runtime_state_restore" && event["status"] == "restored" } &&
          apply_success_count == 1
        exit(ok ? 0 : 1)
      '; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

need_command docker
need_command curl
need_command ruby

if ! docker compose version >/dev/null 2>&1; then
  fail "Docker daemon or docker compose is unavailable"
fi

log "starting local Docker stack"
docker compose -f "$COMPOSE_FILE" up -d postgres migrate panel-api node-agent >/dev/null
wait_for_url "$PANEL_URL/healthz" "panel-api" 45
wait_for_url "$AGENT_URL/healthz" "node-agent" 45

log "bootstrapping admin"
docker compose -f "$COMPOSE_FILE" --profile setup run --rm bootstrap-admin >/dev/null

log "logging in as admin"
login_json="$(curl -fsS "$PANEL_URL/api/v1/auth/admin/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}")"
admin_token="$(printf '%s' "$login_json" | json_get data.session.token)"

node_name="runtime-restore-smoke-$(date -u +%Y%m%d%H%M%S)"

log "creating bootstrap token"
bootstrap_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/bootstrap-token" \
  -H "Authorization: Bearer $admin_token" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"$node_name\",\"region\":\"eu\",\"country_code\":\"FI\",\"hostname\":\"$node_name\",\"expires_in_minutes\":30}")"
node_id="$(printf '%s' "$bootstrap_json" | json_get data.node_id)"
bootstrap_token="$(printf '%s' "$bootstrap_json" | json_get data.bootstrap_token)"

log "registering node"
register_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/register" \
  -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$node_id\",\"bootstrap_token\":\"$bootstrap_token\",\"agent_version\":\"0.1.0-dev\",\"hostname\":\"$node_name\"}")"
node_token="$(printf '%s' "$register_json" | json_get data.node_token)"

log "creating config revision"
revision_json="$(curl -fsS -X POST "$PANEL_URL/api/v1/nodes/$node_id/config-revisions" \
  -H "Authorization: Bearer $admin_token")"
revision_id="$(printf '%s' "$revision_json" | json_get data.id)"
revision_number="$(printf '%s' "$revision_json" | json_get data.revision_number)"

log "starting node-agent with registered identity"
LENKER_AGENT_NODE_ID="$node_id" \
LENKER_AGENT_NODE_TOKEN="$node_token" \
LENKER_AGENT_CONFIG_POLL_INTERVAL="$POLL_INTERVAL" \
LENKER_AGENT_HEARTBEAT_INTERVAL="$HEARTBEAT_INTERVAL" \
docker compose -f "$COMPOSE_FILE" up -d node-agent >/dev/null
wait_for_url "$AGENT_URL/healthz" "node-agent" 45

log "waiting for initial apply/report"
if ! wait_for_apply "$node_id" "$revision_id" "$admin_token" 45; then
  fail "node-agent did not apply and report revision $revision_number"
fi

before_detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id" -H "Authorization: Bearer $admin_token")"
before_revision_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id/config-revisions/$revision_id" -H "Authorization: Bearer $admin_token")"
before_status_json="$(curl -fsS "$AGENT_URL/status")"
applied_at="$(printf '%s' "$before_revision_json" | json_get data.applied_at)"

log "checking persisted active artifacts before restart"
docker compose -f "$COMPOSE_FILE" exec -T node-agent sh -c \
  'test -s /var/lib/lenker/node-agent/active/config.json &&
   test -s /var/lib/lenker/node-agent/active/metadata.json &&
   test -s /var/lib/lenker/node-agent/state.json' >/dev/null

log "restarting node-agent only"
docker compose -f "$COMPOSE_FILE" restart node-agent >/dev/null
wait_for_url "$AGENT_URL/healthz" "node-agent after restart" 45

log "checking local restored status"
after_status_json="$(curl -fsS "$AGENT_URL/status")"
STATUS="$after_status_json" EXPECTED_REVISION="$revision_number" ruby -rjson -e '
  status = JSON.parse(ENV.fetch("STATUS")).fetch("data")
  expected_revision = ENV.fetch("EXPECTED_REVISION").to_i
  events = status.fetch("runtime_events", [])
  apply_success_count = events.count { |event| event["type"] == "apply_success" && event["status"] == "applied" }
  abort("active revision was not restored") unless status["active_revision"].to_i == expected_revision
  abort("last applied revision was not restored") unless status["last_applied_revision"].to_i == expected_revision
  abort("runtime state was not restored") unless status["runtime_state"] == "active_config_ready"
  abort("validation status was not restored") unless status["last_validation_status"] == "applied"
  abort("active config path missing") if status["active_config_path"].to_s == ""
  abort("missing restore event") unless events.any? { |event| event["type"] == "runtime_state_restore" && event["status"] == "restored" }
  abort("unexpected extra apply_success event after restore") unless apply_success_count == 1
'

log "waiting for restored heartbeat ingestion"
if ! wait_for_restore_heartbeat "$node_id" "$revision_id" "$admin_token" "$applied_at" 60; then
  fail "panel-api did not observe coherent restored runtime readiness"
fi

after_detail_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id" -H "Authorization: Bearer $admin_token")"
after_revision_json="$(curl -fsS "$PANEL_URL/api/v1/nodes/$node_id/config-revisions/$revision_id" -H "Authorization: Bearer $admin_token")"

BEFORE_DETAIL="$before_detail_json" BEFORE_REVISION="$before_revision_json" BEFORE_STATUS="$before_status_json" AFTER_DETAIL="$after_detail_json" AFTER_REVISION="$after_revision_json" AFTER_STATUS="$after_status_json" ruby -rjson -e '
  before_detail = JSON.parse(ENV.fetch("BEFORE_DETAIL")).fetch("data")
  before_revision = JSON.parse(ENV.fetch("BEFORE_REVISION")).fetch("data")
  before_status = JSON.parse(ENV.fetch("BEFORE_STATUS")).fetch("data")
  after_detail = JSON.parse(ENV.fetch("AFTER_DETAIL")).fetch("data")
  after_revision = JSON.parse(ENV.fetch("AFTER_REVISION")).fetch("data")
  after_status = JSON.parse(ENV.fetch("AFTER_STATUS")).fetch("data")
  node_events = after_detail.fetch("runtime_events", [])
  agent_events = after_status.fetch("runtime_events", [])
  summary = {
    node_id: after_detail["id"],
    revision_number: after_revision["revision_number"],
    revision_status: after_revision["status"],
    applied_at_unchanged: before_revision["applied_at"] == after_revision["applied_at"],
    active_revision_id_before: before_detail["active_revision_id"],
    active_revision_id_after: after_detail["active_revision_id"],
    agent_active_revision_after_restart: after_status["active_revision"],
    runtime_state: after_detail["runtime_state"],
    last_validation_status: after_detail["last_validation_status"],
    active_config_path_present: after_detail["active_config_path"].to_s != "",
    agent_restore_events: agent_events.count { |event| event["type"] == "runtime_state_restore" },
    persisted_restore_events: node_events.count { |event| event["type"] == "runtime_state_restore" },
    apply_success_events_after_restore: node_events.count { |event| event["type"] == "apply_success" },
    local_status_before_restart_events: before_status.fetch("runtime_events", []).length,
    local_status_after_restart_events: agent_events.length
  }
  puts JSON.pretty_generate(summary)
'

log "runtime restore smoke passed"
