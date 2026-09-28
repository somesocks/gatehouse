import { describe, expect, it } from "vitest"
import {
  encodeFieldValue,
  fieldValue,
  parseInputDocument,
  patchDraft,
  requiredBooleanDefaults,
  type InputForm,
} from "./form-data"

const form: InputForm = {
  version: "1",
  type: "form",
  title: "Review",
  fields: [
    { id: "name", label: "Name", type: "text" },
    { id: "check", label: "Check", type: "boolean" },
    { id: "maybe", label: "Maybe", type: "boolean", optional: true },
    { id: "profile", label: "Profile", type: "object", fields: [
      { id: "region", label: "Region", type: "options", choices: ["north", "south"] },
      { id: "entries", label: "Entries", type: "list", item: { type: "object", fields: [
        { id: "amount", label: "Amount", type: "number", min: "9007199254740993" },
        { id: "ok", label: "OK", type: "boolean" },
      ] } },
    ] },
  ],
}

describe("input form JSON", () => {
  it("preserves file size text in saved summary arrays", () => {
    const document = parseInputDocument(`{"form":{"version":1,"type":"form","title":"Files","fields":[{"id":"files","label":"Files","type":"files"}]},"draft":{"files":[{"id":"sfi_1","name":"large.bin","size":9007199254740993}]}}`)
    expect(document.draft.files).toEqual([{ id: "sfi_1", name: "large.bin", size: "9007199254740993" }])
    expect(encodeFieldValue(document.form.fields[0], document.draft.files)).toBe('[{"id":"sfi_1","name":"large.bin","size":9007199254740993}]')
  })
  it("reads numeric draft values and bounds without rounding", () => {
    const result = parseInputDocument(`{"form":{"version":1,"type":"form","title":"Review","fields":[{"type":"number","id":"amount","min":9007199254740993}]},"draft":{"amount":9007199254740993,"list":[{"value":1.234567890123456789}]},"updated_at":null}`)
    expect(result.form.fields[0].min).toBe("9007199254740993")
    expect(result.draft.amount).toBe("9007199254740993")
    expect(result.draft.list).toEqual([{ value: "1.234567890123456789" }])
    expect(encodeFieldValue(result.form.fields[0], result.draft.amount)).toBe("9007199254740993")
  })

  it("serializes whole lists with exact numbers and rejects invalid number text", () => {
    const list = form.fields[3].fields![1]
    expect(encodeFieldValue(list, [{ amount: "9007199254740993", ok: false }])).toBe('[{"amount":9007199254740993,"ok":false}]')
    expect(() => encodeFieldValue(list, [{ amount: "NaN" }])).toThrow()
    expect(() => encodeFieldValue(list, [{ unknown: "1" }])).toThrow()
  })

  it("patches named paths without writing null for unanswered fields", () => {
    const first = patchDraft({}, ["profile", "region"], "north")
    const second = patchDraft(first, ["maybe"], false)
    const removed = patchDraft(second, ["maybe"], undefined, true)
    expect(fieldValue(removed, ["profile", "region"])).toBe("north")
    expect(removed).toEqual({ profile: { region: "north" } })
    expect(first).toEqual({ profile: { region: "north" } })
  })

  it("treats reserved and dotted field IDs as literal data keys", () => {
    const draft = patchDraft({}, ["__proto__", "a.b"], "value")
    expect(fieldValue(draft, ["__proto__", "a.b"])).toBe("value")
    expect(Object.getPrototypeOf(draft)).toBe(Object.prototype)
    expect(Object.hasOwn(draft, "__proto__")).toBe(true)
    expect(fieldValue({}, ["__proto__"])).toBeUndefined()
    const removed = patchDraft(draft, ["__proto__", "a.b"], undefined, true)
    expect(Object.hasOwn(removed, "__proto__")).toBe(true)
    expect(fieldValue(removed, ["__proto__"])).toEqual({})
  })

  it("fills required unchecked booleans, including inside repeated objects", () => {
    const draft = { name: "Ada", profile: { region: "north", entries: [{ amount: "9007199254740993" }] } }
    expect(requiredBooleanDefaults(form, draft)).toEqual([
      { path: ["check"], value: false },
      { path: ["profile", "entries"], value: [{ amount: "9007199254740993", ok: false }] },
    ])
    expect(draft.profile.entries[0]).toEqual({ amount: "9007199254740993" })
  })
})
