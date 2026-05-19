# node-agent

`node-agent` is the Go service foundation that runs on managed Lenker nodes.

Planned responsibilities for `MVP v0.1`:

- one-time bootstrap registration
- `HTTPS + mTLS` trust establishment
- node health reporting
- basic metrics reporting
- signed config bundle retrieval
- atomic config apply and rollback
- drain mode support
- local Xray process control for the main protocol path

Current foundation:

- application entrypoint at `cmd/node-agent`
- environment-based config loading
- structured JSON logging through Go `slog`
- HTTP server with graceful shutdown
- local `GET /healthz`
- local `GET /status`
- agent identity, registration payload, heartbeat payload, status, config revision, and rollback placeholder models
- registration and heartbeat request builders
- periodic heartbeat sender when panel URL, node id, and node token are configured
- periodic traffic report sender when panel URL, node id, and node token are configured
- signed config revision validation with in-memory metadata storage and local config artifact serialization
- config revision tracking and rollback planning skeleton
- no-process runtime supervisor skeleton state around validated active config artifacts

Configuration:

- `LENKER_AGENT_HTTP_ADDR`
- `LENKER_AGENT_NODE_ID`
- `LENKER_AGENT_BOOTSTRAP_TOKEN`
- `LENKER_AGENT_NODE_TOKEN`
- `LENKER_AGENT_PANEL_URL`
- `LENKER_AGENT_STATE_DIR`
- `LENKER_AGENT_LOG_LEVEL`
- `LENKER_AGENT_HEARTBEAT_INTERVAL`
- `LENKER_AGENT_CONFIG_POLL_INTERVAL`
- `LENKER_AGENT_TRAFFIC_REPORT_INTERVAL`
- `LENKER_AGENT_XRAY_BIN`
- `LENKER_AGENT_XRAY_API_ADDRESS`
- `LENKER_AGENT_RUNTIME_PROCESS_MODE`
- `LENKER_AGENT_TLS_ENABLED`

Local run:

```sh
go run ./cmd/node-agent
```

From the repository root:

```sh
make run-node-agent
make test-node-agent
```

Local HTTP surface:

- `GET /healthz`
- `GET /status`

Panel contract currently implemented:

- `POST /api/v1/nodes/bootstrap-token`
- `POST /api/v1/nodes/register`
- `POST /api/v1/nodes/{id}/heartbeat`
- `GET /api/v1/nodes/{id}/config-revisions/pending`
- `POST /api/v1/nodes/{id}/config-revisions/{revisionId}/report`

Registration payload:

```json
{
  "node_id": "<node_id-from-bootstrap-token-response>",
  "bootstrap_token": "<plaintext-bootstrap-token>",
  "agent_version": "0.1.0-dev",
  "hostname": "node-hostname"
}
```

Heartbeat payload:

```json
{
  "node_id": "<registered-node-id>",
  "agent_version": "0.1.0-dev",
  "status": "active",
  "active_revision": 0,
  "sent_at": "2026-05-15T00:00:00Z"
}
```

Current node lifecycle statuses are `pending`, `active`, `unhealthy`,
`drained`, and `disabled`. The node-agent foundation builds payloads only; it
does not implement a retrying network client or mTLS certificate lifecycle yet.

Drain and disable operations are controlled by `panel-api` admin endpoints. The
agent continues to build heartbeat payloads; disabled nodes are rejected by the
panel until an admin enables them again.

Conservative note:

`LENKER_AGENT_TLS_ENABLED` is a foundation flag only. Full mTLS bootstrap,
certificate rotation, and production network retry policy are intentionally not
implemented in this step.

Config delivery/apply foundation:

The agent has a small panel client for fetching the latest pending signed
revision metadata with `Authorization: Bearer <node_token>`. A polling loop runs
on `LENKER_AGENT_CONFIG_POLL_INTERVAL`, treats `404 not_found` as no-op, rejects
unauthorized or malformed responses, validates the bundle hash and deterministic
dev signature, enforces the deterministic subscription-aware VLESS Reality Xray
compatibility gate, stores metadata in memory, serializes local config artifacts
through a staged -> active file switch, and updates the active/applied revision
in status and heartbeat payloads after the active switch succeeds.

The validation gate is focused on the current single MVP path. It requires the
rendered config object to contain `log`, `policy`, `stats`, one VLESS inbound,
TCP + Reality stream settings, coherent VLESS client entries, a direct/freedom
outbound, and routing rules that reference known inbound/outbound tags. It is
not a full Xray schema validator.

Optional Xray binary dry-run validation can be enabled with
`LENKER_AGENT_XRAY_BIN=/path/to/xray`. When configured, the agent writes a
temporary candidate config after internal validation and runs:

```sh
xray run -test -config <candidate-config>
```

Only a successful one-shot validation lets the agent continue to the staged ->
active file switch. Without `LENKER_AGENT_XRAY_BIN`, the current internal
validation and staged apply path remains unchanged. The agent does not download
Xray, start it as a daemon, reload it, restart it, or supervise it.

