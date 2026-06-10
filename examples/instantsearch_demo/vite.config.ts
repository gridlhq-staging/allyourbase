import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    host: "127.0.0.1",
    port: 8096,
    strictPort: true,
  },
  preview: {
    host: "127.0.0.1",
    port: 8096,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.ts",
    include: ["tests/**/*.{test,spec}.{js,ts,tsx}"],
  },
});
