# Lenker Architecture for MVP v0.1

## Purpose

This document describes the system architecture for `Lenker MVP v0.1`. It is limited to the first release scope and excludes marketplace and billing subsystems.

## Technology Stack

- `panel backend`: Go
- `node agent`: Go
- `panel web`: React + TypeScript
- `client app`: Flutter
- `database`: PostgreSQL
- `panel-node transport`: HTTPS + mTLS

## System Components

### 1. Panel Backend

The panel backend is the control plane of the system.

Responsibilities:

- admin authentication and authorization
- RBAC enforcement
- users, plans, subscriptions, devices, and key lifecycle
- node registration and node lifecycle
- protocol preset storage
- config generation for Xray
- signed config bundle creation
- webhook ingest and delivery
- audit logging
- client-facing REST endpoints

The backend is the source of truth for provider configuration and subscription state.

### 2. Panel Web

The web panel is the operational UI for providers.

Responsibilities:

- display dashboard state
- manage users and subscriptions
- manage plans
- manage nodes
- manage protocol preset selection
- manage API tokens and webhooks
- review audit logs

The panel web talks only to the panel backend REST API.

### 3. Node Agent

The node agent runs on each managed VPN node.

Responsibilities:

- register with the panel using a one-time bootstrap flow
- maintain authenticated transport with the panel
- report node health and basic metrics
- receive signed config bundles
- validate and apply configs atomically
- restart Xray as required
- roll back to the last working config on failure
- expose local state required for operations
- support drain mode

The node agent must not require the panel to store SSH passwords after bootstrap.

### 4. VPN Core on Node

For `MVP v0.1`, the only required deployed core is:

- `Xray`

The deployed protocol path is:

- `VLESS + Reality + XTLS Vision`

The node agent manages local Xray configuration and lifecycle.

### 5. Client Application

The client app is the end-user surface.

Supported platforms:

- Android
- Windows
- macOS

Responsibilities:

- email-first sign-in
- provider onboarding via deeplink or provider code
- subscription sync
- connect and disconnect
- node or region selection
- basic diagnostics
- key rotation
- secure local storage

The client app consumes provider APIs exposed by the panel backend.

### 6. PostgreSQL Database

PostgreSQL stores:

- provider operational data
- users and subscriptions
- nodes and protocol metadata
- device associations
- audit records
- API tokens and webhook configuration
- config revision history

## High-Level Interaction Model

### Provider Control Flow

1. Admin uses the web panel.
2. Panel web calls the panel backend REST API.
3. Panel backend reads and writes state in PostgreSQL.
4. Panel backend generates signed config bundles.
5. Panel backend delivers config bundles to node agents over `HTTPS + mTLS`.
6. Node agents apply configs and report status back.

### User Connection Flow

1. User signs in from the client app using email-first auth.
2. Client app retrieves the user subscription and available node or region choices.
3. Client app requests the active subscription configuration metadata.
4. Client app connects using the provider-issued configuration.
5. Client app reports only the minimum app state required for subscription lifecycle operations.

## Trust Boundaries

### Boundary 1: Admin to Panel

- protected by admin authentication and RBAC
- sensitive actions require audit logging

### Boundary 2: Panel to Node Agent

- protected by `HTTPS + mTLS`
- bootstrap uses one-time registration material
- config bundles are signed

Current implementation note:

The current contract slice provides admin-created one-time bootstrap tokens,
node registration, heartbeat endpoints, and node-agent local health/status
endpoints. The panel stores only bootstrap token hashes, expires tokens, and
marks tokens used after successful registration. Full mTLS establishment,
certificate rotation, Xray process control, and rollback execution are still
skeleton work.

The config delivery foundation creates deterministic signed subscription-aware
VLESS Reality Xray-compatible skeleton payloads for the single MVP path. The
renderer derives simple `subscription_inputs` and `access_entries` from active
subscriptions, active users, plans, and the target node region without adding a
production allocation engine. The rendered config object is close to Xray JSON
for one VLESS + Reality + XTLS Vision inbound. A node-facing endpoint lets the
node-agent fetch the latest pending signed revision with the node Bearer token.

The node-agent polling loop fetches pending revisions, verifies hash/signature
and payload shape, enforces an explicit Xray compatibility gate for the current
VLESS + Reality + XTLS Vision shape, writes revision-specific and staged
artifacts, switches active local config files only after staging succeeds, stores
metadata in memory, and reports `applied` or `failed` status back to the panel.
When `LENKER_AGENT_XRAY_BIN` is configured, node-agent also performs a one-shot
`xray run -test -config <candidate>` dry-run after internal validation and
before staged -> active. In the local Docker profile this is dev-only wiring:
the host binary directory can be bind-mounted with `LENKER_LOCAL_XRAY_DIR` and
exposed to the agent through `LENKER_AGENT_XRAY_BIN=/opt/lenker/xray/xray`; the
default profile leaves the variable empty and mounts an empty directory.
Panel-api also runs a lightweight renderer precheck before signing, but
node-agent is the authoritative apply boundary.
Rollback is a revision-level file switch foundation: panel-api can create a
pending rollback revision from an applied source, and the agent applies it
through the same validation and staged -> active local file path. The agent
reports read-only runtime readiness metadata (`last_validation_status`, error,
timestamp, last applied revision, active config path, and a bounded
`runtime_events` slice) through the revision report and heartbeat contracts so
panel admins can inspect the latest local validation result.
On restart, node-agent restores this local runtime readiness from `state.json`
first and falls back to `active/metadata.json` only if the active config and
metadata artifacts are still valid JSON. Restore is a local status recovery
step, not a fresh apply: it does not re-run deployment, does not emit an applied
report by itself, and moves broken/incomplete local state to a degraded
`prepare_failed` status.

