import { readFileSync, existsSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { pathToFileURL, fileURLToPath } from "node:url"
import { compileModule } from "svelte/compiler"
import { transpileModule, ModuleKind, ScriptTarget } from "typescript"

const require = createRequire(import.meta.url)
const root = dirname(require.resolve("svelte/package.json"))
export const clientEntry = pathToFileURL(join(root, "src/index-client.js")).href
const internal = pathToFileURL(join(root, "src/internal/client/index.js")).href
const cache = new Map<string, string>()

export function clientCode(source: string): string {
  const javascript = transpileModule(source, {
    compilerOptions: { module: ModuleKind.ESNext, target: ScriptTarget.ESNext },
  }).outputText
  return compileModule(javascript, {
    filename: "test-client.svelte.ts",
    generate: "client",
  })
    .js.code.replace(
      /(['"])svelte\/internal\/client\1/g,
      JSON.stringify(internal),
    )
    .replace(/(['"])svelte\1/g, JSON.stringify(clientEntry))
}

// Compile actual rune modules and their relative dependencies for client-runtime
// tests. No DOM or browser process is required for controller/effect lifecycles.
export function clientModuleURL(url: URL, source?: string): string {
  const cached = source === undefined ? cache.get(url.href) : undefined
  if (cached) return cached
  let code = clientCode(source ?? readFileSync(url, "utf8"))
  for (const match of [...code.matchAll(/from\s+(['"])(\.[^'"]+)\1/g)]) {
    const candidate = new URL(match[2], url)
    const filename = fileURLToPath(candidate)
    const dependency = existsSync(filename)
      ? candidate
      : existsSync(`${filename}.ts`)
        ? new URL(`${candidate.href}.ts`)
        : new URL(candidate.href.replace(/\.js$/, ".ts"))
    code = code.replace(
      match[0],
      `from ${JSON.stringify(clientModuleURL(dependency))}`,
    )
  }
  const uri = `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  if (source === undefined) cache.set(url.href, uri)
  return uri
}

export async function importClientModule(url: URL) {
  return await import(/* @vite-ignore */ clientModuleURL(url))
}
