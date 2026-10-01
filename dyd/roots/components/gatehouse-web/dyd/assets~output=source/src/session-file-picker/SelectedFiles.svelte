<script lang="ts">
  import { Paperclip, X, Download } from "@lucide/svelte"
  import type { SessionFile } from "./controller.svelte"

  let { files, busy = false, opening = "", onRemove, onOpen }: {
    files: SessionFile[]
    busy?: boolean
    opening?: string
    onRemove: (id: string) => void
    onOpen?: (file: SessionFile) => void | Promise<void>
  } = $props()
</script>

<div class="attachment-list" aria-label="Selected files">
  {#each files as file (file.id)}
    <span class="attachment-chip badge">
      <span class="attachment-chip-icon"><Paperclip size={14} strokeWidth={2} aria-hidden="true" /></span>
      <span class="attachment-chip-label"><span>{file.name}</span><small>{file.unavailable ? "Unavailable" : file.size === undefined ? file.id : `${file.size} bytes${file.media_type ? ` · ${file.media_type}` : ""}`}</small></span>
      {#if onOpen !== undefined && !file.unavailable}
        <button class="icon inline secondary" type="button" disabled={busy || opening !== ""} aria-label={`Download ${file.name}`} title={`Download ${file.name}`} onclick={() => void onOpen?.(file)}>
          {#if opening === file.id}<span class="spinner" aria-hidden="true"></span>{:else}<Download size={14} strokeWidth={2} aria-hidden="true" />{/if}
        </button>
      {/if}
      <button class="icon inline secondary" type="button" disabled={busy} aria-label={`Remove ${file.name}`} title={`Remove ${file.name}`} onclick={() => onRemove(file.id)}><X size={14} strokeWidth={2} aria-hidden="true" /></button>
    </span>
  {/each}
</div>
