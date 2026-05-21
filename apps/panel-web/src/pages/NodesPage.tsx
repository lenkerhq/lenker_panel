import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  applyNodeProfile,
  createNodeConfigRevision,
  createNodeBootstrapToken,
  disableNode,
  drainNode,
  enableNode,
  getNode,
  getNodeConfigRevision,
  getNodeTraffic,
  listNodeConfigRevisions,
  listNodeProfiles,
  listNodes,
  PanelApiError,
  rollbackNodeConfigRevision,
  undrainNode,
  type ConfigRevision,
  type Node,
  type NodeBootstrapToken,
  type NodeProfile,
  type NodeSummary,
  type TrafficUsage,
} from "../lib/api";
import { Modal } from "../components/Modal";
import {
  buildCreateNodeBootstrapTokenInput,
  canDisable,
  canDrain,
  canEnable,
  canUndrain,
  configRevisionStatusClass,
  emptyNodeBootstrapForm,
  formatConfigRevisionBundle,
  formatNodeTimestamp,
  formatRuntimeEventType,
  nodeDrainClass,
  nodeStatusClass,
  validateNodeBootstrapForm,
  type NodeBootstrapFormState,
} from "../lib/nodeForm";
import type { StoredSession } from "../lib/session";
import { useI18n } from "../lib/i18n";

interface NodesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";
type NodeAction = "drain" | "undrain" | "disable" | "enable";

