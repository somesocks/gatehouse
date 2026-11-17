<script lang="ts">
  import { Copy, Ellipsis, MessageSquareReply } from "@lucide/svelte"
  import Button from "./Button.svelte"
  import * as DropdownMenu from "./dropdown-menu"

  let {
    text,
    onreply,
  }: {
    text?: string
    onreply?: () => void
  } = $props()
  let error = $state("")

  $effect(() => {
    if (!error) return
    const timeout = setTimeout(() => {
      error = ""
    }, 2500)
    return () => clearTimeout(timeout)
  })

  async function copy() {
    if (!text) return
    error = ""
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      error = "Could not copy message. Try again."
    }
  }
</script>

{#if text || onreply}
  <div class="relative" role="group" aria-label="Message actions">
    <div
      data-slot="message-toolbar"
      class="pointer-events-none hidden items-center gap-0.5 rounded-md border bg-background p-0.5 opacity-0 shadow-sm group-focus-within/message:pointer-events-auto group-focus-within/message:opacity-100 group-hover/message:pointer-events-auto group-hover/message:opacity-100 [@media(hover:hover)_and_(pointer:fine)_and_(not_(any-pointer:coarse))]:flex"
    >
      {#if text}
        <Button
          variant="ghost"
          size="icon-sm"
          type="button"
          aria-label="Copy message"
          title="Copy message"
          onclick={() => void copy()}
        >
          <Copy aria-hidden="true" />
        </Button>
      {/if}
      {#if onreply}
        <Button
          variant="ghost"
          size="icon-sm"
          type="button"
          aria-label="Reply"
          title="Reply in thread"
          onclick={onreply}
        >
          <MessageSquareReply aria-hidden="true" />
        </Button>
      {/if}
    </div>
    <div
      data-slot="message-menu"
      class="[@media(hover:hover)_and_(pointer:fine)_and_(not_(any-pointer:coarse))]:hidden"
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          size="icon"
          aria-label="Message actions"
          title="Message actions"
        >
          <Ellipsis aria-hidden="true" />
        </DropdownMenu.Trigger>
        <DropdownMenu.Content align="end" class="min-w-40">
          {#if text}
            <DropdownMenu.Item
              class="min-h-10 px-3"
              onSelect={() => void copy()}
            >
              <Copy aria-hidden="true" /> Copy message
            </DropdownMenu.Item>
          {/if}
          {#if onreply}
            <DropdownMenu.Item class="min-h-10 px-3" onSelect={onreply}>
              <MessageSquareReply aria-hidden="true" /> Reply
            </DropdownMenu.Item>
          {/if}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </div>
    {#if error}
      <div
        role="alert"
        aria-live="polite"
        class="absolute end-0 top-full mt-1 w-max max-w-64 rounded-md border bg-background px-2 py-1 text-xs shadow-sm"
      >
        {error}
      </div>
    {/if}
  </div>
{/if}
