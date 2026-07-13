import { useCallback, useEffect, useMemo, useState } from "react";
import type { MFAFactor } from "../types";
import {
  beginPasskeyEnroll,
  confirmPasskeyEnroll,
  deletePasskey,
  listPasskeys,
  renamePasskey,
} from "../api_passkeys";
import type { PasskeyCredentialMetadata } from "../api_passkeys";
import { createPasskeyAttestation } from "../webauthn";
import { Fingerprint, Loader2, Save, Trash2 } from "lucide-react";
import { ConfirmDialog } from "./shared/ConfirmDialog";

interface PasskeysProps {
  factors: MFAFactor[];
  onChanged: () => Promise<void> | void;
  onPasskeyRegistered?: () => void;
}

type VisiblePasskey = PasskeyCredentialMetadata & {
  optimistic?: boolean;
};

type DeleteTarget = {
  credentialId: string;
  displayName: string;
};

function formatCredentialDate(value: string): string {
  if (!value) {
    return "Unknown";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date);
}

function requestErrorMessage(requestError: unknown, fallback: string): string {
  return requestError instanceof Error ? requestError.message : fallback;
}

export function Passkeys({ factors, onChanged, onPasskeyRegistered }: PasskeysProps) {
  const passkeyRefreshKey = useMemo(
    () =>
      factors
        .filter((factor) => factor.method === "webauthn")
        .map((factor) => `${factor.id}:${factor.display_name ?? ""}:${factor.label ?? ""}`)
        .join("|"),
    [factors],
  );
  const [passkeys, setPasskeys] = useState<PasskeyCredentialMetadata[]>([]);
  const [optimisticPasskeyName, setOptimisticPasskeyName] = useState<string | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [registering, setRegistering] = useState(false);
  const [renameDrafts, setRenameDrafts] = useState<Record<string, string>>({});
  const [renamingCredentialId, setRenamingCredentialId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [deletingCredentialId, setDeletingCredentialId] = useState<string | null>(null);
  const [loadingPasskeys, setLoadingPasskeys] = useState(true);

  const refreshPasskeys = useCallback(async (options?: { background?: boolean }) => {
    const background = options?.background === true;
    if (!background) {
      setLoadingPasskeys(true);
    }
    try {
      const credentials = await listPasskeys();
      setPasskeys(credentials);
      setRenameDrafts(
        Object.fromEntries(
          credentials.map((credential) => [credential.credentialId, credential.displayName]),
        ),
      );
    } catch (requestError) {
      setError(requestErrorMessage(requestError, "Failed to load passkeys"));
    } finally {
      if (!background) {
        setLoadingPasskeys(false);
      }
    }
  }, []);

  useEffect(() => {
    void refreshPasskeys();
  }, [refreshPasskeys, passkeyRefreshKey]);

  useEffect(() => {
    if (
      optimisticPasskeyName &&
      passkeys.some((passkey) => passkey.displayName === optimisticPasskeyName)
    ) {
      setOptimisticPasskeyName(null);
    }
  }, [optimisticPasskeyName, passkeys]);

  const visiblePasskeys = useMemo<VisiblePasskey[]>(() => {
    if (!optimisticPasskeyName) {
      return passkeys;
    }
    if (passkeys.some((passkey) => passkey.displayName === optimisticPasskeyName)) {
      return passkeys;
    }
    return [
      {
        credentialId: "",
        displayName: optimisticPasskeyName,
        transports: [],
        createdAt: "",
        optimistic: true,
      },
      ...passkeys,
    ];
  }, [optimisticPasskeyName, passkeys]);

  const handleRegister = async () => {
    const trimmedName = displayName.trim();
    if (!trimmedName) {
      setError("Passkey name is required");
      return;
    }

    setRegistering(true);
    setError(null);
    setSuccess(null);
    try {
      const options = await beginPasskeyEnroll();
      const attestationResponse = await createPasskeyAttestation(options);
      await confirmPasskeyEnroll(trimmedName, attestationResponse);
      setOptimisticPasskeyName(trimmedName);
      setDisplayName("");
      setSuccess(`Passkey "${trimmedName}" registered`);
      onPasskeyRegistered?.();
      await onChanged();
      await refreshPasskeys({ background: true });
    } catch (requestError) {
      setError(requestErrorMessage(requestError, "Failed to register passkey"));
    } finally {
      setRegistering(false);
    }
  };

  const handleRename = async (credentialId: string) => {
    const trimmedName = (renameDrafts[credentialId] ?? "").trim();
    if (!trimmedName) {
      setError("Passkey name is required");
      return;
    }

    setRenamingCredentialId(credentialId);
    setError(null);
    setSuccess(null);
    try {
      await renamePasskey(credentialId, trimmedName);
      setSuccess(`Passkey "${trimmedName}" renamed`);
      await onChanged();
      await refreshPasskeys({ background: true });
    } catch (requestError) {
      setError(requestErrorMessage(requestError, "Failed to rename passkey"));
    } finally {
      setRenamingCredentialId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) {
      return;
    }

    setDeletingCredentialId(deleteTarget.credentialId);
    setError(null);
    setSuccess(null);
    try {
      await deletePasskey(deleteTarget.credentialId);
      setOptimisticPasskeyName(null);
      setSuccess("Passkey deleted");
      setDeleteTarget(null);
      await onChanged();
      await refreshPasskeys({ background: true });
    } catch (requestError) {
      setDeleteTarget(null);
      setError(requestErrorMessage(requestError, "Failed to delete passkey"));
    } finally {
      setDeletingCredentialId(null);
    }
  };

  const updateRenameDraft = (credentialId: string, value: string) => {
    setRenameDrafts((current) => ({ ...current, [credentialId]: value }));
  };

  return (
    <section className="p-4 border rounded-lg space-y-4">
      <div>
        <h3 className="text-sm font-semibold">Passkeys</h3>
        <p className="text-sm text-gray-600 dark:text-gray-300">
          Register a device-backed passkey for WebAuthn MFA step-up challenges.
        </p>
      </div>

      {error && (
        <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-red-800 text-sm">
          {error}
        </div>
      )}
      {success && (
        <div className="px-4 py-2 bg-green-50 border border-green-200 rounded-lg text-green-800 text-sm">
          {success}
        </div>
      )}

      <div className="space-y-2">
        <label htmlFor="passkey-display-name" className="block text-sm font-medium text-gray-700 dark:text-gray-200">
          Passkey name
        </label>
        <div className="flex flex-wrap gap-2">
          <input
            id="passkey-display-name"
            data-testid="passkey-display-name-input"
            type="text"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="MacBook Touch ID"
            className="flex-1 min-w-[16rem] px-3 py-2 border rounded text-sm"
          />
          <button
            type="button"
            data-testid="passkey-register-button"
            onClick={handleRegister}
            disabled={registering}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 flex items-center gap-2"
          >
            {registering ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Registering...
              </>
            ) : (
              "Register Passkey"
            )}
          </button>
        </div>
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-gray-700 dark:text-gray-200">Registered passkeys</h4>
        {loadingPasskeys ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading passkeys...</p>
        ) : visiblePasskeys.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">No passkeys registered</p>
        ) : (
          <div className="space-y-2">
            {visiblePasskeys.map((passkey) => {
              const draft = renameDrafts[passkey.credentialId] ?? passkey.displayName;
              const isRenaming = renamingCredentialId === passkey.credentialId;
              const isDeleting = deletingCredentialId === passkey.credentialId;
              return (
                <div
                  key={passkey.optimistic ? `optimistic:${passkey.displayName}` : passkey.credentialId}
                  data-testid={passkey.optimistic ? "passkey-row-optimistic" : `passkey-row-${passkey.credentialId}`}
                  className="flex flex-col gap-3 p-3 border rounded-lg sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <Fingerprint className="w-4 h-4 text-blue-500 shrink-0" />
                    <div className="min-w-0">
                      <span data-testid="passkey-name" className="block text-sm font-medium truncate">
                        {passkey.displayName || "Passkey"}
                      </span>
                      <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                        <span>Created {formatCredentialDate(passkey.createdAt)}</span>
                        {passkey.lastUsedAt && (
                          <span>Last used {formatCredentialDate(passkey.lastUsedAt)}</span>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2 sm:justify-end">
                    <input
                      data-testid="passkey-rename-input"
                      type="text"
                      value={draft}
                      onChange={(event) => updateRenameDraft(passkey.credentialId, event.target.value)}
                      disabled={passkey.optimistic || isRenaming}
                      aria-label={`Rename ${passkey.displayName || "passkey"}`}
                      className="min-w-[12rem] flex-1 px-3 py-1.5 border rounded text-sm sm:flex-none"
                    />
                    <button
                      type="button"
                      data-testid="passkey-rename-button"
                      onClick={() => handleRename(passkey.credentialId)}
                      disabled={passkey.optimistic || isRenaming || !draft.trim()}
                      className="px-3 py-1.5 text-sm border border-blue-200 text-blue-700 rounded hover:bg-blue-50 disabled:opacity-50 flex items-center gap-2"
                    >
                      {isRenaming ? (
                        <>
                          <Loader2 className="w-4 h-4 animate-spin" />
                          Saving...
                        </>
                      ) : (
                        <>
                          <Save className="w-4 h-4" />
                          Save
                        </>
                      )}
                    </button>
                    <button
                      type="button"
                      data-testid="passkey-delete-button"
                      onClick={() =>
                        setDeleteTarget({
                          credentialId: passkey.credentialId,
                          displayName: passkey.displayName || "Passkey",
                        })
                      }
                      disabled={isDeleting || passkey.optimistic}
                      className="px-3 py-1.5 text-sm border border-red-200 text-red-700 rounded hover:bg-red-50 disabled:opacity-50 flex items-center gap-2"
                    >
                      {isDeleting ? (
                        <>
                          <Loader2 className="w-4 h-4 animate-spin" />
                          Deleting...
                        </>
                      ) : (
                        <>
                          <Trash2 className="w-4 h-4" />
                          Delete
                        </>
                      )}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete passkey"
        message={`Delete ${deleteTarget?.displayName ?? "this passkey"}? This removes that credential from MFA.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={deletingCredentialId !== null}
      />
    </section>
  );
}
