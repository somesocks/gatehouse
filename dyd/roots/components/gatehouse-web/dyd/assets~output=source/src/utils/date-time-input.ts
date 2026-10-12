export function dateTimeInput(value: unknown): string {
  if (!value) return ""
  const date = new Date(String(value))
  if (!Number.isFinite(date.getTime())) return ""
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, -1)
}

export function dateTimeValue(input: string): string {
  if (!input) return ""
  const date = new Date(input)
  return Number.isFinite(date.getTime()) ? date.toISOString() : input
}
