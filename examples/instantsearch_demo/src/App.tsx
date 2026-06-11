import {
  Configure,
  Highlight,
  Hits,
  InstantSearch,
  Pagination,
  RangeInput,
  RefinementList,
  SearchBox,
  Stats,
} from "react-instantsearch";
import type { Hit } from "instantsearch.js/es/types/results";
import type { ComponentProps, ComponentPropsWithoutRef } from "react";
import { searchClient } from "./lib/ayb";
import "./App.css";

const COLLECTION_NAME = "instantsearch_products";
const HITS_PER_PAGE = 6;
const FACETS = ["category", "price_cents"];
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

export default function App() {
  return (
    <main className="app-shell">
      <InstantSearch searchClient={instantSearchClient} indexName={COLLECTION_NAME}>
        <Configure hitsPerPage={HITS_PER_PAGE} facets={FACETS} />
        <section className="toolbar" aria-label="Search controls">
          <SearchBox placeholder="Search products" />
          <div data-testid="results-summary">
            <Stats />
          </div>
        </section>
        <section className="content">
          <aside className="filters" aria-label="Filters">
            <h2>Category</h2>
            <RefinementList attribute="category" operator="or" searchable={false} />
            <section className="price-range" aria-label="Price range">
              <h2>Price range</h2>
              <RangeInput attribute="price_cents" />
            </section>
          </aside>
          <section className="results" aria-label="Search results">
            <Hits hitComponent={ProductCard} />
            <Pagination />
          </section>
        </section>
      </InstantSearch>
    </main>
  );
}
