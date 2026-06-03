import { useEffect, useMemo, useState } from "react";
import type { MFAFactor } from "../types";
import {
  beginPasskeyEnroll,
  confirmPasskeyEnroll,
  deletePasskey,
} from "../api_passkeys";
import { createPasskeyAttestation } from "../webauthn";
import { Fingerprint, Loader2, Trash2 } from "lucide-react";

interface PasskeysProps {
  factors: MFAFactor[];
  onChanged: () => Promise<void> | void;
  onPasskeyRegistered?: () => void;
}

export function Passkeys({ factors, onChanged, onPasskeyRegistered }: PasskeysProps) {
  const passkeys = useMemo(
    () => factors.filter((factor) => factor.method === "webauthn"),
    [factors],
  );
  const [optimisticPasskeyName, setOptimisticPasskeyName] = useState<string | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [registering, setRegistering] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (!optimisticPasskeyName) {
      return;
    }
    const passkeyWasRefreshed = passkeys.some((factor) => {
      const resolvedName = factor.display_name || factor.label || "Passkey";
      return resolvedName === optimisticPasskeyName;
    });
    if (passkeyWasRefreshed) {
      setOptimisticPasskeyName(null);
    }
  }, [optimisticPasskeyName, passkeys]);

  const visiblePasskeys = useMemo(() => {
    if (!optimisticPasskeyName) {
      return passkeys;
    }
    const passkeyWasRefreshed = passkeys.some((factor) => {
      const resolvedName = factor.display_name || factor.label || "Passkey";
      return resolvedName === optimisticPasskeyName;
    });
    if (passkeyWasRefreshed) {
      return passkeys;
    }
    return [
      {
        id: "__optimistic_passkey__",
        method: "webauthn",
        label: optimisticPasskeyName,
        display_name: optimisticPasskeyName,
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
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to register passkey");
    } finally {
      setRegistering(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    setError(null);
    setSuccess(null);
    try {
      await deletePasskey();
      setOptimisticPasskeyName(null);
      setSuccess("Passkey deleted");
      await onChanged();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to delete passkey");
    } finally {
      setDeleting(false);
    }
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
        {visiblePasskeys.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">No passkeys registered</p>
        ) : (
          <div className="space-y-2">
            {visiblePasskeys.map((factor) => {
              const passkeyName = factor.display_name || factor.label || "Passkey";
              return (
                <div key={factor.id} className="flex items-center justify-between gap-3 p-3 border rounded-lg">
                  <div className="flex items-center gap-3 min-w-0">
                    <Fingerprint className="w-4 h-4 text-blue-500 shrink-0" />
                    <span data-testid="passkey-name" className="text-sm font-medium truncate">
                      {passkeyName}
                    </span>
                  </div>
                  <button
                    type="button"
                    data-testid="passkey-delete-button"
                    onClick={handleDelete}
                    disabled={deleting}
                    className="px-3 py-1.5 text-sm border border-red-200 text-red-700 rounded hover:bg-red-50 disabled:opacity-50 flex items-center gap-2"
                  >
                    {deleting ? (
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
              );
            })}
          </div>
        )}
      </div>
    </section>
  );
}
