# Node Bootstrap Smoke Checklist

This checklist verifies the local node bootstrap, registration, heartbeat,
admin node lifecycle, and config revision report flow. It is for local
development only.

It does not verify mTLS, Xray runtime process control, process restart/reload,
metrics, or traffic handling.

## Prerequisites

- Go 1.22+
- PostgreSQL
- `golang-migrate/migrate`
- `curl`

From the repository root:

```sh
export LENKER_DATABASE_URL='postgres://lenker:lenker@localhost:5432/lenker?sslmode=disable'
export LENKER_DATABASE_PING=true
```

## 1. Apply Migrations

```sh
make migrate-up
```

Expected result:

- migrations finish successfully;
- `nodes`, `node_bootstrap_tokens`, and `node_registrations` tables exist.

## 2. Create First Admin

```sh
ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD='change-me-now' make bootstrap-admin
```

Expected result:

- admin exists;
- password hash is stored as bcrypt.

## 3. Run panel-api

```sh
make run-panel-api
```

In another terminal:

```sh
curl -i http://localhost:8080/healthz
```

Expected response:

```json
{"data":{"status":"ok"}}
```

## 4. Login As Admin

```sh
curl -s http://localhost:8080/api/v1/auth/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"change-me-now"}'
```

Expected result:

- response contains `data.session.token`.

Export it:

```sh
export LENKER_ADMIN_TOKEN='<session_token>'
```

## 5. Create Bootstrap Token

```sh
curl -s http://localhost:8080/api/v1/nodes/bootstrap-token \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"finland-1","region":"eu","country_code":"FI","hostname":"node-fi-1","expires_in_minutes":30}'
```

Expected result:

- response contains `data.node_id`;
- response contains `data.bootstrap_token`;
- plaintext bootstrap token is shown only once.

Export returned values:

```sh
export LENKER_NODE_ID='<node_id>'
export LENKER_NODE_BOOTSTRAP_TOKEN='<bootstrap_token>'
```

## 6. Register Node

```sh
curl -s http://localhost:8080/api/v1/nodes/register \
  -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$LENKER_NODE_ID\",\"bootstrap_token\":\"$LENKER_NODE_BOOTSTRAP_TOKEN\",\"agent_version\":\"0.1.0-dev\",\"hostname\":\"node-fi-1\"}"
```

Expected result:

- response contains `data.node_token`;
- `data.status` is `active`;
- bootstrap token is consumed and cannot be reused.

Export the node token:

```sh
export LENKER_NODE_TOKEN='<node_token>'
```

Reusing the same bootstrap token should return `bootstrap_token_used`.

## 7. Send Heartbeat

```sh
curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/heartbeat \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$LENKER_NODE_ID\",\"agent_version\":\"0.1.0-dev\",\"status\":\"active\",\"active_revision\":0}"
```

Expected result:

- `data.status` is `active`;
- `data.drain_state` is `active`;
- `data.last_seen_at` is set.

## 8. List And Inspect Nodes

```sh
curl -s http://localhost:8080/api/v1/nodes \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"

curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- list includes the registered node;
- details include `status`, `drain_state`, `last_seen_at`, `registered_at`,
  `agent_version`, and `active_revision_id`.

## 9. Create, Fetch, And Report Config Revision Metadata

Create signed config revision metadata as an admin:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- response contains `data.revision_number`;
- response contains `data.bundle_hash`, `data.signature`, `data.signer`, and
  `data.bundle`;
- `data.bundle` contains a deterministic subscription-aware VLESS Reality Xray-
  compatible skeleton payload for the single MVP path;
- `data.bundle.config` contains Xray-like `log`, `policy`, `stats`, `inbounds`,
  `outbounds`, and `routing` sections;
- `data.bundle.subscription_inputs` and `data.bundle.access_entries` are arrays,
  empty if there are no active eligible subscriptions for this node region;
- `data.rollback_target_revision` points at the node's active revision, or `0`
  if none has been applied yet.

Fetch the latest pending revision as the node-agent:

```sh
curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions/pending \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN"
```

Expected result:

- response contains the latest pending revision for this node;
- using another node token or a missing token does not return this revision;
- if no pending revision exists, the endpoint returns `not_found`.

For local Docker end-to-end polling, restart `node-agent` with the registered
node identity after registration and revision creation:

```sh
LENKER_AGENT_NODE_ID="$LENKER_NODE_ID" \
LENKER_AGENT_NODE_TOKEN="$LENKER_NODE_TOKEN" \
LENKER_AGENT_CONFIG_POLL_INTERVAL=2s \
make docker-up
```

Expected result:

- node-agent polling starts instead of logging that config polling is disabled;
- the pending revision is fetched and applied through validation plus staged ->
  active local file switch;
- the report endpoint marks the revision `applied`;
- node detail shows runtime readiness metadata and recent `runtime_events`.

The node-agent unit tests verify that the fetched metadata can be hash/signature
validated, checked against the single-path Xray compatibility gate, optionally
checked through a configured Xray binary dry-run, serialized to local config
artifacts, stored in memory, and reflected in the heartbeat active revision
payload. Set `LENKER_AGENT_XRAY_BIN=/path/to/xray` to enable the optional
one-shot `xray run -test -config <candidate>` boundary. Leave it unset to use
the default internal validation path.

For Docker local development, the default profile leaves Xray dry-run disabled:

```sh
unset LENKER_AGENT_XRAY_BIN
make docker-up
```

If a local Xray binary is already installed, opt in explicitly without
downloading or baking it into the image:

```sh
# Inspect the exports for the Xray binary Docker bind mount.
make docker-xray-dry-run-env

