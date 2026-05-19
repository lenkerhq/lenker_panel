import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  createSubscriptionHandoffInvite,
  createSubscriptionAccessToken,
  createSubscription,
  deactivateDevice,
  deleteDevice,
  getDeviceTraffic,
  getSubscriptionAccess,
  getSubscriptionHandoffInviteStatus,
  getSubscriptionAccessTokenStatus,
  getSubscriptionQuota,
  getSubscriptionTraffic,
  listPlans,
  listSubscriptionDevices,
  listSubscriptions,
  listUsers,
  PanelApiError,
  renewSubscription,
  resetSubscriptionQuota,
  revokeSubscriptionHandoffInvite,
  revokeSubscriptionAccessToken,
  rotateSubscriptionAccessToken,
  setSubscriptionQuota,
  updateSubscription,
  type Device,
  type Plan,
  type Subscription,
  type SubscriptionAccess,
  type SubscriptionHandoffInvite,
  type SubscriptionHandoffInviteStatus,
  type SubscriptionAccessToken,
  type SubscriptionAccessTokenStatus,
  type TrafficQuota,
  type TrafficUsage,
  type User,
} from "../lib/api";
import type { StoredSession } from "../lib/session";
import {
  buildCreateSubscriptionInput,
  buildRenewSubscriptionInput,
  buildUpdateSubscriptionInput,
  emptySubscriptionForm,
  subscriptionToForm,
  validateCreateSubscriptionForm,
  validateRenewSubscriptionForm,
  validateUpdateSubscriptionForm,
  type SubscriptionFormState,
  type SubscriptionStatus,
} from "../lib/subscriptionForm";

interface SubscriptionsPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";
type FormMode = "create" | "edit";

