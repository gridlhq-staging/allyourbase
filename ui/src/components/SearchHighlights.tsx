import type { ListResponse } from "../types";

const SEARCH_HIGHLIGHT_START = "<b>";
const SEARCH_HIGHLIGHT_END = "</b>";
export const SEARCH_HIGHLIGHT_RESPONSE_FIELD = "_highlight";
const SEARCH_HIGHLIGHT_ENTITY_PATTERN = /&(?:#\d+|#x[\da-fA-F]+|[a-zA-Z][\da-zA-Z]+);/g;
const SEARCH_HIGHLIGHT_NAMED_ENTITIES: Record<string, string> = {
  amp: "&",
  apos: "'",
  gt: ">",
  lt: "<",
  quot: '"',
};
const MAX_UNICODE_CODE_POINT = 0x10ffff;

interface HighlightFragment {
  text: string;
  emphasized: boolean;
}

interface HighlightSnippet {
  rowIndex: number;
  fragments: HighlightFragment[];
}

function decodeSearchHighlightEntity(entity: string): string {
  const body = entity.slice(1, -1);
  if (body.startsWith("#x") || body.startsWith("#X")) {
    const codePoint = Number.parseInt(body.slice(2), 16);
    return decodeSearchHighlightCodePoint(codePoint, entity);
  }
  if (body.startsWith("#")) {
    const codePoint = Number.parseInt(body.slice(1), 10);
    return decodeSearchHighlightCodePoint(codePoint, entity);
  }
  return SEARCH_HIGHLIGHT_NAMED_ENTITIES[body] ?? entity;
}

function decodeSearchHighlightCodePoint(codePoint: number, fallback: string): string {
  if (!Number.isFinite(codePoint) || codePoint < 0 || codePoint > MAX_UNICODE_CODE_POINT) {
    return fallback;
  }
  return String.fromCodePoint(codePoint);
}

function decodeSearchHighlightText(text: string): string {
  return text.replace(SEARCH_HIGHLIGHT_ENTITY_PATTERN, decodeSearchHighlightEntity);
}

function parseHighlightFragments(snippet: string): HighlightFragment[] {
  const fragments: HighlightFragment[] = [];
  let cursor = 0;

  while (cursor < snippet.length) {
    const start = snippet.indexOf(SEARCH_HIGHLIGHT_START, cursor);
    if (start === -1) {
      fragments.push({ text: decodeSearchHighlightText(snippet.slice(cursor)), emphasized: false });
      break;
    }

    const end = snippet.indexOf(SEARCH_HIGHLIGHT_END, start + SEARCH_HIGHLIGHT_START.length);
    if (end === -1) {
      fragments.push({ text: decodeSearchHighlightText(snippet.slice(cursor)), emphasized: false });
      break;
    }

    if (start > cursor) {
      fragments.push({ text: decodeSearchHighlightText(snippet.slice(cursor, start)), emphasized: false });
    }
    fragments.push({
      text: decodeSearchHighlightText(snippet.slice(start + SEARCH_HIGHLIGHT_START.length, end)),
      emphasized: true,
    });
    cursor = end + SEARCH_HIGHLIGHT_END.length;
  }

  return fragments.filter((fragment) => fragment.text !== "");
}

export function searchHighlightSnippets(data: ListResponse | null): HighlightSnippet[] {
  if (!data) {
    return [];
  }
  return data.items.flatMap((row, rowIndex) => {
    const highlight = row[SEARCH_HIGHLIGHT_RESPONSE_FIELD];
    if (typeof highlight !== "string" || highlight.trim() === "") {
      return [];
    }
    const fragments = parseHighlightFragments(highlight);
    if (fragments.length === 0) {
      return [];
    }
    return [{ rowIndex, fragments }];
  });
}

function withoutFields(
  row: Record<string, unknown>,
  fields: Set<string>,
): Record<string, unknown> {
  const plainRow: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(row)) {
    if (!fields.has(key)) {
      plainRow[key] = value;
    }
  }
  return plainRow;
}

/**
 * Removes the synthetic search fields the Search screen renders in its own panels
 * (for example `_highlight` and `_rank`) from grid rows. Callers pass only the
 * fields the backend actually generated, so real user-owned columns of the same
 * name are never stripped.
 */
export function gridDataWithoutSyntheticSearchFields(
  data: ListResponse | null,
  fields: string[],
): ListResponse | null {
  if (!data || fields.length === 0) {
    return data;
  }
  const strippedFields = new Set(fields);
  return {
    ...data,
    items: data.items.map((row) => withoutFields(row, strippedFields)),
  };
}

export function SearchHighlightResults({ snippets }: { snippets: HighlightSnippet[] }) {
  if (snippets.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Highlighted matches"
      data-testid="search-highlight-results"
      className="mb-4 rounded-lg border border-yellow-200 bg-yellow-50 p-3 text-sm text-gray-800 dark:border-yellow-700/70 dark:bg-yellow-950/30 dark:text-yellow-50"
    >
      <h2 className="text-sm font-medium">Highlighted matches</h2>
      <ol className="mt-2 space-y-2">
        {snippets.map((snippet) => (
          <li
            key={snippet.rowIndex}
            data-testid={`search-highlight-snippet-${snippet.rowIndex}`}
            className="text-xs leading-5"
          >
            <span className="font-medium text-gray-600 dark:text-yellow-100">
              Result {snippet.rowIndex + 1}:{" "}
            </span>
            {snippet.fragments.map((fragment, fragmentIndex) =>
              fragment.emphasized ? (
                <mark
                  key={`${snippet.rowIndex}-${fragmentIndex}`}
                  className="rounded bg-yellow-200 px-0.5 text-gray-950 dark:bg-yellow-300"
                >
                  {fragment.text}
                </mark>
              ) : (
                <span key={`${snippet.rowIndex}-${fragmentIndex}`}>{fragment.text}</span>
              ),
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
