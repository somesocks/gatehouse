<script lang="ts">
  import * as UI from "../ui"
  import { Paperclip, X, Download } from "../ui"
  import type { SessionFile } from "./controller.svelte"

  let {
    files,
    busy = false,
    opening = "",
    onRemove,
    onOpen,
  }: {
    files: SessionFile[]
    busy?: boolean
    opening?: string
    onRemove: (id: string) => void
    onOpen?: (file: SessionFile) => void | Promise<void>
  } = $props()
</script>

<UI.Row aria-label="Selected files">
  {#each files as file (file.id)}
    <UI.Chip>
      <UI.Text
        ><Paperclip size={14} strokeWidth={2} aria-hidden="true" /></UI.Text
      >
      <UI.Row as="span"
        ><UI.Text>{file.name}</UI.Text><UI.Text as="small" size="small"
          >{file.unavailable
            ? "Unavailable"
            : file.size === undefined
              ? "Size unavailable"
              : `${file.size} bytes${file.media_type ? ` · ${file.media_type}` : ""}`}</UI.Text
        ></UI.Row
      >
      {#if onOpen !== undefined && !file.unavailable}
        <UI.Button
          variant="ghost"
          size="icon"
          type="button"
          disabled={busy || opening !== ""}
          aria-label={`Download ${file.name}`}
          title={`Download ${file.name}`}
          onclick={() => void onOpen?.(file)}
        >
          {#if opening === file.id}<UI.Spinner aria-hidden="true"
            ></UI.Spinner>{:else}<Download
              size={14}
              strokeWidth={2}
              aria-hidden="true"
            />{/if}
        </UI.Button>
      {/if}
      <UI.Button
        variant="ghost"
        size="icon"
        type="button"
        disabled={busy}
        aria-label={`Remove ${file.name}`}
        title={`Remove ${file.name}`}
        onclick={() => onRemove(file.id)}
        ><X size={14} strokeWidth={2} aria-hidden="true" /></UI.Button
      >
    </UI.Chip>
  {/each}
</UI.Row>
