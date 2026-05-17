# panel-web

`panel-web` is the React + TypeScript web application for Lenker provider operations.

Current implemented foundation:

- Vite + React entrypoint;
- TypeScript source compilation;
- base responsive layout;
- admin login against `panel-api`;
- admin session stored in `sessionStorage`;
- expired or malformed admin sessions are cleared on load;
- unauthorized API responses clear the session and return the admin to login;
- dashboard shell;
- users management page with list, create, update, suspend, and activate flows;
- plans management page with list, create, update, and archive flows;
- subscriptions management page with list, create, update, and renew flows;
- subscriptions page can load a compact read-only access export for the single MVP VLESS Reality path;
- subscriptions page can show read-only subscription access token lifecycle status (`never_issued`, `active`, `revoked`) and issue, rotate, or revoke the current token for provider handoff; plaintext tokens are only shown from issue/rotate responses and are not recoverable from later status reads;
- subscriptions page can issue or revoke a short-lived one-time client handoff invite; plaintext handoff invite tokens are only shown from issue responses and are not recoverable from later status reads;
- provider handoff remains out-of-band in this UI layer: the panel shows lifecycle status and one-time token responses but does not implement a client portal, invite link, or deeplink delivery flow;
- operator handoff steps for issue, external delivery, verification, rotation, and revocation are documented in `docs/smoke/node-bootstrap.md`;
- `make docker-handoff-smoke` verifies provider invite issue, consumer claim, access read, repeated claim rejection, and safe summary output;
- nodes management page with list, detail, bootstrap token creation, drain, undrain, disable, enable, read-only config revision metadata, and rollback revision creation flows.

Run from the repository root:

```sh
npm install
npm run panel-web:dev
```

Build:

```sh
npm run panel-web:build
```

Type-check:

```sh
npm run panel-web:lint
```

Focused session utility tests:

```sh
npm --workspace @lenker/panel-web run test:session
```

Focused users form tests:

```sh
npm --workspace @lenker/panel-web run test:users
```

Focused plans form tests:

```sh
npm --workspace @lenker/panel-web run test:plans
```

Focused subscriptions form tests:

```sh
npm --workspace @lenker/panel-web run test:subscriptions
```

Focused nodes form/action tests:

```sh
npm --workspace @lenker/panel-web run test:nodes
```

Planned `MVP v0.1` provider UI scope:

- real `panel-api` admin login;
- protected dashboard shell;
- users list/create/update/suspend/activate;
- plans list/create/update/archive;
- subscriptions list/create/renew;
- subscription access export inspection for the current provider-side MVP path;
- subscription access token issue, rotate, revoke, and out-of-band handoff controls for the current provider-side MVP path;
- short-lived client handoff invite issue/revoke controls for the future client bootstrap path;
- nodes list/detail/drain/undrain/disable/enable;
- node config revisions list/detail metadata view with applied-revision rollback action;
- loading, empty, unauthorized, and API error states.

Not planned for this first UI layer:

- marketplace UI;
- billing UI;
- config apply, Xray process control, or node-side rollback execution;
- marketing landing;
- advanced analytics;
- client app UI.

Nodes page note:

The nodes page uses the existing `panel-api` admin Bearer session flow. It can
create one-time plaintext bootstrap token responses and show them in memory, but
does not store bootstrap tokens in browser storage. It can show existing config
revision metadata and create the backend's dummy signed revision metadata for a
selected node. It can request a backend rollback revision from an applied
revision and refresh metadata after the action. The page does not execute config
apply, node file switching, rollback execution, or Xray runtime control itself.

## Provider API UI coverage

Covered in panel-web:

- admin login and session expiry handling;
- users list/create/update/suspend/activate;
- plans list/create/update/archive;
- subscriptions list/create/update/renew;
- provider-side subscription access export read view;
- subscription access token status, issue, rotate, and revoke controls;
- one-time handoff invite status, issue, and revoke controls;
- nodes list/detail/bootstrap-token creation/drain/undrain/disable/enable;
- node runtime readiness and recent runtime events read-only detail;
- node config revisions list/detail/create and applied-revision rollback request.

Provider APIs that remain API/docs-only for now:

- direct single-resource reads that duplicate list/detail data already loaded in
  the current views, such as `GET /users/{id}`, `GET /plans/{id}`, and
  `GET /subscriptions/{id}`;
- node-agent contract endpoints for registration, heartbeat, pending revision
  fetch, and revision report;
- consumer-facing endpoints for handoff claim and subscription access read;
- health and local smoke/debug paths.

This is intentional for the current provider UI: routine operator workflows are
available in the panel, while node-agent, consumer-only, smoke, and duplicate
read endpoints stay API-first until a larger product flow needs them.
