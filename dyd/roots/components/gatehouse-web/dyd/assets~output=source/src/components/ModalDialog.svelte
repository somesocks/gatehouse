<script lang="ts">
  import type { Snippet } from "svelte"

  let { dialog = $bindable(), labelledBy, onClose, header, children }: { dialog?: HTMLDialogElement; labelledBy?: string; onClose: () => void; header: Snippet; children: Snippet } = $props()
</script>

<dialog class="note-history-dialog" aria-labelledby={labelledBy} bind:this={dialog} onclose={onClose}>
  {@render header()}
  {@render children()}
</dialog>
