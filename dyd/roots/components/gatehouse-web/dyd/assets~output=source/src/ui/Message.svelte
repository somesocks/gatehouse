<script lang="ts">
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let {
    purpose = "message",
    avatar,
    header,
    timestamp,
    metadata,
    actions,
    children,
    ...rest
  }: Attributes & {
    purpose?: "message" | "output" | "form"
    avatar?: Snippet
    header?: Snippet
    timestamp?: string
    metadata?: Snippet
    actions?: Snippet
  } = $props()
  const date = $derived(timestamp ? new Date(timestamp) : undefined)
  const validDate = $derived(
    date !== undefined && Number.isFinite(date.getTime()) ? date : undefined,
  )
</script>

<article
  data-slot="message"
  data-purpose={purpose}
  class={avatar
    ? "group/message relative grid w-full min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2"
    : "group/message relative w-full min-w-0"}
  {...rest}
>
  {#if avatar}<div data-slot="message-avatar" class="pt-1">
      {@render avatar()}
    </div>{/if}
  <div data-slot="message-layout" class="flex min-w-0 flex-col gap-0">
    {#if header || validDate || metadata || actions}
      <header class="min-w-0 break-words">
        {#if header}<span
            data-slot="message-author"
            class="me-2 font-semibold text-foreground">{@render header()}</span
          >{/if}
        {#if validDate}
          <time
            data-slot="message-timestamp"
            datetime={timestamp}
            title={validDate.toLocaleString()}
            aria-label={validDate.toLocaleString()}
            class="me-2 inline-block text-xs text-muted-foreground"
            >{validDate.toLocaleTimeString(undefined, {
              hour: "numeric",
              minute: "2-digit",
            })}</time
          >
        {/if}
        {#if metadata}<span
            data-slot="message-metadata"
            class="me-2 text-xs text-muted-foreground empty:hidden"
            >{@render metadata()}</span
          >{/if}
        {#if actions}
          <div
            data-slot="message-actions"
            class="relative inline-grid h-0 align-middle"
          >
            <div class="relative -top-1 z-10 -translate-y-1/2">
              {@render actions()}
            </div>
          </div>
        {/if}
      </header>
    {/if}
    <div data-slot="message-content" class="flex min-w-0 flex-col gap-1">
      {@render children?.()}
    </div>
  </div>
</article>