# Then run the printed exports, for example:
export LENKER_LOCAL_XRAY_DIR="$(dirname "$(command -v xray)")"
export LENKER_AGENT_XRAY_BIN=/opt/lenker/xray/$(basename "$(command -v xray)")
make docker-up
```

`make docker-xray-dry-run-env` is a helper: it checks `command -v xray`, or
`XRAY_BIN=/absolute/path/to/xray`, and prints the two exports plus
`make docker-up`. It verifies that the binary is executable, does not download
Xray, and does not start Docker.

The compose file bind-mounts `$LENKER_LOCAL_XRAY_DIR` to `/opt/lenker/xray`.
After startup, confirm the optional boundary is active:

```sh
curl -s http://localhost:8090/status
```

Expected signals before creating/applying a revision:

- `xray_dry_run_enabled` is `true`;
- `last_validation_status` may be empty if no revision has been applied yet.

A successful pending revision apply then proves the candidate passed
`xray run -test -config <candidate>` before staged -> active. Expected signals
after apply:

- `last_validation_status` is `applied`;
- `last_validation_error` is empty;
- `last_applied_revision` and `active_revision` match the applied revision;
- `active_config_path` points to the active config artifact.

To verify the missing-binary failure path, set `LENKER_AGENT_XRAY_BIN` to a
missing path and create a new revision; the revision should report `failed` with
`xray_dry_run_failed:xray_binary_not_found`, while the previous active config
remains unchanged.

For a reproducible failed dry-run fixture without downloading Xray or running a
supervisor, use the node-agent fixture test:

```sh
cd services/node-agent
go test ./internal/agent -run CommandDryRunFixture
```

The fixture binary at `internal/agent/testdata/xray-dry-run-fail.sh` accepts the
same `run -test -config <candidate>` command shape, verifies that the candidate
config exists, then exits non-zero. Expected result:

- the pending revision is reported as `failed`;
- `error_message` stays compact:
  `xray_dry_run_failed:xray_dry_run_failed_invalid_inbound_for_smoke_fixture`;
- `last_validation_status`, `last_validation_error`, and
  `last_validation_at` reflect the failure;
- the previous `active/config.json` and active revision remain unchanged.

The node detail response also exposes read-only runtime readiness metadata after
apply/failure: `last_validation_status`, `last_validation_error`,
`last_validation_at`, `last_applied_revision`, and `active_config_path`.
It also exposes the runtime supervisor skeleton state:
`runtime_mode`, `runtime_process_mode`, `runtime_process_state`,
`runtime_desired_state`, `runtime_state`, `last_dry_run_status`,
`last_runtime_attempt_status`,
`last_runtime_prepared_revision`, `last_runtime_transition_at`, and
`last_runtime_error`. In the default no-process mode, a successful apply should
show `runtime_state=active_config_ready`,
`runtime_process_mode=disabled`, `runtime_process_state=disabled`,
`last_runtime_attempt_status=skipped`, and no runtime error.
`LENKER_AGENT_RUNTIME_PROCESS_MODE=local` opts into only a local process runner
skeleton: it records a prepare/start intent after active files are ready and
should report `runtime_mode=future-process-managed`,
`runtime_process_mode=local`, `runtime_process_state=ready`, and
`last_runtime_attempt_status=ready`. It still does not launch, reload, restart,
watch, or supervise Xray. A validation or dry-run failure should show
`runtime_state=validation_failed`, preserve the previous active revision/config
artifact, and set a compact runtime error.

Report the pending revision as applied:

```sh
export LENKER_REVISION_ID='<revision_id>'

curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions/$LENKER_REVISION_ID/report \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"status":"applied","active_revision":1}'
```

Expected result:

- `data.status` is `applied`;
- `data.applied_at` is set;
- a later admin config revision fetch shows the applied status.

Validation failures can be reported as failed:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions/$LENKER_REVISION_ID/report \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"status":"failed","error_message":"invalid_xray_config:missing_stream_settings"}'
```

Optional Xray binary dry-run failures use the same failed report path with a
compact reason such as `xray_dry_run_failed:invalid_config`.

Expected result:

- `data.status` is `failed`;
- `data.failed_at` is set;
- `data.error_message` contains the compact failure reason;
- node `active_revision` does not advance.

This smoke path still does not restart processes or control Xray. The node-agent
compatibility and serialization foundation writes local artifacts only under its
state directory after hash/signature validation, internal Xray compatibility
validation, and optional Xray binary dry-run validation all pass:

```text
revisions/<revision_number>/config.json
revisions/<revision_number>/metadata.json
staged/config.json
staged/metadata.json
active/config.json
active/metadata.json
state.json
```

On node-agent restart, local runtime readiness is restored from `state.json`.
If that file is absent or malformed, the agent falls back to `active/metadata.json`
only when active config and metadata artifacts are still valid JSON. A restore
does not create a new applied report by itself; the next heartbeat should show
the restored active revision, validation/runtime metadata, artifact paths, and a
`runtime_state_restore` event. Broken or incomplete local state should surface
as degraded `prepare_failed` metadata rather than a false ready state.

Rollback is requested by creating a new pending revision from an applied source:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions/$LENKER_REVISION_ID/rollback \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- response contains a new pending revision;
- `data.bundle.operation_kind` is `rollback`;
- `data.bundle.source_revision_number` points at the applied source revision;
- the agent later applies it with the same staged -> active file switch path;
- no Xray process is restarted or controlled.

After metadata apply in the agent skeleton, heartbeat can report the applied
revision number and runtime readiness metadata:

```sh
curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/heartbeat \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$LENKER_NODE_ID\",\"agent_version\":\"0.1.0-dev\",\"status\":\"active\",\"active_revision\":1,\"last_validation_status\":\"applied\",\"last_validation_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"last_applied_revision\":1,\"active_config_path\":\"/var/lib/lenker/node-agent/active/config.json\"}"
```

Expected result:

- `data.active_revision` matches the reported metadata revision.
- `data.last_validation_status` is `applied`.

## 10. Runtime Event Ingestion Check

This check verifies the durable recent `runtime_events` path in the local stack.
It uses the existing heartbeat/report ingestion contracts; it does not add a new
admin workflow or backend feature.

For the scripted local Docker path, run from the repository root:

```sh
make docker-build
make docker-runtime-smoke
```

The helper starts the local stack, bootstraps the admin, creates and registers a
node, creates a config revision, restarts node-agent with the registered node
id/token, waits for polling apply/report, checks active local artifacts, and
verifies node detail runtime readiness plus persisted `runtime_events`.

For the scripted failure-mode path, run:

```sh
make docker-runtime-failure-smoke
```

The failure helper first applies a baseline revision, then creates a second
revision and restarts node-agent with `LENKER_AGENT_XRAY_BIN` pointing at a
missing binary. Expected result:

- the second revision reports `failed`, not `applied`;
- `last_validation_error` starts with
  `xray_dry_run_failed:xray_binary_not_found`;
- node active revision and active local artifact stay on the baseline revision;
- runtime readiness moves to `validation_failed`;
- persisted `runtime_events` include `dry_run_failure`.

For the scripted restart/restore path, run:

```sh
make docker-runtime-restore-smoke
```

The restore helper applies a pending revision, checks persisted active
artifacts, restarts only the `node-agent` container, then verifies local
`/status` and panel-api node detail. Expected result:

- `active_revision` and `last_applied_revision` still match the applied
  revision;
- `runtime_state` is `active_config_ready`;
- `last_validation_status` remains `applied`;
- `active_config_path` is present;
- `runtime_events` include `runtime_state_restore`;
- the original revision `applied_at` stays unchanged, which is the observable
  smoke check that restore did not send a fake applied report.

