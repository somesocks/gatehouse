<script lang="ts">
  import type { Snippet } from "svelte"

  let { mobileMenuOpen, onMenuClose, brandTheme = false, workspaceMainElement = $bindable<HTMLElement | undefined>(undefined), sidebar, header, children }: { mobileMenuOpen: boolean; onMenuClose: () => void; brandTheme?: boolean; workspaceMainElement?: HTMLElement; sidebar: Snippet; header: Snippet; children: Snippet } = $props()
</script>

<div class={`app-shell${brandTheme ? " brand-workspace-frame" : ""}`}>
  {#if mobileMenuOpen}<button class="mobile-menu-backdrop" type="button" aria-label="Close navigation menu" onclick={onMenuClose}></button>{/if}
  <aside class:mobile-menu-open={mobileMenuOpen} class="sidebar">{@render sidebar()}</aside>
  <main class="workspace-main" bind:this={workspaceMainElement}>
    <header class="workspace-header">{@render header()}</header>
    {@render children()}
  </main>
</div>