export function NodesPage({ session, onUnauthorized }: NodesPageProps) {
  const { t } = useI18n();
  const [nodes, setNodes] = useState<NodeSummary[]>([]);
  const [selectedNodeID, setSelectedNodeID] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [detailLoadState, setDetailLoadState] = useState<LoadState>("idle");
  const [formState, setFormState] = useState<NodeBootstrapFormState>(() => emptyNodeBootstrapForm());
  const [bootstrapToken, setBootstrapToken] = useState<NodeBootstrapToken | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isCreatingToken, setIsCreatingToken] = useState(false);
  const [mutatingAction, setMutatingAction] = useState<NodeAction | null>(null);
  const [mutatingNodeID, setMutatingNodeID] = useState<string | null>(null);
  const [configRevisions, setConfigRevisions] = useState<ConfigRevision[]>([]);
  const [selectedRevisionID, setSelectedRevisionID] = useState<string | null>(null);
  const [selectedRevision, setSelectedRevision] = useState<ConfigRevision | null>(null);
  const [revisionsLoadState, setRevisionsLoadState] = useState<LoadState>("idle");
  const [revisionDetailLoadState, setRevisionDetailLoadState] = useState<LoadState>("idle");
  const [revisionErrorMessage, setRevisionErrorMessage] = useState<string | null>(null);
  const [isCreatingRevision, setIsCreatingRevision] = useState(false);
  const [rollbackRevisionID, setRollbackRevisionID] = useState<string | null>(null);
  const [profiles, setProfiles] = useState<NodeProfile[]>([]);
  const [profileByNode, setProfileByNode] = useState<Record<string, string>>({});

  const activeNodes = useMemo(() => nodes.filter((node) => node.status === "active").length, [nodes]);
  const drainingNodes = useMemo(() => nodes.filter((node) => node.drain_state === "draining").length, [nodes]);

  const loadRevisionDetail = useCallback(
    async (nodeID: string, revisionID: string) => {
      setRevisionDetailLoadState("loading");
      setRevisionErrorMessage(null);
      try {
        const revision = await getNodeConfigRevision(session, nodeID, revisionID);
        setSelectedRevision(revision);
        setSelectedRevisionID(revision.id);
        setRevisionDetailLoadState("loaded");
      } catch (error) {
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setSelectedRevision(null);
        setRevisionErrorMessage(formatPanelError(error, t("nodes.unable_load_revision_detail")));
        setRevisionDetailLoadState("failed");
      }
    },
    [onUnauthorized, session, t],
  );

  const loadConfigRevisions = useCallback(
    async (nodeID: string, preferredRevisionID?: string | null) => {
      setRevisionsLoadState("loading");
      setRevisionErrorMessage(null);
      try {
        const revisions = await listNodeConfigRevisions(session, nodeID);
        setConfigRevisions(revisions);
        setRevisionsLoadState("loaded");
        const nextRevisionID = preferredRevisionID ?? revisions[0]?.id ?? null;
        setSelectedRevisionID(nextRevisionID);
        if (nextRevisionID) {
          await loadRevisionDetail(nodeID, nextRevisionID);
        } else {
          setSelectedRevision(null);
          setRevisionDetailLoadState("idle");
        }
      } catch (error) {
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setConfigRevisions([]);
        setSelectedRevision(null);
        setSelectedRevisionID(null);
        setRevisionErrorMessage(formatPanelError(error, t("nodes.unable_load_revisions")));
        setRevisionsLoadState("failed");
      }
    },
    [loadRevisionDetail, onUnauthorized, session, t],
  );

  const loadNodeDetail = useCallback(
    async (nodeID: string) => {
      setDetailLoadState("loading");
      setErrorMessage(null);
      try {
        const loadedNode = await getNode(session, nodeID);
        setSelectedNode(loadedNode);
        setDetailLoadState("loaded");
        await loadConfigRevisions(loadedNode.id);
      } catch (error) {
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setSelectedNode(null);
        setConfigRevisions([]);
        setSelectedRevision(null);
        setSelectedRevisionID(null);
        setRevisionsLoadState("idle");
        setErrorMessage(formatPanelError(error, t("nodes.unable_detail")));
        setDetailLoadState("failed");
      }
    },
    [loadConfigRevisions, onUnauthorized, session, t],
  );

  const loadNodes = useCallback(
    async (preferredNodeID?: string | null) => {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const loadedNodes = await listNodes(session);
        setNodes(loadedNodes);
        setLoadState("loaded");
        const nextSelectedNodeID =
          preferredNodeID ??
          (selectedNodeID && loadedNodes.some((node) => node.id === selectedNodeID) ? selectedNodeID : loadedNodes[0]?.id ?? null);
        setSelectedNodeID(nextSelectedNodeID);
        if (nextSelectedNodeID) {
          await loadNodeDetail(nextSelectedNodeID);
        } else {
          setSelectedNode(null);
          setConfigRevisions([]);
          setSelectedRevision(null);
          setSelectedRevisionID(null);
          setDetailLoadState("idle");
          setRevisionsLoadState("idle");
        }
      } catch (error) {
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, t("nodes.unable_load")));
        setLoadState("failed");
      }
    },
    [loadNodeDetail, onUnauthorized, selectedNodeID, session, t],
  );

  useEffect(() => {
    let isMounted = true;
    async function loadInitialNodes() {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const [loadedNodes, loadedProfiles] = await Promise.all([listNodes(session), listNodeProfiles(session)]);
        if (!isMounted) return;
        setNodes(loadedNodes);
        setProfiles(loadedProfiles);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) return;
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, t("nodes.unable_load")));
        setLoadState("failed");
      }
    }
    loadInitialNodes();
    return () => { isMounted = false; };
  }, [onUnauthorized, session, t]);

  function updateFormField(fieldName: keyof NodeBootstrapFormState, value: string) {
    setFormState((currentValue) => ({ ...currentValue, [fieldName]: value }));
  }

  async function selectNode(nodeID: string) {
    setSelectedNodeID(nodeID);
    setSuccessMessage(null);
    setBootstrapToken(null);
    setSelectedRevision(null);
    setSelectedRevisionID(null);
    await loadNodeDetail(nodeID);
  }

  async function selectRevision(revisionID: string) {
    if (!selectedNodeID) return;
    setSuccessMessage(null);
    setSelectedRevisionID(revisionID);
    await loadRevisionDetail(selectedNodeID, revisionID);
  }

  async function submitBootstrapTokenForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateNodeBootstrapForm(formState);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }
    setIsCreatingToken(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);
    try {
      const token = await createNodeBootstrapToken(session, buildCreateNodeBootstrapTokenInput(formState));
      setBootstrapToken(token);
      setSuccessMessage(t("nodes.token_created"));
      setFormState(emptyNodeBootstrapForm());
      await loadNodes(token.node_id);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("nodes.unable_create_token")));
    } finally {
      setIsCreatingToken(false);
    }
  }

  async function runNodeAction(node: NodeSummary | Node, action: NodeAction) {
    setMutatingNodeID(node.id);
    setMutatingAction(action);
    setErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);
    try {
      if (action === "drain") { await drainNode(session, node.id); setSuccessMessage(t("nodes.drained")); }
      else if (action === "undrain") { await undrainNode(session, node.id); setSuccessMessage(t("nodes.undrained")); }
      else if (action === "disable") { await disableNode(session, node.id); setSuccessMessage(t("nodes.disabled")); }
      else { await enableNode(session, node.id); setSuccessMessage(t("nodes.enabled")); }
      await loadNodes(node.id);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("nodes.unable_lifecycle")));
    } finally {
      setMutatingNodeID(null);
      setMutatingAction(null);
    }
  }

  async function createDummyRevision() {
    if (!selectedNodeID) return;
    setIsCreatingRevision(true);
    setRevisionErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);
    try {
      const revision = await createNodeConfigRevision(session, selectedNodeID);
      setSuccessMessage(`Config revision #${revision.revision_number} created.`);
      await loadNodes(selectedNodeID);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setRevisionErrorMessage(formatPanelError(error, t("nodes.unable_create_revision")));
    } finally {
      setIsCreatingRevision(false);
    }
  }

  async function rollbackRevision(revision: ConfigRevision) {
    if (!selectedNodeID) return;
    setRollbackRevisionID(revision.id);
    setRevisionErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);
    try {
      const rollback = await rollbackNodeConfigRevision(session, selectedNodeID, revision.id);
      setSuccessMessage(`Rollback revision #${rollback.revision_number} created from #${revision.revision_number}.`);
      await loadNodes(selectedNodeID);
      await loadConfigRevisions(selectedNodeID, rollback.id);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setRevisionErrorMessage(formatPanelError(error, t("nodes.unable_rollback")));
    } finally {
      setRollbackRevisionID(null);
    }
  }

  async function applyProfileToNode(nodeID: string) {
    const profileID = profileByNode[nodeID];
    if (!profileID) return;
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await applyNodeProfile(session, profileID, nodeID);
      setSuccessMessage(t("nodes.profile_applied"));
      setProfileByNode((prev) => { const next = { ...prev }; delete next[nodeID]; return next; });
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, t("nodes.unable_apply_profile")));
    }
  }

  return (
    <div className="page-stack" id="nodes">
      <section className="page-header">
        <div>
          <p className="eyebrow">{t("nodes.eyebrow")}</p>
          <h2>{t("nodes.title")}</h2>
          <p>{t("nodes.description")}</p>
        </div>
        <div className="header-actions">
          <span className="pill">{nodes.length} {t("common.total")}</span>
          <span className="pill">{activeNodes} {t("common.active")}</span>
          <span className="pill">{drainingNodes} {t("nodes.draining")}</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitBootstrapTokenForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{t("nodes.bootstrap_eyebrow")}</p>
              <h3>{t("nodes.create_token_title")}</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="node-name">{t("nodes.name")}</label>
          <input id="node-name" className="text-field" type="text" autoComplete="off" value={formState.name} onChange={(event) => updateFormField("name", event.target.value)} />

          <div className="form-grid">
            <div>
              <label className="field-label" htmlFor="node-region">{t("nodes.region")}</label>
              <input id="node-region" className="text-field" type="text" autoComplete="off" value={formState.region} onChange={(event) => updateFormField("region", event.target.value)} />
            </div>
            <div>
              <label className="field-label" htmlFor="node-country-code">{t("nodes.country_code")}</label>
              <input id="node-country-code" className="text-field" type="text" autoComplete="off" value={formState.countryCode} onChange={(event) => updateFormField("countryCode", event.target.value)} />
            </div>
          </div>

          <label className="field-label" htmlFor="node-hostname">{t("nodes.hostname")}</label>
          <input id="node-hostname" className="text-field" type="text" autoComplete="off" value={formState.hostname} onChange={(event) => updateFormField("hostname", event.target.value)} />

          <label className="field-label" htmlFor="node-expires-in">{t("nodes.expires_in")}</label>
          <input id="node-expires-in" className="text-field" type="number" min="1" max="10080" inputMode="numeric" value={formState.expiresInMinutes} onChange={(event) => updateFormField("expiresInMinutes", event.target.value)} />

          <button className="primary-button" type="submit" disabled={isCreatingToken}>
            {isCreatingToken ? t("common.creating") : t("nodes.create_bootstrap_token")}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">{t("common.state")}</p>
          {loadState === "loading" ? <p className="state-text">{t("nodes.loading")}</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">{t("nodes.list_ready")}</p> : null}
          {detailLoadState === "loading" ? <p className="state-text">{t("nodes.loading_selected")}</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}

          {bootstrapToken ? (
            <dl className="token-result">
              <div><dt>{t("nodes.node_id_label")}</dt><dd className="mono-cell">{bootstrapToken.node_id}</dd></div>
              <div><dt>{t("nodes.bootstrap_token_label")}</dt><dd className="mono-cell token-value">{bootstrapToken.bootstrap_token}</dd></div>
              <div><dt>{t("nodes.expires_label")}</dt><dd>{formatNodeTimestamp(bootstrapToken.expires_at)}</dd></div>
            </dl>
          ) : null}

          <button className="secondary-button" type="button" onClick={() => loadNodes()} disabled={loadState === "loading"}>{t("common.refresh")}</button>
        </div>
      </section>

      {loadState === "loaded" && nodes.length === 0 ? <p className="state-card">{t("nodes.empty")}</p> : null}

      {nodes.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table nodes-table">
            <thead>
              <tr>
                <th>{t("nodes.th_name")}</th>
                <th>{t("nodes.th_region")}</th>
                <th>{t("nodes.th_country")}</th>
                <th>{t("nodes.th_hostname")}</th>
                <th>{t("nodes.th_status")}</th>
                <th>{t("nodes.th_drain")}</th>
                <th>{t("nodes.th_agent")}</th>
                <th>{t("nodes.th_revision")}</th>
                <th>{t("nodes.th_last_seen")}</th>
                <th>{t("nodes.th_registered")}</th>
                <th>{t("nodes.th_id")}</th>
                <th>{t("nodes.th_actions")}</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.id} className={`clickable-row${node.id === selectedNodeID ? " selected-row" : ""}`} onClick={() => selectNode(node.id)}>
                  <td>{node.name || "-"}</td>
                  <td>{node.region || "-"}</td>
                  <td>{node.country_code || (selectedNode?.id === node.id ? selectedNode.country_code : "") || "-"}</td>
                  <td>{node.hostname || (selectedNode?.id === node.id ? selectedNode.hostname : "") || "-"}</td>
                  <td><span className={`status-badge ${nodeStatusClass(node.status)}`}>{node.status}</span></td>
                  <td><span className={`status-badge ${nodeDrainClass(node.drain_state)}`}>{node.drain_state}</span></td>
                  <td>{node.agent_version || "-"}</td>
                  <td>{node.active_revision_id}</td>
                  <td>{formatNodeTimestamp(node.last_seen_at)}</td>
                  <td>{formatNodeTimestamp(node.registered_at)}</td>
                  <td className="mono-cell">{node.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => selectNode(node.id)} disabled={detailLoadState === "loading"}>{t("nodes.details")}</button>
                      {renderLifecycleButtons(node, runNodeAction, mutatingNodeID, mutatingAction, t)}
                      {profiles.length > 0 ? (
                        <>
                          <select className="select-field" value={profileByNode[node.id] ?? ""} onChange={(e) => setProfileByNode((prev) => ({ ...prev, [node.id]: e.target.value }))} aria-label="Select profile">
                            <option value="">{t("nodes.profile_select")}</option>
                            {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                          </select>
                          <button className="table-button" type="button" onClick={() => applyProfileToNode(node.id)} disabled={!profileByNode[node.id]}>{t("nodes.apply")}</button>
                        </>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={selectedNode !== null} onClose={() => { setSelectedNodeID(null); setSelectedNode(null); }} title={selectedNode ? (selectedNode.name || selectedNode.id) : ""} size="large">
        {selectedNode ? (
          <>
            <section className="surface-card">
              <div className="section-heading">
                <div>
                  <p className="eyebrow">{t("nodes.detail_eyebrow")}</p>
                  <h3>{selectedNode.name || selectedNode.id}</h3>
                </div>
                <div className="row-actions">
                  {renderLifecycleButtons(selectedNode, runNodeAction, mutatingNodeID, mutatingAction, t)}
                </div>
              </div>

              <dl className="node-detail-grid">
                <DetailItem label="ID" value={selectedNode.id} mono />
                <DetailItem label={t("nodes.name")} value={selectedNode.name} />
                <DetailItem label={t("nodes.region")} value={selectedNode.region} />
                <DetailItem label={t("nodes.th_country")} value={selectedNode.country_code} />
                <DetailItem label={t("nodes.hostname")} value={selectedNode.hostname} />
                <DetailItem label={t("nodes.th_status")} value={selectedNode.status} />
                <DetailItem label={t("nodes.th_drain")} value={selectedNode.drain_state} />
                <DetailItem label="Agent version" value={selectedNode.agent_version} />
                <DetailItem label="Xray version" value={selectedNode.xray_version} />
                <DetailItem label="Active revision" value={String(selectedNode.active_revision_id)} />
                <DetailItem label="Last health" value={formatNodeTimestamp(selectedNode.last_health_at)} />
                <DetailItem label={t("nodes.th_last_seen")} value={formatNodeTimestamp(selectedNode.last_seen_at)} />
                <DetailItem label={t("nodes.th_registered")} value={formatNodeTimestamp(selectedNode.registered_at)} />
                <DetailItem label="Updated" value={formatNodeTimestamp(selectedNode.updated_at)} />
              </dl>

              <section className="runtime-status-panel" aria-label="Node runtime validation status">
                <div className="section-heading compact-heading">
                  <div>
                    <p className="eyebrow">{t("nodes.runtime_eyebrow")}</p>
                    <h4>{t("nodes.runtime_title")}</h4>
                  </div>
                  <span className={`status-badge ${runtimeValidationStatusClass(selectedNode.last_validation_status)}`}>
                    {selectedNode.last_validation_status || "not validated"}
                  </span>
                </div>

                <dl className="runtime-status-grid">
                  <DetailItem label="Runtime mode" value={selectedNode.runtime_mode || "no-process"} />
                  <DetailItem label="Process mode" value={selectedNode.runtime_process_mode || "disabled"} />
                  <DetailItem label="Process state" value={selectedNode.runtime_process_state || "disabled"} />
                  <DetailItem label="Runtime desired state" value={selectedNode.runtime_desired_state || "validated-config-ready"} />
                  <DetailItem label="Runtime state" value={selectedNode.runtime_state || "not prepared"} />
                  <DetailItem label="Dry-run status" value={selectedNode.last_dry_run_status || "not configured"} />
                  <DetailItem label="Runtime attempt" value={selectedNode.last_runtime_attempt_status || "skipped"} />
                  <DetailItem label="Runtime prepared revision" value={formatRevisionNumber(selectedNode.last_runtime_prepared_revision)} />
                  <DetailItem label="Runtime transition at" value={formatNodeTimestamp(selectedNode.last_runtime_transition_at)} />
                  <DetailItem label="Runtime error" value={selectedNode.last_runtime_error} mono />
                  <DetailItem label="Last validation status" value={selectedNode.last_validation_status || "not yet validated"} />
                  <DetailItem label="Last validation error" value={selectedNode.last_validation_error} mono />
                  <DetailItem label="Last validation at" value={formatNodeTimestamp(selectedNode.last_validation_at)} />
                  <DetailItem label="Last applied revision" value={formatRevisionNumber(selectedNode.last_applied_revision)} />
                  <DetailItem label="Active config path" value={selectedNode.active_config_path} mono />
                </dl>

                <RuntimeEventsBlock events={selectedNode.runtime_events ?? []} t={t} />
              </section>
            </section>

            <NodeTrafficSection session={session} nodeID={selectedNode.id} onUnauthorized={onUnauthorized} t={t} />

            <section className="surface-card">
              <div className="section-heading">
                <div>
                  <p className="eyebrow">{t("nodes.revisions_eyebrow")}</p>
                  <h3>{t("nodes.revisions_title")}</h3>
                </div>
                <div className="row-actions">
                  <button className="secondary-button" type="button" onClick={() => loadConfigRevisions(selectedNode.id)} disabled={revisionsLoadState === "loading"}>
                    {revisionsLoadState === "loading" ? t("nodes.revisions_refreshing") : t("common.refresh")}
                  </button>
                  <button className="table-button" type="button" onClick={createDummyRevision} disabled={isCreatingRevision}>
                    {isCreatingRevision ? t("common.creating") : t("nodes.create_revision")}
                  </button>
                </div>
              </div>

              {revisionsLoadState === "loading" ? <p className="state-text">{t("nodes.revisions_loading")}</p> : null}
              {revisionErrorMessage ? <p className="error-text">{revisionErrorMessage}</p> : null}
              {revisionsLoadState === "loaded" && configRevisions.length === 0 ? (
                <p className="state-card compact">{t("nodes.revisions_empty")}</p>
              ) : null}

              {configRevisions.length > 0 ? (
                <div className="table-wrap revisions-table-wrap">
                  <table className="data-table revisions-table">
                    <thead>
                      <tr>
                        <th>Revision</th>
                        <th>{t("nodes.th_status")}</th>
                        <th>Bundle hash</th>
                        <th>Signer</th>
                        <th>Rollback target</th>
                        <th>Created</th>
                        <th>Applied</th>
                        <th>Failed</th>
                        <th>Rolled back</th>
                        <th>Error</th>
                        <th>{t("nodes.th_actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {configRevisions.map((revision) => (
                        <tr key={revision.id} className={revision.id === selectedRevisionID ? "selected-row" : undefined}>
                          <td>#{revision.revision_number}</td>
                          <td><span className={`status-badge ${configRevisionStatusClass(revision.status)}`}>{revision.status}</span></td>
                          <td className="mono-cell">{revision.bundle_hash || "-"}</td>
                          <td>{revision.signer || "-"}</td>
                          <td>{revision.rollback_target_revision}</td>
                          <td>{formatNodeTimestamp(revision.created_at)}</td>
                          <td>{formatNodeTimestamp(revision.applied_at)}</td>
                          <td>{formatNodeTimestamp(revision.failed_at)}</td>
                          <td>{formatNodeTimestamp(revision.rolled_back_at)}</td>
                          <td className={revision.error_message ? "revision-error-cell mono-cell" : undefined}>{revision.error_message || "-"}</td>
                          <td>
                            <div className="row-actions">
                              <button className="table-button" type="button" onClick={() => selectRevision(revision.id)} disabled={revisionDetailLoadState === "loading"}>{t("nodes.details")}</button>
                              <button className="table-button" type="button" onClick={() => rollbackRevision(revision)} disabled={revision.status !== "applied" || rollbackRevisionID !== null}>
                                {rollbackRevisionID === revision.id ? t("nodes.rolling_back") : t("nodes.rollback")}
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : null}

              {selectedRevision ? (
                <div className="revision-detail">
                  <div className="section-heading">
                    <div>
                      <p className="eyebrow">{t("nodes.revision_detail_eyebrow")}</p>
                      <h3>#{selectedRevision.revision_number}</h3>
                    </div>
                  </div>
                  <dl className="node-detail-grid">
                    <DetailItem label="ID" value={selectedRevision.id} mono />
                    <DetailItem label="Node ID" value={selectedRevision.node_id} mono />
                    <DetailItem label={t("nodes.th_status")} value={selectedRevision.status} />
                    <DetailItem label="Signer" value={selectedRevision.signer} />
                    <DetailItem label="Bundle hash" value={selectedRevision.bundle_hash} mono />
                    <DetailItem label="Signature" value={selectedRevision.signature} mono />
                    <DetailItem label="Rollback target" value={String(selectedRevision.rollback_target_revision)} />
                    <DetailItem label="Operation" value={readBundleString(selectedRevision.bundle, "operation_kind")} />
                    <DetailItem label="Source revision" value={readBundleNumber(selectedRevision.bundle, "source_revision_number")} />
                    <DetailItem label="Source revision ID" value={readBundleString(selectedRevision.bundle, "source_revision_id")} mono />
                    <DetailItem label="Created" value={formatNodeTimestamp(selectedRevision.created_at)} />
                    <DetailItem label="Applied" value={formatNodeTimestamp(selectedRevision.applied_at)} />
                    <DetailItem label="Failed" value={formatNodeTimestamp(selectedRevision.failed_at)} />
                    <DetailItem label="Rolled back" value={formatNodeTimestamp(selectedRevision.rolled_back_at)} />
                    <DetailItem label="Error" value={selectedRevision.error_message} />
                  </dl>
                  {selectedRevision.status === "failed" ? <RevisionFailureBlock revision={selectedRevision} /> : null}
                  <pre className="json-block">{formatConfigRevisionBundle(selectedRevision.bundle)}</pre>
                </div>
              ) : null}
            </section>
          </>
        ) : null}
      </Modal>
    </div>
  );
}

function renderLifecycleButtons(
  node: Pick<Node, "id" | "status" | "drain_state">,
  onAction: (node: NodeSummary | Node, action: NodeAction) => void,
  mutatingNodeID: string | null,
  mutatingAction: NodeAction | null,
  t: (key: any) => string,
) {
  const isMutatingNode = mutatingNodeID === node.id;
  return (
    <>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "drain")} disabled={!canDrain(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "drain" ? t("nodes.draining_action") : t("nodes.drain")}
      </button>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "undrain")} disabled={!canUndrain(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "undrain" ? t("nodes.undraining") : t("nodes.undrain")}
      </button>
      <button className="table-button danger" type="button" onClick={() => onAction(node as NodeSummary, "disable")} disabled={!canDisable(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "disable" ? t("nodes.disabling") : t("nodes.disable")}
      </button>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "enable")} disabled={!canEnable(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "enable" ? t("nodes.enabling") : t("nodes.enable")}
      </button>
    </>
  );
}

interface DetailItemProps { label: string; value?: string | null; mono?: boolean; }
function DetailItem({ label, value, mono }: DetailItemProps) {
  return (<div><dt>{label}</dt><dd className={mono ? "mono-cell" : undefined}>{value || "-"}</dd></div>);
}

function runtimeValidationStatusClass(status?: string | null): string {
  if (status === "applied") return "status-active";
  if (status === "failed") return "status-disabled";
  return "status-pending";
}

function formatRevisionNumber(value?: number): string {
  return value && value > 0 ? String(value) : "-";
}

interface RuntimeEventsBlockProps { events: NonNullable<Node["runtime_events"]>; t: (key: any) => string; }
function RuntimeEventsBlock({ events, t }: RuntimeEventsBlockProps) {
  return (
    <section className="runtime-events-panel" aria-label="Recent node runtime events">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">{t("nodes.runtime_events_eyebrow")}</p>
          <h4>{t("nodes.runtime_events_title")}</h4>
        </div>
        <span className="pill">{events.length} {t("nodes.recent")}</span>
      </div>
      {events.length === 0 ? (
        <p className="state-card compact">{t("nodes.runtime_events_empty")}</p>
      ) : (
        <ol className="runtime-events-list">
          {events.map((event, index) => (
            <li key={`${event.type || "event"}-${event.at || index}-${index}`}>
              <div className="runtime-event-main">
                <span className="runtime-event-title">{formatRuntimeEventType(event.type)}</span>
                {event.status ? <span className={`status-badge ${runtimeValidationStatusClass(event.status)}`}>{event.status}</span> : null}
              </div>
              <div className="runtime-event-meta">
                <span>{formatNodeTimestamp(event.at)}</span>
                {event.revision_number && event.revision_number > 0 ? <span>Revision #{event.revision_number}</span> : null}
                {event.runtime_mode ? <span>{event.runtime_mode}</span> : null}
              </div>
              {event.message ? <p className="runtime-event-message mono-cell">{event.message}</p> : null}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

interface RevisionFailureBlockProps { revision: ConfigRevision; }
function RevisionFailureBlock({ revision }: RevisionFailureBlockProps) {
  const operation = readBundleString(revision.bundle, "operation_kind");
  const sourceRevisionNumber = readBundleNumber(revision.bundle, "source_revision_number");
  const sourceRevisionID = readBundleString(revision.bundle, "source_revision_id");
  return (
    <section className="revision-failure-panel" aria-label="Config revision failure details">
      <div className="section-heading compact-heading">
        <div><p className="eyebrow">Failure</p><h4>Apply validation failed</h4></div>
        <span className={`status-badge ${configRevisionStatusClass(revision.status)}`}>{revision.status}</span>
      </div>
      <dl className="revision-failure-grid">
        <DetailItem label="Error message" value={revision.error_message} mono />
        <DetailItem label="Failed at" value={formatNodeTimestamp(revision.failed_at)} />
        <DetailItem label="Operation" value={operation} />
        <DetailItem label="Rollback target" value={String(revision.rollback_target_revision)} />
        <DetailItem label="Source revision" value={sourceRevisionNumber} />
        <DetailItem label="Source revision ID" value={sourceRevisionID} mono />
      </dl>
    </section>
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

function readBundleString(bundle: unknown, key: string): string | null {
  if (!bundle || typeof bundle !== "object" || Array.isArray(bundle)) return null;
  const value = (bundle as Record<string, unknown>)[key];
  return typeof value === "string" && value.trim() ? value : null;
}

function readBundleNumber(bundle: unknown, key: string): string | null {
  if (!bundle || typeof bundle !== "object" || Array.isArray(bundle)) return null;
  const value = (bundle as Record<string, unknown>)[key];
  return typeof value === "number" && Number.isFinite(value) ? String(value) : null;
}

interface NodeTrafficSectionProps { session: StoredSession; nodeID: string; onUnauthorized: () => void; t: (key: any) => string; }
function NodeTrafficSection({ session, nodeID, onUnauthorized, t }: NodeTrafficSectionProps) {
  const [usage, setUsage] = useState<TrafficUsage | null>(null);
  const [loadState, setLoadState] = useState<"idle" | "loading" | "loaded" | "failed">("idle");
  const loadTraffic = useCallback(async () => {
    setLoadState("loading");
    try { const result = await getNodeTraffic(session, nodeID); setUsage(result); setLoadState("loaded"); }
    catch (err) { if (err instanceof PanelApiError && err.status === 401) { onUnauthorized(); return; } setLoadState("failed"); }
  }, [session, nodeID, onUnauthorized]);
  useEffect(() => { loadTraffic(); }, [loadTraffic]);
  return (
    <section className="surface-card">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">{t("nodes.traffic_eyebrow")}</p>
          {usage ? (<h3>↑ {formatNodeBytes(usage.bytes_up)} / ↓ {formatNodeBytes(usage.bytes_down)} / Total {formatNodeBytes(usage.bytes_total)}</h3>) : (<h3>—</h3>)}
        </div>
      </div>
      {loadState === "loading" ? <p className="state-text">{t("nodes.traffic_loading")}</p> : null}
      {loadState === "failed" ? <p className="state-text">{t("nodes.traffic_failed")}</p> : null}
    </section>
  );
}

function formatNodeBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}
