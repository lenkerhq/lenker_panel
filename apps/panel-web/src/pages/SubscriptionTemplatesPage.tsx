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
import { Modal } from "../components/Modal";
import type { StoredSession } from "../lib/session";
import { useI18n } from "../lib/i18n";

interface SubscriptionTemplatesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";

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

interface TemplateFormFieldsProps {
  form: TemplateFormState;
  onChange: (field: keyof TemplateFormState, value: string | boolean) => void;
  plans: Plan[];
  idPrefix: string;
  t: (key: any) => string;
}

function TemplateFormFields({ form, onChange, plans, idPrefix, t }: TemplateFormFieldsProps) {
  const activePlans = plans.filter((p) => p.status === "active");
  return (
    <>
      <label className="field-label" htmlFor={`${idPrefix}-name`}>{t("templates.name")}</label>
      <input id={`${idPrefix}-name`} className="text-field" type="text" autoComplete="off" value={form.name} onChange={(e) => onChange("name", e.target.value)} />

      <label className="field-label" htmlFor={`${idPrefix}-description`}>{t("templates.description_label")}</label>
      <input id={`${idPrefix}-description`} className="text-field" type="text" autoComplete="off" value={form.description} onChange={(e) => onChange("description", e.target.value)} />

      <label className="field-label" htmlFor={`${idPrefix}-plan`}>{t("templates.plan_label")}</label>
      <select id={`${idPrefix}-plan`} className="select-field" value={form.planID} onChange={(e) => onChange("planID", e.target.value)}>
        <option value="">{t("templates.no_plan")}</option>
        {activePlans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}
      </select>

      <div className="form-grid">
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-duration-days`}>{t("templates.duration_days")}</label>
          <input id={`${idPrefix}-duration-days`} className="text-field" type="number" min="1" inputMode="numeric" value={form.durationDays} onChange={(e) => onChange("durationDays", e.target.value)} />
        </div>
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-device-limit`}>{t("templates.device_limit")}</label>
          <input id={`${idPrefix}-device-limit`} className="text-field" type="number" min="1" inputMode="numeric" value={form.deviceLimit} onChange={(e) => onChange("deviceLimit", e.target.value)} />
        </div>
      </div>

      <label className="check-row" htmlFor={`${idPrefix}-has-traffic-limit`}>
        <input id={`${idPrefix}-has-traffic-limit`} type="checkbox" checked={form.hasTrafficLimit} onChange={(e) => onChange("hasTrafficLimit", e.target.checked)} />
        <span>{t("templates.set_traffic_limit")}</span>
      </label>

      {form.hasTrafficLimit ? (
        <>
          <label className="field-label" htmlFor={`${idPrefix}-traffic-limit`}>{t("templates.traffic_limit_bytes")}</label>
          <input id={`${idPrefix}-traffic-limit`} className="text-field" type="number" min="1" inputMode="numeric" value={form.trafficLimitBytes} onChange={(e) => onChange("trafficLimitBytes", e.target.value)} />
        </>
      ) : null}
    </>
  );
}

