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

// nodeMain returns the TypeScript entry point source that checks AYB server health and lists items, injecting the provided body for template-specific item display logic.
func nodeMain(listItemsBody string) string {
	return fmt.Sprintf(`import { ayb } from "./lib/ayb";

async function main() {
  try {
    const health = await ayb.health();
    console.log("AYB server:", health.status);
  } catch {
    console.error("Cannot connect to AYB. Run 'ayb start' first.");
    process.exit(1);
  }

  try {
    const { items } = await ayb.records.list("items");
%s
  } catch {
    console.error("Cannot list items. Run 'ayb sql < schema.sql' first.");
    process.exit(1);
  }
}

main();
`, listItemsBody)
}

// expressMain returns the entry point source code for Express template projects, including health check and example record listing from the AYB API.
func expressMain() string {
	return nodeMain(`    console.log("Items:", items.length);
    for (const item of items) {
      console.log(" -", item.name);
    }`)
}

// plainMain returns the entry point source code for plain Node.js template projects, with AYB health check and example record listing.
func plainMain() string {
	return nodeMain(`    console.log("Items:", items.length);`)
}
