<script lang="ts">
  import type { Snippet } from "svelte"

  let { dialog = $bindable(), labelledBy, onClose, brandTheme = false, header, children, class: className = "note-history-dialog" }: { dialog?: HTMLDialogElement; labelledBy?: string; onClose: () => void; brandTheme?: boolean; header: Snippet; children: Snippet; class?: string } = $props()
</script>

<dialog class={`${className}${brandTheme ? " brand-modal-dialog" : ""}`} aria-labelledby={labelledBy} bind:this={dialog} onclose={onClose}>
  {@render header()}
  {@render children()}
</dialog>