export function SubscriptionsPage({ session, onUnauthorized }: SubscriptionsPageProps) {
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [formMode, setFormMode] = useState<FormMode>("create");
  const [editingSubscription, setEditingSubscription] = useState<Subscription | null>(null);
  const [formState, setFormState] = useState<SubscriptionFormState>(() => emptySubscriptionForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingSubscriptionID, setMutatingSubscriptionID] = useState<string | null>(null);
  const [accessSubscriptionID, setAccessSubscriptionID] = useState<string | null>(null);
  const [subscriptionAccess, setSubscriptionAccess] = useState<SubscriptionAccess | null>(null);
  const [accessTokenResult, setAccessTokenResult] = useState<SubscriptionAccessToken | null>(null);
  const [accessTokenStatus, setAccessTokenStatus] = useState<SubscriptionAccessTokenStatus | null>(null);
  const [handoffInviteResult, setHandoffInviteResult] = useState<SubscriptionHandoffInvite | null>(null);
  const [handoffInviteStatus, setHandoffInviteStatus] = useState<SubscriptionHandoffInviteStatus | null>(null);
  const [tokenAction, setTokenAction] = useState<"issue" | "rotate" | "revoke" | null>(null);
  const [handoffAction, setHandoffAction] = useState<"issue" | "revoke" | null>(null);

  const activeSubscriptions = useMemo(
    () => subscriptions.filter((subscription) => subscription.status === "active").length,
    [subscriptions],
  );
  const activePlans = useMemo(() => plans.filter((plan) => plan.status === "active"), [plans]);

  const loadPageData = useCallback(async () => {
    setLoadState("loading");
    setErrorMessage(null);

    try {
      const [loadedSubscriptions, loadedUsers, loadedPlans] = await Promise.all([
        listSubscriptions(session),
        listUsers(session),
        listPlans(session),
      ]);
      setSubscriptions(loadedSubscriptions);
      setUsers(loadedUsers);
      setPlans(loadedPlans);
      setLoadState("loaded");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to load subscriptions."));
      setLoadState("failed");
    }
  }, [onUnauthorized, session]);

  useEffect(() => {
    let isMounted = true;

    async function loadInitialPageData() {
      setLoadState("loading");
      setErrorMessage(null);

      try {
        const [loadedSubscriptions, loadedUsers, loadedPlans] = await Promise.all([
          listSubscriptions(session),
          listUsers(session),
          listPlans(session),
        ]);

        if (!isMounted) {
          return;
        }

        setSubscriptions(loadedSubscriptions);
        setUsers(loadedUsers);
        setPlans(loadedPlans);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) {
          return;
        }

        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }

        setErrorMessage(formatPanelError(error, "Unable to load subscriptions."));
        setLoadState("failed");
      }
    }

    loadInitialPageData();

    return () => {
      isMounted = false;
    };
  }, [onUnauthorized, session]);

  function updateFormField(fieldName: keyof SubscriptionFormState, value: string | boolean) {
    setFormState((currentValue) => ({ ...currentValue, [fieldName]: value }));
  }

  function resetForm(message?: string) {
    setFormMode("create");
    setEditingSubscription(null);
    setFormState(emptySubscriptionForm());
    setSuccessMessage(message ?? null);
  }

  async function loadSubscriptionAccess(subscription: Subscription) {
    setAccessSubscriptionID(subscription.id);
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const [access, tokenStatus, handoffStatus] = await Promise.all([
        getSubscriptionAccess(session, subscription.id),
        getSubscriptionAccessTokenStatus(session, subscription.id),
        getSubscriptionHandoffInviteStatus(session, subscription.id),
      ]);
      setSubscriptionAccess(access);
      setAccessTokenResult(null);
      setAccessTokenStatus(tokenStatus);
      setHandoffInviteResult(null);
      setHandoffInviteStatus(handoffStatus);
      setSuccessMessage("Subscription access export loaded.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setSubscriptionAccess(null);
      setErrorMessage(formatPanelError(error, "Unable to load subscription access."));
    } finally {
      setAccessSubscriptionID(null);
    }
  }

  async function issueAccessToken() {
    if (!subscriptionAccess) {
      return;
    }

    setTokenAction("issue");
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const token = await createSubscriptionAccessToken(session, subscriptionAccess.subscription_id);
      setAccessTokenResult(token);
      setAccessTokenStatus((currentStatus) => tokenStatusFromToken(token, currentStatus?.generation));
      setSuccessMessage("Subscription access token issued.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to issue subscription access token."));
    } finally {
      setTokenAction(null);
    }
  }

  async function rotateAccessToken() {
    if (!subscriptionAccess) {
      return;
    }

    setTokenAction("rotate");
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const token = await rotateSubscriptionAccessToken(session, subscriptionAccess.subscription_id);
      setAccessTokenResult(token);
      setAccessTokenStatus((currentStatus) => tokenStatusFromToken(token, currentStatus?.generation));
      setSuccessMessage("Subscription access token rotated.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to rotate subscription access token."));
    } finally {
      setTokenAction(null);
    }
  }

  async function revokeAccessToken() {
    if (!subscriptionAccess) {
      return;
    }

    setTokenAction("revoke");
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const tokenStatus = await revokeSubscriptionAccessToken(session, subscriptionAccess.subscription_id);
      setAccessTokenResult(null);
      setAccessTokenStatus(tokenStatus);
      setSuccessMessage("Subscription access token revoked.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to revoke subscription access token."));
    } finally {
      setTokenAction(null);
    }
  }

  async function issueHandoffInvite() {
    if (!subscriptionAccess) {
      return;
    }

    setHandoffAction("issue");
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const invite = await createSubscriptionHandoffInvite(session, subscriptionAccess.subscription_id);
      setHandoffInviteResult(invite);
      setHandoffInviteStatus((currentStatus) => handoffStatusFromInvite(invite, currentStatus?.generation));
      setSuccessMessage("Client handoff invite issued.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to issue client handoff invite."));
    } finally {
      setHandoffAction(null);
    }
  }

  async function revokeHandoffInvite() {
    if (!subscriptionAccess) {
      return;
    }

    setHandoffAction("revoke");
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const inviteStatus = await revokeSubscriptionHandoffInvite(session, subscriptionAccess.subscription_id);
      setHandoffInviteResult(null);
      setHandoffInviteStatus(inviteStatus);
      setSuccessMessage("Client handoff invite revoked.");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to revoke client handoff invite."));
    } finally {
      setHandoffAction(null);
    }
  }

  function startEdit(subscription: Subscription) {
    setFormMode("edit");
    setEditingSubscription(subscription);
    setFormState(subscriptionToForm(subscription));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  async function submitSubscriptionForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const validationError =
      formMode === "edit" ? validateUpdateSubscriptionForm(formState) : validateCreateSubscriptionForm(formState);
    if (validationError) {
      setErrorMessage(validationError);
      setSuccessMessage(null);
      return;
    }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      if (formMode === "edit" && editingSubscription) {
        await updateSubscription(session, editingSubscription.id, buildUpdateSubscriptionInput(formState));
        resetForm("Subscription updated.");
      } else {
        await createSubscription(session, buildCreateSubscriptionInput(formState));
        resetForm("Subscription created.");
      }
      await loadPageData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to save subscription."));
    } finally {
      setIsMutating(false);
    }
  }

  async function renewSelectedSubscription(subscription: Subscription) {
    const validationError = validateRenewSubscriptionForm(formState);
    if (validationError) {
      setErrorMessage(validationError);
      setSuccessMessage(null);
      return;
    }

    setMutatingSubscriptionID(subscription.id);
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      await renewSubscription(session, subscription.id, buildRenewSubscriptionInput(formState));
      setSuccessMessage("Subscription renewed.");
      await loadPageData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to renew subscription."));
    } finally {
      setMutatingSubscriptionID(null);
    }
  }

  return (
    <div className="page-stack" id="subscriptions">
      <section className="page-header">
        <div>
          <p className="eyebrow">Subscriptions</p>
          <h2>Subscriptions</h2>
          <p>Create, update, and renew subscriptions through the panel-api admin API.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{subscriptions.length} total</span>
          <span className="pill">{activeSubscriptions} active</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitSubscriptionForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{formMode === "edit" ? "Edit subscription" : "New subscription"}</p>
              <h3>{formMode === "edit" ? editingSubscription?.id : "Create subscription"}</h3>
            </div>
            {formMode === "edit" ? (
              <button className="ghost-button" type="button" onClick={() => resetForm()} disabled={isMutating}>
                Cancel
              </button>
            ) : null}
          </div>

          {formMode === "create" ? (
            <>
              <label className="field-label" htmlFor="subscription-user">
                User
              </label>
              <select
                id="subscription-user"
                className="select-field"
                value={formState.userID}
                onChange={(event) => updateFormField("userID", event.target.value)}
              >
                <option value="">Select user</option>
                {users.map((user) => (
                  <option key={user.id} value={user.id}>
                    {user.email}
                  </option>
                ))}
              </select>

              <label className="field-label" htmlFor="subscription-plan">
                Plan
              </label>
              <select
                id="subscription-plan"
                className="select-field"
                value={formState.planID}
                onChange={(event) => updateFormField("planID", event.target.value)}
              >
                <option value="">Select plan</option>
                {activePlans.map((plan) => (
                  <option key={plan.id} value={plan.id}>
                    {plan.name}
                  </option>
                ))}
              </select>
            </>
          ) : (
            <>
              <label className="field-label" htmlFor="subscription-status">
                Status
              </label>
              <select
                id="subscription-status"
                className="select-field"
                value={formState.status}
                onChange={(event) => updateFormField("status", event.target.value as SubscriptionStatus)}
              >
                <option value="active">active</option>
                <option value="expired">expired</option>
                <option value="suspended">suspended</option>
              </select>

              <label className="field-label" htmlFor="subscription-device-limit">
                Device limit
              </label>
              <input
                id="subscription-device-limit"
                className="text-field"
                type="number"
                min="1"
                inputMode="numeric"
                value={formState.deviceLimit}
                onChange={(event) => updateFormField("deviceLimit", event.target.value)}
              />

              <label className="check-row" htmlFor="subscription-has-traffic-limit">
                <input
                  id="subscription-has-traffic-limit"
                  type="checkbox"
                  checked={formState.hasTrafficLimit}
                  onChange={(event) => updateFormField("hasTrafficLimit", event.target.checked)}
                />
                <span>Set traffic limit</span>
              </label>

              {formState.hasTrafficLimit ? (
                <>
                  <label className="field-label" htmlFor="subscription-traffic-limit">
                    Traffic limit bytes
                  </label>
                  <input
                    id="subscription-traffic-limit"
                    className="text-field"
                    type="number"
                    min="1"
                    inputMode="numeric"
                    value={formState.trafficLimitBytes}
                    onChange={(event) => updateFormField("trafficLimitBytes", event.target.value)}
                  />
                </>
              ) : null}
            </>
          )}

          <label className="check-row" htmlFor="subscription-has-preferred-region">
            <input
              id="subscription-has-preferred-region"
              type="checkbox"
              checked={formState.hasPreferredRegion}
              onChange={(event) => updateFormField("hasPreferredRegion", event.target.checked)}
            />
            <span>Set preferred region</span>
          </label>

          {formState.hasPreferredRegion ? (
            <>
              <label className="field-label" htmlFor="subscription-preferred-region">
                Preferred region
              </label>
              <input
                id="subscription-preferred-region"
                className="text-field"
                type="text"
                autoComplete="off"
                value={formState.preferredRegion}
                onChange={(event) => updateFormField("preferredRegion", event.target.value)}
              />
            </>
          ) : null}

          <label className="field-label" htmlFor="subscription-renew-days">
            Renew days
          </label>
          <input
            id="subscription-renew-days"
            className="text-field"
            type="number"
            min="1"
            inputMode="numeric"
            value={formState.renewDays}
            onChange={(event) => updateFormField("renewDays", event.target.value)}
          />

          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? "Saving..." : formMode === "edit" ? "Save changes" : "Create subscription"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading subscriptions...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? (
            <p className="state-text">Subscriptions list is ready.</p>
          ) : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadPageData} disabled={loadState === "loading"}>
            Refresh
          </button>
        </div>
      </section>

      {loadState === "loaded" && subscriptions.length === 0 ? (
        <p className="state-card">No subscriptions yet. Create the first subscription above.</p>
      ) : null}

      {subscriptions.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table subscriptions-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Plan</th>
                <th>Status</th>
                <th>Expires</th>
                <th>Traffic</th>
                <th>Devices</th>
                <th>Region</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {subscriptions.map((subscription) => (
                <tr key={subscription.id}>
                  <td>{userLabel(users, subscription.user_id)}</td>
                  <td>{planLabel(plans, subscription.plan_id)}</td>
                  <td>
                    <span className={`status-badge status-${subscription.status}`}>{subscription.status}</span>
                  </td>
                  <td>{formatDate(subscription.expires_at)}</td>
                  <td>{formatTraffic(subscription.traffic_used_bytes, subscription.traffic_limit_bytes)}</td>
                  <td>{subscription.device_limit}</td>
                  <td>{subscription.preferred_region || "-"}</td>
                  <td className="mono-cell">{subscription.id}</td>
                  <td>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => startEdit(subscription)} disabled={isMutating}>
                        Edit
                      </button>
                      <button
                        className="table-button"
                        type="button"
                        onClick={() => renewSelectedSubscription(subscription)}
                        disabled={mutatingSubscriptionID === subscription.id}
                      >
                        {mutatingSubscriptionID === subscription.id ? "Renewing..." : `Renew ${formState.renewDays || "?"}d`}
                      </button>
                      <button
                        className="table-button"
                        type="button"
                        onClick={() => loadSubscriptionAccess(subscription)}
                        disabled={accessSubscriptionID === subscription.id}
                      >
                        {accessSubscriptionID === subscription.id ? "Loading..." : "Access"}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {subscriptionAccess ? (
        <section className="management-panel subscription-access-panel">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Access export</p>
              <h3>{subscriptionAccess.display_name}</h3>
            </div>
            <span className="status-badge status-active">{subscriptionAccess.protocol}</span>
          </div>
          <dl className="node-detail-grid">
            <div>
              <dt>subscription</dt>
              <dd>{subscriptionAccess.subscription_id}</dd>
            </div>
            <div>
              <dt>user</dt>
              <dd>{subscriptionAccess.user_label || subscriptionAccess.user_id}</dd>
            </div>
            <div>
              <dt>plan</dt>
              <dd>{subscriptionAccess.plan_name}</dd>
            </div>
            <div>
              <dt>node</dt>
              <dd>{subscriptionAccess.node.name || subscriptionAccess.node.id}</dd>
            </div>
            <div>
              <dt>address</dt>
              <dd>{subscriptionAccess.endpoint.address}</dd>
            </div>
            <div>
              <dt>port</dt>
              <dd>{subscriptionAccess.endpoint.port}</dd>
            </div>
            <div>
              <dt>security</dt>
              <dd>{subscriptionAccess.endpoint.security}</dd>
            </div>
            <div>
              <dt>flow</dt>
              <dd>{subscriptionAccess.client.flow}</dd>
            </div>
            <div>
              <dt>client id</dt>
              <dd>{subscriptionAccess.client.id}</dd>
            </div>
          </dl>
          <label className="field-label" htmlFor="subscription-access-uri">
            VLESS URI
          </label>
          <textarea id="subscription-access-uri" className="readonly-textarea" value={subscriptionAccess.uri} readOnly rows={3} />
          <div className="section-heading compact-heading">
            <div>
              <p className="eyebrow">Client token</p>
              <h4>Access token lifecycle</h4>
            </div>
            <div className="row-actions">
              <button className="table-button" type="button" onClick={issueAccessToken} disabled={tokenAction !== null}>
                {tokenAction === "issue" ? "Issuing..." : "Issue"}
              </button>
              <button className="table-button" type="button" onClick={rotateAccessToken} disabled={tokenAction !== null}>
                {tokenAction === "rotate" ? "Rotating..." : "Rotate"}
              </button>
              <button className="table-button danger" type="button" onClick={revokeAccessToken} disabled={tokenAction !== null}>
                {tokenAction === "revoke" ? "Revoking..." : "Revoke"}
              </button>
            </div>
          </div>
          {accessTokenStatus ? (
            <>
              <dl className="node-detail-grid">
                <div>
                  <dt>status</dt>
                  <dd>{formatAccessTokenStatus(accessTokenStatus)}</dd>
                </div>
                <div>
                  <dt>generation</dt>
                  <dd>{accessTokenStatus.generation || "-"}</dd>
                </div>
                <div>
                  <dt>issued</dt>
                  <dd>{accessTokenStatus.issued_at ? formatDate(accessTokenStatus.issued_at) : "never issued"}</dd>
                </div>
                <div>
                  <dt>revoked</dt>
                  <dd>{accessTokenStatus.revoked_at ? formatDate(accessTokenStatus.revoked_at) : "-"}</dd>
                </div>
              </dl>
              <p className="state-text">{accessTokenStatusHint(accessTokenStatus)}</p>
            </>
          ) : (
            <p className="state-text">Token lifecycle status is not loaded.</p>
          )}
          {accessTokenResult ? (
            <>
              <dl className="node-detail-grid">
                <div>
                  <dt>expires</dt>
                  <dd>{formatDate(accessTokenResult.expires_at)}</dd>
                </div>
                <div>
                  <dt>created</dt>
                  <dd>{formatDate(accessTokenResult.created_at)}</dd>
                </div>
              </dl>
              <label className="field-label" htmlFor="subscription-access-token">
                Plaintext access token
              </label>
              <textarea
                id="subscription-access-token"
                className="readonly-textarea"
                value={accessTokenResult.access_token}
                readOnly
                rows={3}
              />
            </>
          ) : (
            <p className="state-text">No plaintext access token is currently shown.</p>
          )}
          <div className="section-heading compact-heading">
            <div>
              <p className="eyebrow">Client handoff</p>
              <h4>Bootstrap invite</h4>
            </div>
            <div className="row-actions">
              <button className="table-button" type="button" onClick={issueHandoffInvite} disabled={handoffAction !== null}>
                {handoffAction === "issue" ? "Issuing..." : "Issue invite"}
              </button>
              <button className="table-button danger" type="button" onClick={revokeHandoffInvite} disabled={handoffAction !== null}>
                {handoffAction === "revoke" ? "Revoking..." : "Revoke invite"}
              </button>
            </div>
          </div>
          {handoffInviteStatus ? (
            <>
              <dl className="node-detail-grid">
                <div>
                  <dt>status</dt>
                  <dd>{formatHandoffInviteStatus(handoffInviteStatus)}</dd>
                </div>
                <div>
                  <dt>generation</dt>
                  <dd>{handoffInviteStatus.generation || "-"}</dd>
                </div>
                <div>
                  <dt>issued</dt>
                  <dd>{handoffInviteStatus.issued_at ? formatDate(handoffInviteStatus.issued_at) : "never issued"}</dd>
                </div>
                <div>
                  <dt>expires</dt>
                  <dd>{handoffInviteStatus.expires_at ? formatDate(handoffInviteStatus.expires_at) : "-"}</dd>
                </div>
                <div>
                  <dt>claimed</dt>
                  <dd>{handoffInviteStatus.claimed_at ? formatDate(handoffInviteStatus.claimed_at) : "-"}</dd>
                </div>
                <div>
                  <dt>revoked</dt>
                  <dd>{handoffInviteStatus.revoked_at ? formatDate(handoffInviteStatus.revoked_at) : "-"}</dd>
                </div>
              </dl>
              <p className="state-text">{handoffInviteStatusHint(handoffInviteStatus)}</p>
            </>
          ) : (
            <p className="state-text">Client handoff invite status is not loaded.</p>
          )}
          {handoffInviteResult ? (
            <>
              <label className="field-label" htmlFor="subscription-handoff-token">
                Plaintext handoff invite token
              </label>
              <textarea
                id="subscription-handoff-token"
                className="readonly-textarea"
                value={handoffInviteResult.handoff_token}
                readOnly
                rows={3}
              />
            </>
          ) : (
            <p className="state-text">No plaintext handoff invite token is currently shown.</p>
          )}
        </section>
      ) : null}

      {subscriptionAccess ? (
        <DevicesSection session={session} subscriptionID={subscriptionAccess.subscription_id} deviceLimit={editingSubscription?.device_limit ?? subscriptions.find(s => s.id === subscriptionAccess.subscription_id)?.device_limit ?? 0} onUnauthorized={onUnauthorized} />
      ) : null}

      {subscriptionAccess ? (
        <TrafficSection session={session} subscriptionID={subscriptionAccess.subscription_id} onUnauthorized={onUnauthorized} />
      ) : null}
    </div>
  );
}

function userLabel(users: User[], userID: string): string {
  return users.find((user) => user.id === userID)?.email ?? userID;
}

function planLabel(plans: Plan[], planID: string): string {
  return plans.find((plan) => plan.id === planID)?.name ?? planID;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatTraffic(usedBytes: number, limitBytes: number | null): string {
  const used = new Intl.NumberFormat(undefined).format(usedBytes);
  if (limitBytes === null) {
    return `${used} / unlimited`;
  }
  return `${used} / ${new Intl.NumberFormat(undefined).format(limitBytes)}`;
}

function formatAccessTokenStatus(status: SubscriptionAccessTokenStatus): string {
  if (!status.issued || status.status === "never_issued") {
    return "never issued";
  }
  return status.status;
}

function accessTokenStatusHint(status: SubscriptionAccessTokenStatus): string {
  if (!status.issued || status.status === "never_issued") {
    return "No client access token has been issued yet.";
  }
  if (status.status === "revoked") {
    return "The latest client access token is revoked; client reads will be rejected.";
  }
  return "The current client access token can read the redacted subscription access payload.";
}

function formatHandoffInviteStatus(status: SubscriptionHandoffInviteStatus): string {
  if (!status.issued || status.status === "never_issued") {
    return "never issued";
  }
  return status.status;
}

function handoffInviteStatusHint(status: SubscriptionHandoffInviteStatus): string {
  if (!status.issued || status.status === "never_issued") {
    return "No client handoff invite has been issued yet.";
  }
  if (status.status === "active") {
    return "The current invite can be claimed once to bootstrap a client access token.";
  }
  if (status.status === "claimed") {
    return "The latest invite has already been claimed and cannot be reused.";
  }
  if (status.status === "expired") {
    return "The latest invite expired before it was claimed.";
  }
  return "The latest invite is revoked; client bootstrap claims will be rejected.";
}

function tokenStatusFromToken(token: SubscriptionAccessToken, previousGeneration = 0): SubscriptionAccessTokenStatus {
  return {
    subscription_id: token.subscription_id,
    status: "active",
    issued: true,
    issued_at: token.created_at,
    revoked_at: null,
    generation: Math.max(previousGeneration + 1, 1),
  };
}

function handoffStatusFromInvite(
  invite: SubscriptionHandoffInvite,
  previousGeneration = 0,
): SubscriptionHandoffInviteStatus {
  return {
    subscription_id: invite.subscription_id,
    status: "active",
    issued: true,
    issued_at: invite.created_at,
    expires_at: invite.expires_at,
    claimed_at: null,
    revoked_at: null,
    generation: Math.max(previousGeneration + 1, 1),
  };
}

function handleUnauthorizedError(error: unknown, onUnauthorized: () => void): boolean {
  if (error instanceof PanelApiError && error.status === 401) {
    onUnauthorized();
    return true;
  }
  return false;
}

function formatPanelError(error: unknown, fallbackMessage: string): string {
  if (error instanceof PanelApiError) {
    return `${error.message} (${error.code})`;
  }
  return error instanceof Error ? error.message : fallbackMessage;
}

// --- Devices section ---

interface DevicesSectionProps {
  session: StoredSession;
  subscriptionID: string;
  deviceLimit: number;
  onUnauthorized: () => void;
}

function DevicesSection({ session, subscriptionID, deviceLimit, onUnauthorized }: DevicesSectionProps) {
  const [devices, setDevices] = useState<Device[]>([]);
  const [loadState, setLoadState] = useState<"idle" | "loading" | "loaded" | "failed">("idle");
  const [actionID, setActionID] = useState<string | null>(null);
  const [deviceTraffic, setDeviceTraffic] = useState<Record<string, TrafficUsage>>({});

  const loadDevices = useCallback(async () => {
    setLoadState("loading");
    try {
      const result = await listSubscriptionDevices(session, subscriptionID);
      setDevices(result);
      setLoadState("loaded");
      const trafficMap: Record<string, TrafficUsage> = {};
      const results = await Promise.allSettled(result.map((d) => getDeviceTraffic(session, d.id)));
      results.forEach((r, i) => {
        if (r.status === "fulfilled") trafficMap[result[i].id] = r.value;
      });
      setDeviceTraffic(trafficMap);
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
        return;
      }
      setLoadState("failed");
    }
  }, [session, subscriptionID, onUnauthorized]);

  useEffect(() => {
    loadDevices();
  }, [loadDevices]);

  const handleRevoke = async (deviceID: string) => {
    setActionID(deviceID);
    try {
      await deleteDevice(session, deviceID);
      setDevices((prev) => prev.filter((d) => d.id !== deviceID));
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
      }
    } finally {
      setActionID(null);
    }
  };

  const handleDeactivate = async (deviceID: string) => {
    setActionID(deviceID);
    try {
      await deactivateDevice(session, deviceID);
      setDevices((prev) => prev.map((d) => (d.id === deviceID ? { ...d, is_active: false } : d)));
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
      }
    } finally {
      setActionID(null);
    }
  };

  const activeCount = devices.filter((d) => d.is_active).length;

  return (
    <section className="management-panel">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">Devices</p>
          <h4>
            {activeCount}/{deviceLimit} devices used
          </h4>
        </div>
      </div>
      {loadState === "loading" ? <p className="state-text">Loading devices...</p> : null}
      {loadState === "failed" ? <p className="state-text">Failed to load devices.</p> : null}
      {loadState === "loaded" && devices.length === 0 ? <p className="state-text">No devices registered yet.</p> : null}
      {loadState === "loaded" && devices.length > 0 ? (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Platform</th>
                <th>Last seen</th>
                <th>IP</th>
                <th>Traffic</th>
                <th>Active</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((device) => (
                <tr key={device.id}>
                  <td>{device.device_name || device.device_fingerprint.slice(0, 12)}</td>
                  <td>{device.platform || "-"}</td>
                  <td>{formatDate(device.last_seen_at)}</td>
                  <td>{device.last_ip || "-"}</td>
                  <td>{deviceTraffic[device.id] ? `↑${formatBytes(deviceTraffic[device.id].bytes_up)} ↓${formatBytes(deviceTraffic[device.id].bytes_down)}` : "—"}</td>
                  <td>{device.is_active ? "yes" : "no"}</td>
                  <td>
                    <div className="row-actions">
                      {device.is_active ? (
                        <button
                          className="table-button"
                          type="button"
                          onClick={() => handleDeactivate(device.id)}
                          disabled={actionID === device.id}
                        >
                          Deactivate
                        </button>
                      ) : null}
                      <button
                        className="table-button danger"
                        type="button"
                        onClick={() => handleRevoke(device.id)}
                        disabled={actionID === device.id}
                      >
                        Revoke
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}

// --- Traffic section ---

interface TrafficSectionProps {
  session: StoredSession;
  subscriptionID: string;
  onUnauthorized: () => void;
}

function TrafficSection({ session, subscriptionID, onUnauthorized }: TrafficSectionProps) {
  const [usage, setUsage] = useState<TrafficUsage | null>(null);
  const [quota, setQuota] = useState<TrafficQuota | null>(null);
  const [loadState, setLoadState] = useState<"idle" | "loading" | "loaded" | "failed">("idle");
  const [isMutating, setIsMutating] = useState(false);
  const [quotaLimitInput, setQuotaLimitInput] = useState("");
  const [showQuotaForm, setShowQuotaForm] = useState(false);

  const loadTraffic = useCallback(async () => {
    setLoadState("loading");
    try {
      const [usageResult, quotaResult] = await Promise.all([
        getSubscriptionTraffic(session, subscriptionID),
        getSubscriptionQuota(session, subscriptionID).catch(() => null),
      ]);
      setUsage(usageResult);
      setQuota(quotaResult);
      setLoadState("loaded");
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
        return;
      }
      setLoadState("failed");
    }
  }, [session, subscriptionID, onUnauthorized]);

  useEffect(() => {
    loadTraffic();
  }, [loadTraffic]);

  const handleSetQuota = async (e: FormEvent) => {
    e.preventDefault();
    setIsMutating(true);
    try {
      const limitBytes = quotaLimitInput.trim() === "" ? null : parseInt(quotaLimitInput, 10) * 1024 * 1024 * 1024;
      const result = await setSubscriptionQuota(session, subscriptionID, { bytes_limit: limitBytes });
      setQuota(result);
      setShowQuotaForm(false);
      setQuotaLimitInput("");
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
      }
    } finally {
      setIsMutating(false);
    }
  };

  const handleResetQuota = async () => {
    setIsMutating(true);
    try {
      const result = await resetSubscriptionQuota(session, subscriptionID);
      setQuota(result);
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
      }
    } finally {
      setIsMutating(false);
    }
  };

  return (
    <section className="management-panel">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">Traffic</p>
          {usage ? (
            <h4>↑ {formatBytes(usage.bytes_up)} / ↓ {formatBytes(usage.bytes_down)} / Total {formatBytes(usage.bytes_total)}</h4>
          ) : (
            <h4>—</h4>
          )}
        </div>
        <div className="row-actions">
          <button className="table-button" type="button" onClick={() => setShowQuotaForm(!showQuotaForm)} disabled={isMutating}>
            Set Quota
          </button>
          <button className="table-button" type="button" onClick={handleResetQuota} disabled={isMutating}>
            Reset Usage
          </button>
        </div>
      </div>

      {loadState === "loading" ? <p className="state-text">Loading traffic...</p> : null}
      {loadState === "failed" ? <p className="state-text">Failed to load traffic data.</p> : null}

      {quota ? (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Used</th>
                <th>Limit</th>
                <th>Remaining</th>
                <th>Exceeded</th>
                <th>Reset at</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>{formatBytes(quota.bytes_used)}</td>
                <td>{quota.bytes_limit !== null ? formatBytes(quota.bytes_limit) : "unlimited"}</td>
                <td>{quota.bytes_remaining !== null ? formatBytes(quota.bytes_remaining) : "—"}</td>
                <td>{quota.exceeded ? "yes" : "no"}</td>
                <td>{quota.reset_at ? formatDate(quota.reset_at) : "—"}</td>
              </tr>
            </tbody>
          </table>
        </div>
      ) : null}

      {showQuotaForm ? (
        <form onSubmit={handleSetQuota} className="inline-form">
          <label className="field-label" htmlFor="quota-limit-gb">
            Limit (GB, empty = unlimited)
          </label>
          <input
            id="quota-limit-gb"
            type="number"
            min="0"
            step="1"
            value={quotaLimitInput}
            onChange={(e) => setQuotaLimitInput(e.target.value)}
            placeholder="e.g. 100"
          />
          <button className="table-button" type="submit" disabled={isMutating}>
            Save
          </button>
        </form>
      ) : null}
    </section>
  );
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}
