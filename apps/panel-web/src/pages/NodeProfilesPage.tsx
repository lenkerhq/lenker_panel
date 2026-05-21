import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  createNodeProfile,
  deleteNodeProfile,
  listNodeProfiles,
  PanelApiError,
  updateNodeProfile,
  type NodeProfile,
  type NodeProfileRoutingRule,
} from "../lib/api";
import { Modal } from "../components/Modal";
import type { StoredSession } from "../lib/session";
import { useI18n } from "../lib/i18n";

interface NodeProfilesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";

interface RuleFormState {
  ruleType: string;
  target: string;
  action: string;
  outboundTag: string;
  priority: string;
}

interface ProfileFormState {
  name: string;
  description: string;
  rules: RuleFormState[];
}

function emptyRule(): RuleFormState {
  return { ruleType: "geosite", target: "", action: "proxy", outboundTag: "", priority: "0" };
}

function emptyProfileForm(): ProfileFormState {
  return { name: "", description: "", rules: [] };
}

function profileToForm(p: NodeProfile): ProfileFormState {
  return {
    name: p.name,
    description: p.description ?? "",
    rules: (p.config.routing_rules ?? []).map((r) => ({
      ruleType: r.rule_type,
      target: r.target,
      action: r.action,
      outboundTag: r.outbound_tag ?? "",
      priority: String(r.priority),
    })),
  };
}

function validateProfileForm(form: ProfileFormState): string | null {
  if (!form.name.trim()) return "Name is required.";
  for (let i = 0; i < form.rules.length; i++) {
    if (!form.rules[i].target.trim()) return `Rule #${i + 1}: target is required.`;
  }
  return null;
}

function buildRules(rules: RuleFormState[]): NodeProfileRoutingRule[] {
  return rules.map((r) => ({
    rule_type: r.ruleType as NodeProfileRoutingRule["rule_type"],
    target: r.target.trim(),
    action: r.action as NodeProfileRoutingRule["action"],
    outbound_tag: r.outboundTag.trim() || null,
    priority: parseInt(r.priority, 10) || 0,
  }));
}

interface ProfileFormFieldsProps {
  form: ProfileFormState;
  onChange: (next: ProfileFormState) => void;
  idPrefix: string;
  t: (key: any) => string;
}

function ProfileFormFields({ form, onChange, idPrefix, t }: ProfileFormFieldsProps) {
  function addRule() { onChange({ ...form, rules: [...form.rules, emptyRule()] }); }
  function removeRule(index: number) { onChange({ ...form, rules: form.rules.filter((_, i) => i !== index) }); }
  function updateRule(index: number, field: keyof RuleFormState, value: string) {
    onChange({ ...form, rules: form.rules.map((r, i) => (i === index ? { ...r, [field]: value } : r)) });
  }

  return (
    <>
      <label className="field-label" htmlFor={`${idPrefix}-name`}>{t("profiles.name")}</label>
      <input id={`${idPrefix}-name`} className="text-field" type="text" autoComplete="off" value={form.name} onChange={(e) => onChange({ ...form, name: e.target.value })} />

      <label className="field-label" htmlFor={`${idPrefix}-description`}>{t("profiles.description_label")}</label>
      <input id={`${idPrefix}-description`} className="text-field" type="text" autoComplete="off" value={form.description} onChange={(e) => onChange({ ...form, description: e.target.value })} />

      <div className="section-heading compact-heading">
        <div><p className="eyebrow">{t("profiles.routing_rules_eyebrow")}</p></div>
        <button className="table-button" type="button" onClick={addRule}>{t("profiles.add_rule")}</button>
      </div>

      {form.rules.map((rule, index) => (
        <div key={index} className="form-grid rule-row">
          <div>
            <label className="field-label">{t("profiles.type")}</label>
            <select className="select-field" value={rule.ruleType} onChange={(e) => updateRule(index, "ruleType", e.target.value)}>
              <option value="geosite">geosite</option>
              <option value="geoip">geoip</option>
              <option value="domain">domain</option>
              <option value="ip">ip</option>
              <option value="port">port</option>
              <option value="protocol">protocol</option>
            </select>
          </div>
          <div>
            <label className="field-label">{t("profiles.target")}</label>
            <input className="text-field" type="text" value={rule.target} onChange={(e) => updateRule(index, "target", e.target.value)} />
          </div>
          <div>
            <label className="field-label">{t("profiles.action")}</label>
            <select className="select-field" value={rule.action} onChange={(e) => updateRule(index, "action", e.target.value)}>
              <option value="proxy">proxy</option>
              <option value="direct">direct</option>
              <option value="block">block</option>
              <option value="warp">warp</option>
            </select>
          </div>
          <div>
            <label className="field-label">{t("profiles.priority")}</label>
            <input className="text-field" type="number" value={rule.priority} onChange={(e) => updateRule(index, "priority", e.target.value)} />
          </div>
          <div>
            <button className="table-button danger" type="button" onClick={() => removeRule(index)}>{t("profiles.remove")}</button>
          </div>
        </div>
      ))}
    </>
  );
}

