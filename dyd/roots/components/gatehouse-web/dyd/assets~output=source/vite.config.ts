import { svelte } from "@sveltejs/vite-plugin-svelte"
import { defineConfig } from "vite"

export default defineConfig(({ mode }) => ({
  base: mode === "session-file-picker" ? "/app/tools/session-file-picker/" : "/app/",
  plugins: [svelte()],
}))
