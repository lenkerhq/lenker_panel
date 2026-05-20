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
import type { StoredSession } from "../lib/session";

interface NodeProfilesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";
type FormMode = "create" | "edit";

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

export function NodeProfilesPage({ session, onUnauthorized }: NodeProfilesPageProps) {
  const [profiles, setProfiles] = useState<NodeProfile[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [formMode, setFormMode] = useState<FormMode>("create");
  const [editingProfile, setEditingProfile] = useState<NodeProfile | null>(null);
  const [formState, setFormState] = useState<ProfileFormState>(() => emptyProfileForm());
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
      setErrorMessage(formatPanelError(error, "Unable to load profiles."));
      setLoadState("failed");
    }
  }, [onUnauthorized, session]);

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
        setErrorMessage(formatPanelError(error, "Unable to load profiles."));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session]);

  function resetForm(message?: string) {
    setFormMode("create");
    setEditingProfile(null);
    setFormState(emptyProfileForm());
    setSuccessMessage(message ?? null);
  }

  function startEdit(profile: NodeProfile) {
    setFormMode("edit");
    setEditingProfile(profile);
    setFormState(profileToForm(profile));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function addRule() {
    setFormState((cur) => ({ ...cur, rules: [...cur.rules, emptyRule()] }));
  }

  function removeRule(index: number) {
    setFormState((cur) => ({ ...cur, rules: cur.rules.filter((_, i) => i !== index) }));
  }

  function updateRule(index: number, field: keyof RuleFormState, value: string) {
    setFormState((cur) => ({
      ...cur,
      rules: cur.rules.map((r, i) => (i === index ? { ...r, [field]: value } : r)),
    }));
  }

  async function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateProfileForm(formState);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);

    try {
      const config = { routing_rules: buildRules(formState.rules) };
      if (formMode === "edit" && editingProfile) {
        await updateNodeProfile(session, editingProfile.id, {
          name: formState.name.trim(),
          description: formState.description.trim() || undefined,
          config,
        });
        resetForm("Profile updated.");
      } else {
        await createNodeProfile(session, {
          name: formState.name.trim(),
          description: formState.description.trim() || undefined,
          config,
        });
        resetForm("Profile created.");
      }
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to save profile."));
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
      setSuccessMessage("Profile deleted.");
      if (editingProfile?.id === profile.id) resetForm("Profile deleted.");
      await loadData();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to delete profile."));
    } finally {
      setMutatingID(null);
    }
  }

  return (
    <div className="page-stack" id="node-profiles">
      <section className="page-header">
        <div>
          <p className="eyebrow">Node Profiles</p>
          <h2>Profiles</h2>
          <p>Create and manage node routing profiles. Apply profiles to nodes to configure routing rules.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{profiles.length} total</span>
          <span className="pill">{userProfiles} custom</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{formMode === "edit" ? "Edit profile" : "New profile"}</p>
              <h3>{formMode === "edit" ? editingProfile?.name : "Create profile"}</h3>
            </div>
            {formMode === "edit" ? (
              <button className="ghost-button" type="button" onClick={() => resetForm()} disabled={isMutating}>Cancel</button>
            ) : null}
          </div>

          <label className="field-label" htmlFor="profile-name">Name</label>
          <input id="profile-name" className="text-field" type="text" autoComplete="off" value={formState.name} onChange={(e) => setFormState((cur) => ({ ...cur, name: e.target.value }))} />

          <label className="field-label" htmlFor="profile-description">Description</label>
          <input id="profile-description" className="text-field" type="text" autoComplete="off" value={formState.description} onChange={(e) => setFormState((cur) => ({ ...cur, description: e.target.value }))} />

          <div className="section-heading compact-heading">
            <div><p className="eyebrow">Routing rules</p></div>
            <button className="table-button" type="button" onClick={addRule}>Add rule</button>
          </div>

          {formState.rules.map((rule, index) => (
            <div key={index} className="form-grid rule-row">
              <div>
                <label className="field-label">Type</label>
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
                <label className="field-label">Target</label>
                <input className="text-field" type="text" value={rule.target} onChange={(e) => updateRule(index, "target", e.target.value)} />
              </div>
              <div>
                <label className="field-label">Action</label>
                <select className="select-field" value={rule.action} onChange={(e) => updateRule(index, "action", e.target.value)}>
                  <option value="proxy">proxy</option>
                  <option value="direct">direct</option>
                  <option value="block">block</option>
                  <option value="warp">warp</option>
                </select>
              </div>
              <div>
                <label className="field-label">Priority</label>
                <input className="text-field" type="number" value={rule.priority} onChange={(e) => updateRule(index, "priority", e.target.value)} />
              </div>
              <div>
                <button className="table-button danger" type="button" onClick={() => removeRule(index)}>Remove</button>
              </div>
            </div>
          ))}

          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? "Saving..." : formMode === "edit" ? "Save changes" : "Create profile"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading profiles...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">Profiles list is ready.</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadData} disabled={loadState === "loading"}>Refresh</button>
        </div>
      </section>

      {loadState === "loaded" && profiles.length === 0 ? <p className="state-card">No profiles yet. Create the first profile above.</p> : null}

      {profiles.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Description</th>
                <th>Rules</th>
                <th>System</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {profiles.map((profile) => (
                <tr key={profile.id}>
                  <td>{profile.name}</td>
                  <td>{profile.description || "-"}</td>
                  <td>{profile.config.routing_rules?.length ?? 0}</td>
                  <td>{profile.is_system ? "yes" : "no"}</td>
                  <td className="mono-cell">{profile.id}</td>
                  <td>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => startEdit(profile)} disabled={isMutating || profile.is_system}>Edit</button>
                      <button className="table-button danger" type="button" onClick={() => deleteProfile(profile)} disabled={profile.is_system || mutatingID === profile.id}>
                        {mutatingID === profile.id ? "Deleting..." : "Delete"}
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

function handleUnauthorizedError(error: unknown, onUnauthorized: () => void): boolean {
  if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return true; }
  return false;
}

function formatPanelError(error: unknown, fallbackMessage: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallbackMessage;
}
