<script lang="ts">
  import { onMount, tick, type Snippet } from "svelte"
  import type { HTMLFormAttributes } from "svelte/elements"
  import { cn } from "../lib/utils"

  let {
    children,
    input,
    leading,
    trailing,
    feedback,
    hasText = false,
    overlayOpen = false,
    ...rest
  }: Omit<HTMLFormAttributes, "class" | "style"> & {
    input?: Snippet
    leading?: Snippet
    trailing?: Snippet
    feedback?: Snippet
    hasText?: boolean
    overlayOpen?: boolean
  } = $props()
  let element = $state<HTMLFormElement>()
  let editing = $state(false)
  let interacting = $state(false)
  let releaseTimer: ReturnType<typeof setTimeout> | undefined
  const expanded = $derived(editing || hasText || interacting || overlayOpen)

  function release(): void {
    clearTimeout(releaseTimer)
    releaseTimer = undefined
    interacting = false
  }

  onMount(() => {
    // Keep the expanded controls in place from pointerdown through click.
    // Otherwise blurring the textarea can move Send out from under a tap.
    const pointerUp = () => {
      if (interacting) releaseTimer = setTimeout(release, 100)
    }
    window.addEventListener("pointerup", pointerUp)
    window.addEventListener("pointercancel", release)
    return () => {
      window.removeEventListener("pointerup", pointerUp)
      window.removeEventListener("pointercancel", release)
      clearTimeout(releaseTimer)
    }
  })

  $effect(() => {
    if (expanded) return
    void tick().then(() => {
      const textarea = element?.querySelector("textarea")
      if (textarea && document.activeElement !== textarea) {
        textarea.scrollTop = 0
        textarea.scrollLeft = 0
      }
    })
  })
</script>

<form
  bind:this={element}
  class="flex w-full min-w-0 flex-col gap-2 [--composer-control-size:2.75rem] md:[--composer-control-size:2rem]"
  {...rest}
  onfocusin={(event) => {
    if (event.target instanceof HTMLTextAreaElement) editing = true
    rest.onfocusin?.(event)
  }}
  onfocusout={(event) => {
    if (event.target instanceof HTMLTextAreaElement) editing = false
    rest.onfocusout?.(event)
  }}
  onpointerdown={(event) => {
    if (editing && event.target instanceof Element && event.target.closest("button")) {
      clearTimeout(releaseTimer)
      interacting = true
      // Keep the keyboard open until the button's click has been handled.
      // Pointerdown cancellation prevents focus transfer, not the native click.
      event.preventDefault()
    }
    rest.onpointerdown?.(event)
  }}
  onclick={(event) => {
    rest.onclick?.(event)
    release()
  }}
>
  {@render children?.()}
  {#if input}
    <div
      data-slot="composer-controls"
      data-expanded={expanded ? "" : undefined}
      class="group/composer grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 gap-y-2 [&_[data-slot=button]]:size-(--composer-control-size) [&_[data-slot=button]_svg]:size-(--composer-icon-size)"
    >
      <div class={cn("col-start-1 flex items-center gap-1 md:gap-2", expanded ? "row-start-2" : "row-start-1")}>
        {@render leading?.()}
      </div>
      <div class={cn("row-start-1 min-w-0", expanded ? "col-span-3 col-start-1" : "col-start-2")}>
        {@render input()}
      </div>
      <div class={cn("col-start-3", expanded ? "row-start-2" : "row-start-1")}>
        {@render trailing?.()}
      </div>
    </div>
  {/if}
  {@render feedback?.()}
</form>

<style>
  form {
    --composer-icon-size: 1rem;
    --composer-line-height: 1.5rem;
    /* Match the visible icon edge without moving or shrinking its hit area. */
    --composer-inset: calc((var(--composer-control-size) - var(--composer-icon-size)) / 2);
    --composer-compact-padding: calc((var(--composer-control-size) - var(--composer-line-height)) / 2);
  }
</style>
