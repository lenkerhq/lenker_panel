# Lenker

English | [Русский](README.ru.md)

Lenker is an open-source VPN operations platform for providers and users.

It provides a self-hosted control plane for managing nodes, subscriptions, users, and config deployment — with one production protocol path: **VLESS + Reality + XTLS Vision**.

## Ecosystem

| Repository | Role |
|---|---|
| `lenker_panel` (this repo) | Backend API, node agent, web admin panel, migrations, deployment |
| [`lenker_app`](https://github.com/lenkerhq/lenker_app) | Cross-platform desktop/mobile VPN client (Flutter) |

## Architecture

```text
┌─────────────┐     ┌─────────────┐     ┌──────────────┐
│  panel-web  │────▶│  panel-api  │────▶│  PostgreSQL  │
└─────────────┘     └──────┬──────┘     └──────────────┘
                           │
                    ┌──────▼──────┐
                    │  node-agent │──▶ Xray runtime
                    └─────────────┘
```

## Current Status

**Foundation complete. Runtime verified. Consumer auth deployed.**

What works today:

- **panel-api**: admin auth, users, plans, subscriptions, nodes, config revisions, runtime apply/rollback, handoff/access tokens, audit logs, devices, traffic accounting, routing rules, global settings, WARP credentials, node profiles, subscription templates, consumer account registration/login
- **node-agent**: registration, heartbeat, config apply, Xray process supervisor, state restore, runtime events
- **panel-web**: full admin UI — login, CRUD for all entities, config revisions, runtime visibility, traffic/quota, subscription templates, node profiles, WARP config, routing rules, global settings
- **Consumer account auth**: email/password registration and login for end-user app
- **Deployment**: Docker Compose local dev, production deployment to VPS verified

## Quick Start (Docker)

```sh
make docker-build
make docker-up
make docker-bootstrap-admin
make docker-smoke
```

This starts PostgreSQL, applies migrations, starts `panel-api` + `node-agent`, creates a local admin, and verifies health endpoints.

Default admin: `owner@example.com` / `change-me-now`

Stop:
```sh
make docker-down
```

## Manual Development

Prerequisites: Go 1.22+, PostgreSQL, [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate)

```sh
export LENKER_DATABASE_URL='postgres://lenker:lenker@localhost:5432/lenker?sslmode=disable'

make migrate-up
ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD='change-me-now' make bootstrap-admin
make run-panel-api
make run-node-agent
```

## Panel Web (Admin UI)

```sh
cd apps/panel-web
npm install
npm run dev
```

Opens at `http://localhost:5173`. Requires `panel-api` running.

## Tests & Checks

```sh
make test              # all backend tests + OpenAPI lint
make test-panel-api    # panel-api unit/integration tests
make test-node-agent   # node-agent tests
make openapi-lint      # OpenAPI spec validation
make docker-smoke      # Docker-based smoke tests
```

GitHub Actions runs `make test` on push and PRs.

## Repository Layout

```text
.
├── apps/
│   ├── panel-web/          # React admin panel
│   └── client-app/         # (legacy placeholder, see lenker_app repo)
├── services/
│   ├── panel-api/          # Go backend API
│   └── node-agent/         # Go node agent + Xray supervisor
├── migrations/             # PostgreSQL migrations (golang-migrate)
├── deploy/docker/          # Docker Compose for local dev
├── docs/
│   ├── openapi/            # OpenAPI spec
│   ├── adr/                # Architecture decision records
│   ├── MVP_SPEC.md
│   ├── architecture.md
│   ├── database.md
│   └── roadmap.md
├── scripts/                # Helper scripts
├── Makefile
└── go.work
```

## API Highlights

Admin endpoints (require `Authorization: Bearer <session_token>`):
- Users, Plans, Subscriptions CRUD
- Nodes: list, detail, bootstrap token, lifecycle actions
- Config revisions: create, list, detail, report, rollback
- Subscription templates, node profiles, routing rules, WARP, global settings
- Audit logs, devices, traffic accounting

Client endpoints (public or access-token-protected):
- `POST /api/v1/accounts/register` — consumer account creation
- `POST /api/v1/accounts/login` — consumer sign-in
- `POST /api/v1/client/handoff/claim` — exchange invite for access token
- `GET /api/v1/client/subscription-access` — subscription info + node configs

## Roadmap

| Phase | Goal | Status |
|---|---|---|
| Foundation | Backend, admin UI, node agent, config flow | ✅ done |
| Runtime | Xray supervisor, apply/rollback, state restore | ✅ done |
| Consumer Auth | Account registration/login for end-user app | ✅ done |
| VPN Client | sing-box engine in lenker_app (Stage C2) | next |
| Self-Host Installer | App-driven VPS bootstrap | planned |
| Provider App Mode | Provider console inside lenker_app | planned |
| Hardening | Backup/restore, security pass, release packaging | planned |

Not in MVP: marketplace, billing, multi-protocol UI, iOS, enterprise SSO.

## Documentation

- [MVP Spec](docs/MVP_SPEC.md)
- [Architecture](docs/architecture.md)
- [Database](docs/database.md)
- [API Plan](docs/api.md)
- [OpenAPI Spec](docs/openapi/panel-api.v1.yaml)
- [Roadmap](docs/roadmap.md)
- [Business Model](docs/business-model.md)
- [ADRs](docs/adr/README.md)
- [Docker Dev](deploy/docker/README.md)
- [panel-api README](services/panel-api/README.md)
- [node-agent README](services/node-agent/README.md)

## Security

- Minimal logging by default
- No sale of user data or traffic history
- No hidden telemetry
- Session tokens stored as hashes
- Full mTLS planned (not yet complete)

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## Contributing

The project is in early development. Architecture feedback, security concerns, and focused reviews are welcome. Align with MVP scope before opening large PRs.

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[AGPL-3.0-only](LICENSE) — keeps the self-hosted core open-source.

See [docs/licensing.md](docs/licensing.md) and [TRADEMARK.md](TRADEMARK.md).
