<script lang="ts">
  import Panel from "./Panel.svelte"
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let {
    align = "start",
    purpose = "message",
    header,
    metadata,
    actions,
    children,
    ...rest
  }: Attributes & {
    align?: "start" | "end"
    purpose?: "message" | "output" | "form"
    header?: Snippet
    metadata?: Snippet
    actions?: Snippet
  } = $props()
</script>

<article
  data-slot="message"
  data-align={align}
  data-purpose={purpose}
  class={align === "end"
    ? "relative w-fit min-w-[min(28rem,100%)] max-w-[min(42rem,100%)] self-end"
    : "relative w-fit min-w-[min(28rem,100%)] max-w-[min(42rem,100%)] self-start"}
  {...rest}
>
  <Panel tone={align === "end" ? "primary-tint" : "secondary"}>
    {#if header}<header class="flex flex-col gap-1 pr-8">
        <div class="text-base font-semibold text-foreground">{@render header()}</div>
        {#if metadata}{@render metadata()}{/if}
      </header>{/if}
    {@render children?.()}
  </Panel>
  {#if actions}<div class="absolute end-2 top-2">{@render actions()}</div>{/if}
</article>
