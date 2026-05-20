import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  deleteNodeWarp,
  generateWarpKeypair,
  getNodeWarp,
  listNodes,
  PanelApiError,
  setNodeWarp,
  type NodeSummary,
  type WarpCredentials,
} from "../lib/api";
import type { StoredSession } from "../lib/session";

interface WarpConfigurationPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

interface WarpFormState {
  privateKey: string;
  publicKey: string;
  address: string;
  endpoint: string;
}

function emptyWarpForm(): WarpFormState {
  return { privateKey: "", publicKey: "", address: "", endpoint: "" };
}

export function WarpConfigurationPage({ session, onUnauthorized }: WarpConfigurationPageProps) {
  const [nodes, setNodes] = useState<NodeSummary[]>([]);
  const [selectedNodeID, setSelectedNodeID] = useState<string>("");
  const [credentials, setCredentials] = useState<WarpCredentials | null>(null);
  const [formState, setFormState] = useState<WarpFormState>(() => emptyWarpForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    let isMounted = true;
    async function load() {
      try {
        const loaded = await listNodes(session);
        if (isMounted) setNodes(loaded);
      } catch (error) {
        if (error instanceof PanelApiError && error.status === 401) onUnauthorized();
      }
    }
    load();
    return () => { isMounted = false; };
  }, [session, onUnauthorized]);

  const loadCredentials = useCallback(async (nodeID: string) => {
    setIsLoading(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    setCredentials(null);
    setFormState(emptyWarpForm());
    try {
      const creds = await getNodeWarp(session, nodeID);
      setCredentials(creds);
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      if (error instanceof PanelApiError && error.status === 404) {
        setCredentials(null);
      } else {
        setErrorMessage(formatError(error, "Unable to load WARP credentials."));
      }
    } finally {
      setIsLoading(false);
    }
  }, [session, onUnauthorized]);

  function selectNode(nodeID: string) {
    setSelectedNodeID(nodeID);
    if (nodeID) loadCredentials(nodeID);
    else { setCredentials(null); setFormState(emptyWarpForm()); }
  }

  async function handleGenerate() {
    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      const result = await generateWarpKeypair(session);
      setFormState((cur) => ({ ...cur, privateKey: result.private_key, publicKey: result.public_key }));
      setSuccessMessage("Keypair generated. Fill address/endpoint and save.");
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to generate keypair."));
    } finally {
      setIsMutating(false);
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedNodeID) return;
    if (!formState.privateKey.trim() || !formState.publicKey.trim() || !formState.address.trim()) {
      setErrorMessage("Private key, public key, and address are required.");
      return;
    }
    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      const creds = await setNodeWarp(session, selectedNodeID, {
        private_key: formState.privateKey.trim(),
        public_key: formState.publicKey.trim(),
        address: formState.address.trim(),
        endpoint: formState.endpoint.trim() || undefined,
      });
      setCredentials(creds);
      setSuccessMessage("WARP credentials saved.");
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to save WARP credentials."));
    } finally {
      setIsMutating(false);
    }
  }

  async function handleDelete() {
    if (!selectedNodeID) return;
    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await deleteNodeWarp(session, selectedNodeID);
      setCredentials(null);
      setFormState(emptyWarpForm());
      setSuccessMessage("WARP credentials deleted.");
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to delete WARP credentials."));
    } finally {
      setIsMutating(false);
    }
  }

  return (
    <div className="page-stack" id="warp-configuration">
      <section className="page-header">
        <div>
          <p className="eyebrow">WARP</p>
          <h2>WARP Configuration</h2>
          <p>Manage WireGuard WARP credentials for nodes. Generate keypairs and assign them to nodes.</p>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={handleSave}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">Credentials</p>
              <h3>Node WARP settings</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="warp-node">Node</label>
          <select id="warp-node" className="select-field" value={selectedNodeID} onChange={(e) => selectNode(e.target.value)}>
            <option value="">Select node</option>
            {nodes.map((n) => <option key={n.id} value={n.id}>{n.name || n.id}</option>)}
          </select>

          {selectedNodeID ? (
            <>
              <div className="section-heading compact-heading">
                <div><p className="eyebrow">Keypair</p></div>
                <button className="table-button" type="button" onClick={handleGenerate} disabled={isMutating}>Generate</button>
              </div>

              <label className="field-label" htmlFor="warp-private-key">Private key</label>
              <input id="warp-private-key" className="text-field" type="text" autoComplete="off" value={formState.privateKey} onChange={(e) => setFormState((cur) => ({ ...cur, privateKey: e.target.value }))} />

              <label className="field-label" htmlFor="warp-public-key">Public key</label>
              <input id="warp-public-key" className="text-field" type="text" autoComplete="off" value={formState.publicKey} onChange={(e) => setFormState((cur) => ({ ...cur, publicKey: e.target.value }))} />

              <label className="field-label" htmlFor="warp-address">Address</label>
              <input id="warp-address" className="text-field" type="text" autoComplete="off" value={formState.address} onChange={(e) => setFormState((cur) => ({ ...cur, address: e.target.value }))} />

              <label className="field-label" htmlFor="warp-endpoint">Endpoint (optional)</label>
              <input id="warp-endpoint" className="text-field" type="text" autoComplete="off" value={formState.endpoint} onChange={(e) => setFormState((cur) => ({ ...cur, endpoint: e.target.value }))} />

              <div className="row-actions">
                <button className="primary-button" type="submit" disabled={isMutating}>{isMutating ? "Saving..." : "Save"}</button>
                {credentials ? (
                  <button className="table-button danger" type="button" onClick={handleDelete} disabled={isMutating}>Delete</button>
                ) : null}
              </div>
            </>
          ) : null}
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">State</p>
          {isLoading ? <p className="state-text">Loading credentials...</p> : null}
          {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          {!isLoading && !errorMessage && !successMessage && selectedNodeID ? (
            <p className="state-text">{credentials ? "Credentials loaded." : "No WARP credentials for this node."}</p>
          ) : null}
          {!selectedNodeID ? <p className="state-text">Select a node to manage WARP credentials.</p> : null}

          {credentials ? (
            <dl className="node-detail-grid">
              <div><dt>Public key</dt><dd className="mono-cell">{credentials.public_key}</dd></div>
              <div><dt>Address</dt><dd>{credentials.address}</dd></div>
              <div><dt>Endpoint</dt><dd>{credentials.endpoint || "-"}</dd></div>
              <div><dt>Enabled</dt><dd>{credentials.enabled ? "yes" : "no"}</dd></div>
              <div><dt>Created</dt><dd>{formatDate(credentials.created_at)}</dd></div>
            </dl>
          ) : null}
        </div>
      </section>
    </div>
  );
}

function formatError(error: unknown, fallback: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallback;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}
