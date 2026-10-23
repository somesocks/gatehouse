import { expect, it } from "vitest"
import { reducesRoleAccess } from "./role-access"

it("identifies permission reductions across workspace, project, chat, and group roles", () => {
  expect(reducesRoleAccess("manager", "contributor")).toBe(true)
  expect(reducesRoleAccess("manager", "member")).toBe(true)
  expect(reducesRoleAccess("contributor", "member")).toBe(true)
  expect(reducesRoleAccess("member", "manager")).toBe(false)
  expect(reducesRoleAccess("member", "member")).toBe(false)
})
