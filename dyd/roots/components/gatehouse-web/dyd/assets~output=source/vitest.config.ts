import { defineConfig } from "vitest/config"
import { svelte } from "@sveltejs/vite-plugin-svelte"
import { fileURLToPath } from "node:url"

export default defineConfig({
  plugins: [svelte()],
  cacheDir: ".vitest-cache",
  resolve: {
    alias: { $lib: fileURLToPath(new URL("./src/lib", import.meta.url)) },
  },
  test: {
    projects: [
      {
        extends: true,
        test: {
          name: "unit",
          environment: "node",
          include: ["src/**/*.test.ts"],
          exclude: ["src/**/*.dom.test.ts"],
        },
      },
      {
        extends: true,
        resolve: { conditions: ["browser"] },
        test: {
          name: "components",
          environment: "happy-dom",
          include: ["src/**/*.dom.test.ts"],
          setupFiles: ["src/testing/dom-setup.ts"],
        },
      },
    ],
  },
})
