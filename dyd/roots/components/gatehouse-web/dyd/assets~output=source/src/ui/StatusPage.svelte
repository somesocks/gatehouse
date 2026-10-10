<script lang="ts">
  import { GalleryVerticalEnd } from "@lucide/svelte"
  import type { Snippet } from "svelte"
  import * as Card from "../lib/vendor/shadcn/card"
  import Text from "./Text.svelte"
  import Spinner from "./Spinner.svelte"
  let {
    eyebrow,
    brand,
    title,
    description,
    busy = false,
    live,
    width = "md",
    centered = false,
    children,
  }: {
    eyebrow?: string
    brand?: string
    title?: string
    description?: string
    busy?: boolean
    live?: "polite" | "assertive" | "off"
    width?: "sm" | "md"
    centered?: boolean
    children?: Snippet
  } = $props()
</script>

<main
  class="flex min-h-svh flex-col items-center justify-center gap-6 bg-muted p-6 md:p-10"
  aria-busy={busy || undefined}
  aria-live={live}
>
  <div
    class={width === "sm"
      ? "flex w-full max-w-sm flex-col gap-6"
      : "flex w-full max-w-md flex-col gap-6"}
  >
    {#if brand !== undefined || busy}<div
        class="flex items-center gap-2 self-center font-medium"
      >
        <span
          class="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"
          ><GalleryVerticalEnd class="size-4" aria-hidden="true" /></span
        >
        {brand ?? eyebrow ?? "Gatehouse"}
      </div>{/if}
    <Card.Root class="ring-0 shadow-sm">
      {#if (eyebrow && brand === undefined && !busy) || title || description}<Card.Header
          class={centered ? "text-center" : undefined}
        >
          {#if eyebrow && brand === undefined && !busy}<Text size="eyebrow"
              >{eyebrow}</Text
            >{/if}
          {#if title}<Card.Title class={centered ? "text-xl" : undefined}
              >{title}</Card.Title
            >{/if}{#if description}<Card.Description
              >{description}</Card.Description
            >{/if}
        </Card.Header>{/if}
      <Card.Content>
        <div class="flex min-w-0 flex-col gap-4">
          {#if busy}<Spinner role="status" aria-label="Loading" />{/if}
          {@render children?.()}
        </div>
      </Card.Content>
    </Card.Root>
  </div>
</main>
