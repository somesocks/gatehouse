<script lang="ts">
  import Panel from "./Panel.svelte"
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let {
    align = "start",
    header,
    actions,
    children,
    ...rest
  }: Attributes & {
    align?: "start" | "end"
    header?: Snippet
    actions?: Snippet
  } = $props()
</script>

<article
  data-slot="message"
  data-align={align}
  class={align === "end"
    ? "relative w-fit min-w-[min(28rem,100%)] max-w-[min(42rem,100%)] self-end"
    : "relative w-fit min-w-[min(28rem,100%)] max-w-[min(42rem,100%)] self-start"}
  {...rest}
>
  <Panel tone={align === "end" ? "primary-tint" : "secondary"}>
    {#if header}<header class="pr-8 text-base font-semibold text-foreground">
        {@render header()}
      </header>{/if}
    {@render children?.()}
  </Panel>
  {#if actions}<div class="absolute end-2 top-2">{@render actions()}</div>{/if}
</article>
