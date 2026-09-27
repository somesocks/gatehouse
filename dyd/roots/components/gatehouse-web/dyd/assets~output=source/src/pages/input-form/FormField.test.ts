import { describe, expect, it } from "vitest"
import { render } from "svelte/server"
import FormField from "./FormField.svelte"
import type { InputField } from "./form-data"

describe("form field headings and labels", () => {
  it("uses nested heading levels and marks only required fields", () => {
    const field: InputField = {
      id: "details", label: "Details", type: "object", fields: [
        { id: "contact", label: "Contact", type: "object", fields: [
          { id: "name", label: "Name", type: "text" },
          { id: "notes", label: "Notes", type: "text", optional: true },
        ] },
        { id: "visits", label: "Visits", type: "list", item: {
          type: "object", fields: [{ id: "city", label: "City", type: "text" }],
        } },
      ],
    }
    const { body } = render(FormField, {
      props: {
        field, path: ["details"], value: { contact: {}, visits: [{}] },
        onSet: async () => true, onRemove: async () => true,
      },
    })
    expect(body).toMatch(/<h2\b[^>]*>/)
    expect(body).toMatch(/<h3\b[^>]*>/)
    expect(body).not.toMatch(/<h4\b[^>]*>/)
    expect(body).toContain("Details")
    expect(body).toContain("Contact")
    expect(body).toMatch(/<legend>.*?Visits.*?<\/legend>/s)
    const headings = [...body.matchAll(/<h[2-6]\b[^>]*>(.*?)<\/h[2-6]>/gs)].map(([, content]) => content.replace(/<[^>]*>/g, ""))
    expect(headings.some((heading) => heading.includes("Visits"))).toBe(false)
    expect(headings.some((heading) => heading.includes("Entry 1"))).toBe(true)
    const nameLabel = body.match(/<label[^>]*for="input-details-contact-name"[^>]*>.*?<\/label>/s)?.[0]
    const notesLabel = body.match(/<label[^>]*for="input-details-contact-notes"[^>]*>.*?<\/label>/s)?.[0]
    expect(nameLabel).toContain('aria-hidden="true">*</span>')
    expect(notesLabel).not.toContain("*</span>")
  })

  it("places the clear action inside answered optional controls", () => {
    for (const [type, value] of [
      ["text", "Note"],
      ["number", "42"],
      ["options", "north"],
      ["boolean", false],
    ] as const) {
      const field: InputField = {
        id: type,
        label: "Answer",
        type,
        optional: true,
        ...(type === "options" ? { choices: ["north"] } : {}),
      }
      const props = {
        field,
        path: [type],
        onSet: async () => true,
        onRemove: async () => true,
      }
      const answered = render(FormField, { props: { ...props, value } }).body
      const unanswered = render(FormField, { props: { ...props, value: undefined } }).body
      expect(answered).toMatch(/class="input-control"[^>]*>.*?class="icon inline input-clear"/s)
      expect(answered).toContain('aria-label="Clear Answer"')
      expect(answered).not.toContain("Clear answer</button>")
      expect(unanswered).not.toContain('class="icon inline input-clear"')
    }
  })
})
