import path from "node:path";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const api = env.VITE_API_URL || "http://localhost:8080";
  return {
    plugins: [react()],
    resolve: {
      alias: { "@": path.resolve(__dirname, "src") },
    },
    optimizeDeps: {
      include: ["monaco-editor", "@monaco-editor/react"],
    },
    server: {
      port: 5173,
      proxy: {
        "/api": { target: api, changeOrigin: true, ws: true },
      },
    },
  };
});