For the local Docker profile, keep `LENKER_AGENT_XRAY_BIN` unset for the default
happy path. The compose file mounts an empty local directory at
`/opt/lenker/xray`, so no binary is present unless you opt in. To test against a
host-installed Xray binary, mount the directory that contains it and point the
agent at the container path:

```sh
# Inspect the exports for the Xray binary Docker bind mount.
make docker-xray-dry-run-env

# Then run the printed exports, for example:
export LENKER_LOCAL_XRAY_DIR="$(dirname "$(command -v xray)")"
export LENKER_AGENT_XRAY_BIN=/opt/lenker/xray/$(basename "$(command -v xray)")
make docker-up
```

If the binary is not named `xray` or is outside `PATH`, use
`XRAY_BIN=/absolute/path/to/xray make docker-xray-dry-run-env`; the target only
prints the required exports, verifies the binary is executable, and does not
start Docker. After `make docker-up`, `curl -s http://localhost:8090/status`
should include `"xray_dry_run_enabled":true`.

The local compose profile also accepts `LENKER_AGENT_NODE_ID`,
`LENKER_AGENT_NODE_TOKEN`, `LENKER_AGENT_BOOTSTRAP_TOKEN`, and
`LENKER_AGENT_CONFIG_POLL_INTERVAL`. `LENKER_AGENT_HEARTBEAT_INTERVAL` can also
be shortened for local smoke checks. After creating and registering a node
through panel-api, restart `node-agent` with the registered node id/token to
exercise the polling apply path:

```sh
LENKER_AGENT_NODE_ID="$LENKER_NODE_ID" \
LENKER_AGENT_NODE_TOKEN="$LENKER_NODE_TOKEN" \
LENKER_AGENT_CONFIG_POLL_INTERVAL=2s \
make docker-up
```

For a scripted local Docker check of bootstrap/register -> pending revision ->
polling apply -> runtime readiness -> persisted runtime events, run:

```sh
make docker-build
make docker-runtime-smoke
```

For the controlled failure-mode check, run:

```sh
make docker-runtime-failure-smoke
```

It applies a baseline revision, forces a missing-binary Xray dry-run failure on
the next revision, and verifies that the failed report, runtime readiness, and
`dry_run_failure` event are persisted while the previous active artifact stays
active.

For the restart/restore check, run:

```sh
make docker-runtime-restore-smoke
```

It applies a pending revision, verifies `active/config.json`,
`active/metadata.json`, and `state.json`, restarts only the `node-agent`
container, and confirms that `/status` plus panel-api node detail restore the
active revision, validation/runtime readiness, artifact path, and
`runtime_state_restore` event. The helper checks observable signals that no new
revision was created and no fake applied report changed the original
`applied_at`. This is a local-dev smoke only; it does not start, reload,
restart, or supervise a real Xray daemon.

`LENKER_LOCAL_XRAY_DIR` is only a local bind-mount source for
`deploy/docker/docker-compose.local.yml`; no Xray binary is downloaded or baked
into the image. If `LENKER_AGENT_XRAY_BIN` is set but the binary is missing, the
apply report fails with `xray_dry_run_failed:xray_binary_not_found`. Local
`GET /status` exposes `xray_dry_run_enabled` so the dev profile can confirm that
the optional boundary is active.

For a reproducible failed dry-run path without a real Xray binary, the
node-agent tests include `internal/agent/testdata/xray-dry-run-fail.sh`. The
fixture accepts the same `run -test -config <candidate>` invocation, verifies
that a candidate config file exists, then exits non-zero with a stable message.
`go test ./internal/agent -run CommandDryRunFixture` proves that node-agent
reports `failed`, preserves the previous active config, and records compact
runtime readiness metadata without starting, restarting, reloading, or
supervising Xray.

After validation, the agent reports `applied` to panel-api. Validation failures
such as bad hash, bad signature, malformed payload, incompatible Xray config, or
Xray dry-run failure, or local artifact write failure are reported as `failed`
with a concise `error_message` such as
`invalid_xray_config:missing_stream_settings` or
`xray_dry_run_failed:invalid_config`.

The agent also exposes the latest runtime readiness metadata in `/status` and
heartbeat/report payloads: `last_validation_status`, `last_validation_error`,
`last_validation_at`, `last_applied_revision`, `active_config_path`, and the
bounded `runtime_events` trail.
For a successful real-binary dry-run apply, expected signals are
`xray_dry_run_enabled=true`, `last_validation_status=applied`, an empty
`last_validation_error`, a non-zero `last_applied_revision`, and an
`active_config_path` under the agent state directory.

Runtime supervisor skeleton:

