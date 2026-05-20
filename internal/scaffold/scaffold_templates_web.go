package scaffold

import "fmt"

// tsConfigJSON returns the TypeScript compiler configuration for React and Vite projects, configured for ES2020 target with strict type checking and JSX support.
func tsConfigJSON() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
`
}

func viteConfig() string {
	return `import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
});
`
}

func indexHTML(opts Options) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, opts.Name)
}

func reactMain() string {
	return `import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
`
}

// reactApp returns the root React component with server health display and example items listing from the items table.
func reactApp() string {
	return `import { useEffect, useState } from "react";
import { ayb } from "./lib/ayb";

function App() {
  const [items, setItems] = useState<any[]>([]);
  const [status, setStatus] = useState("loading...");

  useEffect(() => {
    ayb.health()
      .then(() => setStatus("connected"))
      .catch(() => setStatus("disconnected - run 'ayb start'"));

    ayb.records
      .list("items")
      .then((res) => setItems(res.items))
      .catch(() => {});
  }, []);

  return (
    <div style={{ maxWidth: 600, margin: "2rem auto", fontFamily: "system-ui" }}>
      <h1>Welcome to your AYB app</h1>
      <p>
        Server: <strong>{status}</strong>
      </p>
      <h2>Items ({items.length})</h2>
      <ul>
        {items.map((item: any) => (
          <li key={item.id}>{item.name}</li>
        ))}
      </ul>
      <p style={{ color: "#888", fontSize: "0.9rem" }}>
        Edit <code>src/App.tsx</code> to get started.
        <br />
        Admin dashboard: <a href="http://localhost:8090/admin">localhost:8090/admin</a>
      </p>
    </div>
  );
}

export default App;
`
}

func minimalCSS() string {
	return `body {
  margin: 0;
  -webkit-font-smoothing: antialiased;
}
`
}

// nextTSConfig returns the TypeScript compiler configuration for Next.js projects, configured for ES2017 target with strict type checking and the Next.js plugin.
func nextTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2017",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
`
}

func nextConfig() string {
	return `/** @type {import('next').NextConfig} */
const nextConfig = {};
module.exports = nextConfig;
`
}

func nextLayout(opts Options) string {
	return fmt.Sprintf(`export const metadata = {
  title: "%s",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
`, opts.Name)
}

// nextPage returns the root page component for Next.js projects with server connection status display and example items listing.
func nextPage() string {
	return `"use client";

import { useEffect, useState } from "react";
import { ayb } from "@/lib/ayb";

export default function Home() {
  const [items, setItems] = useState<any[]>([]);
  const [status, setStatus] = useState("loading...");

  useEffect(() => {
    ayb.health()
      .then(() => setStatus("connected"))
      .catch(() => setStatus("disconnected - run 'ayb start'"));

    ayb.records
      .list("items")
      .then((res) => setItems(res.items))
      .catch(() => {});
  }, []);

  return (
    <main style={{ maxWidth: 600, margin: "2rem auto", fontFamily: "system-ui" }}>
      <h1>Welcome to your AYB app</h1>
      <p>Server: <strong>{status}</strong></p>
      <h2>Items ({items.length})</h2>
      <ul>
        {items.map((item: any) => (
          <li key={item.id}>{item.name}</li>
        ))}
      </ul>
    </main>
  );
}
`
}
