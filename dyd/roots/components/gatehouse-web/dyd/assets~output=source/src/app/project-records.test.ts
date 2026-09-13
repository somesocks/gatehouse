import { afterEach, describe, expect, it, vi } from "vitest"
import { createProjectRecord, createProjectRecordAttribute, createProjectRecordSchema, fetchProjectRecordIncomingReferences, fetchProjectRecordValues, mutateProjectRecordValues, updateProjectRecordAttribute } from "./project-records"

afterEach(() => vi.unstubAllGlobals())

describe("project records transport", () => {
  it("uses encoded schema and record paths with the batch mutation endpoint", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    await createProjectRecordSchema("wsp/test", "prj/test", { name: "contacts", label: "Contacts", description: "" })
    await createProjectRecord("wsp/test", "prj/test", "sch/test", [])
    await createProjectRecordAttribute("wsp/test", "prj/test", "sch/test", { name: "email", label: "Email", description: "", type: "text", cardinality: "one", uniqueness: "none", display: "primary", display_order: 1 })
    await updateProjectRecordAttribute("wsp/test", "prj/test", "sch/test", "att/test", { label: "Email address", description: "", type: "text", cardinality: "one", uniqueness: "none", display: "primary", display_order: 2 })
    await fetchProjectRecordValues("wsp/test", "prj/test", "sch/test", "rec/test")
    await fetchProjectRecordIncomingReferences("wsp/test", "prj/test", "sch/test", "rec/test", "prv/test")
    await mutateProjectRecordValues("wsp/test", "prj/test", "sch/test", "rec/test", { create: [], update: [], delete: [] })
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/attributes", expect.objectContaining({ method: "POST", credentials: "same-origin", body: JSON.stringify({ name: "email", label: "Email", description: "", type: "text", cardinality: "one", uniqueness: "none", display: "primary", display_order: 1 }) }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/attributes/att%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin", body: JSON.stringify({ label: "Email address", description: "", type: "text", cardinality: "one", uniqueness: "none", display: "primary", display_order: 2 }) }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/values", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(6, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/references?cursor=prv%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(7, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/values/mutate", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
  })
})
