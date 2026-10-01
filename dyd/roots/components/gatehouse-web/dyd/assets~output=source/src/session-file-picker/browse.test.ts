import { describe, expect, it } from "vitest"
import { fileBrowseFromSearch, fileBrowseURL } from "./browse"

describe("file picker URL state", () => {
  it("round-trips filters and paging while preserving the credential fragment", () => {
    const current = "https://gatehouse.test/app/tools/session-file-picker/#version=1&api=encoded-api&capability=field-token"
    const state = { name: "report %_", mediaType: "image/*", limit: 50, cursor: "sfi_cursor", direction: "previous" as const }
    const url = new URL(fileBrowseURL(current, state))
    expect(fileBrowseFromSearch(url.search)).toEqual(state)
    expect(url.hash).toBe(new URL(current).hash)
    expect(url.searchParams.has("capability")).toBe(false)
    expect(url.search).not.toContain("field-token")
    expect(fileBrowseFromSearch(new URL(fileBrowseURL(url.href, fileBrowseFromSearch(""))).search)).toEqual(fileBrowseFromSearch(""))
  })

  it("bounds page sizes and normalizes invalid backward navigation", () => {
    expect(fileBrowseFromSearch("?limit=100000&direction=previous")).toEqual(fileBrowseFromSearch(""))
    expect(fileBrowseFromSearch("?limit=0").limit).toBe(25)
    expect(fileBrowseFromSearch("?limit=10").limit).toBe(10)
  })
})