For the scripted subscription access handoff path, run:

```sh
make docker-subscription-access-smoke
```

For the same scripted path with a name focused on the invite claim demo, run:

```sh
make docker-handoff-smoke
```

The helper starts the same local stack, bootstraps admin auth, creates a user,
plan, active subscription, and active node in a unique smoke region, creates a
subscription-aware config revision, waits for node-agent polling apply/report,
then exercises the provider handoff flow:

1. provider inspects `GET /api/v1/subscriptions/{id}/access`;
2. provider issues a one-time visible plaintext token with
   `POST /api/v1/subscriptions/{id}/access-token`;
3. the token is treated as the out-of-band handoff artifact;
4. consumer reads `GET /api/v1/client/subscription-access` with
   `Authorization: Bearer <subscription_access_token>`;
5. provider issues a one-time `handoff_token`;
6. consumer claims it with `POST /api/v1/client/handoff/claim`;
7. the claim returns a normal `access_token` plus redacted access payload;
8. consumer reads `GET /api/v1/client/subscription-access` with the claimed
   access token;
9. provider rotates and revokes the token to prove old/current tokens stop
   working.

It verifies:

- the access endpoint returns `subscription_access.v1alpha1`;
- missing and invalid client access tokens return `401`;
- initial token lifecycle status is `never_issued`;
- the issued access token can read the redacted client access payload;
- status becomes `active` with generation `1` after issue;
- initial handoff invite lifecycle status is `never_issued`;
- the issued handoff token has the `lnkhi_` prefix and is shown only inside the
  helper for the scripted claim call;
- `POST /api/v1/client/handoff/claim` succeeds once, marks invite status
  `claimed`, returns a normal `lnksa_` access token, and returns the same
  redacted access payload;
- repeated claim with the same handoff token returns `401`;
- the claimed access token can read the same redacted client access payload;
- the pre-claim access token is rejected after claim because claim creates the
  current normal access token;
- `POST /api/v1/subscriptions/{id}/access-token/rotate` invalidates the old
  token and the rotated token can read the same redacted payload;
- status remains `active` and generation advances after rotate;
- `DELETE /api/v1/subscriptions/{id}/access-token` invalidates the rotated
  token without returning token material;
- status becomes `revoked`, and repeated revoke remains safe/idempotent;
- the selected access node is the same active node whose config was applied;
- the revision bundle and active `config.json` contain the exported
  subscription/client entry;
- endpoint network/security/SNI/short id/port match the active config;
- the deterministic VLESS URI matches the structured endpoint/client fields;
- the client access payload matches provider-side endpoint/client data without
  leaking provider-internal `user_id`, `user_label`, or `plan_id`.

On success, the helper prints a short provider-facing handoff summary with the
subscription id, selected node, endpoint host/port, protocol path, lifecycle
transition result, handoff claim result, repeated-claim rejection,
client-read result, rotate/revoke checks, and redaction status. The summary
intentionally does not print the plaintext handoff token or access token; tokens
remain visible only inside the helper for scripted API calls.

### Provider Release Readiness Checklist

Use this compact checklist before a provider-side demo or release marker. It
confirms the runtime/config apply path and the handoff/bootstrap/access path;
it does not validate client-app, marketplace, billing, deeplinks, or real Xray
process supervision.

Prerequisites:

- Docker daemon is available;
- local images are current; run `make docker-build` after backend or agent
  changes;
- local admin bootstrap credentials match `ADMIN_EMAIL` and `ADMIN_PASSWORD`,
  or those env vars are exported for the smoke helpers;
- optional Xray binary dry-run remains opt-in; the default checklist works
  without `LENKER_AGENT_XRAY_BIN`.

Run, in order:

1. `make docker-runtime-smoke`
   - node bootstrap/register succeeds;
   - config revision is fetched, validated, staged, activated, and reported as
     `applied`;
   - active config artifacts exist;
   - node detail exposes runtime readiness and persisted `runtime_events`.
2. `make docker-runtime-failure-smoke`
   - baseline active config remains intact;
   - forced dry-run failure reports `failed`;
   - runtime readiness moves to `validation_failed`;
   - persisted `runtime_events` include the failure signal.
3. `make docker-runtime-restore-smoke`
   - active config artifacts and `state.json` survive node-agent restart;
   - local `/status` restores active revision and readiness metadata;
   - panel-api node detail ingests the restored heartbeat;
   - no fake applied report changes the revision `applied_at`.
