import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  createGlobalRoutingRule,
  createNodeRoutingRule,
  deleteGlobalRoutingRule,
  deleteNodeRoutingRule,
  listGlobalRoutingRules,
  listNodeRoutingRules,
  listNodes,
  PanelApiError,
  reorderNodeRoutingRules,
  updateGlobalRoutingRule,
  updateNodeRoutingRule,
  type NodeSummary,
  type RoutingRule,
  type RoutingRuleAction,
  type RoutingRuleType,
} from "../lib/api";
import { Modal } from "../components/Modal";
import type { StoredSession } from "../lib/session";

interface RoutingRulesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type Tab = "global" | "node";

interface RuleFormState {
  ruleType: RoutingRuleType;
  target: string;
  action: RoutingRuleAction;
  outboundTag: string;
  priority: string;
  enabled: boolean;
  description: string;
}

function emptyRuleForm(): RuleFormState {
  return { ruleType: "geosite", target: "", action: "proxy", outboundTag: "", priority: "100", enabled: true, description: "" };
}

function ruleToForm(r: RoutingRule): RuleFormState {
  return {
    ruleType: r.rule_type,
    target: r.target,
    action: r.action,
    outboundTag: r.outbound_tag ?? "",
    priority: String(r.priority),
    enabled: r.enabled,
    description: r.description ?? "",
  };
}

function validateRuleForm(form: RuleFormState): string | null {
  if (!form.target.trim()) return "Target is required.";
  const p = parseInt(form.priority, 10);
  if (isNaN(p) || p < 0) return "Priority must be a non-negative integer.";
  return null;
}

interface RuleFormFieldsProps {
  form: RuleFormState;
  onChange: (next: RuleFormState) => void;
  idPrefix: string;
}

