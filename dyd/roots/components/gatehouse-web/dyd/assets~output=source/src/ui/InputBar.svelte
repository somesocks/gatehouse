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
    overlayOpen = false,
    ...rest
  }: Omit<HTMLFormAttributes, "class" | "style"> & {
    input?: Snippet
    leading?: Snippet
    trailing?: Snippet
    feedback?: Snippet
    overlayOpen?: boolean
  } = $props()
  let element = $state<HTMLFormElement>()
  let editing = $state(false)
  let interacting = $state(false)
  let releaseTimer: ReturnType<typeof setTimeout> | undefined
  const expanded = $derived(editing || interacting || overlayOpen)

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
  class="flex w-full min-w-0 flex-col gap-2"
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
      class="group/composer grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-end gap-x-2 gap-y-2 [&_[data-slot=button]]:size-11 md:[&_[data-slot=button]]:size-8"
    >
      <div class={cn("col-start-1 flex items-center gap-1 md:row-start-2 md:gap-2", expanded ? "row-start-2" : "row-start-1")}>
        {@render leading?.()}
      </div>
      <div class={cn("row-start-1 min-w-0 md:col-span-3 md:col-start-1", expanded ? "col-span-3 col-start-1" : "col-start-2")}>
        {@render input()}
      </div>
      <div class={cn("col-start-3 md:row-start-2", expanded ? "row-start-2" : "row-start-1")}>
        {@render trailing?.()}
      </div>
    </div>
  {/if}
  {@render feedback?.()}
</form>
