import { expect, it } from "vitest"
import { reducesRoleAccess } from "./role-access"

it("identifies permission reductions across workspace, project, chat, and group roles", () => {
  expect(reducesRoleAccess("admin", "member")).toBe(true)
  expect(reducesRoleAccess("admin", "guest")).toBe(true)
  expect(reducesRoleAccess("member", "guest")).toBe(true)
  expect(reducesRoleAccess("guest", "admin")).toBe(false)
  expect(reducesRoleAccess("member", "admin")).toBe(false)
  expect(reducesRoleAccess("guest", "guest")).toBe(false)
})
