<script lang="ts">
  import { X, RouterLink } from "../../ui"
  import * as UI from "../../ui"

  let {
    path,
    value,
    busy,
    onOpen,
    onRemove,
    label,
  }: {
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
    void opener(JSON.parse(key))
      .then((destination) => {
        if (active) url = destination
      })
      .catch((reason) => {
        if (active)
          error =
            reason instanceof Error
              ? reason.message
              : "The tool could not be opened."
      })
    return () => {
      active = false
    }
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

<UI.Stack>
  <UI.Row>
    {#if busy}
      <UI.Text size="small">Tool unavailable while the form is busy.</UI.Text>
    {:else if saving}
      <UI.Text role="status">Wait for the field update to finish.</UI.Text>
    {:else if url !== ""}
      <RouterLink
        variant="outline"
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        referrerpolicy="no-referrer">Open</RouterLink
      >
    {:else if error === ""}
      <UI.Text role="status">Preparing tool link…</UI.Text>
    {/if}
    {#if value !== undefined || saving}
      {#if value !== undefined}<UI.Text size="small" role="status"
          >Saved</UI.Text
        >{/if}
      <UI.Button
        variant="ghost"
        size="icon"
        type="button"
        aria-label={`Clear ${label}`}
        title={`Clear ${label}`}
        disabled={busy || saving}
        onclick={() => void clearValue()}
      >
        {#if saving}<UI.Spinner aria-hidden="true"></UI.Spinner><UI.Text hidden
            >Clearing {label}</UI.Text
          >
        {:else}<X size={16} strokeWidth={2} aria-hidden="true" />{/if}
      </UI.Button>
    {/if}
  </UI.Row>
  {#if error !== ""}
    <UI.Text as="p" role="alert">{error}</UI.Text>
    <UI.Button
      variant="outline"
      size="sm"
      type="button"
      disabled={busy}
      onclick={() => (retry += 1)}>Retry tool link</UI.Button
    >
  {/if}
</UI.Stack>
