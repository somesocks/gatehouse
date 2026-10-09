import { readFileSync, readdirSync } from "node:fs"
import { resolve, relative } from "node:path"
import { parse } from "svelte/compiler"
import { describe, expect, it } from "vitest"

const src = resolve("src")
const compositions = readdirSync(src, { recursive: true })
  .map(String)
  .filter(
    (file) =>
      file.endsWith(".svelte") &&
      !file.startsWith("ui/") &&
      !file.startsWith("lib/"),
  )

describe("component-only presentation boundary", () => {
  it.each(compositions)(
    "%s uses the ui API without local styling or HTML presentation",
    (file) => {
      const source = readFileSync(resolve(src, file), "utf8")
      const ast = parse(source, { modern: true })
      expect(ast.css, file).toBeNull()
      expect(source, file).not.toMatch(
        /from\s+["'](?:bits-ui|@lucide\/svelte|[^"']*vendor\/)/,
      )
      function inspect(node: unknown): void {
        if (!node || typeof node !== "object") return
        const n = node as Record<string, any>
        if (n.type === "SvelteHead") return
        expect(n.type, `${file}: ${n.name ?? n.type}`).not.toBe(
          "RegularElement",
        )
        expect(n.type, file).not.toBe("SvelteElement")
        expect(n.type, file).not.toBe("HtmlTag")
        for (const attribute of n.attributes ?? []) {
          expect(attribute.type, file).not.toBe("StyleDirective")
          expect(attribute.type, file).not.toBe("ClassDirective")
          expect(attribute.name, file).not.toBe("class")
          expect(attribute.name, file).not.toBe("style")
        }
        for (const [key, value] of Object.entries(n)) {
          if (["metadata", "expression", "attributes"].includes(key)) continue
          if (Array.isArray(value)) value.forEach(inspect)
          else if (typeof value === "object") inspect(value)
        }
      }
      inspect(ast.fragment)
    },
  )

  it("keeps additional CSS rules out of owned UI components", () => {
    for (const file of readdirSync(resolve(src, "ui"), { recursive: true })
      .map(String)
      .filter((file) => file.endsWith(".svelte"))) {
      const source = readFileSync(resolve(src, "ui", file), "utf8")
      expect(
        parse(source, { modern: true }).css,
        relative(src, file),
      ).toBeNull()
    }
  })

  it("excludes generated dependencies and outputs from Dryad source", () => {
    const ignore = readFileSync(".dyd-ignore", "utf8").split(/\s+/)
    expect(ignore).toContain("node_modules")
    expect(ignore).toContain("dist")
  })
})
