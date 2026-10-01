<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { capabilityFromFragment, createInputTransport } from "./input-api"
  import { createInputFormController } from "./input-controller.svelte"
  import FormField from "./FormField.svelte"

  const capability = capabilityFromFragment(window.location.hash)
  const controller = capability === null ? null : untrack(() => createInputFormController(createInputTransport(capability)))

  onMount(() => {
    if (controller === null) return
    // Keep the capability in the fragment so a browser reload can reopen the
    // form; the fragment is not sent with HTTP requests.
    void controller.load()
    const blur = () => {
      if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    }
    const focus = () => {
      if (controller.state.status === "ready" && !controller.state.terminalPending)
        void controller.refresh()
    }
    window.addEventListener("blur", blur)
    window.addEventListener("focus", focus)
    return () => {
      window.removeEventListener("blur", blur)
      window.removeEventListener("focus", focus)
      controller.dispose()
    }
  })
</script>

<svelte:head><title>{controller?.state.form?.title ?? "Input"} · Gatehouse</title></svelte:head>

<main class="input-form-page stack">
  <header class="stack">
    <p class="eyebrow">Input form</p>
    <h1>{controller?.state.form?.title ?? "Input form"}</h1>
  </header>
  {#if controller === null}
    <p role="alert">The input link is missing its capability. Reopen the form from chat.</p>
  {:else if controller.state.status === "loading"}
    <p role="status">Loading the shared form draft...</p>
  {:else if controller.state.status === "unavailable"}
    <p role="alert">The form could not be loaded.</p>
    <button class="secondary" type="button" onclick={() => void controller.load()}>Try again</button>
  {:else if controller.state.status === "denied"}
    <p role="alert">This input link is unavailable or you no longer have access.</p>
  {:else if controller.state.status === "resolved"}
    <p role="status">This input has already been answered. Return to chat for its status.</p>
  {:else if controller.state.status === "submitted" || controller.state.status === "cancelled"}
    <p role="status">{controller.state.status === "submitted" ? "Input submitted." : "Input cancelled."} You can return to chat.</p>
  {:else if controller.state.form !== null}
    <form class="stack" onsubmit={(event) => {
      event.preventDefault()
      void controller.submit()
    }}>
      {#each controller.state.form.fields as field (field.id)}
        <FormField {field} path={[field.id ?? ""]} value={controller.value([field.id ?? ""])} busy={controller.state.terminalPending || controller.state.needsReload} onSet={controller.set} onRemove={controller.remove} onUpload={controller.upload} onOpen={controller.openCustom} />
      {/each}
      {#if controller.state.error !== ""}<p class="field-help" role="alert">{controller.state.error}</p>{/if}
      {#if controller.state.needsReload}<button type="button" class="secondary" onclick={() => void controller.load()}>Reload shared draft</button>{/if}
      <div class="cluster input-form-actions">
        <button class="secondary" type="button" disabled={controller.state.terminalPending} onclick={() => void controller.cancel()}>Cancel</button>
        <button class="primary" type="submit" disabled={controller.state.terminalPending || controller.state.needsReload}>Submit</button>
      </div>
    </form>
  {/if}
</main>
