import { describe, expect, it } from "vitest"
import { render } from "svelte/server"
import InputForm from "./InputForm.svelte"
import { createInputTransport } from "./input-api"
import { createInputFormController } from "./input-controller.svelte"

describe("inline input form", () => {
  it("namespaces labels and nested controls for multiple forms in one chat", async () => {
    const transport = createInputTransport("test")
    transport.read = async () => Response.json({
      form: { version: 1, type: "form", title: "Review", fields: [
        { id: "name", label: "Name", type: "text" },
        { id: "profile", label: "Profile", type: "object", fields: [
          { id: "city", label: "City", type: "text" },
        ] },
        { id: "visits", label: "Visits", type: "list", item: { type: "text" } },
      ] },
      draft: { visits: ["Paris"] },
    })
    const controller = createInputFormController(transport)
    await controller.load()
    const ids: string[][] = []
    for (const inputID of ["sev_first", "sev_second"]) {
      const body = render(InputForm, { props: { controller, inputID, depth: 2 } }).body
      const controls = [...body.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1])
      const labels = [...body.matchAll(/\bfor="([^"]+)"/g)].map((match) => match[1])
      expect(controls).toContain(`input-${inputID}-name`)
      expect(controls).toContain(`input-${inputID}-profile-city`)
      expect(controls).toContain(`input-${inputID}-visits-0`)
      expect(labels.every((label) => controls.includes(label))).toBe(true)
      expect(body).toMatch(/<h3\b/)
      expect(body).toContain('type="submit"')
      ids.push(controls)
    }
    expect(ids[0].some((id) => ids[1].includes(id))).toBe(false)
  })
})