4. `make docker-handoff-smoke`
   - subscription access export matches the applied node/config payload;
   - handoff invite is issued and claimed exactly once;
   - resulting access token can read the redacted client access payload;
   - repeated claim, rotated old token, and revoked token are rejected;
   - final summary is redacted and does not print plaintext tokens.

Panel/API visibility checks:

- Nodes detail in panel-web shows runtime readiness and recent runtime events
  for the smoke node.
- Subscriptions access block shows selected node, endpoint, protocol path,
  VLESS URI, access token status, and handoff invite status.
- Admin node/subscription API responses match the statuses shown in panel-web.
- Smoke summaries show sane subscription, node, endpoint, protocol, applied
  revision, and lifecycle values.

Release blockers:

- runtime revision never becomes `applied`;
- failure smoke changes the previous active revision or active artifact;
- runtime readiness fields or `runtime_events` are missing after apply/failure;
- handoff claim succeeds more than once;
- rotated old token or revoked token still reads access;
- smoke summary prints plaintext handoff or access token material.

### Provider Handoff Operator Runbook

Goal:

- issue a subscription access token;
- hand it to the subscriber through an external channel;
- verify that the client read path works;
- rotate or revoke the token when operationally needed.

Preconditions:

- the provider panel or local Docker stack is running;
- an admin session is available;
- the subscription is active;
- at least one eligible node has an applied active config revision for the
  subscription's current MVP path;
- the Subscriptions access block or admin API can load the provider-side access
  export.

Happy path:

1. In panel-web, open Subscriptions and select the subscription access block, or
   call `GET /api/v1/subscriptions/{id}/access`.
2. Confirm the selected node, endpoint host/port, protocol path, and VLESS URI
   match the expected active node/config path.
3. Issue a token with the panel Issue action or
   `POST /api/v1/subscriptions/{id}/access-token`.
4. Copy the plaintext token from that one response only. Do not store it in git,
   docs, tickets, screenshots, logs, or long-lived notes.
5. Hand the token to the subscriber out-of-band.
6. The consumer path reads `GET /api/v1/client/subscription-access` with
   `Authorization: Bearer <subscription_access_token>`.
7. Verify the returned redacted payload and URI match the provider-side access
   export.

Expected signals:

- token status is `active`, `issued=true`, and `issued_at` is visible in
  panel-web/API status;
- smoke summary shows `client_read: ok`,
  `client_payload_redacted: true`, and `plaintext_token_printed: false`;
- smoke summary selected node, endpoint, protocol path, and applied revision
  match the active node/config path;
- provider status reads do not return plaintext token material.
- optional bootstrap handoff invite claim returns a normal client access token
  plus the same redacted access payload.

Pre-release operator demo checklist:

- login to panel-web as admin;
- open Nodes and confirm runtime readiness plus recent runtime events are
  visible for the demo node;
- open Subscriptions and confirm the access export block shows the selected
  node, endpoint, protocol path, and VLESS URI;
- issue a subscription access token and confirm token status becomes `active`;
- read `GET /api/v1/client/subscription-access` with that Bearer token and
  confirm the redacted payload matches the provider access block;
- issue a handoff invite, claim it with
  `POST /api/v1/client/handoff/claim`, and confirm the invite becomes
  `claimed`;
- rotate the token and confirm the old token returns `401`;
- read again with the rotated token and confirm the payload still matches;
- revoke the token and confirm the rotated token now returns `401`;
- run or review `make docker-handoff-smoke` output and confirm the
  summary shows sane subscription/node/endpoint values, `client_read: ok`,
  `handoff_claim: ok`, `repeated_claim_rejected: true`, rotate/revoke checks
  passed, and `plaintext_token_printed: false`.

Rotation path:

1. Rotate with the panel Rotate action or
   `POST /api/v1/subscriptions/{id}/access-token/rotate`.
2. Copy and hand off the new plaintext token once.
3. Verify the old token is rejected by the client read endpoint.
4. Verify the rotated token can read the same redacted access payload.
5. Confirm status remains `active` and generation advances.

Revocation path:

1. Revoke with the panel Revoke action or
   `DELETE /api/v1/subscriptions/{id}/access-token`.
2. Verify the current token is rejected by the client read endpoint.
3. Confirm status is `revoked` and `revoked_at` remains visible.
4. Repeated revoke is safe and should keep the same revoked lifecycle state.

Common operator notes:

