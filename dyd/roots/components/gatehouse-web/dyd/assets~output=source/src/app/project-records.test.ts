import { afterEach, describe, expect, it, vi } from "vitest"
import { createProjectRecord, createProjectRecordSchema, fetchProjectRecordIncomingReferences, fetchProjectRecordValues, mutateProjectRecordValues } from "./project-records"

afterEach(() => vi.unstubAllGlobals())

describe("project records transport", () => {
  it("uses encoded schema and record paths with the batch mutation endpoint", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    await createProjectRecordSchema("wsp/test", "prj/test", { name: "contacts", label: "Contacts", description: "" })
    await createProjectRecord("wsp/test", "prj/test", "sch/test", [])
    await fetchProjectRecordValues("wsp/test", "prj/test", "sch/test", "rec/test")
    await fetchProjectRecordIncomingReferences("wsp/test", "prj/test", "sch/test", "rec/test", "prv/test")
    await mutateProjectRecordValues("wsp/test", "prj/test", "sch/test", "rec/test", { create: [], update: [], delete: [] })
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/values", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/references?cursor=prv%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/record-schemas/sch%2Ftest/records/rec%2Ftest/values/mutate", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
  })
})
