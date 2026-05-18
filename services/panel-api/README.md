# panel-api

`panel-api` is the Go service for the Lenker provider control plane.

Planned responsibilities for `MVP v0.1`:

- admin authentication
- RBAC
- users, plans, subscriptions, devices
- node registration and lifecycle
- protocol profile management
- config revision and deployment coordination
- API tokens and webhooks
- audit logging

Current foundation:

- application entrypoint at `cmd/panel-api`
- environment-based config loading
- structured JSON logging through Go `slog`
- HTTP server with graceful shutdown
- router and response envelope helpers
- health endpoint
- PostgreSQL storage bootstrap through `database/sql` and the `pgx` stdlib driver
- repository interfaces and initial query implementations for admins, users, plans, and subscriptions
- minimal admin login service with password hash verification, inactive admin check, and session creation
- admin session validation middleware using `Authorization: Bearer <session_token>`
- bcrypt password verification for admin accounts
- minimal admin CRUD slice for users, plans, and subscriptions
- admin-created one-time node bootstrap tokens
- node registration with token expiry and one-time token consumption
- node heartbeat status and `last_seen_at` updates
- config revision metadata storage with deterministic signed subscription-aware VLESS Reality Xray config skeleton payloads
- provider-side subscription access export foundation for the single VLESS Reality MVP path
- minimal subscription access token lifecycle and read boundary for consumer-facing access export
- one-time client handoff invite bootstrap foundation for future client access setup
- RBAC and audit package-level contracts without a full permission engine
- package placeholders for the MVP control-plane domains

Local run:

```sh
go run ./cmd/panel-api
```

Configuration:

- `LENKER_APP_ENV`
- `LENKER_HTTP_ADDR`
- `LENKER_LOG_LEVEL`
- `LENKER_SHUTDOWN_TIMEOUT_SECONDS`
- `LENKER_DATABASE_URL`
- `LENKER_DATABASE_PING`
- `LENKER_VLESS_PORT`
- `LENKER_REALITY_SNI`
- `LENKER_REALITY_DEST`
- `LENKER_REALITY_SHORT_ID`
- `LENKER_REALITY_PRIVATE_KEY`
- `LENKER_REALITY_PUBLIC_KEY`
- `LENKER_REALITY_FINGERPRINT`
- `LENKER_REALITY_SPIDER_X`

Implemented foundation routes:

- `GET /healthz`
- `POST /api/v1/auth/admin/login`
- `GET /api/v1/users`
- `POST /api/v1/users`
- `GET /api/v1/users/{id}`
- `PATCH /api/v1/users/{id}`
- `POST /api/v1/users/{id}/suspend`
- `POST /api/v1/users/{id}/activate`
- `GET /api/v1/plans`
- `POST /api/v1/plans`
- `GET /api/v1/plans/{id}`
- `PATCH /api/v1/plans/{id}`
- `POST /api/v1/plans/{id}/archive`
- `GET /api/v1/subscriptions`
- `POST /api/v1/subscriptions`
- `GET /api/v1/subscriptions/{id}`
- `PATCH /api/v1/subscriptions/{id}`
- `POST /api/v1/subscriptions/{id}/renew`
- `GET /api/v1/subscriptions/{id}/access`
- `GET /api/v1/subscriptions/{id}/access-token`
- `POST /api/v1/subscriptions/{id}/access-token`
- `DELETE /api/v1/subscriptions/{id}/access-token`
- `POST /api/v1/subscriptions/{id}/access-token/rotate`
- `GET /api/v1/subscriptions/{id}/handoff-invite`
- `POST /api/v1/subscriptions/{id}/handoff-invite`
- `DELETE /api/v1/subscriptions/{id}/handoff-invite`
- `POST /api/v1/client/handoff/claim`
- `GET /api/v1/client/subscription-access`
- `GET /api/v1/nodes`
- `POST /api/v1/nodes/bootstrap-token`
- `GET /api/v1/nodes/{id}`
- `POST /api/v1/nodes/{id}/drain`
- `POST /api/v1/nodes/{id}/undrain`
- `POST /api/v1/nodes/{id}/disable`
- `POST /api/v1/nodes/{id}/enable`
- `POST /api/v1/nodes/{id}/config-revisions`
- `GET /api/v1/nodes/{id}/config-revisions`
- `GET /api/v1/nodes/{id}/config-revisions/pending`
- `GET /api/v1/nodes/{id}/config-revisions/{revisionId}`
- `POST /api/v1/nodes/{id}/config-revisions/{revisionId}/report`
- `POST /api/v1/nodes/register`
- `POST /api/v1/nodes/{id}/heartbeat`

