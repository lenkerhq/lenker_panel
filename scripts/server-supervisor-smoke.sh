#!/usr/bin/env bash
set -euo pipefail

# Server Supervisor Smoke Check
# Requires: SERVER_HOST, SERVER_USER, SERVER_KEY (SSH key path)
# Optional: PANEL_URL (default http://localhost:8080), ADMIN_TOKEN, NODE_ID

: "${SERVER_HOST:?SERVER_HOST is required}"
: "${SERVER_USER:?SERVER_USER is required}"
: "${SERVER_KEY:?SERVER_KEY is required}"
: "${PANEL_URL:=http://localhost:8080}"
: "${ADMIN_TOKEN:?ADMIN_TOKEN is required}"
: "${NODE_ID:?NODE_ID is required}"
: "${NODE_AGENT_PORT:=8090}"

SSH_CMD="ssh -i $SERVER_KEY -o StrictHostKeyChecking=no $SERVER_USER@$SERVER_HOST"

echo "=== Server Supervisor Smoke ==="

# a) Check node-agent runtime process mode
echo "Checking node-agent runtime process mode..."
AGENT_STATUS=$($SSH_CMD "curl -sf http://localhost:${NODE_AGENT_PORT}/status")
PROCESS_MODE=$(echo "$AGENT_STATUS" | jq -r '.runtime_process_mode // empty')
if [ "$PROCESS_MODE" != "local" ]; then
  echo "FAIL: runtime_process_mode=$PROCESS_MODE (expected local)"
  exit 1
fi
echo "  runtime_process_mode=local ✓"

# b) Check XRAY_BIN is executable
echo "Checking xray binary..."
$SSH_CMD "test -x \$(cat /proc/\$(pgrep -f node-agent | head -1)/environ 2>/dev/null | tr '\0' '\n' | grep LENKER_AGENT_XRAY_BIN | cut -d= -f2) 2>/dev/null || test -x /usr/local/bin/xray"
echo "  xray binary executable ✓"

# c) Create config revision
echo "Creating config revision..."
REVISION=$(curl -sf -X POST "${PANEL_URL}/api/v1/nodes/${NODE_ID}/config-revisions" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}")
REVISION_ID=$(echo "$REVISION" | jq -r '.data.id')
echo "  revision_id=$REVISION_ID"

# d) Wait for apply (runtime_process_state=running)
echo "Waiting for node-agent to apply and start xray..."
for i in $(seq 1 30); do
  STATUS=$($SSH_CMD "curl -sf http://localhost:${NODE_AGENT_PORT}/status" 2>/dev/null || true)
  PROC_STATE=$(echo "$STATUS" | jq -r '.runtime_process_state // empty' 2>/dev/null || true)
  XRAY_PID=$(echo "$STATUS" | jq -r '.xray_pid // 0' 2>/dev/null || true)
  if [ "$PROC_STATE" = "running" ] && [ "$XRAY_PID" != "0" ] && [ -n "$XRAY_PID" ]; then
    break
  fi
  sleep 2
done
if [ "$PROC_STATE" != "running" ]; then
  echo "FAIL: runtime_process_state=$PROC_STATE after 60s (expected running)"
  exit 1
fi
echo "  runtime_process_state=running, xray_pid=$XRAY_PID ✓"

# e) Check systemd xray unit is disabled/stopped
echo "Checking systemd xray unit..."
SYSTEMD_STATUS=$($SSH_CMD "systemctl is-active xray 2>/dev/null || echo inactive")
if [ "$SYSTEMD_STATUS" = "active" ]; then
  echo "WARN: systemd xray unit is still active — should be disabled"
fi
echo "  systemd_xray_status=$SYSTEMD_STATUS"

# f) Check xray is listening on 8443/tcp
echo "Checking xray listening on 8443/tcp..."
LISTEN_CHECK=$($SSH_CMD "ss -tlnp | grep -q ':8443 ' && echo ok || echo fail")
if [ "$LISTEN_CHECK" != "ok" ]; then
  echo "FAIL: xray not listening on 8443/tcp"
  exit 1
fi
echo "  xray listening on 8443/tcp ✓"

# g) Kill xray and verify restart
echo "Killing xray pid=$XRAY_PID..."
$SSH_CMD "kill $XRAY_PID"
sleep 5
NEW_STATUS=$($SSH_CMD "curl -sf http://localhost:${NODE_AGENT_PORT}/status")
NEW_PROC_STATE=$(echo "$NEW_STATUS" | jq -r '.runtime_process_state // empty')
NEW_PID=$(echo "$NEW_STATUS" | jq -r '.xray_pid // 0')
EVENTS=$(echo "$NEW_STATUS" | jq -r '[.runtime_events[]? | .type] | join(",")')

if [ "$NEW_PROC_STATE" != "running" ]; then
  echo "FAIL: after kill, runtime_process_state=$NEW_PROC_STATE (expected running)"
  exit 1
fi
if [ "$NEW_PID" = "$XRAY_PID" ] || [ "$NEW_PID" = "0" ]; then
  echo "FAIL: xray was not restarted (pid=$NEW_PID)"
  exit 1
fi
echo "  xray restarted: new_pid=$NEW_PID ✓"

# Summary
echo ""
echo "=== Summary ==="
echo "  supervisor_enabled: true"
echo "  xray_running: true (pid=$NEW_PID)"
echo "  systemd_xray_status: $SYSTEMD_STATUS"
echo "  restart_verified: true"
echo "  runtime_events: $EVENTS"
echo "=== PASS ==="
