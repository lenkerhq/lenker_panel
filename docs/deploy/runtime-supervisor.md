# Runtime Supervisor

The node-agent includes an opt-in local Xray process supervisor. When enabled,
the agent starts and manages an Xray child process using the active applied
config revision.

## Enabling the Supervisor

Set two environment variables on the node-agent:

```sh
LENKER_AGENT_RUNTIME_PROCESS_MODE=local
LENKER_AGENT_XRAY_BIN=/usr/local/bin/xray
```

- `RUNTIME_PROCESS_MODE=local` activates the supervisor. Any other value (or
  unset) keeps the agent in `disabled` mode where it validates and stages config
  but does not run Xray.
- `XRAY_BIN` must point to the Xray binary. The agent uses it for both optional
  dry-run validation (`xray run -test -config <path>`) and process management.

### Docker Compose

In `deploy/docker/docker-compose.local.yml`, uncomment:

```yaml
environment:
  LENKER_AGENT_RUNTIME_PROCESS_MODE: local
  LENKER_AGENT_XRAY_BIN: /opt/lenker/xray/xray
```

### systemd

Add the variables to the unit file or an environment file:

```ini
[Service]
Environment=LENKER_AGENT_RUNTIME_PROCESS_MODE=local
Environment=LENKER_AGENT_XRAY_BIN=/usr/local/bin/xray
```

## Behavior

### Startup

When a config revision is applied and `RUNTIME_PROCESS_MODE=local`:

1. The supervisor stops any existing Xray process (SIGTERM → 5 s timeout → KILL).
2. Starts `xray run -config <active/config.json>`.
3. Records a `process_started` runtime event with the PID.

### Crash Recovery

If the Xray process exits unexpectedly:

1. A `process_crashed` event is recorded with the exit error.
2. The supervisor waits with exponential backoff (1 s → 2 s → 4 s → … → 30 s max).
3. Restarts the process from the same active config.
4. Records a `process_restarted` event.

If the restart itself fails, the state moves to `failed` and no further
automatic restarts are attempted until a new config revision is applied.

### Graceful Shutdown

On agent shutdown or new config apply:

1. SIGTERM is sent to the Xray process.
2. The supervisor waits up to 5 seconds for clean exit.
3. If the process does not exit, SIGKILL is sent.
4. A `process_stopped` event is recorded.

### Failed Config Does Not Kill Active Process

When a new config revision fails validation (internal or dry-run), the active
config and running Xray process are not affected. The failed revision is
reported as `failed` and the active revision remains unchanged.

When `PrepareStart` is called with a new config, the supervisor stops the
existing process first, then starts with the new config. If the new process
fails to start, the state moves to `failed` — the old process is already
stopped. This is intentional: the agent reports the failure and waits for a
corrected revision.

## Observable State

The following fields appear in heartbeat payloads and the panel-api node detail:

| Field | Values | Description |
|-------|--------|-------------|
| `runtime_process_mode` | `disabled`, `local` | Whether supervisor is active |
| `runtime_process_state` | `disabled`, `stopped`, `running`, `restarting`, `failed` | Current process lifecycle state |
| `xray_pid` | integer or 0 | PID of the managed Xray process |
| `runtime_state` | `not_prepared`, `active_config_ready`, `validation_failed`, `prepare_failed` | Overall runtime readiness |
| `runtime_events` | array (max 20) | Recent lifecycle events |

### Runtime Event Types

| Type | Meaning |
|------|---------|
| `apply_success` | Config revision applied and staged |
| `apply_failure` | Config revision apply failed |
| `validation_failure` | Internal config validation failed |
| `dry_run_failure` | Xray dry-run (`xray run -test`) failed |
| `process_prepare_start_intent` | Supervisor start was requested |
| `process_started` | Xray process started successfully |
| `process_stopped` | Xray process stopped (graceful) |
| `process_crashed` | Xray process exited unexpectedly |
| `process_restarted` | Xray process restarted after crash |
| `runtime_state_restore` | State restored from disk after agent restart |
| `runtime_state_restore_degraded` | Partial state restore (some data missing) |

## Logs and Secrets

- Node tokens and bootstrap tokens are excluded from JSON serialization
  (`json:"-"` tags on Identity struct).
- Runtime event messages use sanitized error codes (e.g.,
  `invalid_xray_config`, `xray_dry_run_failed:reason`) — no raw secrets.
- Xray stdout/stderr is forwarded to the agent's stdout/stderr. Ensure your
  Xray config does not log sensitive data at debug level in production.
- The crash error message contains only the process exit status (e.g.,
  `exit status 1`), not config contents.

## Smoke Tests

Three repeatable smoke tests verify the runtime path:

```sh
make docker-runtime-smoke          # Apply → report → events
make docker-runtime-failure-smoke  # Dry-run failure → active unchanged
make docker-runtime-restore-smoke  # Restart → state restore from disk
```

Prerequisites: Docker, curl, ruby (for JSON assertions).

Each test:
1. Starts the local Docker stack (PostgreSQL, panel-api, node-agent).
2. Bootstraps an admin and registers a node.
3. Creates config revisions and verifies apply/failure/restore behavior.
4. Asserts runtime events, revision status, and active artifact integrity.

## Limitations

- The supervisor manages a single Xray process per agent instance.
- No multi-core or multi-protocol process management.
- No automatic Xray binary installation or update.
- Port conflicts must be resolved externally (Xray config must use available ports).
- If the Xray binary is missing or not executable, `PrepareStart` fails immediately.
