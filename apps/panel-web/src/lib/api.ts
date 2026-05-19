import type { StoredSession } from "./session";

const DEFAULT_API_BASE_URL = import.meta.env.DEV ? "http://localhost:8080" : "/panel-api";

interface LoginResponse {
  data: StoredSession;
}

interface UserListResponse {
  data?: User[] | null;
}

interface UserResponse {
  data: User;
}

interface PlanListResponse {
  data?: Plan[] | null;
}

interface PlanResponse {
  data: Plan;
}

interface SubscriptionListResponse {
  data?: Subscription[] | null;
}

interface SubscriptionResponse {
  data: Subscription;
}

interface SubscriptionAccessResponse {
  data: SubscriptionAccess;
}

interface SubscriptionAccessTokenResponse {
  data: SubscriptionAccessToken;
}

interface SubscriptionAccessTokenStatusResponse {
  data: SubscriptionAccessTokenStatus;
}

interface SubscriptionHandoffInviteResponse {
  data: SubscriptionHandoffInvite;
}

interface SubscriptionHandoffInviteStatusResponse {
  data: SubscriptionHandoffInviteStatus;
}

interface NodeListResponse {
  data?: NodeSummary[] | null;
}

interface NodeResponse {
  data: Node;
}

interface NodeBootstrapTokenResponse {
  data: NodeBootstrapToken;
}

interface ConfigRevisionListResponse {
  data?: ConfigRevision[] | null;
}

interface ConfigRevisionResponse {
  data: ConfigRevision;
}

interface ApiErrorResponse {
  error?: {
    code?: string;
    message?: string;
  };
}

export interface User {
  id: string;
  email: string;
  status: "active" | "suspended" | "expired";
  display_name: string;
}

export interface CreateUserInput {
  email: string;
  display_name?: string;
}

export interface UpdateUserInput {
  email?: string;
  display_name?: string;
}

export interface Plan {
  id: string;
  name: string;
  duration_days: number;
  traffic_limit_bytes: number | null;
  device_limit: number;
  status: "active" | "archived";
}

export interface CreatePlanInput {
  name: string;
  duration_days: number;
  traffic_limit_bytes?: number | null;
  device_limit: number;
}

export interface UpdatePlanInput {
  name?: string;
  duration_days?: number;
  traffic_limit_bytes?: number | null;
  clear_traffic_limit?: boolean;
  device_limit?: number;
}

export interface Subscription {
  id: string;
  user_id: string;
  plan_id: string;
  status: "active" | "expired" | "suspended";
  starts_at: string;
  expires_at: string;
  traffic_limit_bytes: number | null;
  traffic_used_bytes: number;
  device_limit: number;
  preferred_region: string | null;
}

export interface SubscriptionAccess {
  export_kind: string;
  subscription_id: string;
  user_id: string;
  user_label: string;
  plan_id: string;
  plan_name: string;
  status: string;
  protocol: string;
  protocol_path: string;
  node: {
    id: string;
    name: string;
    region: string;
    country_code: string;
    hostname: string;
    status: string;
    drain_state: string;
    active_revision: number;
  };
  endpoint: {
    address: string;
    port: number;
    network: string;
    security: string;
    sni: string;
    public_key: string;
    short_id: string;
    fingerprint: string;
    spider_x: string;
  };
  client: {
    id: string;
    email: string;
    flow: string;
    level: number;
    plan_id: string;
  };
  display_name: string;
  uri: string;
}

export interface SubscriptionAccessToken {
  subscription_id: string;
  access_token: string;
  expires_at: string;
  created_at: string;
}

export interface SubscriptionAccessTokenStatus {
  subscription_id: string;
  status: "never_issued" | "active" | "revoked";
  issued: boolean;
  issued_at?: string | null;
  revoked_at?: string | null;
  generation: number;
}

export interface SubscriptionHandoffInvite {
  subscription_id: string;
  handoff_token: string;
  expires_at: string;
  created_at: string;
}

export interface SubscriptionHandoffInviteStatus {
  subscription_id: string;
  status: "never_issued" | "active" | "claimed" | "revoked" | "expired";
  issued: boolean;
  issued_at?: string | null;
  expires_at?: string | null;
  claimed_at?: string | null;
  revoked_at?: string | null;
  generation: number;
}


export interface CreateSubscriptionInput {
  user_id: string;
  plan_id: string;
  preferred_region?: string | null;
}

