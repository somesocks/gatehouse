export function reducesRoleAccess(before: string, after: string): boolean {
  const roles = ["guest", "member", "admin"]
  const previous = roles.indexOf(before)
  const next = roles.indexOf(after)
  return previous >= 0 && next >= 0 && next < previous
}
