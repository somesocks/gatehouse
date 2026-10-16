import { svelte } from "@sveltejs/vite-plugin-svelte"
import tailwindcss from "@tailwindcss/vite"
import { fileURLToPath } from "node:url"
import { defineConfig } from "vite"

export default defineConfig(({ mode }) => ({
  base:
    mode === "session-file-picker"
      ? "/app/tools/session-file-picker/"
      : "/app/",
  plugins: [tailwindcss(), svelte()],
  build:
    mode === "session-file-picker"
      ? undefined
      : {
          rollupOptions: {
            input: {
              main: fileURLToPath(new URL("./index.html", import.meta.url)),
              "set-password": fileURLToPath(
                new URL("./set-password.html", import.meta.url),
              ),
            },
          },
        },
  resolve: {
    alias: { $lib: fileURLToPath(new URL("./src/lib", import.meta.url)) },
  },
}))
