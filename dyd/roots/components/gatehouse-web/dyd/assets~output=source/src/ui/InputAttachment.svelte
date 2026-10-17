<script lang="ts">
  import { CircleAlert, FileText, X } from "@lucide/svelte"
  import Button from "./Button.svelte"
  import Spinner from "./Spinner.svelte"
  import { cn } from "../lib/utils"

  let {
    name,
    size,
    status,
    uploaded = false,
    error,
    disabled = false,
    onremove,
  }: {
    name: string
    size: number
    status: "pending" | "uploading" | "failed"
    uploaded?: boolean
    error?: string
    disabled?: boolean
    onremove: () => void
  } = $props()

  const fileSize = $derived(
    size < 1024
      ? `${size} B`
      : size < 1024 * 1024
        ? `${(size / 1024).toFixed(1)} KB`
        : `${(size / (1024 * 1024)).toFixed(1)} MB`,
  )
</script>

<div
  data-state={status}
  class={cn(
    "flex w-60 max-w-full min-w-0 items-center gap-2 rounded-xl border bg-muted/40 py-2 pr-1 pl-3",
    status === "failed" && "border-destructive/40 bg-destructive/5",
  )}
>
  <span class={cn("shrink-0 text-muted-foreground", status === "failed" && "text-destructive")}>
    {#if status === "uploading"}
      <Spinner aria-hidden="true" />
    {:else if status === "failed"}
      <CircleAlert size={18} aria-hidden="true" />
    {:else}
      <FileText size={18} aria-hidden="true" />
    {/if}
  </span>
  <div class="min-w-0 flex-1 text-sm">
    <p class="truncate font-medium" title={name}>{name}</p>
    <p
      aria-live="polite"
      class={cn("truncate text-xs text-muted-foreground", status === "failed" && "text-destructive")}
      title={status === "failed" ? error : undefined}
    >
      {status === "uploading"
        ? "Uploading…"
        : status === "failed"
          ? (error ?? "Upload failed")
          : uploaded
            ? "Ready"
            : fileSize}
    </p>
  </div>
  <Button
    variant="ghost"
    size="icon-sm"
    type="button"
    aria-label={`Remove ${name}`}
    title={`Remove ${name}`}
    {disabled}
    onclick={onremove}
  ><X size={14} aria-hidden="true" /></Button>
</div>
