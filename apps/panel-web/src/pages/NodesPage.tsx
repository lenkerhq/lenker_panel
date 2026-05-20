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

interface NodesPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";
type NodeAction = "drain" | "undrain" | "disable" | "enable";

export function NodesPage({ session, onUnauthorized }: NodesPageProps) {
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
  const [applyingNodeID, setApplyingNodeID] = useState<string | null>(null);
  const [selectedProfileID, setSelectedProfileID] = useState<string>("");

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
        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }
        setSelectedRevision(null);
        setRevisionErrorMessage(formatPanelError(error, "Unable to load config revision details."));
        setRevisionDetailLoadState("failed");
      }
    },
    [onUnauthorized, session],
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
        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }
        setConfigRevisions([]);
        setSelectedRevision(null);
        setSelectedRevisionID(null);
        setRevisionErrorMessage(formatPanelError(error, "Unable to load config revisions."));
        setRevisionsLoadState("failed");
      }
    },
    [loadRevisionDetail, onUnauthorized, session],
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
        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }
        setSelectedNode(null);
        setConfigRevisions([]);
        setSelectedRevision(null);
        setSelectedRevisionID(null);
        setRevisionsLoadState("idle");
        setErrorMessage(formatPanelError(error, "Unable to load node details."));
        setDetailLoadState("failed");
      }
    },
    [loadConfigRevisions, onUnauthorized, session],
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
        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }
        setErrorMessage(formatPanelError(error, "Unable to load nodes."));
        setLoadState("failed");
      }
    },
    [loadNodeDetail, onUnauthorized, selectedNodeID, session],
  );

  useEffect(() => {
    let isMounted = true;

    async function loadInitialNodes() {
      setLoadState("loading");
      setErrorMessage(null);

      try {
        const [loadedNodes, loadedProfiles] = await Promise.all([listNodes(session), listNodeProfiles(session)]);
        if (!isMounted) {
          return;
        }

        setNodes(loadedNodes);
        setProfiles(loadedProfiles);
        setLoadState("loaded");

        const firstNodeID = loadedNodes[0]?.id ?? null;
        setSelectedNodeID(firstNodeID);
        if (firstNodeID) {
          await loadNodeDetail(firstNodeID);
        }
      } catch (error) {
        if (!isMounted) {
          return;
        }

        if (handleUnauthorizedError(error, onUnauthorized)) {
          return;
        }
        setErrorMessage(formatPanelError(error, "Unable to load nodes."));
        setLoadState("failed");
      }
    }

    loadInitialNodes();

    return () => {
      isMounted = false;
    };
  }, [loadNodeDetail, onUnauthorized, session]);

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
    if (!selectedNodeID) {
      return;
    }

    setSuccessMessage(null);
    setSelectedRevisionID(revisionID);
    await loadRevisionDetail(selectedNodeID, revisionID);
  }

  async function submitBootstrapTokenForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const validationError = validateNodeBootstrapForm(formState);
    if (validationError) {
      setErrorMessage(validationError);
      setSuccessMessage(null);
      return;
    }

    setIsCreatingToken(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);

    try {
      const token = await createNodeBootstrapToken(session, buildCreateNodeBootstrapTokenInput(formState));
      setBootstrapToken(token);
      setSuccessMessage("Bootstrap token created.");
      setFormState(emptyNodeBootstrapForm());
      await loadNodes(token.node_id);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to create bootstrap token."));
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
      if (action === "drain") {
        await drainNode(session, node.id);
        setSuccessMessage("Node marked as draining.");
      } else if (action === "undrain") {
        await undrainNode(session, node.id);
        setSuccessMessage("Node returned to active drain state.");
      } else if (action === "disable") {
        await disableNode(session, node.id);
        setSuccessMessage("Node disabled.");
      } else {
        await enableNode(session, node.id);
        setSuccessMessage("Node enabled as unhealthy.");
      }
      await loadNodes(node.id);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setErrorMessage(formatPanelError(error, "Unable to update node lifecycle."));
    } finally {
      setMutatingNodeID(null);
      setMutatingAction(null);
    }
  }

  async function createDummyRevision() {
    if (!selectedNodeID) {
      return;
    }

    setIsCreatingRevision(true);
    setRevisionErrorMessage(null);
    setSuccessMessage(null);
    setBootstrapToken(null);

    try {
      const revision = await createNodeConfigRevision(session, selectedNodeID);
      setSuccessMessage(`Config revision #${revision.revision_number} created.`);
      await loadNodes(selectedNodeID);
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setRevisionErrorMessage(formatPanelError(error, "Unable to create config revision."));
    } finally {
      setIsCreatingRevision(false);
    }
  }

  async function rollbackRevision(revision: ConfigRevision) {
    if (!selectedNodeID) {
      return;
    }

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
      if (handleUnauthorizedError(error, onUnauthorized)) {
        return;
      }
      setRevisionErrorMessage(formatPanelError(error, "Unable to create rollback revision."));
    } finally {
      setRollbackRevisionID(null);
    }
  }

  async function applyProfileToNode(nodeID: string) {
    if (!selectedProfileID) return;
    setApplyingNodeID(nodeID);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await applyNodeProfile(session, selectedProfileID, nodeID);
      setSuccessMessage("Profile applied to node.");
      setApplyingNodeID(null);
      setSelectedProfileID("");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to apply profile."));
      setApplyingNodeID(null);
    }
  }

  return (
    <div className="page-stack" id="nodes">
      <section className="page-header">
        <div>
          <p className="eyebrow">Nodes</p>
          <h2>Nodes</h2>
          <p>List, inspect, bootstrap, drain, undrain, disable, and enable managed nodes through the panel-api admin API.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{nodes.length} total</span>
          <span className="pill">{activeNodes} active</span>
          <span className="pill">{drainingNodes} draining</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={submitBootstrapTokenForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">Bootstrap</p>
              <h3>Create token</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="node-name">
            Name
          </label>
          <input
            id="node-name"
            className="text-field"
            type="text"
            autoComplete="off"
            value={formState.name}
            onChange={(event) => updateFormField("name", event.target.value)}
          />

          <div className="form-grid">
            <div>
              <label className="field-label" htmlFor="node-region">
                Region
              </label>
              <input
                id="node-region"
                className="text-field"
                type="text"
                autoComplete="off"
                value={formState.region}
                onChange={(event) => updateFormField("region", event.target.value)}
              />
            </div>
            <div>
              <label className="field-label" htmlFor="node-country-code">
                Country code
              </label>
              <input
                id="node-country-code"
                className="text-field"
                type="text"
                autoComplete="off"
                value={formState.countryCode}
                onChange={(event) => updateFormField("countryCode", event.target.value)}
              />
            </div>
          </div>

          <label className="field-label" htmlFor="node-hostname">
            Hostname
          </label>
          <input
            id="node-hostname"
            className="text-field"
            type="text"
            autoComplete="off"
            value={formState.hostname}
            onChange={(event) => updateFormField("hostname", event.target.value)}
          />

          <label className="field-label" htmlFor="node-expires-in">
            Expires in minutes
          </label>
          <input
            id="node-expires-in"
            className="text-field"
            type="number"
            min="1"
            max="10080"
            inputMode="numeric"
            value={formState.expiresInMinutes}
            onChange={(event) => updateFormField("expiresInMinutes", event.target.value)}
          />

          <button className="primary-button" type="submit" disabled={isCreatingToken}>
            {isCreatingToken ? "Creating..." : "Create bootstrap token"}
          </button>
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading nodes...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">Nodes list is ready.</p> : null}
          {detailLoadState === "loading" ? <p className="state-text">Loading selected node...</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}

          {bootstrapToken ? (
            <dl className="token-result">
              <div>
                <dt>Node ID</dt>
                <dd className="mono-cell">{bootstrapToken.node_id}</dd>
              </div>
              <div>
                <dt>Bootstrap token</dt>
                <dd className="mono-cell token-value">{bootstrapToken.bootstrap_token}</dd>
              </div>
              <div>
                <dt>Expires</dt>
                <dd>{formatNodeTimestamp(bootstrapToken.expires_at)}</dd>
              </div>
            </dl>
          ) : null}

          <button className="secondary-button" type="button" onClick={() => loadNodes()} disabled={loadState === "loading"}>
            Refresh
          </button>
        </div>
      </section>

      {loadState === "loaded" && nodes.length === 0 ? <p className="state-card">No nodes yet. Create a bootstrap token above.</p> : null}

      {nodes.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table nodes-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Region</th>
                <th>Country</th>
                <th>Hostname</th>
                <th>Status</th>
                <th>Drain</th>
                <th>Agent</th>
                <th>Revision</th>
                <th>Last seen</th>
                <th>Registered</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.id} className={`clickable-row${node.id === selectedNodeID ? " selected-row" : ""}`} onClick={() => selectNode(node.id)}>
                  <td>{node.name || "-"}</td>
                  <td>{node.region || "-"}</td>
                  <td>{node.country_code || (selectedNode?.id === node.id ? selectedNode.country_code : "") || "-"}</td>
                  <td>{node.hostname || (selectedNode?.id === node.id ? selectedNode.hostname : "") || "-"}</td>
                  <td>
                    <span className={`status-badge ${nodeStatusClass(node.status)}`}>{node.status}</span>
                  </td>
                  <td>
                    <span className={`status-badge ${nodeDrainClass(node.drain_state)}`}>{node.drain_state}</span>
                  </td>
                  <td>{node.agent_version || "-"}</td>
                  <td>{node.active_revision_id}</td>
                  <td>{formatNodeTimestamp(node.last_seen_at)}</td>
                  <td>{formatNodeTimestamp(node.registered_at)}</td>
                  <td className="mono-cell">{node.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      <button className="table-button" type="button" onClick={() => selectNode(node.id)} disabled={detailLoadState === "loading"}>
                        Details
                      </button>
                      {renderLifecycleButtons(node, runNodeAction, mutatingNodeID, mutatingAction)}
                      {profiles.length > 0 ? (
                        <>
                          <select className="select-field" value={applyingNodeID === node.id ? selectedProfileID : ""} onChange={(e) => { setApplyingNodeID(node.id); setSelectedProfileID(e.target.value); }} aria-label="Select profile">
                            <option value="">Profile...</option>
                            {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                          </select>
                          <button className="table-button" type="button" onClick={() => applyProfileToNode(node.id)} disabled={applyingNodeID === node.id && !selectedProfileID}>
                            Apply
                          </button>
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
                  <p className="eyebrow">Node detail</p>
                  <h3>{selectedNode.name || selectedNode.id}</h3>
                </div>
                <div className="row-actions">
                  {renderLifecycleButtons(selectedNode, runNodeAction, mutatingNodeID, mutatingAction)}
            </div>
          </div>

          <dl className="node-detail-grid">
            <DetailItem label="ID" value={selectedNode.id} mono />
            <DetailItem label="Name" value={selectedNode.name} />
            <DetailItem label="Region" value={selectedNode.region} />
            <DetailItem label="Country" value={selectedNode.country_code} />
            <DetailItem label="Hostname" value={selectedNode.hostname} />
            <DetailItem label="Status" value={selectedNode.status} />
            <DetailItem label="Drain state" value={selectedNode.drain_state} />
            <DetailItem label="Agent version" value={selectedNode.agent_version} />
            <DetailItem label="Xray version" value={selectedNode.xray_version} />
            <DetailItem label="Active revision" value={String(selectedNode.active_revision_id)} />
            <DetailItem label="Last health" value={formatNodeTimestamp(selectedNode.last_health_at)} />
            <DetailItem label="Last seen" value={formatNodeTimestamp(selectedNode.last_seen_at)} />
            <DetailItem label="Registered" value={formatNodeTimestamp(selectedNode.registered_at)} />
            <DetailItem label="Updated" value={formatNodeTimestamp(selectedNode.updated_at)} />
          </dl>

          <section className="runtime-status-panel" aria-label="Node runtime validation status">
            <div className="section-heading compact-heading">
              <div>
                <p className="eyebrow">Runtime readiness</p>
                <h4>Validation status</h4>
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

            <RuntimeEventsBlock events={selectedNode.runtime_events ?? []} />
          </section>
        </section>

        <NodeTrafficSection session={session} nodeID={selectedNode.id} onUnauthorized={onUnauthorized} />

        <section className="surface-card">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Config revisions</p>
              <h3>Revision metadata</h3>
            </div>
            <div className="row-actions">
              <button
                className="secondary-button"
                type="button"
                onClick={() => loadConfigRevisions(selectedNode.id)}
                disabled={revisionsLoadState === "loading"}
              >
                {revisionsLoadState === "loading" ? "Refreshing..." : "Refresh"}
              </button>
              <button className="table-button" type="button" onClick={createDummyRevision} disabled={isCreatingRevision}>
                {isCreatingRevision ? "Creating..." : "Create revision"}
              </button>
            </div>
          </div>

          {revisionsLoadState === "loading" ? <p className="state-text">Loading config revisions...</p> : null}
          {revisionErrorMessage ? <p className="error-text">{revisionErrorMessage}</p> : null}
          {revisionsLoadState === "loaded" && configRevisions.length === 0 ? (
            <p className="state-card compact">No config revisions for this node.</p>
          ) : null}

          {configRevisions.length > 0 ? (
            <div className="table-wrap revisions-table-wrap">
              <table className="data-table revisions-table">
                <thead>
                  <tr>
                    <th>Revision</th>
                    <th>Status</th>
                    <th>Bundle hash</th>
                    <th>Signer</th>
                    <th>Rollback target</th>
                    <th>Created</th>
                    <th>Applied</th>
                    <th>Failed</th>
                    <th>Rolled back</th>
                    <th>Error</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {configRevisions.map((revision) => (
                    <tr key={revision.id} className={revision.id === selectedRevisionID ? "selected-row" : undefined}>
                      <td>#{revision.revision_number}</td>
                      <td>
                        <span className={`status-badge ${configRevisionStatusClass(revision.status)}`}>{revision.status}</span>
                      </td>
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
                          <button
                            className="table-button"
                            type="button"
                            onClick={() => selectRevision(revision.id)}
                            disabled={revisionDetailLoadState === "loading"}
                          >
                            Details
                          </button>
                          <button
                            className="table-button"
                            type="button"
                            onClick={() => rollbackRevision(revision)}
                            disabled={revision.status !== "applied" || rollbackRevisionID !== null}
                          >
                            {rollbackRevisionID === revision.id ? "Rolling back..." : "Rollback"}
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
                  <p className="eyebrow">Revision detail</p>
                  <h3>#{selectedRevision.revision_number}</h3>
                </div>
              </div>

              <dl className="node-detail-grid">
                <DetailItem label="ID" value={selectedRevision.id} mono />
                <DetailItem label="Node ID" value={selectedRevision.node_id} mono />
                <DetailItem label="Status" value={selectedRevision.status} />
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
) {
  const isMutatingNode = mutatingNodeID === node.id;

  return (
    <>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "drain")} disabled={!canDrain(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "drain" ? "Draining..." : "Drain"}
      </button>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "undrain")} disabled={!canUndrain(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "undrain" ? "Undraining..." : "Undrain"}
      </button>
      <button className="table-button danger" type="button" onClick={() => onAction(node as NodeSummary, "disable")} disabled={!canDisable(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "disable" ? "Disabling..." : "Disable"}
      </button>
      <button className="table-button" type="button" onClick={() => onAction(node as NodeSummary, "enable")} disabled={!canEnable(node) || isMutatingNode}>
        {isMutatingNode && mutatingAction === "enable" ? "Enabling..." : "Enable"}
      </button>
    </>
  );
}

interface DetailItemProps {
  label: string;
  value?: string | null;
  mono?: boolean;
}

function DetailItem({ label, value, mono }: DetailItemProps) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={mono ? "mono-cell" : undefined}>{value || "-"}</dd>
    </div>
  );
}

function runtimeValidationStatusClass(status?: string | null): string {
  if (status === "applied") {
    return "status-active";
  }
  if (status === "failed") {
    return "status-disabled";
  }
  return "status-pending";
}

function formatRevisionNumber(value?: number): string {
  return value && value > 0 ? String(value) : "-";
}

interface RuntimeEventsBlockProps {
  events: NonNullable<Node["runtime_events"]>;
}

function RuntimeEventsBlock({ events }: RuntimeEventsBlockProps) {
  return (
    <section className="runtime-events-panel" aria-label="Recent node runtime events">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">Runtime events</p>
          <h4>Recent events</h4>
        </div>
        <span className="pill">{events.length} recent</span>
      </div>

      {events.length === 0 ? (
        <p className="state-card compact">No runtime events yet.</p>
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

interface RevisionFailureBlockProps {
  revision: ConfigRevision;
}

function RevisionFailureBlock({ revision }: RevisionFailureBlockProps) {
  const operation = readBundleString(revision.bundle, "operation_kind");
  const sourceRevisionNumber = readBundleNumber(revision.bundle, "source_revision_number");
  const sourceRevisionID = readBundleString(revision.bundle, "source_revision_id");

  return (
    <section className="revision-failure-panel" aria-label="Config revision failure details">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">Failure</p>
          <h4>Apply validation failed</h4>
        </div>
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

function readBundleString(bundle: unknown, key: string): string | null {
  if (!bundle || typeof bundle !== "object" || Array.isArray(bundle)) {
    return null;
  }
  const value = (bundle as Record<string, unknown>)[key];
  return typeof value === "string" && value.trim() ? value : null;
}

function readBundleNumber(bundle: unknown, key: string): string | null {
  if (!bundle || typeof bundle !== "object" || Array.isArray(bundle)) {
    return null;
  }
  const value = (bundle as Record<string, unknown>)[key];
  return typeof value === "number" && Number.isFinite(value) ? String(value) : null;
}

// --- Node Traffic section ---

interface NodeTrafficSectionProps {
  session: StoredSession;
  nodeID: string;
  onUnauthorized: () => void;
}

function NodeTrafficSection({ session, nodeID, onUnauthorized }: NodeTrafficSectionProps) {
  const [usage, setUsage] = useState<TrafficUsage | null>(null);
  const [loadState, setLoadState] = useState<"idle" | "loading" | "loaded" | "failed">("idle");

  const loadTraffic = useCallback(async () => {
    setLoadState("loading");
    try {
      const result = await getNodeTraffic(session, nodeID);
      setUsage(result);
      setLoadState("loaded");
    } catch (err) {
      if (err instanceof PanelApiError && err.status === 401) {
        onUnauthorized();
        return;
      }
      setLoadState("failed");
    }
  }, [session, nodeID, onUnauthorized]);

  useEffect(() => {
    loadTraffic();
  }, [loadTraffic]);

  return (
    <section className="surface-card">
      <div className="section-heading compact-heading">
        <div>
          <p className="eyebrow">Traffic</p>
          {usage ? (
            <h3>↑ {formatNodeBytes(usage.bytes_up)} / ↓ {formatNodeBytes(usage.bytes_down)} / Total {formatNodeBytes(usage.bytes_total)}</h3>
          ) : (
            <h3>—</h3>
          )}
        </div>
      </div>
      {loadState === "loading" ? <p className="state-text">Loading traffic...</p> : null}
      {loadState === "failed" ? <p className="state-text">Failed to load traffic data.</p> : null}
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
