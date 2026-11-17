import { createRawSnippet } from "svelte"
import { render } from "svelte/server"
import { expect, it } from "vitest"
import Heading from "./Heading.svelte"

it("supports compact headings without changing their semantic level or the default size", () => {
  const children = createRawSnippet(() => ({ render: () => "Replying to" }))
  const compact = render(Heading, {
    props: { level: 2, size: "small", children },
  }).body
  expect(compact).toMatch(/<h2\b/)
  expect(compact).toContain("text-base font-semibold")
  expect(compact).not.toContain("text-muted-foreground")
  const standard = render(Heading, { props: { level: 2, children } }).body
  expect(standard).toMatch(/<h2\b/)
  expect(standard).toContain("text-xl font-semibold")
})
