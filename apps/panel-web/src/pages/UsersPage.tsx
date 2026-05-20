import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  activateUser,
  createUser,
  listUsers,
  PanelApiError,
  suspendUser,
  updateUser,
  type User,
} from "../lib/api";
import { Modal } from "../components/Modal";
import type { StoredSession } from "../lib/session";
import {
  buildCreateUserInput,
  buildUpdateUserInput,
  emptyUserForm,
  userToForm,
  validateUserForm,
  type UserFormState,
} from "../lib/userForm";

interface UsersPageProps {
  session: StoredSession;
  onUnauthorized: () => void;
}

type LoadState = "idle" | "loading" | "loaded" | "failed";

export function UsersPage({ session, onUnauthorized }: UsersPageProps) {
  const [users, setUsers] = useState<User[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const [createFormState, setCreateFormState] = useState<UserFormState>(() => emptyUserForm());
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [editFormState, setEditFormState] = useState<UserFormState>(() => emptyUserForm());
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutatingUserID, setMutatingUserID] = useState<string | null>(null);

  const activeUsers = useMemo(() => users.filter((user) => user.status === "active").length, [users]);

  const loadUsers = useCallback(async () => {
    setLoadState("loading");
    setErrorMessage(null);
    try {
      const loaded = await listUsers(session);
      setUsers(loaded);
      setLoadState("loaded");
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to load users."));
      setLoadState("failed");
    }
  }, [onUnauthorized, session]);

  useEffect(() => {
    let isMounted = true;
    async function load() {
      setLoadState("loading");
      setErrorMessage(null);
      try {
        const loaded = await listUsers(session);
        if (!isMounted) return;
        setUsers(loaded);
        setLoadState("loaded");
      } catch (error) {
        if (!isMounted) return;
        if (handleUnauthorizedError(error, onUnauthorized)) return;
        setErrorMessage(formatPanelError(error, "Unable to load users."));
        setLoadState("failed");
      }
    }
    load();
    return () => { isMounted = false; };
  }, [onUnauthorized, session]);

  function openEdit(user: User) {
    setEditingUser(user);
    setEditFormState(userToForm(user));
    setErrorMessage(null);
    setSuccessMessage(null);
  }

  function closeEdit() {
    setEditingUser(null);
    setEditFormState(emptyUserForm());
  }

  async function submitCreateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validationError = validateUserForm(createFormState);
    if (validationError) { setErrorMessage(validationError); setSuccessMessage(null); return; }

    setIsMutating(true);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      await createUser(session, buildCreateUserInput(createFormState));
      setCreateFormState(emptyUserForm());
      setSuccessMessage("User created.");
      await loadUsers();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to create user."));
    } finally {
      setIsMutating(false);
    }
  }

  async function submitEditForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingUser) return;
    const validationError = validateUserForm(editFormState);
    if (validationError) { setErrorMessage(validationError); return; }

    setIsMutating(true);
    setErrorMessage(null);
    try {
      await updateUser(session, editingUser.id, buildUpdateUserInput(editFormState));
      closeEdit();
      setSuccessMessage("User updated.");
      await loadUsers();
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to update user."));
    } finally {
      setIsMutating(false);
    }
  }

  async function updateUserStatus(user: User, action: "suspend" | "activate") {
    setMutatingUserID(user.id);
    setErrorMessage(null);
    setSuccessMessage(null);
    try {
      if (action === "suspend") {
        await suspendUser(session, user.id);
        setSuccessMessage("User suspended.");
      } else {
        await activateUser(session, user.id);
        setSuccessMessage("User activated.");
      }
      const loaded = await listUsers(session);
      setUsers(loaded);
      const updated = loaded.find((u) => u.id === user.id);
      if (updated && editingUser?.id === user.id) {
        setEditingUser(updated);
      }
    } catch (error) {
      if (handleUnauthorizedError(error, onUnauthorized)) return;
      setErrorMessage(formatPanelError(error, "Unable to update user status."));
    } finally {
      setMutatingUserID(null);
    }
  }

  return (
    <div className="page-stack" id="users">
      <section className="page-header">
        <div>
          <p className="eyebrow">Users</p>
          <h2>Users</h2>
          <p>Create, edit, suspend, and activate provider users through the panel-api admin API.</p>
        </div>
        <div className="header-actions">
          <span className="pill">{users.length} total</span>
          <span className="pill">{activeUsers} active</span>
        </div>
      </section>

      <section className="management-grid">
        <form className="user-form-panel" onSubmit={submitCreateForm}>
          <div className="section-heading">
            <div>
              <p className="eyebrow">New user</p>
              <h3>Create user</h3>
            </div>
          </div>

          <label className="field-label" htmlFor="user-email">Email</label>
          <input id="user-email" className="text-field" type="email" autoComplete="off" value={createFormState.email} onChange={(e) => setCreateFormState((c) => ({ ...c, email: e.target.value }))} />

          <label className="field-label" htmlFor="user-display-name">Display name</label>
          <input id="user-display-name" className="text-field" type="text" autoComplete="off" value={createFormState.displayName} onChange={(e) => setCreateFormState((c) => ({ ...c, displayName: e.target.value }))} />

          <button className="primary-button" type="submit" disabled={isMutating}>
            {isMutating ? "Saving..." : "Create user"}
          </button>
        </form>

        <div className="users-feedback-panel">
          <p className="eyebrow">State</p>
          {loadState === "loading" ? <p className="state-text">Loading users...</p> : null}
          {loadState === "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {loadState === "loaded" && !errorMessage && !successMessage ? <p className="state-text">Users list is ready.</p> : null}
          {errorMessage && loadState !== "failed" ? <p className="error-text">{errorMessage}</p> : null}
          {successMessage ? <p className="success-text">{successMessage}</p> : null}
          <button className="secondary-button" type="button" onClick={loadUsers} disabled={loadState === "loading"}>Refresh</button>
        </div>
      </section>

      {loadState === "loaded" && users.length === 0 ? <p className="state-card">No users yet. Create the first user above.</p> : null}

      {users.length > 0 ? (
        <div className="table-wrap">
          <table className="data-table users-table">
            <thead>
              <tr>
                <th>Email</th>
                <th>Display name</th>
                <th>Status</th>
                <th>ID</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id} className="clickable-row" onClick={() => openEdit(user)}>
                  <td>{user.email}</td>
                  <td>{user.display_name || "-"}</td>
                  <td>
                    <span className={`status-badge status-${user.status}`}>{user.status}</span>
                  </td>
                  <td className="mono-cell">{user.id}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="row-actions">
                      {user.status === "active" ? (
                        <button className="table-button danger" type="button" onClick={() => updateUserStatus(user, "suspend")} disabled={mutatingUserID === user.id}>
                          {mutatingUserID === user.id ? "Suspending..." : "Suspend"}
                        </button>
                      ) : (
                        <button className="table-button" type="button" onClick={() => updateUserStatus(user, "activate")} disabled={mutatingUserID === user.id}>
                          {mutatingUserID === user.id ? "Activating..." : "Activate"}
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Modal isOpen={editingUser !== null} onClose={closeEdit} title={editingUser ? `Edit ${editingUser.email}` : ""} size="small">
        {editingUser ? (
          <form onSubmit={submitEditForm}>
            <label className="field-label" htmlFor="user-edit-email">Email</label>
            <input id="user-edit-email" className="text-field" type="email" autoComplete="off" value={editFormState.email} onChange={(e) => setEditFormState((c) => ({ ...c, email: e.target.value }))} />

            <label className="field-label" htmlFor="user-edit-display-name">Display name</label>
            <input id="user-edit-display-name" className="text-field" type="text" autoComplete="off" value={editFormState.displayName} onChange={(e) => setEditFormState((c) => ({ ...c, displayName: e.target.value }))} />

            <div className="check-row">
              <span>Status:</span>
              <span className={`status-badge status-${editingUser.status}`}>{editingUser.status}</span>
            </div>

            {errorMessage ? <p className="error-text">{errorMessage}</p> : null}

            <div className="row-actions" style={{ marginTop: 22 }}>
              <button className="primary-button" type="submit" disabled={isMutating} style={{ width: "auto", marginTop: 0 }}>
                {isMutating ? "Saving..." : "Update"}
              </button>
              {editingUser.status === "active" ? (
                <button className="table-button danger" type="button" onClick={() => updateUserStatus(editingUser, "suspend")} disabled={mutatingUserID === editingUser.id}>
                  {mutatingUserID === editingUser.id ? "Suspending..." : "Suspend"}
                </button>
              ) : (
                <button className="table-button" type="button" onClick={() => updateUserStatus(editingUser, "activate")} disabled={mutatingUserID === editingUser.id}>
                  {mutatingUserID === editingUser.id ? "Activating..." : "Activate"}
                </button>
              )}
              <button className="ghost-button" type="button" onClick={closeEdit} disabled={isMutating}>Cancel</button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
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
  if (error instanceof PanelApiError) return `${error.message} (${error.code})`;
  return error instanceof Error ? error.message : fallbackMessage;
}