export interface UpdateSubscriptionInput {
  status?: "active" | "expired" | "suspended";
  traffic_limit_bytes?: number | null;
  clear_traffic_limit?: boolean;
  device_limit?: number;
  preferred_region?: string | null;
  clear_preferred_region?: boolean;
}

export interface RenewSubscriptionInput {
  extend_days: number;
}

export type NodeStatus = "pending" | "active" | "unhealthy" | "drained" | "disabled";
export type NodeDrainState = "active" | "draining" | "drained";
export type ConfigRevisionStatus = "pending" | "applied" | "failed" | "rolled_back";

export interface NodeSummary {
  id: string;
  name: string;
  region: string;
  country_code?: string;
  hostname?: string;
  status: NodeStatus;
  drain_state: NodeDrainState;
  last_seen_at?: string | null;
  registered_at?: string | null;
  agent_version: string;
  active_revision_id: number;
}

export interface Node extends NodeSummary {
  country_code: string;
  hostname: string;
  xray_version: string;
  runtime_mode?: "no-process" | "dry-run-only" | "future-process-managed" | string;
  runtime_process_mode?: "disabled" | "local" | string;
  runtime_process_state?: "disabled" | "ready" | "failed" | string;
  runtime_desired_state?: string;
  runtime_state?: string;
  last_dry_run_status?: "not_configured" | "passed" | "failed" | string;
  last_runtime_attempt_status?: "skipped" | "ready" | "failed" | string;
  last_runtime_prepared_revision?: number;
  last_runtime_transition_at?: string | null;
  last_runtime_error?: string | null;
  last_validation_status?: "applied" | "failed" | "" | null;
  last_validation_error?: string | null;
  last_validation_at?: string | null;
  last_applied_revision?: number;
  active_config_path?: string;
  runtime_events?: RuntimeEvent[] | null;
  last_health_at?: string | null;
  updated_at: string;
}

export interface RuntimeEvent {
  type?: string | null;
  status?: string | null;
  revision_number?: number | null;
  message?: string | null;
  runtime_mode?: string | null;
  runtime_process_mode?: string | null;
  runtime_process_state?: string | null;
  at?: string | null;
}

export interface CreateNodeBootstrapTokenInput {
  name?: string;
  region?: string;
  country_code?: string;
  hostname?: string;
  expires_in_minutes?: number;
}

export interface NodeBootstrapToken {
  id: string;
  node_id: string;
  bootstrap_token: string;
  expires_at: string;
}

export interface ConfigRevision {
  id: string;
  node_id: string;
  revision_number: number;
  status: ConfigRevisionStatus;
  bundle_hash: string;
  signature: string;
  signer: string;
  rollback_target_revision: number;
  bundle?: unknown;
  created_at: string;
  applied_at?: string | null;
  failed_at?: string | null;
  rolled_back_at?: string | null;
  error_message?: string | null;
}

export class PanelApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "PanelApiError";
    this.code = code;
    this.status = status;
  }
}

export function getApiBaseUrl(): string {
  return import.meta.env.VITE_LENKER_PANEL_API_URL || DEFAULT_API_BASE_URL;
}

