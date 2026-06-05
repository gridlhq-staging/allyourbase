import type { MovieListItem } from "../types";

interface Props {
  items: MovieListItem[];
  selectedSlug: string | null;
  onSelect: (slug: string) => void;
}

export default function SearchResults({ items, selectedSlug, onSelect }: Props) {
  if (items.length === 0) {
    return <p className="text-gray-500 text-sm py-4">No results found.</p>;
  }

  return (
    <div className="space-y-2">
      {items.map((item) => (
        <button
          key={item.slug}
          onClick={() => onSelect(item.slug)}
          data-testid={`search-result-row-${item.slug}`}
          className={`w-full text-left p-3 rounded-lg border transition-colors ${
            selectedSlug === item.slug
              ? "border-purple-500 bg-purple-950/30"
              : "border-gray-800 bg-gray-900 hover:border-gray-700"
          }`}
        >
          <div className="flex items-baseline justify-between gap-2">
            <h3 data-testid={`search-result-title-${item.slug}`} className="font-medium text-white truncate">{item.title}</h3>
            <span data-testid={`search-result-year-${item.slug}`} className="text-xs text-gray-500 shrink-0">{item.release_year}</span>
          </div>
          <div className="flex items-center gap-2 mt-1">
            <span
              data-testid={`search-result-genre-${item.slug}`}
              className="text-xs uppercase tracking-wider text-purple-300"
            >
              {item.primary_genre}
            </span>
          </div>
          {item._highlight ? (
            <p
              data-testid={`search-result-overview-${item.slug}`}
              className="text-sm text-gray-400 mt-1 line-clamp-2"
            >
              <span
                aria-label="Highlighted match"
                // ts_headline output from AYB backend wraps matches in <b> tags
                // over server-controlled seed corpus; not user input.
                dangerouslySetInnerHTML={{ __html: item._highlight }}
              />
            </p>
          ) : (
            <p
              data-testid={`search-result-overview-${item.slug}`}
              className="text-sm text-gray-400 mt-1 line-clamp-2"
            >
              {item.overview}
            </p>
          )}
        </button>
      ))}
    </div>
  );
}
