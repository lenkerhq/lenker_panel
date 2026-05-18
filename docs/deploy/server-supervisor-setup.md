# Server Supervisor Setup

This guide transitions a production node from standalone systemd-managed Xray to
node-agent-managed Xray via the runtime supervisor.

## Prerequisites

- Ubuntu server with node-agent deployed (container or systemd unit).
- Xray binary installed at `/usr/local/bin/xray` (executable).
- node-agent registered with panel-api (node ID and token configured).
- Panel-api reachable from the node.

## Step 1: Stop and disable systemd Xray unit

```sh
sudo systemctl stop xray
sudo systemctl disable xray
```

Verify:

```sh
systemctl is-active xray
# Expected: inactive
```

## Step 2: Update node-agent environment

Add or update these environment variables for node-agent:

```sh
LENKER_AGENT_RUNTIME_PROCESS_MODE=local
LENKER_AGENT_XRAY_BIN=/usr/local/bin/xray
```

If node-agent runs as a systemd unit, edit its environment file or override:

```sh
sudo systemctl edit node-agent
```

Add under `[Service]`:

```ini
Environment=LENKER_AGENT_RUNTIME_PROCESS_MODE=local
Environment=LENKER_AGENT_XRAY_BIN=/usr/local/bin/xray
```

## Step 3: Restart node-agent

```sh
sudo systemctl restart node-agent
```

## Step 4: Verify supervisor is running

```sh
curl -s http://localhost:8090/status | jq '{
  runtime_process_mode,
  runtime_process_state,
  xray_pid
}'
```

Expected after a config revision is applied:

```json
{
  "runtime_process_mode": "local",
  "runtime_process_state": "running",
  "xray_pid": 12345
}
```

If no config revision has been applied yet, `runtime_process_state` will be
`stopped` until the first revision is created and polled.

## Step 5: Run supervisor smoke check

From your local machine:

```sh
export SERVER_HOST=<server-ip>
export SERVER_USER=<ssh-user>
export SERVER_KEY=~/.ssh/id_rsa
export PANEL_URL=https://panel.example.com
export ADMIN_TOKEN=<admin-session-token>
export NODE_ID=<registered-node-id>

scripts/server-supervisor-smoke.sh
```

Expected output ends with `=== PASS ===`.

## Troubleshooting

**Supervisor did not start Xray:**

- Check node-agent logs: `journalctl -u node-agent -f`
- Verify xray binary is executable: `ls -la /usr/local/bin/xray`
- Verify a config revision has been applied: check `/status` for
  `active_revision > 0`
- Verify `active/config.json` exists under the state directory

**Xray starts but crashes immediately:**

- Check xray output in node-agent logs
- Validate config manually: `/usr/local/bin/xray run -test -config /var/lib/lenker/node-agent/active/config.json`
- `runtime_events` will show `process_crashed` with the error

**systemd xray unit is still active:**

- `sudo systemctl stop xray && sudo systemctl disable xray`
- Verify no port conflict on 8443/tcp before node-agent starts Xray
