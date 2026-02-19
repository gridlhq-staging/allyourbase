// Package examples embeds the demo application source files into the AYB binary.
// These are extracted at runtime by `ayb demo <name>`.
package examples

import "embed"

// FS contains the source files for all demo applications.
// Tests, READMEs, lock files, and build artifacts are excluded.
//
//go:embed kanban/src kanban/index.html kanban/package.json kanban/schema.sql kanban/tsconfig.json kanban/vite.config.ts kanban/tailwind.config.js kanban/postcss.config.js
//go:embed live-polls/src live-polls/index.html live-polls/package.json live-polls/schema.sql live-polls/tsconfig.json live-polls/vite.config.ts live-polls/tailwind.config.js live-polls/postcss.config.js
//go:embed pixel-canvas/src pixel-canvas/index.html pixel-canvas/package.json pixel-canvas/schema.sql pixel-canvas/tsconfig.json pixel-canvas/vite.config.ts pixel-canvas/tailwind.config.js pixel-canvas/postcss.config.js
var FS embed.FS