export async function loginAdmin(email: string, password: string): Promise<StoredSession> {
  const response = await fetch(`${getApiBaseUrl()}/api/v1/auth/admin/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email, password }),
  });

  const payload = (await response.json().catch(() => null)) as LoginResponse | ApiErrorResponse | null;

  if (!response.ok) {
    throwPanelApiError(response, payload, "Login failed");
  }

  const loginPayload = payload as LoginResponse | null;

  if (!loginPayload?.data?.admin || !loginPayload.data.session?.token) {
    throw new PanelApiError("Unexpected login response", "invalid_response", response.status);
  }

  return loginPayload.data;
}

export async function listUsers(session: StoredSession): Promise<User[]> {
  const payload = await authorizedRequest<UserListResponse>(session, "/api/v1/users");
  return readListData(payload, "users");
}

export async function createUser(session: StoredSession, input: CreateUserInput): Promise<User> {
  const payload = await authorizedRequest<UserResponse>(session, "/api/v1/users", {
    method: "POST",
    body: input,
  });
  return payload.data;
}

export async function updateUser(session: StoredSession, userID: string, input: UpdateUserInput): Promise<User> {
  const payload = await authorizedRequest<UserResponse>(session, `/api/v1/users/${encodeURIComponent(userID)}`, {
    method: "PATCH",
    body: input,
  });
  return payload.data;
}

export async function suspendUser(session: StoredSession, userID: string): Promise<User> {
  const payload = await authorizedRequest<UserResponse>(session, `/api/v1/users/${encodeURIComponent(userID)}/suspend`, {
    method: "POST",
  });
  return payload.data;
}

export async function activateUser(session: StoredSession, userID: string): Promise<User> {
  const payload = await authorizedRequest<UserResponse>(session, `/api/v1/users/${encodeURIComponent(userID)}/activate`, {
    method: "POST",
  });
  return payload.data;
}

export async function listPlans(session: StoredSession): Promise<Plan[]> {
  const payload = await authorizedRequest<PlanListResponse>(session, "/api/v1/plans");
  return readListData(payload, "plans");
}

export async function createPlan(session: StoredSession, input: CreatePlanInput): Promise<Plan> {
  const payload = await authorizedRequest<PlanResponse>(session, "/api/v1/plans", {
    method: "POST",
    body: input,
  });
  return payload.data;
}

export async function updatePlan(session: StoredSession, planID: string, input: UpdatePlanInput): Promise<Plan> {
  const payload = await authorizedRequest<PlanResponse>(session, `/api/v1/plans/${encodeURIComponent(planID)}`, {
    method: "PATCH",
    body: input,
  });
  return payload.data;
}

export async function archivePlan(session: StoredSession, planID: string): Promise<Plan> {
  const payload = await authorizedRequest<PlanResponse>(session, `/api/v1/plans/${encodeURIComponent(planID)}/archive`, {
    method: "POST",
  });
  return payload.data;
}

export async function listSubscriptions(session: StoredSession): Promise<Subscription[]> {
  const payload = await authorizedRequest<SubscriptionListResponse>(session, "/api/v1/subscriptions");
  return readListData(payload, "subscriptions");
}

export async function createSubscription(session: StoredSession, input: CreateSubscriptionInput): Promise<Subscription> {
  const payload = await authorizedRequest<SubscriptionResponse>(session, "/api/v1/subscriptions", {
    method: "POST",
    body: input,
  });
  return payload.data;
}

export async function updateSubscription(
  session: StoredSession,
  subscriptionID: string,
  input: UpdateSubscriptionInput,
): Promise<Subscription> {
  const payload = await authorizedRequest<SubscriptionResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}`,
    {
      method: "PATCH",
      body: input,
    },
  );
  return payload.data;
}

export async function renewSubscription(
  session: StoredSession,
  subscriptionID: string,
  input: RenewSubscriptionInput,
): Promise<Subscription> {
  const payload = await authorizedRequest<SubscriptionResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/renew`,
    {
      method: "POST",
      body: input,
    },
  );
  return payload.data;
}

export async function getSubscriptionAccess(session: StoredSession, subscriptionID: string): Promise<SubscriptionAccess> {
  const payload = await authorizedRequest<SubscriptionAccessResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/access`,
  );
  return payload.data;
}

export async function getSubscriptionAccessTokenStatus(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionAccessTokenStatus> {
  const payload = await authorizedRequest<SubscriptionAccessTokenStatusResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/access-token`,
  );
  return payload.data;
}

export async function createSubscriptionAccessToken(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionAccessToken> {
  const payload = await authorizedRequest<SubscriptionAccessTokenResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/access-token`,
    { method: "POST" },
  );
  return payload.data;
}

export async function rotateSubscriptionAccessToken(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionAccessToken> {
  const payload = await authorizedRequest<SubscriptionAccessTokenResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/access-token/rotate`,
    { method: "POST" },
  );
  return payload.data;
}

export async function revokeSubscriptionAccessToken(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionAccessTokenStatus> {
  const payload = await authorizedRequest<SubscriptionAccessTokenStatusResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/access-token`,
    { method: "DELETE" },
  );
  return payload.data;
}

export async function getSubscriptionHandoffInviteStatus(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionHandoffInviteStatus> {
  const payload = await authorizedRequest<SubscriptionHandoffInviteStatusResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/handoff-invite`,
  );
  return payload.data;
}

export async function createSubscriptionHandoffInvite(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionHandoffInvite> {
  const payload = await authorizedRequest<SubscriptionHandoffInviteResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/handoff-invite`,
    { method: "POST" },
  );
  return payload.data;
}

