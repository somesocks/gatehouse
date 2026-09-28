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

  it("places the clear action inside answered optional text and number controls", () => {
    for (const [type, value] of [
      ["text", "Note"],
      ["number", "42"],
    ] as const) {
      const field: InputField = {
        id: type,
        label: "Answer",
        type,
        optional: true,
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
      expect(unanswered).toContain('class="icon inline input-clear"')
      expect(unanswered).toContain('data-empty="true"')
      expect(unanswered).toContain('tabindex="-1"')
    }
  })

  it("lets answered required text and number fields be cleared", () => {
    for (const [type, value] of [
      ["text", "Note"],
      ["number", "42"],
    ] as const) {
      const field: InputField = { id: type, label: "Answer", type }
      const props = { field, path: [type], onSet: async () => true, onRemove: async () => true }
      expect(render(FormField, { props: { ...props, value } }).body).toContain('aria-label="Clear Answer"')
      const unanswered = render(FormField, { props: { ...props, value: undefined } }).body
      expect(unanswered).toContain('data-empty="true"')
      expect(unanswered).toContain('tabindex="-1"')
    }
  })

  it("uses the unanswered option instead of a clear action for selects", () => {
    for (const field of [
      { id: "required", label: "Answer", type: "options", choices: ["north"] },
      { id: "optional", label: "Answer", type: "options", choices: ["north"], optional: true },
      { id: "boolean", label: "Answer", type: "boolean", optional: true },
    ] satisfies InputField[]) {
      const body = render(FormField, { props: {
        field, path: [field.id!], value: field.type === "boolean" ? false : "north",
        onSet: async () => true, onRemove: async () => true,
      } }).body
      expect(body).toContain('option value=""')
      expect(body).not.toContain('aria-label="Clear Answer"')
    }

    const requiredBoolean: InputField = { id: "approved", label: "Approved", type: "boolean" }
    const body = render(FormField, { props: {
      field: requiredBoolean, path: ["approved"], value: false,
      onSet: async () => true, onRemove: async () => true,
    } }).body
    expect(body).not.toContain('aria-label="Clear Approved"')
  })
})
