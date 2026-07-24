import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const port = Number(process.env.AYB_DEMO_APP_PORT) || 5177;

export default defineConfig({
  plugins: [react()],
  server: {
    port,
    proxy: {
      "/api": "http://localhost:8092",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.ts",
    include: ["tests/**/*.{test,spec}.{ts,tsx}"],
  },
});