export function NodeProfilesPage({ session, onUnauthorized }: NodeProfilesPageProps) {
  const { t } = useI18n();
  const [profiles, setProfiles] = useState<NodeProfile[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [createForm, setCreateForm] = useState<ProfileFormState>(() => emptyProfileForm());
  const [editingProfile, setEditingProfile] = useState<NodeProfile | null>(null);
  const [editForm, setEditForm] = useState<ProfileFormState>(() => emptyProfileForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingID, setMutatingID] = useState<string | null>(null);

  const userProfiles = useMemo(() => profiles.filter((p) => !p.is_system).length, [profiles]);

  const loadData = useCallback(async () => {
    setLoadState("loading");
    setErrorMessage(null);
    try {
      const loaded = await listNodeProfiles(session);
      setProfiles(loaded);
      setLoadState("loaded");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("profiles.unable_load")));
      setLoadState("failed");
    }
  }, [onUnauthorized, session, t]);

  useEffect(() => {
    let isMounted = true;
    async function load() {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const loaded = await listNodeProfiles(session);
        if (!isMounted) return;
        setProfiles(loaded);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) return;
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, t("profiles.unable_load")));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session, t]);

  function openEdit(profile: NodeProfile) {
    if (profile.is_system) return;
    setEditingProfile(profile);
    setEditForm(profileToForm(profile));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function closeEdit() {
    setEditingProfile(null);
    setEditForm(emptyProfileForm());
  }

  async function submitCreateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateProfileForm(createForm);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await createNodeProfile(session, {
        name: createForm.name.trim(),
        description: createForm.description.trim() || undefined,
        config: { routing_rules: buildRules(createForm.rules) },
      });
      setCreateForm(emptyProfileForm());
      setSuccessMessage(t("profiles.created"));
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("profiles.unable_create")));
    } finally {
      setIsMutating(false);
    }
  }

  async function submitEditForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingProfile) return;
    const validationError = validateProfileForm(editForm);
    if (validationError) { setErrorMessage(validationError); return; }

    setIsMutating(true);
    setErrorMessage(null);
    try {
      await updateNodeProfile(session, editingProfile.id, {
        name: editForm.name.trim(),
        description: editForm.description.trim() || undefined,
        config: { routing_rules: buildRules(editForm.rules) },
      });
      closeEdit();
      setSuccessMessage(t("profiles.updated"));
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("profiles.unable_update")));
    } finally {
      setIsMutating(false);
    }
  }

  async function deleteProfile(profile: NodeProfile) {
    setMutatingID(profile.id);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await deleteNodeProfile(session, profile.id);
      setSuccessMessage(t("profiles.deleted"));
      if (editingProfile?.id === profile.id) closeEdit();
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("profiles.unable_delete")));
    } finally {
      setMutatingID(null);
    }
  }

  return (
    <div className="page-stack" id="node-profiles">
      <section className="page-header">
        <div>
          <p className="eyebrow">{t("profiles.eyebrow")}</p>
          <h2>{t("profiles.title")}</h2>
          <p>{t("profiles.description")}</p>
        </div>
        <div className="header-actions">
          <span className="pill">{profiles.length} {t("common.total")}</span>
          <span className="pill">{userProfiles} {t("profiles.custom")}</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitCreateForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{t("profiles.new_eyebrow")}</p>
              <h3>{t("profiles.create_title")}</h3>
            </div>
          </div>
          <ProfileFormFields form={createForm} onChange={setCreateForm} idPrefix="profile-create" t={t} />
          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? t("common.saving") : t("profiles.create_button")}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">{t("common.state")}</p>
          {loadState === "loading" ? <p className="state-text">{t("profiles.loading")}</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">{t("profiles.list_ready")}</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadData} disabled={loadState === "loading"}>{t("common.refresh")}</button>
        </div>
      </section>

      {loadState === "loaded" && profiles.length === 0 ? <p className="state-card">{t("profiles.empty")}</p> : null}

      {profiles.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                <th>{t("profiles.th_name")}</th>
                <th>{t("profiles.th_description")}</th>
                <th>{t("profiles.th_rules")}</th>
                <th>{t("profiles.th_system")}</th>
                <th>{t("profiles.th_id")}</th>
                <th>{t("profiles.th_actions")}</th>
              </tr>
            </thead>
            <tbody>
              {profiles.map((profile) => (
                <tr key={profile.id} className={profile.is_system ? undefined : "clickable-row"} onClick={profile.is_system ? undefined : () => openEdit(profile)}>
                  <td>{profile.name}</td>
                  <td>{profile.description || "-"}</td>
                  <td>{profile.config.routing_rules?.length ?? 0}</td>
                  <td>{profile.is_system ? "yes" : "no"}</td>
                  <td className="mono-cell">{profile.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => openEdit(profile)} disabled={isMutating || profile.is_system}>{t("common.edit")}</button>
                      <button className="table-button danger" type="button" onClick={() => deleteProfile(profile)} disabled={profile.is_system || mutatingID === profile.id}>
                        {mutatingID === profile.id ? t("common.deleting") : t("common.delete")}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={editingProfile !== null} onClose={closeEdit} title={editingProfile ? `Edit ${editingProfile.name}` : ""} size="large">
        {editingProfile ? (
          <form onSubmit={submitEditForm}>
            <ProfileFormFields form={editForm} onChange={setEditForm} idPrefix="profile-edit" t={t} />
            {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
            <div className="row-actions" style={{ marginTop: 22 }}>
              <button className="primary-button" type="submit" disabled={isMutating} style={{ width: "auto", marginTop: 0 }}>
                {isMutating ? t("common.saving") : t("common.update")}
              </button>
              <button className="table-button danger" type="button" onClick={() => deleteProfile(editingProfile)} disabled={mutatingID === editingProfile.id}>
                {mutatingID === editingProfile.id ? t("common.deleting") : t("common.delete")}
              </button>
              <button className="ghost-button" type="button" onClick={closeEdit} disabled={isMutating}>{t("common.cancel")}</button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}

function handleUnauthorizedError(error: unknown, onUnauthorized: () => void): boolean {
  if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return true; }
  return false;
}

function formatPanelError(error: unknown, fallbackMessage: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallbackMessage;
}