Admin-only routes:

- all `/api/v1/users*` routes
- all `/api/v1/plans*` routes
- all `/api/v1/subscriptions*` routes
- `GET /api/v1/nodes`
- `POST /api/v1/nodes/bootstrap-token`
- `GET /api/v1/nodes/{id}`
- `POST /api/v1/nodes/{id}/drain`
- `POST /api/v1/nodes/{id}/undrain`
- `POST /api/v1/nodes/{id}/disable`
- `POST /api/v1/nodes/{id}/enable`
- `POST /api/v1/nodes/{id}/config-revisions`
- `GET /api/v1/nodes/{id}/config-revisions`
- `GET /api/v1/nodes/{id}/config-revisions/{revisionId}`

Node-agent contract routes:

- `POST /api/v1/nodes/register` accepts a one-time bootstrap token and returns a node token
- `POST /api/v1/nodes/{id}/heartbeat` accepts a node heartbeat with `Authorization: Bearer <node_token>`
- `GET /api/v1/nodes/{id}/config-revisions/pending` accepts the same node token and returns latest pending signed revision metadata for that node
- `POST /api/v1/nodes/{id}/config-revisions/{revisionId}/report` accepts the same node token and records `applied` or `failed` revision status

Node heartbeat and revision reports may include read-only runtime preparation
metadata from node-agent: `runtime_mode`, `runtime_process_mode`,
`runtime_process_state`, `runtime_desired_state`, `runtime_state`,
`last_dry_run_status`, `last_runtime_attempt_status`,
`last_runtime_prepared_revision`, `last_runtime_transition_at`, and
`last_runtime_error`. These fields describe the no-process/dry-run-only local
runtime skeleton plus the explicit disabled/local process runner gate. They do
not imply that panel-api starts, reloads, restarts, or supervises Xray.
Heartbeat and revision reports may also carry a compact `runtime_events` slice;
panel-api stores only a bounded recent trail on the node record.

Subscription access export:

- `GET /api/v1/subscriptions/{id}/access` is admin-only.
- It returns a deterministic `subscription_access.v1alpha1` object and VLESS URI
  for the single MVP `VLESS + Reality + XTLS Vision` path.
- VLESS/Reality endpoint values come from panel-api runtime config
  (`LENKER_VLESS_PORT`, `LENKER_REALITY_*`) and are used consistently for
  signed config revisions and subscription access exports. Dev defaults remain
  placeholders and must be overridden for a real node.
- The access token lifecycle is one active token per subscription.
- `GET /api/v1/subscriptions/{id}/access-token` returns read-only lifecycle
  status (`never_issued`, `active`, or `revoked`) plus timestamps and generation
  count without returning plaintext token material.
- `POST /api/v1/subscriptions/{id}/access-token` is admin-only and returns a
  plaintext access token once; issuing revokes any previous active token.
- `POST /api/v1/subscriptions/{id}/access-token/rotate` revokes the previous
  active token and returns a replacement plaintext token once. If no token
  exists yet, rotate behaves as a fresh issue.
- `DELETE /api/v1/subscriptions/{id}/access-token` revokes the active token
  without returning token material. Revoke is idempotent: never-issued
  subscriptions remain `never_issued`, and already revoked tokens remain
  `revoked`.
- Panel-api stores only token hashes and expiry timestamps.
- `GET /api/v1/client/subscription-access` accepts
  `Authorization: Bearer <subscription_access_token>` and returns a redacted
  access export without admin session auth.
- `POST /api/v1/subscriptions/{id}/handoff-invite` issues a short-lived
  one-time plaintext `handoff_token` for bootstrap handoff and stores only its
  hash.
- `GET /api/v1/subscriptions/{id}/handoff-invite` returns invite lifecycle
  status without token material.
- `DELETE /api/v1/subscriptions/{id}/handoff-invite` revokes the active invite.
- `POST /api/v1/client/handoff/claim` accepts a plaintext handoff token,
  consumes it once, creates a normal subscription access token, and returns that
  token plus the redacted access export.
- Provider handoff is deliberately out-of-band at this stage: the provider
  copies the plaintext token from the issue/rotate response and gives it to the
  subscriber through an external channel. Later provider reads show only
  lifecycle status, never the plaintext token.
- The token endpoint is a narrow read boundary only; it is not full end-user app
  authentication, deeplink delivery, device management, marketplace delivery,
  billing, or multi-protocol export.

Provider handoff example:

```sh
curl -fsS -X POST "$PANEL_URL/api/v1/subscriptions/$SUBSCRIPTION_ID/access-token" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -fsS "$PANEL_URL/api/v1/client/subscription-access" \
  -H "Authorization: Bearer $SUBSCRIPTION_ACCESS_TOKEN"
```

