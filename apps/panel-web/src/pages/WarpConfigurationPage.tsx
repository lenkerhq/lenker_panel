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
import { useI18n } from "../lib/i18n";

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
  const { t } = useI18n();
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
      setFormState({ privateKey: "", publicKey: creds.public_key, address: creds.address, endpoint: creds.endpoint || "" });
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      if (error instanceof PanelApiError && error.status === 404) {
        setCredentials(null);
      } else {
        setErrorMessage(formatError(error, t("warp.unable_load")));
      }
    } finally {
      setIsLoading(false);
    }
  }, [session, onUnauthorized, t]);

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
      setSuccessMessage(t("warp.keypair_generated"));
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, t("warp.unable_generate")));
    } finally {
      setIsMutating(false);
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedNodeID) return;
    if (!formState.privateKey.trim() || !formState.publicKey.trim() || !formState.address.trim()) {
      setErrorMessage(t("warp.required_fields"));
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
      setSuccessMessage(t("warp.saved"));
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, t("warp.unable_save")));
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
      setSuccessMessage(t("warp.deleted"));
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, t("warp.unable_delete")));
    } finally {
      setIsMutating(false);
    }
  }

  return (
    <div className="page-stack" id="warp-configuration">
      <section className="page-header">
        <div>
          <p className="eyebrow">{t("warp.eyebrow")}</p>
          <h2>{t("warp.title")}</h2>
          <p>{t("warp.description")}</p>
        </div>
      </section>

      <section className="management-grid">
        <form className="management-panel" onSubmit={handleSave}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">{t("warp.credentials_eyebrow")}</p>
              <h3>{t("warp.settings_title")}</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="warp-node">{t("warp.node")}</label>
          <select id="warp-node" className="select-field" value={selectedNodeID} onChange={(e) => selectNode(e.target.value)}>
            <option value="">{t("warp.select_node")}</option>
            {nodes.map((n) => <option key={n.id} value={n.id}>{n.name || n.id}</option>)}
          </select>

          {selectedNodeID ? (
            <>
              <div className="section-heading compact-heading">
                <div><p className="eyebrow">{t("warp.keypair_eyebrow")}</p></div>
                <button className="table-button" type="button" onClick={handleGenerate} disabled={isMutating}>{t("warp.generate")}</button>
              </div>

              <label className="field-label" htmlFor="warp-private-key">{t("warp.private_key")}</label>
              <input id="warp-private-key" className="text-field" type="text" autoComplete="off" placeholder={credentials ? "••• stored — enter new to replace" : ""} value={formState.privateKey} onChange={(e) => setFormState((cur) => ({ ...cur, privateKey: e.target.value }))} />

              <label className="field-label" htmlFor="warp-public-key">{t("warp.public_key")}</label>
              <input id="warp-public-key" className="text-field" type="text" autoComplete="off" value={formState.publicKey} onChange={(e) => setFormState((cur) => ({ ...cur, publicKey: e.target.value }))} />

              <label className="field-label" htmlFor="warp-address">{t("warp.address")}</label>
              <input id="warp-address" className="text-field" type="text" autoComplete="off" value={formState.address} onChange={(e) => setFormState((cur) => ({ ...cur, address: e.target.value }))} />

              <label className="field-label" htmlFor="warp-endpoint">{t("warp.endpoint")}</label>
              <input id="warp-endpoint" className="text-field" type="text" autoComplete="off" value={formState.endpoint} onChange={(e) => setFormState((cur) => ({ ...cur, endpoint: e.target.value }))} />

              <div className="row-actions">
                <button className="primary-button" type="submit" disabled={isMutating}>{isMutating ? t("common.saving") : t("common.save")}</button>
                {credentials ? (
                  <button className="table-button danger" type="button" onClick={handleDelete} disabled={isMutating}>{t("common.delete")}</button>
                ) : null}
              </div>
            </>
          ) : null}
        </form>

        <div className="feedback-panel">
          <p className="eyebrow">{t("common.state")}</p>
          {isLoading ? <p className="state-text">{t("warp.loading")}</p> : null}
          {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          {!isLoading && !errorMessage && !successMessage && selectedNodeID ? (
            <p className="state-text">{credentials ? t("warp.credentials_loaded") : t("warp.no_credentials")}</p>
          ) : null}
          {!selectedNodeID ? <p className="state-text">{t("warp.select_node_hint")}</p> : null}

          {credentials ? (
            <dl className="node-detail-grid">
              <div><dt>{t("warp.public_key")}</dt><dd className="mono-cell">{credentials.public_key}</dd></div>
              <div><dt>{t("warp.address")}</dt><dd>{credentials.address}</dd></div>
              <div><dt>Endpoint</dt><dd>{credentials.endpoint || "-"}</dd></div>
              <div><dt>{t("common.enabled")}</dt><dd>{credentials.enabled ? "yes" : "no"}</dd></div>
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
