<script lang="ts">
  import { X } from "@lucide/svelte"
  import RouterLink from "../../components/RouterLink.svelte"

  let { path, value, busy, onOpen, onRemove, label }: {
    path: string[]
    label: string
    value: unknown
    busy: boolean
    onOpen?: (path: string[]) => Promise<string>
    onRemove: (path: string[]) => Promise<boolean>
  } = $props()
  let url = $state("")
  let error = $state("")
  let retry = $state(0)
  let saving = $state(false)

  $effect(() => {
    const opener = onOpen
    const key = JSON.stringify(path)
    const attempt = retry
    if (opener === undefined) return
    let active = true
    url = ""
    error = ""
    void opener(JSON.parse(key)).then((destination) => { if (active) url = destination })
      .catch((reason) => { if (active) error = reason instanceof Error ? reason.message : "The tool could not be opened." })
    return () => { active = false }
  })

  async function clearValue(): Promise<void> {
    if (busy || saving || value === undefined) return
    saving = true
    try {
      await onRemove(path)
    } finally {
      saving = false
    }
  }
</script>

<div class="stack custom-field-control">
  <div class="cluster">
    {#if busy}
      <span class="field-help">Tool unavailable while the form is busy.</span>
    {:else if saving}
      <span role="status">Wait for the field update to finish.</span>
    {:else if url !== ""}
      <RouterLink class="secondary" href={url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">Open</RouterLink>
    {:else if error === ""}
      <span role="status">Preparing tool link…</span>
    {/if}
    {#if value !== undefined || saving}
      {#if value !== undefined}<span class="field-help" role="status">Saved</span>{/if}
      <button class="icon inline" type="button" aria-label={`Clear ${label}`} title={`Clear ${label}`} disabled={busy || saving} onclick={() => void clearValue()}>
        {#if saving}<span class="spinner input-action-spinner" aria-hidden="true"></span><span class="visually-hidden">Clearing {label}</span>
        {:else}<X size={16} strokeWidth={2} aria-hidden="true" />{/if}
      </button>
    {/if}
  </div>
  {#if error !== ""}
    <p role="alert">{error}</p>
    <button type="button" class="secondary small" disabled={busy} onclick={() => retry += 1}>Retry tool link</button>
  {/if}
</div>
