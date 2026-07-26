import type { ListResponse } from "../types";

export const SEARCH_RANK_RESPONSE_FIELD = "_rank";

// ts_rank scores are frequently far below 1 (0.06, 0.03, …), so a fixed number of
// decimals would collapse distinct scores to "0.0000". Significant digits keep every
// backend-returned score both distinguishable and parseable as a positive number.
const SEARCH_RANK_SIGNIFICANT_DIGITS = 4;

interface RankScore {
  rowIndex: number;
  score: number;
}

export function formatSearchRankScore(score: number): string {
  return score.toPrecision(SEARCH_RANK_SIGNIFICANT_DIGITS);
}

export function searchRankScores(data: ListResponse | null): RankScore[] {
  if (!data) {
    return [];
  }
  return data.items.flatMap((row, rowIndex) => {
    // A real user-owned `_rank` column can hold any type, and the API client passes
    // the field through untouched, so narrowing to finite numbers happens here.
    const score = row[SEARCH_RANK_RESPONSE_FIELD];
    if (typeof score !== "number" || !Number.isFinite(score)) {
      return [];
    }
    return [{ rowIndex, score }];
  });
}

export function SearchRankResults({ scores }: { scores: RankScore[] }) {
  if (scores.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Relevance scores"
      data-testid="search-relevance-results"
      className="mb-4 rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-gray-800 dark:border-blue-700/70 dark:bg-blue-950/30 dark:text-blue-50"
    >
      <h2 className="text-sm font-medium">Relevance scores</h2>
      <ol className="mt-2 space-y-1">
        {scores.map((score) => (
          <li key={score.rowIndex} className="text-xs leading-5">
            <span className="font-medium text-gray-600 dark:text-blue-100">
              Result {score.rowIndex + 1}:{" "}
            </span>
            <span data-testid={`search-result-rank-${score.rowIndex}`} className="font-mono">
              {formatSearchRankScore(score.score)}
            </span>
          </li>
        ))}
      </ol>
    </section>
  );
}
