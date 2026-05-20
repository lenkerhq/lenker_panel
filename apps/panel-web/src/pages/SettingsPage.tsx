import { useCallback, useEffect, useState } from "react";
import { listSettings, PanelApiError, updateSetting, type GlobalSetting } from "../lib/api";
import type { StoredSession } from "../lib/session";

interface SettingsPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

interface SettingsFormState {
  default_routing_preset: string;
  enable_warp_outbound: boolean;
  default_sniffing: boolean;
  default_fragment: string;
  default_log_level: string;
  default_dns_servers: string;
}

function settingsToForm(settings: GlobalSetting[]): SettingsFormState {
  const get = (key: string) => settings.find((s) => s.key === key)?.value;
  const dns = get("default_dns_servers");
  return {
    default_routing_preset: String(get("default_routing_preset") ?? "standard"),
    enable_warp_outbound: Boolean(get("enable_warp_outbound") ?? false),
    default_sniffing: Boolean(get("default_sniffing") ?? false),
    default_fragment: get("default_fragment") != null ? String(get("default_fragment")) : "",
    default_log_level: String(get("default_log_level") ?? "info"),
    default_dns_servers: Array.isArray(dns) ? JSON.stringify(dns) : "[]",
  };
}

export function SettingsPage({ session, onUnauthorized }: SettingsPageProps) {
  const [settings, setSettings] = useState<GlobalSetting[]>([]);
  const [formState, setFormState] = useState<SettingsFormState>(() => settingsToForm([]));
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const loadSettings = useCallback(async () => {
    setIsLoading(true);
    setErrorMessage(null);
    try {
      const loaded = await listSettings(session);
      setSettings(loaded);
      setFormState(settingsToForm(loaded));
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, "Unable to load settings."));
    } finally {
      setIsLoading(false);
    }
  }, [session, onUnauthorized]);

  useEffect(() => { loadSettings(); }, [loadSettings]);

  async function saveSetting(key: string, value: unknown) {
    setSavingKey(key);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await updateSetting(session, key, value);
      setSuccessMessage(`Setting "${key}" updated.`);
      await loadSettings();
    } catch (error) {
      if (error instanceof PanelApiError && error.status === 401) { onUnauthorized(); return; }
      setErrorMessage(formatError(error, `Unable to update "${key}".`));
    } finally {
      setSavingKey(null);
    }
  }

  function saveRoutingPreset() { saveSetting("default_routing_preset", formState.default_routing_preset); }
  function saveWarpOutbound() { saveSetting("enable_warp_outbound", formState.enable_warp_outbound); }
  function saveSniffing() { saveSetting("default_sniffing", formState.default_sniffing); }
  function saveFragment() { saveSetting("default_fragment", formState.default_fragment.trim() || null); }
  function saveLogLevel() { saveSetting("default_log_level", formState.default_log_level); }
  function saveDnsServers() {
    try {
      const parsed = JSON.parse(formState.default_dns_servers);
      if (!Array.isArray(parsed)) { setErrorMessage("DNS servers must be a JSON array."); return; }
      saveSetting("default_dns_servers", parsed);
    } catch {
      setErrorMessage("Invalid JSON for DNS servers.");
    }
  }

  const descriptionFor = (key: string) => settings.find((s) => s.key === key)?.description ?? null;

  return (
    <div className="page-stack" id="settings">
      <section className="page-header">
        <div>
          <p className="eyebrow">Configuration</p>
          <h2>Global Settings</h2>
          <p>Manage global configuration settings for the panel and nodes.</p>
        </div>
      </section>

      {isLoading ? <p className="state-text">Loading settings...</p> : null}
      {errorMessage ? <p className="error-text">{errorMessage}</p> : null}
      {successMessage ? <p className="success-text">{successMessage}</p> : null}

      <section className="management-panel">
        <SettingRow label="Default routing preset" description={descriptionFor("default_routing_preset")} saving={savingKey === "default_routing_preset"} onSave={saveRoutingPreset}>
          <select className="select-field" value={formState.default_routing_preset} onChange={(e) => setFormState((c) => ({ ...c, default_routing_preset: e.target.value }))}>
            <option value="standard">standard</option>
            <option value="strict">strict</option>
            <option value="permissive">permissive</option>
          </select>
        </SettingRow>

        <SettingRow label="Enable WARP outbound" description={descriptionFor("enable_warp_outbound")} saving={savingKey === "enable_warp_outbound"} onSave={saveWarpOutbound}>
          <label className="check-row" htmlFor="setting-warp">
            <input id="setting-warp" type="checkbox" checked={formState.enable_warp_outbound} onChange={(e) => setFormState((c) => ({ ...c, enable_warp_outbound: e.target.checked }))} />
            <span>{formState.enable_warp_outbound ? "Enabled" : "Disabled"}</span>
          </label>
        </SettingRow>

        <SettingRow label="Default sniffing" description={descriptionFor("default_sniffing")} saving={savingKey === "default_sniffing"} onSave={saveSniffing}>
          <label className="check-row" htmlFor="setting-sniffing">
            <input id="setting-sniffing" type="checkbox" checked={formState.default_sniffing} onChange={(e) => setFormState((c) => ({ ...c, default_sniffing: e.target.checked }))} />
            <span>{formState.default_sniffing ? "Enabled" : "Disabled"}</span>
          </label>
        </SettingRow>

        <SettingRow label="Default fragment" description={descriptionFor("default_fragment")} saving={savingKey === "default_fragment"} onSave={saveFragment}>
          <input className="text-field" type="text" placeholder="Empty = disabled" value={formState.default_fragment} onChange={(e) => setFormState((c) => ({ ...c, default_fragment: e.target.value }))} />
        </SettingRow>

        <SettingRow label="Default log level" description={descriptionFor("default_log_level")} saving={savingKey === "default_log_level"} onSave={saveLogLevel}>
          <select className="select-field" value={formState.default_log_level} onChange={(e) => setFormState((c) => ({ ...c, default_log_level: e.target.value }))}>
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warning">warning</option>
            <option value="error">error</option>
          </select>
        </SettingRow>

        <SettingRow label="Default DNS servers" description={descriptionFor("default_dns_servers")} saving={savingKey === "default_dns_servers"} onSave={saveDnsServers}>
          <textarea className="text-field" rows={3} placeholder='["1.1.1.1", "8.8.8.8"]' value={formState.default_dns_servers} onChange={(e) => setFormState((c) => ({ ...c, default_dns_servers: e.target.value }))} />
        </SettingRow>
      </section>
    </div>
  );
}

interface SettingRowProps {
  label: string;
  description: string | null;
  saving: boolean;
  onSave: () => void;
  children: React.ReactNode;
}

function SettingRow({ label, description, saving, onSave, children }: SettingRowProps) {
  return (
    <div className="setting-row">
      <div className="section-heading compact-heading">
        <div>
          <label className="field-label">{label}</label>
          {description ? <p className="muted-text">{description}</p> : null}
        </div>
        <button className="table-button" type="button" onClick={onSave} disabled={saving}>
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
      {children}
    </div>
  );
}

function formatError(error: unknown, fallback: string): string {
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallback;
}
