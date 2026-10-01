<script lang="ts">
  import { onMount } from "svelte"
  import type { createInputFormController } from "./input-controller.svelte"
  import FormField from "./FormField.svelte"

  let { controller, inputID, depth = 1 }: {
    controller: ReturnType<typeof createInputFormController>
    inputID: string
    depth?: number
  } = $props()
  let formElement = $state<HTMLFormElement | undefined>()

  onMount(() => {
    // Text fields save on change. Commit an active field before leaving for a
    // tool so the focus refresh cannot replace text that has not saved yet.
    const blur = () => {
      if (document.activeElement instanceof HTMLElement && formElement?.contains(document.activeElement))
        document.activeElement.blur()
    }
    const focus = () => void controller.refresh()
    window.addEventListener("blur", blur)
    window.addEventListener("focus", focus)
    return () => {
      window.removeEventListener("blur", blur)
      window.removeEventListener("focus", focus)
    }
  })
</script>

{#if controller.state.status === "loading"}
  <p role="status">Loading the shared form draft...</p>
{:else if controller.state.status === "unavailable"}
  <p role="alert">The form could not be loaded.</p>
  <button class="secondary" type="button" onclick={() => void controller.load()}>Try again</button>
{:else if controller.state.status === "denied"}
  <p role="alert">This input is unavailable or you no longer have access.</p>
{:else if controller.state.status === "resolved"}
  <p role="status">This input has already been answered.</p>
{:else if controller.state.status === "submitted" || controller.state.status === "cancelled"}
  <p role="status">{controller.state.status === "submitted" ? "Input submitted." : "Input cancelled."}</p>
{:else if controller.state.form !== null}
  <form bind:this={formElement} class="stack input-form" aria-label={controller.state.form.title} onsubmit={(event) => {
    event.preventDefault()
    if (document.activeElement instanceof HTMLElement && formElement?.contains(document.activeElement))
      document.activeElement.blur()
    void controller.submit()
  }}>
    {#each controller.state.form.fields as field (field.id)}
      <FormField {field} path={[field.id ?? ""]} controlID={`input-${inputID}-${field.id}`} {depth} value={controller.value([field.id ?? ""])} busy={controller.state.terminalPending || controller.state.needsReload} onSet={controller.set} onRemove={controller.remove} onUpload={controller.upload} onOpen={controller.openCustom} />
    {/each}
    {#if controller.state.error !== ""}<p class="field-help" role="alert">{controller.state.error}</p>{/if}
    {#if controller.state.needsReload}<button type="button" class="secondary" onclick={() => void controller.load()}>Reload shared draft</button>{/if}
    <div class="cluster input-form-actions">
      <button class="secondary" type="button" disabled={controller.state.terminalPending} onclick={() => void controller.cancel()}>Cancel</button>
      <button class="primary" type="submit" disabled={controller.state.terminalPending || controller.state.needsReload}>Submit</button>
    </div>
  </form>
{/if}