The agent now tracks an explicit local runtime preparation state without
starting or supervising Xray. The default supervisor is `no-process`: it records
that a validated config is active on disk and that process control is
unavailable by design. If `LENKER_AGENT_XRAY_BIN` is configured, the runtime
mode becomes `dry-run-only`; the candidate must pass the one-shot Xray dry-run
before the active file switch is reported as prepared.

`LENKER_AGENT_RUNTIME_PROCESS_MODE` is an explicit opt-in gate for the future
local Xray process runner boundary:

- `disabled` is the default. The agent never attempts process control; apply
  still means validation plus staged -> active files ready on disk.
- `local` enables only a fakeable local runner skeleton. It records a
  prepare/start intent after active files are ready and reports the process
  capability as ready, but it does not launch a long-running Xray daemon,
  reload, restart, watch, or integrate with systemd.

Read-only runtime fields in `/status`, heartbeat, and revision reports:

- `runtime_mode`: `no-process`, `dry-run-only`, or
  `future-process-managed`
- `runtime_process_mode`: `disabled` or `local`
- `runtime_process_state`: `disabled`, `ready`, or `failed`
- `runtime_desired_state`: currently `validated-config-ready`
- `runtime_state`: `not_prepared`, `active_config_ready`, `validation_failed`,
  or `prepare_failed`
- `last_dry_run_status`: `not_configured`, `passed`, or `failed`
- `last_runtime_attempt_status`: currently `skipped` for no-process success or
  `failed` for validation/dry-run failures
- `last_runtime_prepared_revision`
- `last_runtime_transition_at`
- `last_runtime_error`

Local `/status` also includes a compact bounded `runtime_events` trail for the
latest node-local runtime events. The trail keeps the newest events only and is
intended for development/operator inspection, not as a full audit system.
Current event types cover apply success, apply failure, validation failure,
Xray dry-run failure, and the local process prepare/start intent in
`LENKER_AGENT_RUNTIME_PROCESS_MODE=local`. Startup restore can also append
`runtime_state_restore` or `runtime_state_restore_degraded` when local state is
loaded or partially unusable. The trail is stored in local `state.json` and is
sent with heartbeat/report payloads so panel-api can retain a bounded recent
node-level trail.

Local artifact layout under `LENKER_AGENT_STATE_DIR`:

```text
revisions/<revision_number>/config.json
revisions/<revision_number>/metadata.json
staged/config.json
staged/metadata.json
active/config.json
active/metadata.json
state.json
```

Writes use a temp-file then rename pattern. The agent writes revision-specific
and staged artifacts first, validates staged JSON, then replaces active
artifacts. `metadata.json` and `state.json` include the revision id, bundle
hash, signer, rollback target revision, operation kind, source revision metadata
when present, config path references, runtime process state, and the bounded
runtime event trail.

On startup the agent restores runtime readiness from `state.json` when present.
If `state.json` is missing or malformed, it falls back to `active/metadata.json`
only when active config and metadata artifacts are readable JSON. Restore never
marks a revision as newly applied and never reports an apply transition by
itself; the next heartbeat simply carries the restored active revision,
validation/runtime metadata, artifact paths, and bounded event trail. Broken or
incomplete local state leaves the agent in a degraded `prepare_failed` runtime
state instead of pretending that active config is ready.

Rollback is file-level only. A rollback-originated pending revision is applied
through the same internal validation, optional Xray dry-run, and staged ->
active path, so active config files can switch back to a previous rendered
config artifact. This step does not start, restart, reload, or supervise Xray.

Production runtime mode:

When `LENKER_AGENT_RUNTIME_PROCESS_MODE=local` and `LENKER_AGENT_XRAY_BIN` is
set to a valid Xray binary path (e.g. `/usr/local/bin/xray`), node-agent manages
the Xray process lifecycle directly:

- After a successful staged -> active config apply, node-agent starts Xray as a
  child process with `active/config.json`.
- If Xray exits unexpectedly, node-agent restarts it with exponential backoff
  (1s, 2s, 4s, ... max 30s).
- When a new config revision is successfully applied, node-agent gracefully stops
  the old Xray process (SIGTERM, 5s wait, SIGKILL) and starts a new one with the
  updated config.
- On agent shutdown, Xray is gracefully stopped (SIGTERM, 5s wait, SIGKILL).
- `runtime_process_state` reflects the live process: `running`, `stopped`,
  `restarting`, or `failed`.
- `runtime_events` trail records `process_started`, `process_stopped`,
  `process_crashed`, and `process_restarted` events.
- Heartbeat payload includes `runtime_process_state` and `xray_pid`.

Example production-like environment:

```sh
export LENKER_AGENT_RUNTIME_PROCESS_MODE=local
export LENKER_AGENT_XRAY_BIN=/usr/local/bin/xray
```

With this configuration, the previous systemd Xray unit should be disabled;
node-agent owns the Xray process lifecycle.

Not included here yet:

- VPN configuration generation
- real rollback engine
- full mTLS or certificate rotation
- support for protocols beyond the main MVP path
