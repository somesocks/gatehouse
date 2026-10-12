export function formatFileSize(bytes: number | undefined): string {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0)
    return "Unknown size"
  if (bytes < 1024) return `${bytes} bytes`
  const units = ["KiB", "MiB", "GiB", "TiB"]
  let size = bytes / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${units[index]}`
}
