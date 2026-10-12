export type ReferenceDraft = {
  id?: string
  value: unknown
  removed?: boolean
  changed?: boolean
}

export function setRecordReference<T extends ReferenceDraft>(
  drafts: T[],
  index: number,
  value: string,
): T[] {
  const draft = drafts[index]
  if (!draft) return drafts
  if (!value && !draft.id)
    return drafts.filter((_, position) => position !== index)
  return drafts.map((item, position) =>
    position !== index
      ? item
      : !value
        ? { ...item, removed: true }
        : { ...item, value, changed: true },
  )
}
