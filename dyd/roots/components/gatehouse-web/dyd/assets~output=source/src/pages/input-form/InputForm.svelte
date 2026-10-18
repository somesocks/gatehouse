<script lang="ts">
  import * as UI from "../../ui"
  import { onMount } from "svelte"
  import type { createInputFormController } from "./input-controller.svelte"
  import FormField from "./FormField.svelte"

  let {
    controller,
    inputID,
    depth = 1,
  }: {
    controller: ReturnType<typeof createInputFormController>
    inputID: string
    depth?: number
  } = $props()
  let formElement = $state<HTMLFormElement | undefined>()

  onMount(() => {
    // Text fields save on change. Commit an active field before leaving for a
    // tool so the focus refresh cannot replace text that has not saved yet.
    const blur = () => {
      if (
        document.activeElement instanceof HTMLElement &&
        formElement?.contains(document.activeElement)
      )
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

{#if controller.state.status === "unavailable"}
  <UI.Text as="p" role="alert">The form could not be loaded.</UI.Text>
  <UI.Button
    variant="outline"
    type="button"
    onclick={() => void controller.load()}>Try again</UI.Button
  >
{:else if controller.state.status === "denied"}
  <UI.Text as="p" role="alert"
    >This input is unavailable or you no longer have access.</UI.Text
  >
{:else if controller.state.status === "resolved"}
  <UI.Text as="p" role="status">This input has already been answered.</UI.Text>
{:else if controller.state.status === "submitted" || controller.state.status === "cancelled"}
  <UI.Text as="p" role="status"
    >{controller.state.status === "submitted"
      ? "Input submitted."
      : "Input cancelled."}</UI.Text
  >
{:else if controller.state.form !== null}
  <UI.Form
    data-input-form
    bind:ref={formElement}
    aria-label={controller.state.form.title}
    onsubmit={(event) => {
      event.preventDefault()
      if (
        document.activeElement instanceof HTMLElement &&
        formElement?.contains(document.activeElement)
      )
        document.activeElement.blur()
      void controller.submit()
    }}
  >
    {#each controller.state.form.fields as field (field.id)}
      <FormField
        {field}
        path={[field.id ?? ""]}
        controlID={`input-${inputID}-${field.id}`}
        {depth}
        value={controller.value([field.id ?? ""])}
        busy={controller.state.terminalPending || controller.state.needsReload}
        onSet={controller.set}
        onRemove={controller.remove}
        onUpload={controller.upload}
        onOpen={controller.openCustom}
      />
    {/each}
    {#if controller.state.error !== ""}<UI.Text as="p" size="small" role="alert"
        >{controller.state.error}</UI.Text
      >{/if}
    {#if controller.state.needsReload || controller.state.invalidAnswer}<UI.Button
        variant="outline"
        type="button"
        disabled={controller.state.reloading}
        onclick={() => void controller.load()}>Retry unsaved changes</UI.Button
      >{/if}
    <UI.Row justify="end">
      <UI.Button
        variant="outline"
        type="button"
        disabled={controller.state.terminalPending}
        onclick={() => void controller.cancel()}>Cancel</UI.Button
      >
      <UI.Button
        variant="default"
        type="submit"
        disabled={controller.state.terminalPending ||
          controller.state.reloading ||
          controller.state.needsReload}>Submit</UI.Button
      >
    </UI.Row>
  </UI.Form>
{/if}
