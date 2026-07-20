/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: process.env.VITE_BASE_PATH || "/",
  plugins: [react()],
  resolve: {
    alias: {
      "@gpu-rental/contracts": new URL(
        "../../packages/contracts/src/index.ts",
        import.meta.url,
      ).pathname,
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:4000",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
});
