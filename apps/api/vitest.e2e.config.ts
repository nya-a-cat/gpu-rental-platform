import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    hookTimeout: 60_000,
    include: ["test/**/*.e2e-spec.ts"],
    restoreMocks: true,
    testTimeout: 30_000,
  },
});
