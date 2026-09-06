<script lang="ts">
  import type { Snippet } from "svelte"

  let { dialog = $bindable(), labelledBy, onClose, header, children, class: className = "note-history-dialog" }: { dialog?: HTMLDialogElement; labelledBy?: string; onClose: () => void; header: Snippet; children: Snippet; class?: string } = $props()
</script>

<dialog class={className} aria-labelledby={labelledBy} bind:this={dialog} onclose={onClose}>
  {@render header()}
  {@render children()}
</dialog>
