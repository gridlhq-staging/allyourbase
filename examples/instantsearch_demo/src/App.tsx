import {
  Configure,
  Highlight,
  Hits,
  InstantSearch,
  Pagination,
  RangeInput,
  SearchBox,
  Stats,
  useInstantSearch,
  useRefinementList,
} from "react-instantsearch";
import type { Hit } from "instantsearch.js/es/types/results";
import type { ComponentProps, ComponentPropsWithoutRef } from "react";
import { searchClient } from "./lib/ayb";
import "./App.css";

const COLLECTION_NAME = "instantsearch_products";
const HITS_PER_PAGE = 6;
const FACETS = ["category", "brand", "price_cents"];
const instantSearchClient = searchClient as unknown as ComponentProps<
  typeof InstantSearch
>["searchClient"];

interface HighlightEntry {
  value: string;
  matchLevel: string;
}

interface ProductFields {
  slug: string;
  title: string;
  description: string;
  category: string;
  brand: string;
  price_cents: number;
  _highlightResult?: {
    title?: HighlightEntry;
    description?: HighlightEntry;
  };
}

type ProductHit = Hit<ProductFields>;

function buildHighlightTag(testID: string) {
  return function HighlightTag(props: ComponentPropsWithoutRef<"mark">) {
    return <mark data-testid={testID} {...props} />;
  };
}

function hasHighlightValue(
  hit: ProductHit,
  attribute: "title" | "description",
): boolean {
  const highlightValue = hit._highlightResult?.[attribute];
  return (
    highlightValue != null &&
    !Array.isArray(highlightValue) &&
    typeof highlightValue.value === "string"
  );
}

function ProductCard({ hit }: { hit: ProductHit }) {
  const TitleHighlightTag = buildHighlightTag(
    `hit-${hit.objectID}-title-highlight`,
  );
  const DescriptionHighlightTag = buildHighlightTag(
    `hit-${hit.objectID}-description-highlight`,
  );

  return (
    <article className="hit" data-testid={`hit-${hit.objectID}`}>
      <div className="hit-meta">
        <span className="category">{hit.category}</span>
        <span className="slug">{hit.objectID}</span>
      </div>
      <h2 data-testid={`hit-${hit.objectID}-title`}>
        {hasHighlightValue(hit, "title") ? (
          <Highlight
            attribute="title"
            hit={hit}
            highlightedTagName={TitleHighlightTag}
          />
        ) : (
          hit.title
        )}
      </h2>
      <p data-testid={`hit-${hit.objectID}-description`}>
        {hasHighlightValue(hit, "description") ? (
          <Highlight
            attribute="description"
            hit={hit}
            highlightedTagName={DescriptionHighlightTag}
          />
        ) : (
          hit.description
        )}
      </p>
      <strong>${(hit.price_cents / 100).toFixed(2)}</strong>
    </article>
  );
}

function hasRealResults(results: unknown): boolean {
  return Boolean(
    results &&
      typeof results === "object" &&
      !("__isArtificial" in results && results.__isArtificial === true),
  );
}

function ResultsSummary() {
  const { status } = useInstantSearch();
  return (
    <div data-testid="results-summary" data-search-status={status}>
      <SearchStateSummary />
    </div>
  );
}

function SearchStateSummary() {
  const { error, results, status } = useInstantSearch({ catchError: true });

  if (status === "error" || error) {
    return (
      <p className="state-message state-message-error" role="alert">
        We could not load products. Check the API connection and retry.
      </p>
    );
  }

  if (!hasRealResults(results)) {
    return (
      <p className="state-message" role="status">
        Loading product search...
      </p>
    );
  }

  if (results.nbHits === 0) {
    return (
      <p className="state-message" role="status">
        No products match this search.
      </p>
    );
  }

  return <Stats />;
}

function SearchResults() {
  const { error, results, status } = useInstantSearch({ catchError: true });

  if (status === "error" || error) {
    return null;
  }

  if (!hasRealResults(results)) {
    return (
      <div className="results-hidden" aria-hidden="true">
        <Hits hitComponent={ProductCard} />
        <Pagination />
      </div>
    );
  }

  if (results.nbHits === 0) {
    return null;
  }

  return (
    <>
      <Hits hitComponent={ProductCard} />
      <Pagination />
    </>
  );
}

function FacetList({ attribute, label }: { attribute: string; label: string }) {
  const { error, results, status } = useInstantSearch({ catchError: true });
  const { items, refine } = useRefinementList({
    attribute,
    operator: "or",
  });

  if (status === "error" || error || !hasRealResults(results)) {
    return null;
  }

  if (items.length === 0) {
    return (
      <p className="facet-empty">
        No {label.toLowerCase()} filters available.
      </p>
    );
  }

  return (
    <ul className="ais-RefinementList-list">
      {items.map((item) => (
        <li className="ais-RefinementList-item" key={item.value}>
          <label className="ais-RefinementList-label">
            <input
              checked={item.isRefined}
              className="ais-RefinementList-checkbox"
              onChange={() => refine(item.value)}
              type="checkbox"
            />
            <span className="ais-RefinementList-labelText">{item.label}</span>
            <span className="ais-RefinementList-count">{item.count}</span>
          </label>
        </li>
      ))}
    </ul>
  );
}

export default function App() {
  return (
    <main className="app-shell">
      <InstantSearch searchClient={instantSearchClient} indexName={COLLECTION_NAME}>
        <Configure hitsPerPage={HITS_PER_PAGE} facets={FACETS} />
        <section className="toolbar" aria-label="Search controls">
          <SearchBox placeholder="Search products" />
          <ResultsSummary />
        </section>
        <section className="content">
          <aside className="filters" aria-label="Filters">
            <h2>Category</h2>
            <FacetList attribute="category" label="Category" />
            <h2>Brand</h2>
            <FacetList attribute="brand" label="Brand" />
            <section className="price-range" aria-label="Price range">
              <h2>Price range</h2>
              {/* Stage 3 will evaluate deterministic RangeSlider driving under Playwright. */}
              <RangeInput attribute="price_cents" />
            </section>
          </aside>
          <section className="results" aria-label="Search results">
            <SearchResults />
          </section>
        </section>
      </InstantSearch>
    </main>
  );
}
