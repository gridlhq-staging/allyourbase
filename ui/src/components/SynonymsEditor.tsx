import { useCallback, useEffect, useMemo, useState } from "react";
import { Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import {
  getCollectionSearchSynonyms,
  updateCollectionSearchSynonyms,
  type CollectionSearchSynonymGroup,
  type CollectionSearchSynonymsResponse,
} from "../api_admin";
import type { SchemaCache, Table } from "../types";
import { cn } from "../lib/utils";
import { collectionLabel, selectedHasUnsafeDuplicateName } from "./selected_collection_helpers";

const MAX_TERM_LENGTH = 128;

const PANEL_CLASS = "p-6 max-w-5xl space-y-4";
const CARD_CLASS = "border border-gray-200 dark:border-gray-800 rounded bg-white dark:bg-gray-900";
const BUTTON_CLASS = "inline-flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium transition-colors disabled:opacity-60 disabled:cursor-not-allowed";
const SECONDARY_BUTTON_CLASS = "border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800";
const PRIMARY_BUTTON_CLASS = "bg-gray-900 text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-950 dark:hover:bg-gray-200";
const DANGER_BUTTON_CLASS = "text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-950/30";

interface SynonymsEditorProps {
  selected: Table;
  schema: SchemaCache;
}

type LoadState = "loading" | "loaded" | "error";

interface ValidationResult {
  message: string | null;
  payload: CollectionSearchSynonymsResponse | null;
}

function cloneGroups(groups: CollectionSearchSynonymGroup[]): CollectionSearchSynonymGroup[] {
  return groups.map((group) => ({ terms: [...group.terms] }));
}

function normalizedGroups(groups: CollectionSearchSynonymGroup[]): CollectionSearchSynonymGroup[] {
  return groups.map((group) => ({
    terms: group.terms.map((term) => term.trim()).filter(Boolean),
  }));
}

function groupsEqual(
  left: CollectionSearchSynonymGroup[],
  right: CollectionSearchSynonymGroup[],
): boolean {
  return JSON.stringify(normalizedGroups(left)) === JSON.stringify(normalizedGroups(right));
}

function extractErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function validateDraft(groups: CollectionSearchSynonymGroup[]): ValidationResult {
  const payload = { groups: normalizedGroups(groups) };
  if (payload.groups.length === 0) {
    return {
      message: "Add at least one synonym group before saving.",
      payload: null,
    };
  }

  for (const group of payload.groups) {
    if (group.terms.length < 2) {
      return {
        message: "Each synonym group needs at least two terms.",
        payload: null,
      };
    }
  }

  if (groups.some((group) => group.terms.some((term) => term.trim().length > MAX_TERM_LENGTH))) {
    return {
      message: "Synonym terms must be 128 characters or fewer.",
      payload: null,
    };
  }

  const seenTerms = new Set<string>();
  for (const group of payload.groups) {
    for (const term of group.terms) {
      const normalized = term.toLowerCase();
      if (seenTerms.has(normalized)) {
        return {
          message: "Duplicate synonym terms are not allowed.",
          payload: null,
        };
      }
      seenTerms.add(normalized);
    }
  }

  return { message: null, payload };
}

export function SynonymsEditor({ selected, schema }: SynonymsEditorProps) {
  const label = collectionLabel(selected);
  const unsafeDuplicateName = useMemo(
    () => selectedHasUnsafeDuplicateName(selected, schema),
    [selected, schema],
  );
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [savedGroups, setSavedGroups] = useState<CollectionSearchSynonymGroup[]>([]);
  const [draftGroups, setDraftGroups] = useState<CollectionSearchSynonymGroup[]>([]);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const loadSynonyms = useCallback(async () => {
    if (unsafeDuplicateName) {
      return;
    }
    setLoadState("loading");
    setFetchError(null);
    setSaveError(null);
    setSaveSuccess(null);
    setValidationError(null);
    try {
      const response = await getCollectionSearchSynonyms(selected.name);
      const groups = cloneGroups(response.groups);
      setSavedGroups(groups);
      setDraftGroups(cloneGroups(groups));
      setLoadState("loaded");
    } catch (error) {
      setFetchError(extractErrorMessage(error, "Failed to load synonyms."));
      setLoadState("error");
    }
  }, [selected.name, unsafeDuplicateName]);

  useEffect(() => {
    if (unsafeDuplicateName) {
      setLoadState("loaded");
      setFetchError(null);
      setSaveError(null);
      setSaveSuccess(null);
      setValidationError(null);
      setSavedGroups([]);
      setDraftGroups([]);
      return;
    }
    void loadSynonyms();
  }, [loadSynonyms, unsafeDuplicateName]);

  const changed = !groupsEqual(draftGroups, savedGroups);
  const controlsDisabled = saving || loadState !== "loaded";

  function updateTerm(groupIndex: number, termIndex: number, value: string) {
    setDraftGroups((current) =>
      current.map((group, index) =>
        index === groupIndex
          ? {
              terms: group.terms.map((term, currentTermIndex) =>
                currentTermIndex === termIndex ? value : term,
              ),
            }
          : group,
      ),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  function addGroup() {
    setDraftGroups((current) => [...current, { terms: ["", ""] }]);
    setValidationError(null);
    setSaveSuccess(null);
  }

  function removeGroup(groupIndex: number) {
    setDraftGroups((current) => current.filter((_, index) => index !== groupIndex));
    setValidationError(null);
    setSaveSuccess(null);
  }

  function addTerm(groupIndex: number) {
    setDraftGroups((current) =>
      current.map((group, index) =>
        index === groupIndex ? { terms: [...group.terms, ""] } : group,
      ),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  function removeTerm(groupIndex: number, termIndex: number) {
    setDraftGroups((current) =>
      current.map((group, index) =>
        index === groupIndex
          ? { terms: group.terms.filter((_, currentTermIndex) => currentTermIndex !== termIndex) }
          : group,
      ),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  async function saveDraft() {
    const validation = validateDraft(draftGroups);
    setValidationError(validation.message);
    setSaveError(null);
    setSaveSuccess(null);
    if (!validation.payload) {
      return;
    }

    setSaving(true);
    try {
      const response = await updateCollectionSearchSynonyms(selected.name, validation.payload);
      const groups = cloneGroups(response.groups);
      setSavedGroups(groups);
      setDraftGroups(cloneGroups(groups));
      setValidationError(null);
      setSaveSuccess("Saved search synonyms.");
    } catch (error) {
      setSaveError(extractErrorMessage(error, "Failed to save synonyms."));
    } finally {
      setSaving(false);
    }
  }

  if (unsafeDuplicateName) {
    return (
      <section className={PANEL_CLASS}>
        <EditorHeader label={label} />
        <div className="border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 rounded p-4 text-sm text-amber-900 dark:text-amber-100">
          The unqualified admin route cannot safely target {label}; another exposed collection
          shares the name {selected.name}. This Synonyms tab is read-only to avoid reading or
          overwriting the wrong collection.
        </div>
      </section>
    );
  }

  return (
    <section className={PANEL_CLASS}>
      <EditorHeader label={label} />

      {loadState === "loading" && (
        <div className={cn(CARD_CLASS, "p-4 text-sm text-gray-600 dark:text-gray-300")}>
          Loading synonyms...
        </div>
      )}

      {loadState === "error" && (
        <div className="border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-950/30 rounded p-4 text-sm text-red-800 dark:text-red-100">
          <p className="mb-3">{fetchError}</p>
          <button
            type="button"
            className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
            onClick={loadSynonyms}
          >
            <RefreshCw className="h-4 w-4" />
            Retry
          </button>
        </div>
      )}

      {loadState === "loaded" && (
        <>
          {draftGroups.length === 0 ? (
            <EmptySynonymsState onAddGroup={addGroup} disabled={controlsDisabled} />
          ) : (
            <div className="space-y-3">
              {draftGroups.map((group, groupIndex) => (
                <SynonymGroupEditor
                  key={groupIndex}
                  group={group}
                  groupIndex={groupIndex}
                  disabled={controlsDisabled}
                  onAddTerm={() => addTerm(groupIndex)}
                  onRemoveGroup={() => removeGroup(groupIndex)}
                  onRemoveTerm={(termIndex) => removeTerm(groupIndex, termIndex)}
                  onUpdateTerm={(termIndex, value) => updateTerm(groupIndex, termIndex, value)}
                />
              ))}
            </div>
          )}

          <div className={cn(CARD_CLASS, "p-4 flex flex-wrap items-center gap-3")}>
            {draftGroups.length > 0 && (
              <button
                type="button"
                className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
                onClick={addGroup}
                disabled={controlsDisabled}
              >
                <Plus className="h-4 w-4" />
                Add group
              </button>
            )}
            <button
              type="button"
              className={cn(BUTTON_CLASS, PRIMARY_BUTTON_CLASS, "ml-auto")}
              onClick={saveDraft}
              disabled={!changed || saving}
            >
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save"}
            </button>
          </div>

          {(validationError || saveError) && (
            <div className="border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-950/30 rounded p-3 text-sm text-red-800 dark:text-red-100">
              {validationError || saveError}
            </div>
          )}

          {saveSuccess && (
            <div
              role="status"
              className="border border-green-300 dark:border-green-800 bg-green-50 dark:bg-green-950/30 rounded p-3 text-sm text-green-800 dark:text-green-100"
            >
              {saveSuccess}
            </div>
          )}
        </>
      )}
    </section>
  );
}

function EditorHeader({ label }: { label: string }) {
  return (
    <div>
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
        Search synonyms for {label}
      </h2>
    </div>
  );
}

function EmptySynonymsState({
  disabled,
  onAddGroup,
}: {
  disabled: boolean;
  onAddGroup: () => void;
}) {
  return (
    <div className={cn(CARD_CLASS, "p-6 text-center")}>
      <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
        No synonym groups configured
      </p>
      <button
        type="button"
        className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS, "mt-4")}
        onClick={onAddGroup}
        disabled={disabled}
      >
        <Plus className="h-4 w-4" />
        Add group
      </button>
    </div>
  );
}

interface SynonymGroupEditorProps {
  group: CollectionSearchSynonymGroup;
  groupIndex: number;
  disabled: boolean;
  onAddTerm: () => void;
  onRemoveGroup: () => void;
  onRemoveTerm: (termIndex: number) => void;
  onUpdateTerm: (termIndex: number, value: string) => void;
}

function SynonymGroupEditor({
  group,
  groupIndex,
  disabled,
  onAddTerm,
  onRemoveGroup,
  onRemoveTerm,
  onUpdateTerm,
}: SynonymGroupEditorProps) {
  const groupNumber = groupIndex + 1;
  return (
    <fieldset
      className={cn(CARD_CLASS, "p-4 space-y-3")}
      aria-label={`Synonym group ${groupNumber}`}
    >
      <div className="flex items-center justify-between gap-3">
        <legend className="text-sm font-semibold text-gray-900 dark:text-gray-100">
          Group {groupNumber}
        </legend>
        <button
          type="button"
          className={cn(BUTTON_CLASS, DANGER_BUTTON_CLASS)}
          onClick={onRemoveGroup}
          disabled={disabled}
          aria-label={`Remove synonym group ${groupNumber}`}
        >
          <Trash2 className="h-4 w-4" />
          Remove group
        </button>
      </div>

      <div className="space-y-2">
        {group.terms.map((term, termIndex) => {
          const termNumber = termIndex + 1;
          const removeLabel = term.trim()
            ? `Remove term ${term.trim()}`
            : `Remove term ${termNumber} in group ${groupNumber}`;
          return (
            <div key={termIndex} className="flex items-center gap-2">
              <label className="sr-only" htmlFor={`synonym-${groupIndex}-${termIndex}`}>
                Term {termNumber} in group {groupNumber}
              </label>
              <input
                id={`synonym-${groupIndex}-${termIndex}`}
                value={term}
                onChange={(event) => onUpdateTerm(termIndex, event.target.value)}
                disabled={disabled}
                className="min-w-0 flex-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-100"
              />
              <button
                type="button"
                className={cn(BUTTON_CLASS, DANGER_BUTTON_CLASS, "px-2")}
                onClick={() => onRemoveTerm(termIndex)}
                disabled={disabled}
                aria-label={removeLabel}
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          );
        })}
      </div>

      <button
        type="button"
        className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
        onClick={onAddTerm}
        disabled={disabled}
      >
        <Plus className="h-4 w-4" />
        Add term
      </button>
    </fieldset>
  );
}