function RuleFormFields({ form, onChange, idPrefix }: RuleFormFieldsProps) {
  return (
    <>
      <div className="form-grid">
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-type`}>Type</label>
          <select id={`${idPrefix}-type`} className="select-field" value={form.ruleType} onChange={(e) => onChange({ ...form, ruleType: e.target.value as RoutingRuleType })}>
            <option value="geosite">geosite</option>
            <option value="geoip">geoip</option>
            <option value="domain">domain</option>
            <option value="ip">ip</option>
            <option value="port">port</option>
            <option value="protocol">protocol</option>
          </select>
        </div>
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-action`}>Action</label>
          <select id={`${idPrefix}-action`} className="select-field" value={form.action} onChange={(e) => onChange({ ...form, action: e.target.value as RoutingRuleAction })}>
            <option value="proxy">proxy</option>
            <option value="direct">direct</option>
            <option value="block">block</option>
            <option value="warp">warp</option>
          </select>
        </div>
      </div>

      <label className="field-label" htmlFor={`${idPrefix}-target`}>Target</label>
      <input id={`${idPrefix}-target`} className="text-field" type="text" autoComplete="off" value={form.target} onChange={(e) => onChange({ ...form, target: e.target.value })} />

      <div className="form-grid">
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-outbound`}>Outbound tag (optional)</label>
          <input id={`${idPrefix}-outbound`} className="text-field" type="text" autoComplete="off" value={form.outboundTag} onChange={(e) => onChange({ ...form, outboundTag: e.target.value })} />
        </div>
        <div>
          <label className="field-label" htmlFor={`${idPrefix}-priority`}>Priority</label>
          <input id={`${idPrefix}-priority`} className="text-field" type="number" min="0" value={form.priority} onChange={(e) => onChange({ ...form, priority: e.target.value })} />
        </div>
      </div>

      <label className="check-row" htmlFor={`${idPrefix}-enabled`}>
        <input id={`${idPrefix}-enabled`} type="checkbox" checked={form.enabled} onChange={(e) => onChange({ ...form, enabled: e.target.checked })} />
        <span>Enabled</span>
      </label>

      <label className="field-label" htmlFor={`${idPrefix}-description`}>Description (optional)</label>
      <input id={`${idPrefix}-description`} className="text-field" type="text" autoComplete="off" value={form.description} onChange={(e) => onChange({ ...form, description: e.target.value })} />
    </>
  );
}

export function RoutingRulesPage({ session, onUnauthorized }: RoutingRulesPageProps) {
  const [tab, setTab] = useState<Tab>("global");
  const [nodes, setNodes] = useState<NodeSummary[]>([]);
  const [selectedNodeID, setSelectedNodeID] = useState<string>("");
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [createForm, setCreateForm] = useState<RuleFormState>(() => emptyRuleForm());
  const [editingRule, setEditingRule] = useState<RoutingRule | null>(null);
  const [editForm, setEditForm] = useState<RuleFormState>(() => emptyRuleForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingID, setMutatingID] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    let m = true;
    listNodes(session).then((n) => { if (m) setNodes(n); }).catch((e) => { if (e instanceof PanelApiError && e.status === 401) onUnauthorized(); });
    return () => { m = false; };
  }, [session, onUnauthorized]);

  const loadRules = useCallback(async () => {
    setIsLoading(true);
    setErrorMessage(null);
    try {
      const loaded = tab === "global"
        ? await listGlobalRoutingRules(session)
        : selectedNodeID ? await listNodeRoutingRules(session, selectedNodeID) : [];
      setRules(loaded);
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to load rules."));
    } finally {
      setIsLoading(false);
    }
  }, [tab, selectedNodeID, session, onUnauthorized]);

  useEffect(() => { loadRules(); }, [loadRules]);

  function openEdit(rule: RoutingRule) {
    if (tab === "node" && rule.node_id === null) return; // read-only inherited
    setEditingRule(rule);
    setEditForm(ruleToForm(rule));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function closeEdit() {
    setEditingRule(null);
    setEditForm(emptyRuleForm());
  }

  function switchTab(newTab: Tab) {
    setTab(newTab);
    setCreateForm(emptyRuleForm());
    closeEdit();
    setRules([]);
  }

  function selectNode(nodeID: string) {
    setSelectedNodeID(nodeID);
    setCreateForm(emptyRuleForm());
    closeEdit();
  }

  function buildInput(form: RuleFormState) {
    return {
      rule_type: form.ruleType,
      target: form.target.trim(),
      action: form.action,
      outbound_tag: form.outboundTag.trim() || undefined,
      priority: parseInt(form.priority, 10),
      enabled: form.enabled,
      description: form.description.trim() || undefined,
    };
  }

  async function submitCreateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (tab === "node" && !selectedNodeID) { setErrorMessage("Select a node first."); return; }
    const err = validateRuleForm(createForm);
    if (err) { setErrorMessage(err); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      const input = buildInput(createForm);
      if (tab === "global") {
        await createGlobalRoutingRule(session, input);
      } else {
        await createNodeRoutingRule(session, selectedNodeID, input);
      }
      setCreateForm(emptyRuleForm());
      setSuccessMessage("Rule created.");
      await loadRules();
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to create rule."));
    } finally {
      setIsMutating(false);
    }
  }

  async function submitEditForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingRule) return;
    const err = validateRuleForm(editForm);
    if (err) { setErrorMessage(err); return; }

    setIsMutating(true);
    setErrorMessage(null);
    try {
      const input = buildInput(editForm);
      if (tab === "global") {
        await updateGlobalRoutingRule(session, editingRule.id, input);
      } else {
        await updateNodeRoutingRule(session, selectedNodeID, editingRule.id, input);
      }
      closeEdit();
      setSuccessMessage("Rule updated.");
      await loadRules();
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to update rule."));
    } finally {
      setIsMutating(false);
    }
  }

  async function deleteRule(rule: RoutingRule) {
    setMutatingID(rule.id);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      if (tab === "global") {
        await deleteGlobalRoutingRule(session, rule.id);
      } else {
        await deleteNodeRoutingRule(session, selectedNodeID, rule.id);
      }
      setSuccessMessage("Rule deleted.");
      if (editingRule?.id === rule.id) closeEdit();
      await loadRules();
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to delete rule."));
    } finally {
      setMutatingID(null);
    }
  }

  async function handleReorder() {
    if (tab !== "node" || !selectedNodeID || rules.length === 0) return;
    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      const entries = rules.filter((r) => r.node_id !== null).map((r, i) => ({ id: r.id, priority: (i + 1) * 10 }));
      if (entries.length > 0) {
        await reorderNodeRoutingRules(session, selectedNodeID, entries);
        setSuccessMessage("Rules reordered.");
        await loadRules();
      }
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to reorder rules."));
    } finally {
      setIsMutating(false);
    }
  }

  function moveRule(index: number, direction: -1 | 1) {
    const newIndex = index + direction;
    if (newIndex < 0 || newIndex >= rules.length) return;
    const newRules = [...rules];
    [newRules[index], newRules[newIndex]] = [newRules[newIndex], newRules[index]];
    setRules(newRules);
  }

  return (
    <div className="page-stack" id="routing-rules">
      <section className="page-header">
        <div>
          <p className="eyebrow">Routing</p>
          <h2>Routing Rules</h2>
          <p>Manage global and per-node routing rules for traffic control.</p>
        </div>
        <div className="header-actions">
          <button className={`pill ${tab === "global" ? "pill-active" : ""}`} type="button" onClick={() => switchTab("global")}>Global</button>
          <button className={`pill ${tab === "node" ? "pill-active" : ""}`} type="button" onClick={() => switchTab("node")}>Node</button>
        </div>
      </section>

      {tab === "node" ? (
        <section>
          <label className="field-label" htmlFor="routing-node-select">Node</label>
          <select id="routing-node-select" className="select-field" value={selectedNodeID} onChange={(e) => selectNode(e.target.value)}>
            <option value="">Select node</option>
            {nodes.map((n) => <option key={n.id} value={n.id}>{n.name || n.id}</option>)}
          </select>
        </section>
      ) : null}

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitCreateForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">New rule</p>
              <h3>Create rule</h3>
            </div>
          </div>
          <RuleFormFields form={createForm} onChange={setCreateForm} idPrefix="rule-create" />
          <button className="primary-button" type="submit" disabled={isMutating || (tab === "node" && !selectedNodeID)}>
            {isMutating ? "Saving..." : "Create rule"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {isLoading ? <p className="state-text">Loading rules...</p> : null}
          {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          {!isLoading && !errorMessage && !successMessage ? <p className="state-text">{rules.length} rules loaded.</p> : null}
          <div className="row-actions">
            <button className="secondary-button" type="button" onClick={loadRules} disabled={isLoading}>Refresh</button>
            {tab === "node" && selectedNodeID && rules.length > 0 ? (
              <button className="table-button" type="button" onClick={handleReorder} disabled={isMutating}>Save order</button>
            ) : null}
          </div>
        </div>
      </section>

      {rules.length === 0 && !isLoading ? <p className="state-card">No rules yet.</p> : null}

      {rules.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table management-table">
            <thead>
              <tr>
                {tab === "node" ? <th>Order</th> : null}
                <th>Type</th>
                <th>Target</th>
                <th>Action</th>
                <th>Outbound</th>
                <th>Priority</th>
                <th>Enabled</th>
                <th>Description</th>
                <th>Scope</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, index) => {
                const editable = !(tab === "node" && rule.node_id === null);
                return (
                  <tr key={rule.id} className={editable ? "clickable-row" : undefined} onClick={editable ? () => openEdit(rule) : undefined}>
                    {tab === "node" ? (
                      <td onClick={(e) => e.stopPropagation()}>
                        <div className="row-actions">
                          <button className="table-button" type="button" onClick={() => moveRule(index, -1)} disabled={index === 0 || rule.node_id === null}>↑</button>
                          <button className="table-button" type="button" onClick={() => moveRule(index, 1)} disabled={index === rules.length - 1 || rule.node_id === null}>↓</button>
                        </div>
                      </td>
                    ) : null}
                    <td>{rule.rule_type}</td>
                    <td>{rule.target}</td>
                    <td>{rule.action}</td>
                    <td>{rule.outbound_tag || "-"}</td>
                    <td>{rule.priority}</td>
                    <td>{rule.enabled ? "yes" : "no"}</td>
                    <td>{rule.description || "-"}</td>
                    <td>{rule.node_id ? "node" : "global"}</td>
                    <td onClick={(e) => e.stopPropagation()}>
                      <div className="row-actions">
                        <button className="table-button" type="button" onClick={() => openEdit(rule)} disabled={isMutating || !editable}>Edit</button>
                        <button className="table-button danger" type="button" onClick={() => deleteRule(rule)} disabled={mutatingID === rule.id || !editable}>
                          {mutatingID === rule.id ? "Deleting..." : "Delete"}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={editingRule !== null} onClose={closeEdit} title={editingRule ? `Edit rule: ${editingRule.target}` : ""} size="medium">
        {editingRule ? (
          <form onSubmit={submitEditForm}>
            <RuleFormFields form={editForm} onChange={setEditForm} idPrefix="rule-edit" />
            {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
            <div className="row-actions" style={{ marginTop: 22 }}>
              <button className="primary-button" type="submit" disabled={isMutating} style={{ width: "auto", marginTop: 0 }}>
                {isMutating ? "Saving..." : "Update"}
              </button>
              <button className="table-button danger" type="button" onClick={() => deleteRule(editingRule)} disabled={mutatingID === editingRule.id}>
                {mutatingID === editingRule.id ? "Deleting..." : "Delete"}
              </button>
              <button className="ghost-button" type="button" onClick={closeEdit} disabled={isMutating}>Cancel</button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}

function formatError(error: unknown, fallback: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallback;
}
