import { describe, expect, it } from "vitest";
import schemaSql from "../schema.sql?raw";
import seedSql from "../seed.sql?raw";

interface SeedProduct {
  slug: string;
  category: string;
  brand: string;
  price_cents: number;
}

const expectedCategoryCounts = {
  Stationery: 4,
  Lighting: 4,
  Office: 4,
  Kitchen: 4,
  Travel: 4,
};

const expectedBrandCounts = {
  Apex: 4,
  Beacon: 4,
  Cobalt: 4,
  Drift: 4,
  Ember: 4,
};

function parseSeedProducts(sql: string): SeedProduct[] {
  return Array.from(
    sql.matchAll(
      /\('([^']+)', '[^']+', '[^']+', '([^']+)', '([^']+)', ([0-9]+)\)/g,
    ),
    ([, slug, category, brand, price]) => ({
      slug,
      category,
      brand,
      price_cents: Number(price),
    }),
  );
}

function countBy(
  products: SeedProduct[],
  field: "category" | "brand",
): Record<string, number> {
  return products.reduce<Record<string, number>>((counts, product) => {
    counts[product[field]] = (counts[product[field]] ?? 0) + 1;
    return counts;
  }, {});
}

describe("instantsearch catalog contract", () => {
  it("declares brand as a required indexed product field", () => {
    expect(schemaSql).toContain(
      "brand TEXT NOT NULL CHECK (length(brand) > 0)",
    );
    expect(schemaSql).toContain(
      "CREATE INDEX IF NOT EXISTS idx_instantsearch_products_brand",
    );
    expect(schemaSql).toContain("ON instantsearch_products(brand)");
  });

  it("keeps the seed rows and expected totals comment in sync", () => {
    const products = parseSeedProducts(seedSql);
    const inBandProducts = products.filter(
      (product) => product.price_cents >= 2000 && product.price_cents <= 6000,
    );

    expect(seedSql).toContain(
      "INSERT INTO instantsearch_products (slug, title, description, category, brand, price_cents) VALUES",
    );
    expect(seedSql).toContain("brand = EXCLUDED.brand");
    expect(products.map((product) => product.slug)).toEqual([
      "red-notebook",
      "blue-index-cards",
      "graphite-pencil-set",
      "archival-pen-case",
      "brass-desk-lamp",
      "linen-task-lamp",
      "walnut-floor-lamp",
      "portable-reading-light",
      "steel-cable-tray",
      "oak-monitor-stand",
      "mesh-desk-organizer",
      "standing-desk-mat",
      "ceramic-coffee-mug",
      "glass-water-bottle",
      "cotton-lunch-wrap",
      "countertop-air-filter",
      "travel-tech-pouch",
      "canvas-weekender",
      "packing-cube-set",
      "aluminum-carry-on",
    ]);
    expect(countBy(products, "category")).toEqual(expectedCategoryCounts);
    expect(countBy(products, "brand")).toEqual(expectedBrandCounts);
    expect(products.some((product) => product.price_cents < 1000)).toBe(true);
    expect(products.some((product) => product.price_cents > 10000)).toBe(true);
    expect(inBandProducts).toHaveLength(9);
    expect(products.length - inBandProducts.length).toBe(11);
    expect(seedSql).toContain("-- expected totals:");
    expect(seedSql).toContain(
      "-- categories: Stationery=4, Lighting=4, Office=4, Kitchen=4, Travel=4",
    );
    expect(seedSql).toContain(
      "-- brands: Apex=4, Beacon=4, Cobalt=4, Drift=4, Ember=4",
    );
    expect(seedSql).toContain("-- total_hits: 20");
    expect(seedSql).toContain(
      "-- price_range price_cents_2000_6000: in_band=9, out_of_band=11",
    );
  });
});
