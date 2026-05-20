import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  createSubscriptionTemplate,
  deleteSubscriptionTemplate,
  listPlans,
  listSubscriptionTemplates,
  PanelApiError,
  updateSubscriptionTemplate,
  type Plan,
  type SubscriptionTemplate,
} from "../lib/api";
import type { StoredSession } from "../lib/session";

interface SubscriptionTemplatesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";
type FormMode = "create" | "edit";

interface TemplateFormState {
  name: string;
  description: string;
  planID: string;
  durationDays: string;
  deviceLimit: string;
  hasTrafficLimit: boolean;
  trafficLimitBytes: string;
}

function emptyTemplateForm(): TemplateFormState {
  return { name: "", description: "", planID: "", durationDays: "30", deviceLimit: "1", hasTrafficLimit: false, trafficLimitBytes: "" };
}

function templateToForm(t: SubscriptionTemplate): TemplateFormState {
  return {
    name: t.name,
    description: t.description ?? "",
    planID: t.plan_id ?? "",
    durationDays: String(t.config.duration_days),
    deviceLimit: String(t.config.device_limit),
    hasTrafficLimit: t.config.traffic_limit_bytes !== null,
    trafficLimitBytes: t.config.traffic_limit_bytes === null ? "" : String(t.config.traffic_limit_bytes),
  };
}

function validateTemplateForm(form: TemplateFormState): string | null {
  if (!form.name.trim()) return "Name is required.";
  if (!parsePositiveInteger(form.durationDays)) return "Duration days must be a positive integer.";
  if (!parsePositiveInteger(form.deviceLimit)) return "Device limit must be a positive integer.";
  if (form.hasTrafficLimit && !parsePositiveInteger(form.trafficLimitBytes)) return "Traffic limit must be a positive integer when enabled.";
  return null;
}

function parsePositiveInteger(value: string): number | null {
  const v = value.trim();
  if (!/^[1-9]\d*$/.test(v)) return null;
  const n = Number(v);
  return Number.isSafeInteger(n) ? n : null;
}

