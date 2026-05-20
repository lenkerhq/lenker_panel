import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { archivePlan, createPlan, listPlans, PanelApiError, updatePlan, type Plan } from "../lib/api";
import { Modal } from "../components/Modal";
import {
  buildCreatePlanInput,
  buildUpdatePlanInput,
  emptyPlanForm,
  planToForm,
  validatePlanForm,
  type PlanFormState,
} from "../lib/planForm";
import type { StoredSession } from "../lib/session";

interface PlansPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";

export function PlansPage({ session, onUnauthorized }: PlansPageProps) {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [createFormState, setCreateFormState] = useState<PlanFormState>(() => emptyPlanForm());
  const [editingPlan, setEditingPlan] = useState<Plan | null>(null);
  const [editFormState, setEditFormState] = useState<PlanFormState>(() => emptyPlanForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingPlanID, setMutatingPlanID] = useState<string | null>(null);

  const activePlans = useMemo(() => plans.filter((plan) => plan.status === "active").length, [plans]);

  const loadPlans = useCallback(async () => {
    setLoadState("loading");
    setErrorMessage(null);
    try {
      const loaded = await listPlans(session);
      setPlans(loaded);
      setLoadState("loaded");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to load plans."));
      setLoadState("failed");
    }
  }, [onUnauthorized, session]);

  useEffect(() => {
    let isMounted = true;
    async function load() {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const loaded = await listPlans(session);
        if (!isMounted) return;
        setPlans(loaded);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) return;
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, "Unable to load plans."));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session]);

  function updateCreateField(fieldName: keyof PlanFormState, value: string | boolean) {
    setCreateFormState((c) => ({ ...c, [fieldName]: value }));
  }

  function updateEditField(fieldName: keyof PlanFormState, value: string | boolean) {
    setEditFormState((c) => ({ ...c, [fieldName]: value }));
  }

  function openEdit(plan: Plan) {
    setEditingPlan(plan);
    setEditFormState(planToForm(plan));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function closeEdit() {
    setEditingPlan(null);
    setEditFormState(emptyPlanForm());
  }

  async function submitCreateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validatePlanForm(createFormState);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await createPlan(session, buildCreatePlanInput(createFormState));
      setCreateFormState(emptyPlanForm());
      setSuccessMessage("Plan created.");
      await loadPlans();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to create plan."));
    } finally {
      setIsMutating(false);
    }
  }

  async function submitEditForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingPlan) return;
    const validationError = validatePlanForm(editFormState);
    if (validationError) { setErrorMessage(validationError); return; }

    setIsMutating(true);
    setErrorMessage(null);
    try {
      await updatePlan(session, editingPlan.id, buildUpdatePlanInput(editFormState));
      closeEdit();
      setSuccessMessage("Plan updated.");
      await loadPlans();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to update plan."));
    } finally {
      setIsMutating(false);
    }
  }

  async function archiveSelectedPlan(plan: Plan) {
    setMutatingPlanID(plan.id);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await archivePlan(session, plan.id);
      setSuccessMessage("Plan archived.");
      if (editingPlan?.id === plan.id) closeEdit();
      await loadPlans();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to archive plan."));
    } finally {
      setMutatingPlanID(null);
    }
  }

  return (
    <div className="page-stack" id="plans">
      <section className="page-header">
        <div>
          <p className="eyebrow">Plans</p>
          <h2>Plans</h2>
          <p>Create, edit, and archive subscription plans through the panel-api admin API.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{plans.length} total</span>
          <span className="pill">{activePlans} active</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitCreateForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">New plan</p>
              <h3>Create plan</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="plan-name">Name</label>
          <input id="plan-name" className="text-field" type="text" autoComplete="off" value={createFormState.name} onChange={(e) => updateCreateField("name", e.target.value)} />

          <div className="form-grid">
            <div>
              <label className="field-label" htmlFor="plan-duration-days">Duration days</label>
              <input id="plan-duration-days" className="text-field" type="number" min="1" inputMode="numeric" value={createFormState.durationDays} onChange={(e) => updateCreateField("durationDays", e.target.value)} />
            </div>
            <div>
              <label className="field-label" htmlFor="plan-device-limit">Device limit</label>
              <input id="plan-device-limit" className="text-field" type="number" min="1" inputMode="numeric" value={createFormState.deviceLimit} onChange={(e) => updateCreateField("deviceLimit", e.target.value)} />
            </div>
          </div>

          <label className="check-row" htmlFor="plan-has-traffic-limit">
            <input id="plan-has-traffic-limit" type="checkbox" checked={createFormState.hasTrafficLimit} onChange={(e) => updateCreateField("hasTrafficLimit", e.target.checked)} />
            <span>Set traffic limit</span>
          </label>

          {createFormState.hasTrafficLimit ? (
            <>
              <label className="field-label" htmlFor="plan-traffic-limit">Traffic limit bytes</label>
              <input id="plan-traffic-limit" className="text-field" type="number" min="1" inputMode="numeric" value={createFormState.trafficLimitBytes} onChange={(e) => updateCreateField("trafficLimitBytes", e.target.value)} />
            </>
          ) : null}

          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? "Saving..." : "Create plan"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading plans...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">Plans list is ready.</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadPlans} disabled={loadState === "loading"}>Refresh</button>
        </div>
      </section>

      {loadState === "loaded" && plans.length === 0 ? <p className="state-card">No plans yet. Create the first plan above.</p> : null}

      {plans.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Duration</th>
                <th>Devices</th>
                <th>Traffic</th>
                <th>Status</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {plans.map((plan) => (
                <tr key={plan.id} className="clickable-row" onClick={() => openEdit(plan)}>
                  <td>{plan.name}</td>
                  <td>{plan.duration_days} days</td>
                  <td>{plan.device_limit}</td>
                  <td>{formatTrafficLimit(plan.traffic_limit_bytes)}</td>
                  <td>
                    <span className={`status-badge status-${plan.status}`}>{plan.status}</span>
                  </td>
                  <td className="mono-cell">{plan.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => openEdit(plan)} disabled={isMutating}>Edit</button>
                      <button className="table-button danger" type="button" onClick={() => archiveSelectedPlan(plan)} disabled={plan.status === "archived" || mutatingPlanID === plan.id}>
                        {mutatingPlanID === plan.id ? "Archiving..." : plan.status === "archived" ? "Archived" : "Archive"}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={editingPlan !== null} onClose={closeEdit} title={editingPlan ? `Edit ${editingPlan.name}` : ""} size="medium">
        {editingPlan ? (
          <form onSubmit={submitEditForm}>
            <label className="field-label" htmlFor="plan-edit-name">Name</label>
            <input id="plan-edit-name" className="text-field" type="text" autoComplete="off" value={editFormState.name} onChange={(e) => updateEditField("name", e.target.value)} />

            <div className="form-grid">
              <div>
                <label className="field-label" htmlFor="plan-edit-duration-days">Duration days</label>
                <input id="plan-edit-duration-days" className="text-field" type="number" min="1" inputMode="numeric" value={editFormState.durationDays} onChange={(e) => updateEditField("durationDays", e.target.value)} />
              </div>
              <div>
                <label className="field-label" htmlFor="plan-edit-device-limit">Device limit</label>
                <input id="plan-edit-device-limit" className="text-field" type="number" min="1" inputMode="numeric" value={editFormState.deviceLimit} onChange={(e) => updateEditField("deviceLimit", e.target.value)} />
              </div>
            </div>

            <label className="check-row" htmlFor="plan-edit-has-traffic-limit">
              <input id="plan-edit-has-traffic-limit" type="checkbox" checked={editFormState.hasTrafficLimit} onChange={(e) => updateEditField("hasTrafficLimit", e.target.checked)} />
              <span>Set traffic limit</span>
            </label>

            {editFormState.hasTrafficLimit ? (
              <>
                <label className="field-label" htmlFor="plan-edit-traffic-limit">Traffic limit bytes</label>
                <input id="plan-edit-traffic-limit" className="text-field" type="number" min="1" inputMode="numeric" value={editFormState.trafficLimitBytes} onChange={(e) => updateEditField("trafficLimitBytes", e.target.value)} />
              </>
            ) : null}

            <div className="check-row">
              <span>Status:</span>
              <span className={`status-badge status-${editingPlan.status}`}>{editingPlan.status}</span>
            </div>

            {errorMessage ? <p className="error-text">{errorMessage}</p> : null}

            <div className="row-actions" style={{ marginTop: 22 }}>
              <button className="primary-button" type="submit" disabled={isMutating} style={{ width: "auto", marginTop: 0 }}>
                {isMutating ? "Saving..." : "Update"}
              </button>
              <button className="table-button danger" type="button" onClick={() => archiveSelectedPlan(editingPlan)} disabled={editingPlan.status === "archived" || mutatingPlanID === editingPlan.id}>
                {mutatingPlanID === editingPlan.id ? "Archiving..." : editingPlan.status === "archived" ? "Archived" : "Archive"}
              </button>
              <button className="ghost-button" type="button" onClick={closeEdit} disabled={isMutating}>Cancel</button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}

function formatTrafficLimit(value: number | null): string {
  if (value === null) return "Unlimited";
  return new Intl.NumberFormat(undefined).format(value);
}

function handleUnauthorizedError(error: unknown, onUnauthorized: () => void): boolean {
  if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return true; }
  return false;
}

function formatPanelError(error: unknown, fallbackMessage: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallbackMessage;
}
