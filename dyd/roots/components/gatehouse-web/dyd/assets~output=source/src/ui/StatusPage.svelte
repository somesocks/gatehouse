<script lang="ts">
  import type { Snippet } from "svelte"
  import Panel from "./Panel.svelte"
  import Stack from "./Stack.svelte"
  import Heading from "./Heading.svelte"
  import Text from "./Text.svelte"
  import Spinner from "./Spinner.svelte"
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

<main
  class="grid min-h-svh place-items-center p-6"
  aria-busy={busy || undefined}
  aria-live={live}
>
  <div class="w-full max-w-md">
    <Panel>
      <Stack>
        {#if eyebrow}<Text size="eyebrow">{eyebrow}</Text>{/if}
        {#if title}<Heading level={1}>{title}</Heading>{/if}
        {#if description}<Text as="p" tone="muted">{description}</Text>{/if}
        {#if busy}<Spinner role="status" aria-label="Loading" />{/if}
        {@render children?.()}
      </Stack>
    </Panel>
  </div>
</main>
