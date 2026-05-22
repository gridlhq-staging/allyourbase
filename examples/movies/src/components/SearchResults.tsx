import type { MovieSearchRow } from "../types";

interface Props {
  rows: MovieSearchRow[];
  selectedSlug: string | null;
  onSelect: (slug: string) => void;
}

export default function SearchResults({ rows, selectedSlug, onSelect }: Props) {
  if (rows.length === 0) {
    return <p className="text-gray-500 text-sm py-4">No results found.</p>;
  }

  return (
    <div className="space-y-2">
      {rows.map((row) => (
        <button
          key={row.slug}
          onClick={() => onSelect(row.slug)}
          className={`w-full text-left p-3 rounded-lg border transition-colors ${
            selectedSlug === row.slug
              ? "border-purple-500 bg-purple-950/30"
              : "border-gray-800 bg-gray-900 hover:border-gray-700"
          }`}
        >
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="font-medium text-white truncate">{row.title}</h3>
            <span className="text-xs text-gray-500 shrink-0">{row.release_year}</span>
          </div>
          <p className="text-sm text-gray-400 mt-1 line-clamp-2">{row.overview}</p>
        </button>
      ))}
    </div>
  );
}