export function SubscriptionTemplatesPage({ session, onUnauthorized }: SubscriptionTemplatesPageProps) {
  const [templates, setTemplates] = useState<SubscriptionTemplate[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [formMode, setFormMode] = useState<FormMode>("create");
  const [editingTemplate, setEditingTemplate] = useState<SubscriptionTemplate | null>(null);
  const [formState, setFormState] = useState<TemplateFormState>(() => emptyTemplateForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingID, setMutatingID] = useState<string | null>(null);

  const userTemplates = useMemo(() => templates.filter((t) => !t.is_system).length, [templates]);

  const loadData = useCallback(async () => {
    setLoadState("loading");
    setErrorMessage(null);
    try {
      const [loadedTemplates, loadedPlans] = await Promise.all([listSubscriptionTemplates(session), listPlans(session)]);
      setTemplates(loadedTemplates);
      setPlans(loadedPlans);
      setLoadState("loaded");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to load templates."));
      setLoadState("failed");
    }
  }, [onUnauthorized, session]);

  useEffect(() => {
    let isMounted = true;
    async function load() {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const [loadedTemplates, loadedPlans] = await Promise.all([listSubscriptionTemplates(session), listPlans(session)]);
        if (!isMounted) return;
        setTemplates(loadedTemplates);
        setPlans(loadedPlans);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) return;
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, "Unable to load templates."));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session]);

  function updateFormField(fieldName: keyof TemplateFormState, value: string | boolean) {
    setFormState((cur) => ({ ...cur, [fieldName]: value }));
  }

  function resetForm(message?: string) {
    setFormMode("create");
    setEditingTemplate(null);
    setFormState(emptyTemplateForm());
    setSuccessMessage(message ?? null);
  }

  function startEdit(template: SubscriptionTemplate) {
    setFormMode("edit");
    setEditingTemplate(template);
    setFormState(templateToForm(template));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  async function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateTemplateForm(formState);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      if (formMode === "edit" && editingTemplate) {
        await updateSubscriptionTemplate(session, editingTemplate.id, {
          name: formState.name.trim(),
          description: formState.description.trim() || undefined,
          plan_id: formState.planID || undefined,
          config: {
            duration_days: parsePositiveInteger(formState.durationDays) ?? undefined,
            traffic_limit_bytes: formState.hasTrafficLimit ? (parsePositiveInteger(formState.trafficLimitBytes) ?? undefined) : null,
            device_limit: parsePositiveInteger(formState.deviceLimit) ?? undefined,
          },
        });
        resetForm("Template updated.");
      } else {
        await createSubscriptionTemplate(session, {
          name: formState.name.trim(),
          description: formState.description.trim() || undefined,
          plan_id: formState.planID || undefined,
          config: {
            duration_days: parsePositiveInteger(formState.durationDays) ?? 30,
            traffic_limit_bytes: formState.hasTrafficLimit ? (parsePositiveInteger(formState.trafficLimitBytes) ?? null) : null,
            device_limit: parsePositiveInteger(formState.deviceLimit) ?? 1,
          },
        });
        resetForm("Template created.");
      }
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to save template."));
    } finally {
      setIsMutating(false);
    }
  }

  async function deleteTemplate(template: SubscriptionTemplate) {
    setMutatingID(template.id);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await deleteSubscriptionTemplate(session, template.id);
      setSuccessMessage("Template deleted.");
      if (editingTemplate?.id === template.id) resetForm("Template deleted.");
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to delete template."));
    } finally {
      setMutatingID(null);
    }
  }

  const activePlans = plans.filter((p) => p.status === "active");

  return (
    <div className="page-stack" id="subscription-templates">
      <section className="page-header">
        <div>
          <p className="eyebrow">Subscription Templates</p>
          <h2>Templates</h2>
          <p>Create and manage subscription templates for quick subscription provisioning.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{templates.length} total</span>
          <span className="pill">{userTemplates} custom</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{formMode === "edit" ? "Edit template" : "New template"}</p>
              <h3>{formMode === "edit" ? editingTemplate?.name : "Create template"}</h3>
            </div>
            {formMode === "edit" ? (
              <button className="ghost-button" type="button" onClick={() => resetForm()} disabled={isMutating}>Cancel</button>
            ) : null}
          </div>

          <label className="field-label" htmlFor="template-name">Name</label>
          <input id="template-name" className="text-field" type="text" autoComplete="off" value={formState.name} onChange={(e) => updateFormField("name", e.target.value)} />

          <label className="field-label" htmlFor="template-description">Description</label>
          <input id="template-description" className="text-field" type="text" autoComplete="off" value={formState.description} onChange={(e) => updateFormField("description", e.target.value)} />

          <label className="field-label" htmlFor="template-plan">Plan (optional)</label>
          <select id="template-plan" className="select-field" value={formState.planID} onChange={(e) => updateFormField("planID", e.target.value)}>
            <option value="">No plan</option>
            {activePlans.map((plan) => (
              <option key={plan.id} value={plan.id}>{plan.name}</option>
            ))}
          </select>

          <div className="form-grid">
            <div>
              <label className="field-label" htmlFor="template-duration-days">Duration days</label>
              <input id="template-duration-days" className="text-field" type="number" min="1" inputMode="numeric" value={formState.durationDays} onChange={(e) => updateFormField("durationDays", e.target.value)} />
            </div>
            <div>
              <label className="field-label" htmlFor="template-device-limit">Device limit</label>
              <input id="template-device-limit" className="text-field" type="number" min="1" inputMode="numeric" value={formState.deviceLimit} onChange={(e) => updateFormField("deviceLimit", e.target.value)} />
            </div>
          </div>

          <label className="check-row" htmlFor="template-has-traffic-limit">
            <input id="template-has-traffic-limit" type="checkbox" checked={formState.hasTrafficLimit} onChange={(e) => updateFormField("hasTrafficLimit", e.target.checked)} />
            <span>Set traffic limit</span>
          </label>

          {formState.hasTrafficLimit ? (
            <>
              <label className="field-label" htmlFor="template-traffic-limit">Traffic limit bytes</label>
              <input id="template-traffic-limit" className="text-field" type="number" min="1" inputMode="numeric" value={formState.trafficLimitBytes} onChange={(e) => updateFormField("trafficLimitBytes", e.target.value)} />
            </>
          ) : null}

          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? "Saving..." : formMode === "edit" ? "Save changes" : "Create template"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading templates...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">Templates list is ready.</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadData} disabled={loadState === "loading"}>Refresh</button>
        </div>
      </section>

      {loadState === "loaded" && templates.length === 0 ? <p className="state-card">No templates yet. Create the first template above.</p> : null}

      {templates.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Duration</th>
                <th>Devices</th>
                <th>Traffic</th>
                <th>Plan</th>
                <th>System</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {templates.map((template) => (
                <tr key={template.id}>
                  <td>{template.name}</td>
                  <td>{template.config.duration_days} days</td>
                  <td>{template.config.device_limit}</td>
                  <td>{formatTrafficLimit(template.config.traffic_limit_bytes)}</td>
                  <td>{template.plan_id ? planLabel(plans, template.plan_id) : "-"}</td>
                  <td>{template.is_system ? "yes" : "no"}</td>
                  <td className="mono-cell">{template.id}</td>
                  <td>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => startEdit(template)} disabled={isMutating || template.is_system}>Edit</button>
                      <button className="table-button danger" type="button" onClick={() => deleteTemplate(template)} disabled={template.is_system || mutatingID === template.id}>
                        {mutatingID === template.id ? "Deleting..." : "Delete"}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}

function planLabel(plans: Plan[], planID: string): string {
  return plans.find((p) => p.id === planID)?.name ?? planID;
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
