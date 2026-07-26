import {
  ALGOLIA_MIGRATION_GUIDE_PATH,
  docsUrl,
  type GuidePath,
  MIGRATIONS_GUIDE_PATH,
  SUPABASE_MIGRATION_GUIDE_PATH,
} from "../lib/docs_url";

const GUIDE_LINKS: Array<{ label: string; path: GuidePath }> = [
  { label: "Migration guide", path: MIGRATIONS_GUIDE_PATH },
  { label: "Supabase migration guide", path: SUPABASE_MIGRATION_GUIDE_PATH },
  { label: "Algolia migration guide", path: ALGOLIA_MIGRATION_GUIDE_PATH },
];

export function MigrationDiscoveryCTA() {
  return (
    <div className="mt-4 space-y-2 text-center">
      <p className="text-sm font-medium text-gray-700 dark:text-gray-200">
        Migrating from another source?
      </p>
      <code className="inline-block rounded border border-gray-200 bg-white px-2 py-1 font-mono text-xs text-gray-700 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200">
        ayb migrate &lt;source&gt; --help
      </code>
      <div className="flex flex-wrap justify-center gap-x-3 gap-y-1 text-xs">
        {GUIDE_LINKS.map((link) => (
          <a
            key={link.path}
            href={docsUrl(link.path)}
            target="_blank"
            rel="noreferrer"
            className="text-blue-600 hover:text-blue-700 hover:underline dark:text-blue-300 dark:hover:text-blue-200"
          >
            {link.label}
          </a>
        ))}
      </div>
    </div>
  );
}