The local `make docker-subscription-access-smoke` and
`make docker-handoff-smoke` helpers exercise the same handoff path and end with
a compact operational summary. That summary confirms the selected
subscription/node/endpoint, invite claim, lifecycle transitions, client read,
rotate/revoke checks, and redaction status without printing plaintext tokens.
For operator steps around issue, out-of-band handoff, verification, rotation,
and revocation, see the provider handoff runbook in
`docs/smoke/node-bootstrap.md`.

Use the token returned by admin login:

```http
Authorization: Bearer <session_token>
```

OpenAPI draft:

- [docs/openapi/panel-api.v1.yaml](/Users/vaceslavibraev/Desktop/vpn_service/docs/openapi/panel-api.v1.yaml)

Not included here yet:

- delete operations
- advanced business rules
- full production authentication policy
- logout and full session lifecycle management
- refresh tokens
- 2FA
- RBAC permission engine
- audit persistence
- devices, key rotation, and end-user client delivery flows
- full node orchestration engine
- full mTLS or certificate rotation
- real Xray process restart/control or rollback executor
- billing
- marketplace
- VPN or Xray logic

Conservative storage note:

The service opens a PostgreSQL handle at startup, but `LENKER_DATABASE_PING` defaults to `false`. This keeps the local HTTP skeleton runnable without a database while still allowing deployments and tests to opt into startup connectivity checks.

Conservative auth note:

Admin password hashes must use bcrypt. This keeps the first auth path stronger than the earlier foundation placeholder without adding a larger auth platform, 2FA, refresh tokens, OAuth, or phone auth.

Migration workflow:

```sh
make migrate-up
make migrate-down
VERSION=1 make migrate-force
```

## Local Development Bootstrap

This flow is for a local development database only. It is not a production installer.

Install PostgreSQL using the package manager for your OS. Examples:

```sh
# macOS with Homebrew
brew install postgresql@16
brew services start postgresql@16

# Debian/Ubuntu
sudo apt-get install postgresql
sudo systemctl start postgresql
```

Create a local database and user:

```sh
createuser lenker --pwprompt
createdb -O lenker lenker
```

Export the database URL:

```sh
export LENKER_DATABASE_URL='postgres://lenker:lenker@localhost:5432/lenker?sslmode=disable'
export LENKER_DATABASE_PING=true
```

Apply migrations from the repository root:

```sh
make migrate-up
```

Create the first local admin:

```sh
ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD='change-me-now' make bootstrap-admin
```

The bootstrap helper stores a bcrypt password hash. If the admin already exists, it prints a clear message and does not change the password.

Run the API:

```sh
make run-panel-api
```

Check health:

```sh
curl -i http://localhost:8080/healthz
```

Login:

```sh
curl -s http://localhost:8080/api/v1/auth/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"change-me-now"}'
```

Copy `data.session.token` from the response and use it as a Bearer token:

```sh
export LENKER_ADMIN_TOKEN='<session_token>'
```

Create and inspect a user:

```sh
curl -s http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","display_name":"Test User"}'

curl -s http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Create and inspect a plan:

```sh
curl -s http://localhost:8080/api/v1/plans \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Monthly","duration_days":30,"device_limit":3}'

curl -s http://localhost:8080/api/v1/plans \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Create and inspect a subscription using the `id` values returned by the user and plan calls:

```sh
curl -s http://localhost:8080/api/v1/subscriptions \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"<user_id>","plan_id":"<plan_id>"}'

curl -s http://localhost:8080/api/v1/subscriptions \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Create a one-time node bootstrap token:

```sh
curl -s http://localhost:8080/api/v1/nodes/bootstrap-token \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"finland-1","region":"eu","country_code":"FI","hostname":"node-fi-1","expires_in_minutes":30}'
```

Copy `data.bootstrap_token` and `data.node_id` from the response. The plaintext
bootstrap token is shown only once; only its hash is stored.

Register the node-agent:

```sh
curl -s http://localhost:8080/api/v1/nodes/register \
  -H 'Content-Type: application/json' \
  -d '{"node_id":"<node_id>","bootstrap_token":"<bootstrap_token>","agent_version":"0.1.0-dev","hostname":"node-fi-1"}'
```

Copy `data.node_token` from the registration response and use it for heartbeat:

```sh
curl -s http://localhost:8080/api/v1/nodes/<node_id>/heartbeat \
  -H "Authorization: Bearer <node_token>" \
  -H 'Content-Type: application/json' \
  -d '{"node_id":"<node_id>","agent_version":"0.1.0-dev","status":"active","active_revision":0}'
