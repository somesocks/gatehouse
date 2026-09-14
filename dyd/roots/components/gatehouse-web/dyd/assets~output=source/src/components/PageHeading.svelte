<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    as = "div",
    actions,
    children,
  }: {
    as?: "div" | "header"
    actions?: Snippet
    children: Snippet
  } = $props()
</script>

<svelte:element this={as} class="page-heading">
  <div class="page-heading-leading">
    {@render children()}
  </div>
  {#if actions}
    <div class="page-heading-actions">
      {@render actions()}
    </div>
  {/if}
</svelte:element>

<style>
  .page-heading {
    align-items: flex-start;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    margin: 0;
    padding: 0;
  }

  .page-heading-leading {
    min-width: 0;
  }

  .page-heading-leading :global(h1),
  .page-heading-leading :global(h2) {
    color: var(--brand-text);
    font-family: var(--brand-font-display);
    font-size: 1.75rem;
    font-weight: var(--brand-font-weight-display);
    letter-spacing: -0.02em;
    line-height: 1.2;
    margin: 0;
  }

  .page-heading-leading :global(p:not(.eyebrow)),
  .page-heading-leading :global(small) {
    color: var(--brand-text-muted);
  }

  .page-heading-leading :global(small) {
    display: block;
    margin-top: 0.5rem;
  }

  .page-heading-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    justify-content: flex-end;
  }

  @media (max-width: 700px) {
    .page-heading {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
