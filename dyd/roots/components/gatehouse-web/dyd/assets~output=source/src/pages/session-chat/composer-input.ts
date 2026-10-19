export function insertedMentionStart(
  value: string,
  cursor: number,
  inserted: string | null,
  composing = false,
): number | undefined {
  if (composing || inserted !== "@" || cursor < 1 || value[cursor - 1] !== "@")
    return undefined
  const start = cursor - 1
  // An email address or an embedded alias is ordinary message text.
  if (start > 0 && /[\p{L}\p{N}_./-]/u.test(value[start - 1])) return undefined
  return start
}

export function shouldSubmitComposerKey(
  event: Pick<KeyboardEvent, "key" | "shiftKey" | "ctrlKey" | "metaKey" | "isComposing" | "keyCode">,
  mobile: boolean,
): boolean {
  return event.key === "Enter" && !event.shiftKey &&
    !event.isComposing && event.keyCode !== 229 &&
    (!mobile || event.ctrlKey || event.metaKey)
}