export function SubscriptionTemplatesPage({ session, onUnauthorized }: SubscriptionTemplatesPageProps) {
  const { t } = useI18n();
  const [templates, setTemplates] = useState<SubscriptionTemplate[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [createForm, setCreateForm] = useState<TemplateFormState>(() => emptyTemplateForm());
  const [editingTemplate, setEditingTemplate] = useState<SubscriptionTemplate | null>(null);
  const [editForm, setEditForm] = useState<TemplateFormState>(() => emptyTemplateForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingID, setMutatingID] = useState<string | null>(null);

  const userTemplates = useMemo(() => templates.filter((tmpl) => !tmpl.is_system).length, [templates]);

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
      setErrorMessage(formatPanelError(error, t("templates.unable_load")));
      setLoadState("failed");
    }
  }, [onUnauthorized, session, t]);

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
        setErrorMessage(formatPanelError(error, t("templates.unable_load")));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session, t]);

  function updateCreateField(field: keyof TemplateFormState, value: string | boolean) {
    setCreateForm((c) => ({ ...c, [field]: value }));
  }

  function updateEditField(field: keyof TemplateFormState, value: string | boolean) {
    setEditForm((c) => ({ ...c, [field]: value }));
  }

  function openEdit(template: SubscriptionTemplate) {
    if (template.is_system) return;
    setEditingTemplate(template);
    setEditForm(templateToForm(template));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function closeEdit() {
    setEditingTemplate(null);
    setEditForm(emptyTemplateForm());
  }

  async function submitCreateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateTemplateForm(createForm);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await createSubscriptionTemplate(session, {
        name: createForm.name.trim(),
        description: createForm.description.trim() || undefined,
        plan_id: createForm.planID || undefined,
        config: {
          duration_days: parsePositiveInteger(createForm.durationDays) ?? 30,
          traffic_limit_bytes: createForm.hasTrafficLimit ? (parsePositiveInteger(createForm.trafficLimitBytes) ?? null) : null,
          device_limit: parsePositiveInteger(createForm.deviceLimit) ?? 1,
        },
      });
      setCreateForm(emptyTemplateForm());
      setSuccessMessage(t("templates.created"));
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("templates.unable_create")));
    } finally {
      setIsMutating(false);
    }
  }

  async function submitEditForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingTemplate) return;
    const validationError = validateTemplateForm(editForm);
    if (validationError) { setErrorMessage(validationError); return; }

    setIsMutating(true);
    setErrorMessage(null);
    try {
      await updateSubscriptionTemplate(session, editingTemplate.id, {
        name: editForm.name.trim(),
        description: editForm.description.trim() || undefined,
        plan_id: editForm.planID || undefined,
        config: {
          duration_days: parsePositiveInteger(editForm.durationDays) ?? undefined,
          traffic_limit_bytes: editForm.hasTrafficLimit ? (parsePositiveInteger(editForm.trafficLimitBytes) ?? undefined) : null,
          device_limit: parsePositiveInteger(editForm.deviceLimit) ?? undefined,
        },
      });
      closeEdit();
      setSuccessMessage(t("templates.updated"));
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("templates.unable_update")));
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
      setSuccessMessage(t("templates.deleted"));
      if (editingTemplate?.id === template.id) closeEdit();
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("templates.unable_delete")));
    } finally {
      setMutatingID(null);
    }
  }

  return (
    <div className="page-stack" id="subscription-templates">
      <section className="page-header">
        <div>
          <p className="eyebrow">{t("templates.eyebrow")}</p>
          <h2>{t("templates.title")}</h2>
          <p>{t("templates.description")}</p>
        </div>
        <div className="header-actions">
          <span className="pill">{templates.length} {t("common.total")}</span>
          <span className="pill">{userTemplates} {t("templates.custom")}</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitCreateForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{t("templates.new_eyebrow")}</p>
              <h3>{t("templates.create_title")}</h3>
            </div>
          </div>
          <TemplateFormFields form={createForm} onChange={updateCreateField} plans={plans} idPrefix="template-create" t={t} />
          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? t("common.saving") : t("templates.create_button")}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">{t("common.state")}</p>
          {loadState === "loading" ? <p className="state-text">{t("templates.loading")}</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">{t("templates.list_ready")}</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadData} disabled={loadState === "loading"}>{t("common.refresh")}</button>
        </div>
      </section>

      {loadState === "loaded" && templates.length === 0 ? <p className="state-card">{t("templates.empty")}</p> : null}

      {templates.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                <th>{t("templates.th_name")}</th>
                <th>{t("templates.th_duration")}</th>
                <th>{t("templates.th_devices")}</th>
                <th>{t("templates.th_traffic")}</th>
                <th>{t("templates.th_plan")}</th>
                <th>{t("templates.th_system")}</th>
                <th>{t("templates.th_id")}</th>
                <th>{t("templates.th_actions")}</th>
              </tr>
            </thead>
            <tbody>
              {templates.map((template) => (
                <tr key={template.id} className={template.is_system ? undefined : "clickable-row"} onClick={template.is_system ? undefined : () => openEdit(template)}>
                  <td>{template.name}</td>
                  <td>{template.config.duration_days} {t("templates.days")}</td>
                  <td>{template.config.device_limit}</td>
                  <td>{formatTrafficLimit(template.config.traffic_limit_bytes, t("templates.unlimited"))}</td>
                  <td>{template.plan_id ? planLabel(plans, template.plan_id) : "-"}</td>
                  <td>{template.is_system ? "yes" : "no"}</td>
                  <td className="mono-cell">{template.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => openEdit(template)} disabled={isMutating || template.is_system}>{t("common.edit")}</button>
                      <button className="table-button danger" type="button" onClick={() => deleteTemplate(template)} disabled={template.is_system || mutatingID === template.id}>
                        {mutatingID === template.id ? t("common.deleting") : t("common.delete")}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={editingTemplate !== null} onClose={closeEdit} title={editingTemplate ? `Edit ${editingTemplate.name}` : ""} size="medium">
        {editingTemplate ? (
          <form onSubmit={submitEditForm}>
            <TemplateFormFields form={editForm} onChange={updateEditField} plans={plans} idPrefix="template-edit" t={t} />
            {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
            <div className="row-actions" style={{ marginTop: 22 }}>
              <button className="primary-button" type="submit" disabled={isMutating} style={{ width: "auto", marginTop: 0 }}>
                {isMutating ? t("common.saving") : t("common.update")}
              </button>
              <button className="table-button danger" type="button" onClick={() => deleteTemplate(editingTemplate)} disabled={mutatingID === editingTemplate.id}>
                {mutatingID === editingTemplate.id ? t("common.deleting") : t("common.delete")}
              </button>
              <button className="ghost-button" type="button" onClick={closeEdit} disabled={isMutating}>{t("common.cancel")}</button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}

function planLabel(plans: Plan[], planID: string): string {
  return plans.find((p) => p.id === planID)?.name ?? planID;
}

function formatTrafficLimit(value: number | null, unlimitedLabel: string): string {
  if (value === null) return unlimitedLabel;
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
