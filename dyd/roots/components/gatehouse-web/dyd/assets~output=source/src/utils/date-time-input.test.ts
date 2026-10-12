import { describe, expect, it } from "vitest"
import { dateTimeInput, dateTimeValue } from "./date-time-input"

describe("record datetime editing", () => {
  it("round-trips an offset timestamp through local input without losing its instant or milliseconds", () => {
    const timestamp = "2026-10-11T10:12:13.125+05:30"
    const local = dateTimeInput(timestamp)
    expect(local).not.toMatch(/Z$/)
    expect(Date.parse(dateTimeValue(local))).toBe(Date.parse(timestamp))
    expect(dateTimeValue(local)).toMatch(/\.125Z$/)
  })
  it("keeps an empty datetime empty instead of introducing an invalid timestamp", () => {
    expect(dateTimeInput("")).toBe("")
    expect(dateTimeValue("")).toBe("")
  })
})