- never use the admin Bearer token in the consumer read flow;
- never expect a later provider status/export read to recover plaintext token
  material;
- treat the smoke summary as operational confirmation, not as the subscriber
  delivery payload;
- this runbook does not cover client-app, deeplinks, device binding,
  marketplace delivery, billing, or end-user account auth.
- client handoff invites are one-time bootstrap artifacts: claiming one returns
  a normal access token for the existing client read endpoint, but does not
  create a user account or device record.

Troubleshooting failed handoff or client-read cases:

- Token was never issued: provider token status is `never_issued` and
  `issued=false`; issue a token before handing anything to the subscriber.
- Old token stopped working after rotation: this is expected. Status should be
  `active` with a higher generation, and only the newest rotate response token
  should work.
- Token was revoked: provider status is `revoked` with `revoked_at`; client
  reads must return `401` until a new issue or rotate response provides a new
  plaintext token.
- Missing or invalid Bearer token: `GET /api/v1/client/subscription-access`
  should return `401`; check that the consumer request uses
  `Authorization: Bearer <subscription_access_token>`, not the admin session
  token.
- Client read works but data differs from provider expectations: compare the
  provider access block or `GET /api/v1/subscriptions/{id}/access` with the
  smoke summary selected node, endpoint, protocol path, and applied revision.
  If they differ, rerun `make docker-subscription-access-smoke` and inspect the
  active node/config revision before handing out a new token.

If node-agent is running in the Docker profile, first inspect local agent status:

```sh
curl -s http://localhost:8090/status
```

Expected result after at least one apply/fail/process-intent event:

- `runtime_events` is present;
- each event has a compact `type`, `at`, optional `status`, optional
  `revision_number`, and optional `message`;
- no node token, bootstrap token, or config secret is present.

To force a minimal heartbeat ingestion check with a synthetic recent event:

```sh
export LENKER_EVENT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/heartbeat \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$LENKER_NODE_ID\",\"agent_version\":\"0.1.0-dev\",\"status\":\"active\",\"active_revision\":1,\"last_validation_status\":\"applied\",\"last_validation_at\":\"$LENKER_EVENT_AT\",\"last_applied_revision\":1,\"active_config_path\":\"/var/lib/lenker/node-agent/active/config.json\",\"runtime_events\":[{\"type\":\"apply_success\",\"status\":\"applied\",\"revision_number\":1,\"message\":\"smoke runtime event\",\"runtime_mode\":\"no-process\",\"runtime_process_mode\":\"disabled\",\"runtime_process_state\":\"disabled\",\"at\":\"$LENKER_EVENT_AT\"}]}"
```

Expected result:

- heartbeat response includes `data.runtime_events`;
- the newest event has `type=apply_success`;
- panel-api stores only the bounded recent event slice for this node.

To verify report ingestion instead of heartbeat ingestion, report a pending
revision with the same compact slice:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/config-revisions/$LENKER_REVISION_ID/report \
  -H "Authorization: Bearer $LENKER_NODE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"status\":\"applied\",\"active_revision\":1,\"runtime_events\":[{\"type\":\"apply_success\",\"status\":\"applied\",\"revision_number\":1,\"message\":\"smoke report runtime event\",\"runtime_mode\":\"no-process\",\"runtime_process_mode\":\"disabled\",\"runtime_process_state\":\"disabled\",\"at\":\"$LENKER_EVENT_AT\"}]}"
```

Then inspect the admin node detail API:

```sh
curl -s http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- `data.runtime_events` is present;
- the slice contains the recent event sent by heartbeat or report;
- the event message stays compact;
- repeated heartbeats/reports keep only the newest bounded slice.

Panel-web verification:

1. Start panel-web with `npm run panel-web:dev`.
2. Open `http://localhost:5173`.
3. Login as admin and open Nodes.
4. Select the node.
5. In Node detail, the Runtime events block shows the recent event type,
   timestamp, status/result, revision, runtime mode, and compact message.

If `runtime_events` is empty, panel-web should show `No runtime events yet.`

## 11. Drain And Undrain

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/drain \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- `data.drain_state` is `draining`;
- heartbeat is still accepted.

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/undrain \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- `data.drain_state` is `active`.

## 12. Disable And Enable

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/disable \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- `data.status` is `disabled`;
- heartbeat for this node no longer updates node state.

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/$LENKER_NODE_ID/enable \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Expected result:

- `data.status` is `unhealthy`;
- the next successful heartbeat can move it back to `active`.