```

Registration rejects invalid, expired, and already used bootstrap tokens. A
heartbeat for an unknown node returns `not_found`; heartbeat does not create
nodes.

## Node Management

List nodes:

```sh
curl -s http://localhost:8080/api/v1/nodes \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Get node details:

```sh
curl -s http://localhost:8080/api/v1/nodes/<node_id> \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Drain and undrain a node:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/drain \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"

curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/undrain \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Disable and enable a node:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/disable \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"

curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/enable \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Conservative lifecycle behavior:

- `drain` sets `drain_state` to `draining`; it does not stop heartbeat.
- `undrain` sets `drain_state` to `active`; disabled nodes stay disabled.
- `disable` sets `status` to `disabled`; disabled nodes do not accept heartbeat updates.
- `enable` returns the node to `unhealthy` until the next successful heartbeat.

## Config Revision Metadata

The config foundation creates deterministic subscription-aware VLESS Reality
Xray-compatible skeleton payloads for the single MVP protocol path and records
revision number, status, bundle hash, signature, signer, rollback target
metadata, and timestamps. The renderer uses real active users, plans, and
subscriptions as `subscription_inputs` and turns eligible subscriptions into
deterministic `access_entries`. Eligibility is intentionally simple for this
stage: active subscriptions for active users whose preferred region is empty or
matches the target node region.

The payload is structured and hash/signature stable. Its `config` object now
uses Xray-like `log`, `policy`, `stats`, `inbounds`, `outbounds`, and `routing`
sections for the single VLESS + Reality + XTLS Vision path. The panel still does
not write runtime Xray config files, restart or control Xray.

Panel-api runs a lightweight render precheck before signing new deploy or
rollback revisions. The authoritative Xray compatibility gate lives in
node-agent, where the signed config object must pass the single-path VLESS
Reality validation contract before staged files can become active. Node-agent
can also run an optional one-shot Xray binary dry-run before apply when
`LENKER_AGENT_XRAY_BIN` is configured.

Create a config revision metadata record:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/config-revisions \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

List and inspect revisions:

```sh
curl -s http://localhost:8080/api/v1/nodes/<node_id>/config-revisions \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"

curl -s http://localhost:8080/api/v1/nodes/<node_id>/config-revisions/<revision_id> \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

Revision creation is rejected for disabled nodes and for nodes in `draining` or
`drained` drain state.

Node-agent fetches the latest pending revision metadata through the node-facing
endpoint:

```sh
curl -s http://localhost:8080/api/v1/nodes/<node_id>/config-revisions/pending \
  -H "Authorization: Bearer <node_token>"
```

The endpoint returns `not_found` when there is no pending revision, the token
does not match the node, or the node is disabled.

Node-agent reports metadata apply status through the node-facing report
endpoint:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/config-revisions/<revision_id>/report \
  -H "Authorization: Bearer <node_token>" \
  -H 'Content-Type: application/json' \
  -d '{"status":"applied","active_revision":1}'
```

Failed validation can be reported as:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/config-revisions/<revision_id>/report \
  -H "Authorization: Bearer <node_token>" \
  -H 'Content-Type: application/json' \
  -d '{"status":"failed","error_message":"invalid_xray_config:missing_stream_settings"}'
```

Optional Xray binary dry-run failures are reported the same way, with compact
messages such as `xray_dry_run_failed:invalid_config`.

Applied reports update revision status and node `active_revision`. Failed
reports persist `failed_at` and `error_message`, and node `active_revision` does
not move. Report and heartbeat payloads can also persist node runtime readiness
metadata: `last_validation_status`, `last_validation_error`,
`last_validation_at`, `last_applied_revision`, `active_config_path`, and the
bounded recent `runtime_events` slice. The report path does not execute
rollback, restart processes, or control Xray.

Rollback is represented as a normal pending revision created from a known-good
applied revision:

```sh
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/config-revisions/<applied_revision_id>/rollback \
  -H "Authorization: Bearer $LENKER_ADMIN_TOKEN"
```

The rollback endpoint recomputes hash/signature for the new pending revision,
sets `operation_kind=rollback` and source revision metadata in the signed
payload, and leaves file switching to node-agent polling. Panel-api does not
mutate node files, restart processes, or run a rollback executor.

Manual smoke checklist:

- [docs/smoke/node-bootstrap.md](/Users/vaceslavibraev/Desktop/vpn_service/docs/smoke/node-bootstrap.md)

Useful local targets from the repository root:

```sh
make migrate-up
make migrate-down
VERSION=1 make migrate-force
ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD='change-me-now' make bootstrap-admin
make run-panel-api
make test-panel-api
```
