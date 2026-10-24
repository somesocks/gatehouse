export type ResourceRole = "guest" | "member" | "admin"

export const resourceRoles: { value: ResourceRole; label: string; description: string }[] = [
  { value: "guest", label: "Guest", description: "Can view content they have access to." },
  { value: "member", label: "Member", description: "Can contribute content and participate." },
  { value: "admin", label: "Admin", description: "Can also manage settings and access within this scope." },
]

export function roleLabel(role: string): string {
  return resourceRoles.find((item) => item.value === role)?.label ?? role
}

export function roleDescription(role: ResourceRole): string {
  return resourceRoles.find((item) => item.value === role)?.description ?? ""
}