export async function revokeSubscriptionHandoffInvite(
  session: StoredSession,
  subscriptionID: string,
): Promise<SubscriptionHandoffInviteStatus> {
  const payload = await authorizedRequest<SubscriptionHandoffInviteStatusResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/handoff-invite`,
    { method: "DELETE" },
  );
  return payload.data;
}

export async function listNodes(session: StoredSession): Promise<NodeSummary[]> {
  const payload = await authorizedRequest<NodeListResponse>(session, "/api/v1/nodes");
  return readListData(payload, "nodes");
}

export async function getNode(session: StoredSession, nodeID: string): Promise<Node> {
  const payload = await authorizedRequest<NodeResponse>(session, `/api/v1/nodes/${encodeURIComponent(nodeID)}`);
  return payload.data;
}

export async function createNodeBootstrapToken(
  session: StoredSession,
  input: CreateNodeBootstrapTokenInput,
): Promise<NodeBootstrapToken> {
  const payload = await authorizedRequest<NodeBootstrapTokenResponse>(session, "/api/v1/nodes/bootstrap-token", {
    method: "POST",
    body: input,
  });
  return payload.data;
}

export async function drainNode(session: StoredSession, nodeID: string): Promise<Node> {
  return nodeLifecycleRequest(session, nodeID, "drain");
}

export async function undrainNode(session: StoredSession, nodeID: string): Promise<Node> {
  return nodeLifecycleRequest(session, nodeID, "undrain");
}

export async function disableNode(session: StoredSession, nodeID: string): Promise<Node> {
  return nodeLifecycleRequest(session, nodeID, "disable");
}

export async function enableNode(session: StoredSession, nodeID: string): Promise<Node> {
  return nodeLifecycleRequest(session, nodeID, "enable");
}

export async function listNodeConfigRevisions(session: StoredSession, nodeID: string): Promise<ConfigRevision[]> {
  const payload = await authorizedRequest<ConfigRevisionListResponse>(
    session,
    `/api/v1/nodes/${encodeURIComponent(nodeID)}/config-revisions`,
  );
  return readListData(payload, "config revisions");
}

export async function getNodeConfigRevision(session: StoredSession, nodeID: string, revisionID: string): Promise<ConfigRevision> {
  const payload = await authorizedRequest<ConfigRevisionResponse>(
    session,
    `/api/v1/nodes/${encodeURIComponent(nodeID)}/config-revisions/${encodeURIComponent(revisionID)}`,
  );
  return payload.data;
}

export async function createNodeConfigRevision(session: StoredSession, nodeID: string): Promise<ConfigRevision> {
  const payload = await authorizedRequest<ConfigRevisionResponse>(
    session,
    `/api/v1/nodes/${encodeURIComponent(nodeID)}/config-revisions`,
    { method: "POST" },
  );
  return payload.data;
}

export async function rollbackNodeConfigRevision(
  session: StoredSession,
  nodeID: string,
  revisionID: string,
): Promise<ConfigRevision> {
  const payload = await authorizedRequest<ConfigRevisionResponse>(
    session,
    `/api/v1/nodes/${encodeURIComponent(nodeID)}/config-revisions/${encodeURIComponent(revisionID)}/rollback`,
    { method: "POST" },
  );
  return payload.data;
}

interface AuthorizedRequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
}

async function authorizedRequest<TPayload>(
  session: StoredSession,
  path: string,
  options: AuthorizedRequestOptions = {},
): Promise<TPayload> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    method: options.method ?? "GET",
    headers: {
      Authorization: `Bearer ${session.session.token}`,
      ...(options.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });

  const payload = (await response.json().catch(() => null)) as TPayload | ApiErrorResponse | null;

  if (!response.ok) {
    throwPanelApiError(response, payload, "Request failed");
  }

  if (!payload) {
    throw new PanelApiError("Unexpected empty response", "invalid_response", response.status);
  }

  return payload as TPayload;
}

function throwPanelApiError(response: Response, payload: unknown, fallbackMessage: string): never {
  const errorPayload = payload as ApiErrorResponse | null;

  throw new PanelApiError(
    errorPayload?.error?.message || fallbackMessage,
    errorPayload?.error?.code || "request_failed",
    response.status,
  );
}

function readListData<TItem>(payload: { data?: TItem[] | null }, resourceName: string): TItem[] {
  if (payload.data === null) {
    return [];
  }

  if (Array.isArray(payload.data)) {
    return payload.data;
  }

  throw new PanelApiError(`Unexpected ${resourceName} response`, "invalid_response", 200);
}

async function nodeLifecycleRequest(session: StoredSession, nodeID: string, action: string): Promise<Node> {
  const payload = await authorizedRequest<NodeResponse>(session, `/api/v1/nodes/${encodeURIComponent(nodeID)}/${action}`, {
    method: "POST",
  });
  return payload.data;
}

// --- Devices ---

export interface Device {
  id: string;
  subscription_id: string;
  device_fingerprint: string;
  device_name: string | null;
  platform: string | null;
  app_version: string | null;
  first_seen_at: string;
  last_seen_at: string;
  last_ip: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

interface DeviceListResponse {
  data?: Device[] | null;
}

export async function listSubscriptionDevices(session: StoredSession, subscriptionID: string): Promise<Device[]> {
  const payload = await authorizedRequest<DeviceListResponse>(
    session,
    `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/devices`,
  );
  return readListData(payload, "devices");
}

export async function deleteDevice(session: StoredSession, deviceID: string): Promise<void> {
  await authorizedRequest(session, `/api/v1/devices/${encodeURIComponent(deviceID)}`, { method: "DELETE" });
}

export async function deactivateDevice(session: StoredSession, deviceID: string): Promise<void> {
  await authorizedRequest(session, `/api/v1/devices/${encodeURIComponent(deviceID)}/deactivate`, { method: "POST" });
}

// --- Traffic ---

export interface TrafficUsage {
  resource_type: string;
  resource_id: string;
  bytes_up: number;
  bytes_down: number;
  bytes_total: number;
  from: string | null;
  to: string | null;
}

export interface TrafficQuota {
  id: string;
  subscription_id: string;
  bytes_limit: number | null;
  bytes_used: number;
  bytes_remaining: number | null;
  exceeded: boolean;
  reset_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SetQuotaInput {
  bytes_limit?: number | null;
  bytes_used?: number;
  reset_at?: string | null;
}

interface TrafficUsageResponse {
  data: TrafficUsage;
}

interface TrafficQuotaResponse {
  data: TrafficQuota;
}

export async function getSubscriptionTraffic(session: StoredSession, subscriptionID: string, from?: string, to?: string): Promise<TrafficUsage> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const qs = params.toString() ? `?${params.toString()}` : "";
  const payload = await authorizedRequest<TrafficUsageResponse>(session, `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/traffic${qs}`);
  return payload.data;
}

export async function getSubscriptionQuota(session: StoredSession, subscriptionID: string): Promise<TrafficQuota> {
  const payload = await authorizedRequest<TrafficQuotaResponse>(session, `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/quota`);
  return payload.data;
}

export async function setSubscriptionQuota(session: StoredSession, subscriptionID: string, input: SetQuotaInput): Promise<TrafficQuota> {
  const payload = await authorizedRequest<TrafficQuotaResponse>(session, `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/quota`, { method: "POST", body: input });
  return payload.data;
}

export async function resetSubscriptionQuota(session: StoredSession, subscriptionID: string): Promise<TrafficQuota> {
  const payload = await authorizedRequest<TrafficQuotaResponse>(session, `/api/v1/subscriptions/${encodeURIComponent(subscriptionID)}/quota/reset`, { method: "POST" });
  return payload.data;
}

export async function getDeviceTraffic(session: StoredSession, deviceID: string, from?: string, to?: string): Promise<TrafficUsage> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const qs = params.toString() ? `?${params.toString()}` : "";
  const payload = await authorizedRequest<TrafficUsageResponse>(session, `/api/v1/devices/${encodeURIComponent(deviceID)}/traffic${qs}`);
  return payload.data;
}

export async function getNodeTraffic(session: StoredSession, nodeID: string, from?: string, to?: string): Promise<TrafficUsage> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const qs = params.toString() ? `?${params.toString()}` : "";
  const payload = await authorizedRequest<TrafficUsageResponse>(session, `/api/v1/nodes/${encodeURIComponent(nodeID)}/traffic${qs}`);
  return payload.data;
}
