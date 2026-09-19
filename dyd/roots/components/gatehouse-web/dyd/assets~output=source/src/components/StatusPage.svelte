<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    eyebrow,
    title,
    description,
    busy = false,
    live,
    children,
  }: {
    eyebrow?: string
    title?: string
    description?: string
    busy?: boolean
    live?: "polite" | "assertive" | "off"
    children?: Snippet
  } = $props()
</script>

<main class="centered-page" aria-busy={busy || undefined} aria-live={live}>
  <section class="card surface stack">
    {#if eyebrow}<small>{eyebrow}</small>{/if}
    {#if title || description}
      <hgroup>
        {#if title}<h1>{title}</h1>{/if}
        {#if description}<p>{description}</p>{/if}
      </hgroup>
    {/if}
    {#if busy}<span class="spinner" role="status"><span class="visually-hidden">Loading</span></span>{/if}
    {#if children}{@render children()}{/if}
  </section>
</main>