Node-agent also has a no-process runtime supervisor skeleton. It tracks
`runtime_mode`, `runtime_process_mode`, `runtime_process_state`, desired
runtime state, latest runtime state, dry-run status, last prepared revision,
transition timestamp, and compact runtime error. The default process mode is
`disabled`: a validated config can become active on disk, but no Xray daemon is
launched or supervised. With `LENKER_AGENT_XRAY_BIN`, the runtime mode is
`dry-run-only`: the one-shot Xray validation must pass before local state moves
to `active_config_ready`. `LENKER_AGENT_RUNTIME_PROCESS_MODE=local` only enables
a fakeable prepare/start intent boundary for a future local runner and reports
`future-process-managed`; it does not implement daemon lifecycle, reload,
restart, process watchdog, or systemd integration. The dev profile does not
download or bake Xray into images.

Node-agent keeps a small bounded `runtime_events` trail in `/status` and
`state.json` for the newest apply success/failure, validation failure, Xray
dry-run failure, and local process prepare/start intent events. The same compact
slice can be ingested by panel-api through heartbeat/report and stored on the
node record as recent operational context. It is deliberately not a general
audit event platform.

### Boundary 3: User App to Panel

- protected by standard HTTPS
- user auth is email-first
- session tokens must be scoped to app use

Current implementation note:

The first client-delivery foundation starts with a narrow token read boundary.
Admins can request `GET /api/v1/subscriptions/{id}/access` to inspect a
deterministic `subscription_access.v1alpha1` export for an active subscription
on the single MVP `VLESS + Reality + XTLS Vision` path, and can issue a
plaintext subscription access token once through
`POST /api/v1/subscriptions/{id}/access-token`. The lifecycle is one active
token per subscription: issue and rotate revoke the previous active token before
returning a new plaintext token once, rotate without an existing token behaves
as a fresh issue, and revoke is idempotent for never-issued or already-revoked
subscriptions. Panel-api stores only the token hash and expiry. Consumers can
call `GET /api/v1/client/subscription-access` with that Bearer token to read a
redacted access export without an admin session. The provider handoff is
out-of-band for this foundation: the panel shows the plaintext token only from
issue/rotate responses, later provider reads expose only lifecycle status, and
consumer calls never use the admin session token. This does not implement full
end-user app authentication, deeplink delivery, device binding, marketplace
provider discovery, billing, or multi-protocol delivery.

The next bootstrap layer adds a one-time subscription handoff invite. A provider
admin can issue a short-lived plaintext `handoff_token` once for an active
subscription; panel-api stores only its hash. A consumer can claim that invite
through `POST /api/v1/client/handoff/claim`, which marks the invite claimed,
creates a normal subscription access token, and returns that token plus the
redacted access payload for the same single MVP path. This prepares a future
`lenker-app` bootstrap path without adding user accounts, device registry,
OAuth-like exchange flows, deeplinks, marketplace discovery, billing, or
multi-protocol delivery.

### Boundary 4: Secrets and Persistent State

- subscription secrets and node trust material must not be stored in plaintext where avoidable
- configuration revisions must be versioned

## Architectural Principles

- nodes are first-class resources
- one operational protocol path for the first release
- API-first design
- self-hosted provider model
- minimal logging by default
- rollback must be built into node config deployment

## Deployment Shape

### Minimum Supported Topology

`MVP v0.1` assumes a small-provider deployment:

- 1 panel backend instance
- 1 panel web deployment
- 1 PostgreSQL instance
- 1 or more node agents
- 1 or more Xray nodes

This is the minimum supported operational shape for the first release.

## Conservative Decisions

### Conservative choice: REST between panel web and backend

The UI uses REST only.

Reason:

- simpler to document and maintain in an open-source repository
- easier external API reuse

### Conservative choice: HTTPS + mTLS instead of gRPC for panel-node transport

`MVP v0.1` uses `HTTPS + mTLS`.

Reason:

- enough for the required operational flows
- easier transport debugging during early development
- lower integration overhead for the first release

### Conservative choice: single-node-core path

Only the Xray path is required in the first release.

Reason:

- avoids premature multi-core orchestration
- keeps rollback and health semantics tractable
