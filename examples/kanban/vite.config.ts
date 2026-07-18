import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const aybServerURL = env.AYB_SERVER_URL || "http://localhost:8090";

  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: {
        // ws: true proxies the realtime WebSocket upgrade for /api/realtime/ws.
        // changeOrigin is left false so the original Host header is preserved —
        // the server's CheckWebSocketOrigin matches the Origin host against Host.
        "/api": {
          target: aybServerURL,
          ws: true,
        },
      },
    },
  };
});
