const DOCUMENTATION_ORIGIN = "https://allyourbase.io";

export type GuidePath = `/guide/${string}`;

export const MIGRATIONS_GUIDE_PATH = "/guide/migrations" satisfies GuidePath;
export const SUPABASE_MIGRATION_GUIDE_PATH = "/guide/migrating-from-supabase" satisfies GuidePath;
export const ALGOLIA_MIGRATION_GUIDE_PATH = "/guide/migrating-from-algolia" satisfies GuidePath;

export function docsUrl(path: GuidePath): string {
  return `${DOCUMENTATION_ORIGIN}${path}`;
}
