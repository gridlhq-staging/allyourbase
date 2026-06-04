package scaffold

import "fmt"

// expressTSConfig returns the TypeScript compiler configuration for Express projects, configured for ES2020 target with strict type checking and output to dist directory.
func expressTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "resolveJsonModule": true
  },
  "include": ["src"]
}
`
}

// nodeMain returns the TypeScript entry point source that checks AYB server health and lists items, injecting collection list setup, expression, and display body for template-specific examples.
func nodeMain(listSetup, listItemsExpression, listItemsBody string) string {
	return fmt.Sprintf(`import { ayb } from "./lib/ayb";

async function main() {
  try {
    const health = await ayb.health();
    console.log("AYB server:", health.status);
  } catch {
    console.error("Cannot connect to AYB. Run 'ayb start' first.");
    process.exit(1);
  }
%s

  try {
    const { items } = await %s;
%s
  } catch (err) {
    console.error(err instanceof Error ? "Cannot list items: " + err.message : "Cannot list items."); // Cannot list items. Run 'ayb sql < schema.sql' first.
    process.exit(1);
  }
}

main();
`, listSetup, listItemsExpression, listItemsBody)
}

// expressMain returns the entry point source code for Express template projects, including health check and example record listing from the AYB API.
func expressMain() string {
	return nodeMain(``, `ayb.records.list("items")`, `    console.log("Items:", items.length);
    for (const item of items) {
      console.log(" -", item.name);
    }`)
}

// plainMain returns the entry point source code for plain Node.js template projects, with AYB health check and example record listing.
func plainMain() string {
	return nodeMain(`  const search = process.argv[2] ?? "demo";`, `ayb.records.list("items", { search, fuzzy: true })`, `    console.log(`+"`"+`Search items for "${search}":`+"`"+`, items.length);
    for (const item of items) {
      console.log(" -", item.name, item.description ?? "");
    }`)
}
