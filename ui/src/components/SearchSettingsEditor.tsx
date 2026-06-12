import { useCallback, useEffect, useMemo, useState } from "react";
import { Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import {
  getCollectionSearchSettings,
  updateCollectionSearchSettings,
  type CollectionSearchSettingsAttribute,
  type CollectionSearchSettingsCustomRanking,
  type CollectionSearchSettingsPayload,
  type CollectionSearchRankingOrder,
  type CollectionSearchWeight,
} from "../api_admin";
import type { Column, SchemaCache, Table } from "../types";
import { cn } from "../lib/utils";
import { collectionLabel, selectedHasUnsafeDuplicateName } from "./selected_collection_helpers";
import { SearchCustomRankingEditor } from "./SearchCustomRankingEditor";

const MAX_SEARCHABLE_ATTRIBUTES = 32;
const MAX_CUSTOM_RANKING = 32;
const SEARCHABLE_TEXT_TYPES = new Set([
  "text",
  "varchar",
  "character varying",
  "char",
  "character",
  "name",
  "citext",
]);

const WEIGHT_OPTIONS: { value: CollectionSearchWeight; label: string }[] = [
  { value: "high", label: "High" },
  { value: "medium", label: "Medium" },
  { value: "low", label: "Low" },
  { value: "lowest", label: "Lowest" },
];

const PANEL_CLASS = "p-6 max-w-5xl space-y-4";
const CARD_CLASS = "border border-gray-200 dark:border-gray-800 rounded bg-white dark:bg-gray-900";
const BUTTON_CLASS = "inline-flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium transition-colors disabled:opacity-60 disabled:cursor-not-allowed";
const SECONDARY_BUTTON_CLASS = "border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800";
const PRIMARY_BUTTON_CLASS = "bg-gray-900 text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-950 dark:hover:bg-gray-200";
const DANGER_BUTTON_CLASS = "text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-950/30";

interface SearchSettingsEditorProps {
  selected: Table;
  schema: SchemaCache;
}

type LoadState = "loading" | "loaded" | "error";

interface ValidationResult {
  message: string | null;
  payload: CollectionSearchSettingsPayload | null;
}

function cloneAttributes(
  attributes: CollectionSearchSettingsAttribute[],
): CollectionSearchSettingsAttribute[] {
  return attributes.map((attribute) => ({ ...attribute }));
}

function cloneCustomRanking(
  ranking: CollectionSearchSettingsCustomRanking[],
): CollectionSearchSettingsCustomRanking[] {
  return ranking.map((item) => ({ ...item }));
}

function normalizeColumnType(type: string): string {
  const lower = type.toLowerCase();
  const parameterIndex = lower.indexOf("(");
  return (parameterIndex >= 0 ? lower.slice(0, parameterIndex) : lower).trim();
}

function isSearchableTextColumn(column: Column): boolean {
  return SEARCHABLE_TEXT_TYPES.has(normalizeColumnType(column.type));
}

function eligibleAttributeColumns(table: Table): Column[] {
  return table.columns.filter(isSearchableTextColumn);
}

function isRankableColumn(column: Column): boolean {
  if (column.jsonType === "array") {
    return false;
  }
  if (column.enumValues && column.enumValues.length > 0) {
    return false;
  }
  const normalizedType = normalizeColumnType(column.type);
  const isGeometryColumn = normalizedType === "geometry" || normalizedType === "geography";
  return column.jsonType !== "object" || isGeometryColumn;
}

function eligibleCustomRankingColumns(table: Table): Column[] {
  return table.columns.filter(isRankableColumn);
}

function normalizeAttributes(
  attributes: CollectionSearchSettingsAttribute[],
): CollectionSearchSettingsAttribute[] {
  return attributes.map((attribute) => ({
    column: attribute.column.trim(),
    weight: attribute.weight,
  }));
}

function attributesEqual(
  left: CollectionSearchSettingsAttribute[],
  right: CollectionSearchSettingsAttribute[],
): boolean {
  return JSON.stringify(normalizeAttributes(left)) === JSON.stringify(normalizeAttributes(right));
}

function normalizeCustomRanking(
  ranking: CollectionSearchSettingsCustomRanking[],
): CollectionSearchSettingsCustomRanking[] {
  return ranking.map((item) => ({
    column: item.column.trim(),
    order: item.order,
  }));
}

function customRankingEqual(
  left: CollectionSearchSettingsCustomRanking[],
  right: CollectionSearchSettingsCustomRanking[],
): boolean {
  return JSON.stringify(normalizeCustomRanking(left)) === JSON.stringify(normalizeCustomRanking(right));
}

function extractErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function validateDraft(
  draftAttributes: CollectionSearchSettingsAttribute[],
  eligibleColumns: Column[],
  draftCustomRanking: CollectionSearchSettingsCustomRanking[],
  rankableColumns: Column[],
): ValidationResult {
  const allowedColumns = new Set(eligibleColumns.map((column) => column.name));
  const attributes = normalizeAttributes(draftAttributes);
  if (attributes.length === 0) {
    return { message: "Add at least one searchable attribute before saving.", payload: null };
  }
  if (attributes.length > MAX_SEARCHABLE_ATTRIBUTES) {
    return { message: "Search settings support up to 32 searchable attributes.", payload: null };
  }
  const seenColumns = new Set<string>();
  for (const attribute of attributes) {
    if (!allowedColumns.has(attribute.column)) {
      return {
        message: "Choose real searchable text columns for every attribute.",
        payload: null,
      };
    }
    if (seenColumns.has(attribute.column)) {
      return {
        message: "Searchable attributes cannot repeat the same column.",
        payload: null,
      };
    }
    seenColumns.add(attribute.column);
  }

  const customRanking = normalizeCustomRanking(draftCustomRanking);
  if (customRanking.length > MAX_CUSTOM_RANKING) {
    return { message: "Search settings support up to 32 custom ranking entries.", payload: null };
  }
  const allowedRankable = new Set(rankableColumns.map((column) => column.name));
  const seenRanking = new Set<string>();
  for (const item of customRanking) {
    if (!allowedRankable.has(item.column)) {
      return { message: "Choose rankable columns for every custom ranking entry.", payload: null };
    }
    if (seenRanking.has(item.column)) {
      return { message: "Custom ranking cannot repeat the same column.", payload: null };
    }
    if (item.order !== "asc" && item.order !== "desc") {
      return { message: "Custom ranking order must be ascending or descending.", payload: null };
    }
    seenRanking.add(item.column);
  }

  return { message: null, payload: { attributes, customRanking } };
}

function firstAvailableColumn(
  eligibleColumns: Column[],
  attributes: CollectionSearchSettingsAttribute[],
): string {
  const used = new Set(normalizeAttributes(attributes).map((attribute) => attribute.column));
  return eligibleColumns.find((column) => !used.has(column.name))?.name ?? eligibleColumns[0]?.name ?? "";
}

function firstAvailableRankingColumn(
  rankableColumns: Column[],
  ranking: CollectionSearchSettingsCustomRanking[],
): string {
  const used = new Set(normalizeCustomRanking(ranking).map((item) => item.column));
  return rankableColumns.find((column) => !used.has(column.name))?.name ?? rankableColumns[0]?.name ?? "";
}

export function SearchSettingsEditor({ selected, schema }: SearchSettingsEditorProps) {
  const label = collectionLabel(selected);
  const unsafeDuplicateName = useMemo(
    () => selectedHasUnsafeDuplicateName(selected, schema),
    [selected, schema],
  );
  const eligibleColumns = useMemo(() => eligibleAttributeColumns(selected), [selected]);
  const rankableColumns = useMemo(() => eligibleCustomRankingColumns(selected), [selected]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [savedAttributes, setSavedAttributes] = useState<CollectionSearchSettingsAttribute[]>([]);
  const [draftAttributes, setDraftAttributes] = useState<CollectionSearchSettingsAttribute[]>([]);
  const [savedCustomRanking, setSavedCustomRanking] = useState<
    CollectionSearchSettingsCustomRanking[]
  >([]);
  const [draftCustomRanking, setDraftCustomRanking] = useState<
    CollectionSearchSettingsCustomRanking[]
  >([]);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const loadSettings = useCallback(async () => {
    if (unsafeDuplicateName) {
      return;
    }
    setLoadState("loading");
    setFetchError(null);
    setSaveError(null);
    setSaveSuccess(null);
    setValidationError(null);
    try {
      const response = await getCollectionSearchSettings(selected.name);
      const attributes = cloneAttributes(response.attributes);
      setSavedAttributes(attributes);
      setDraftAttributes(cloneAttributes(attributes));
      const ranking = cloneCustomRanking(response.customRanking);
      setSavedCustomRanking(ranking);
      setDraftCustomRanking(cloneCustomRanking(ranking));
      setLoadState("loaded");
    } catch (error) {
      setFetchError(extractErrorMessage(error, "Failed to load search settings."));
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
      setSavedAttributes([]);
      setDraftAttributes([]);
      setSavedCustomRanking([]);
      setDraftCustomRanking([]);
      return;
    }
    void loadSettings();
  }, [loadSettings, unsafeDuplicateName]);

  const changed = !attributesEqual(draftAttributes, savedAttributes) || !customRankingEqual(draftCustomRanking, savedCustomRanking);
  const controlsDisabled = saving || loadState !== "loaded";

  function updateAttributeColumn(attributeIndex: number, column: string) {
    setDraftAttributes((current) =>
      current.map((attribute, index) =>
        index === attributeIndex ? { ...attribute, column } : attribute,
      ),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  function updateAttributeWeight(attributeIndex: number, weight: CollectionSearchWeight) {
    setDraftAttributes((current) =>
      current.map((attribute, index) =>
        index === attributeIndex ? { ...attribute, weight } : attribute,
      ),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  function addAttribute() {
    setDraftAttributes((current) => [
      ...current,
      { column: firstAvailableColumn(eligibleColumns, current), weight: "medium" },
    ]);
    setValidationError(null);
    setSaveSuccess(null);
  }

  function removeAttribute(attributeIndex: number) {
    setDraftAttributes((current) => current.filter((_, index) => index !== attributeIndex));
    setValidationError(null);
    setSaveSuccess(null);
  }

  function addRanking() {
    setDraftCustomRanking((current) => [
      ...current,
      { column: firstAvailableRankingColumn(rankableColumns, current), order: "desc" as CollectionSearchRankingOrder },
    ]);
    setValidationError(null);
    setSaveSuccess(null);
  }

  function removeRanking(rowIndex: number) {
    setDraftCustomRanking((current) => current.filter((_, index) => index !== rowIndex));
    setValidationError(null);
    setSaveSuccess(null);
  }

  function updateRankingColumn(rowIndex: number, column: string) {
    setDraftCustomRanking((current) =>
      current.map((item, index) => (index === rowIndex ? { ...item, column } : item)),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  function updateRankingOrder(rowIndex: number, order: CollectionSearchRankingOrder) {
    setDraftCustomRanking((current) =>
      current.map((item, index) => (index === rowIndex ? { ...item, order } : item)),
    );
    setValidationError(null);
    setSaveSuccess(null);
  }

  async function saveDraft() {
    const validation = validateDraft(draftAttributes, eligibleColumns, draftCustomRanking, rankableColumns);
    setValidationError(validation.message);
    setSaveError(null);
    setSaveSuccess(null);
    if (!validation.payload) {
      return;
    }

    setSaving(true);
    try {
      const response = await updateCollectionSearchSettings(selected.name, validation.payload);
      const attributes = cloneAttributes(response.attributes);
      setSavedAttributes(attributes);
      setDraftAttributes(cloneAttributes(attributes));
      const ranking = cloneCustomRanking(response.customRanking);
      setSavedCustomRanking(ranking);
      setDraftCustomRanking(cloneCustomRanking(ranking));
      setValidationError(null);
      setSaveSuccess("Saved search settings.");
    } catch (error) {
      setSaveError(extractErrorMessage(error, "Failed to save search settings."));
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
          shares the name {selected.name}. This Search settings tab is read-only to avoid reading
          or overwriting the wrong collection.
        </div>
      </section>
    );
  }

  return (
    <section className={PANEL_CLASS}>
      <EditorHeader label={label} />

      {loadState === "loading" && (
        <div className={cn(CARD_CLASS, "p-4 text-sm text-gray-600 dark:text-gray-300")}>
          Loading search settings...
        </div>
      )}

      {loadState === "error" && (
        <div className="border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-950/30 rounded p-4 text-sm text-red-800 dark:text-red-100">
          <p className="mb-3">{fetchError}</p>
          <button
            type="button"
            className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
            onClick={loadSettings}
          >
            <RefreshCw className="h-4 w-4" />
            Retry
          </button>
        </div>
      )}

      {loadState === "loaded" && (
        <>
          <div className="space-y-3">
            {draftAttributes.map((attribute, attributeIndex) => (
              <AttributeEditor
                key={attributeIndex}
                attribute={attribute}
                attributeIndex={attributeIndex}
                disabled={controlsDisabled}
                eligibleColumns={eligibleColumns}
                onRemove={() => removeAttribute(attributeIndex)}
                onUpdateColumn={(column) => updateAttributeColumn(attributeIndex, column)}
                onUpdateWeight={(weight) => updateAttributeWeight(attributeIndex, weight)}
              />
            ))}
          </div>

          <SearchCustomRankingEditor
            ranking={draftCustomRanking}
            eligibleColumns={rankableColumns}
            disabled={controlsDisabled}
            onAdd={addRanking}
            onRemove={removeRanking}
            onUpdateColumn={updateRankingColumn}
            onUpdateOrder={updateRankingOrder}
          />

          <div className={cn(CARD_CLASS, "p-4 flex flex-wrap items-center gap-3")}>
            <button
              type="button"
              className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
              onClick={addAttribute}
              disabled={controlsDisabled || eligibleColumns.length === 0}
            >
              <Plus className="h-4 w-4" />
              Add attribute
            </button>
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
        Search settings for {label}
      </h2>
    </div>
  );
}

interface AttributeEditorProps {
  attribute: CollectionSearchSettingsAttribute;
  attributeIndex: number;
  disabled: boolean;
  eligibleColumns: Column[];
  onRemove: () => void;
  onUpdateColumn: (column: string) => void;
  onUpdateWeight: (weight: CollectionSearchWeight) => void;
}

function AttributeEditor({
  attribute,
  attributeIndex,
  disabled,
  eligibleColumns,
  onRemove,
  onUpdateColumn,
  onUpdateWeight,
}: AttributeEditorProps) {
  const attributeNumber = attributeIndex + 1;
  const columnName = attribute.column.trim() || `attribute ${attributeNumber}`;
  return (
    <fieldset
      className={cn(CARD_CLASS, "p-4 grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem_auto]")}
      aria-label={`Searchable attribute ${attributeNumber}`}
    >
      <label className="text-sm font-medium text-gray-800 dark:text-gray-200">
        <span className="sr-only">Column for attribute {attributeNumber}</span>
        <select
          value={attribute.column.trim()}
          onChange={(event) => onUpdateColumn(event.target.value)}
          disabled={disabled}
          aria-label={`Column for attribute ${attributeNumber}`}
          className="mt-1 w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-100"
        >
          {eligibleColumns.map((column) => (
            <option key={column.name} value={column.name}>
              {column.name}
            </option>
          ))}
        </select>
      </label>
      <label className="text-sm font-medium text-gray-800 dark:text-gray-200">
        <span className="sr-only">Weight for {columnName}</span>
        <select
          value={attribute.weight}
          onChange={(event) => onUpdateWeight(event.target.value as CollectionSearchWeight)}
          disabled={disabled}
          aria-label={`Weight for ${columnName}`}
          className="mt-1 w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-100"
        >
          {WEIGHT_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        className={cn(BUTTON_CLASS, DANGER_BUTTON_CLASS, "self-end justify-center")}
        onClick={onRemove}
        disabled={disabled}
        aria-label={`Remove attribute ${columnName}`}
      >
        <Trash2 className="h-4 w-4" />
        Remove
      </button>
    </fieldset>
  );
}
