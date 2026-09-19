<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    modal = $bindable(),
    labelledBy,
    onClose,
    header,
    children,
    class: className,
  }: {
    modal?: HTMLDialogElement
    labelledBy?: string
    onClose: () => void
    header: Snippet
    children: Snippet
    class?: string
  } = $props()
</script>

<dialog
  class={`modal${className === undefined ? "" : ` ${className}`}`}
  aria-labelledby={labelledBy}
  bind:this={modal}
  onclose={onClose}
>
  <div class="modal-body stack">
    <div class="modal-header">
      {@render header()}
    </div>
    {@render children()}
  </div>
</dialog>
