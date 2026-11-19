<script lang="ts">
  import { onMount, tick, type Snippet } from "svelte"
  import type { HTMLFormAttributes } from "svelte/elements"

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
    input?: Snippet<[expanded: boolean]>
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
      class="group/composer flex min-w-0 flex-col gap-2"
    >
      {@render input(expanded)}
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-1 md:gap-2">
          {@render leading?.()}
        </div>
        {@render trailing?.()}
      </div>
    </div>
  {/if}
  {@render feedback?.()}
</form>

<style>
  form {
    --composer-line-height: 1.5rem;
    --composer-text-padding: 0.375rem;
    --composer-expanded-size: calc(
      3 * var(--composer-line-height) + 2 * var(--composer-text-padding)
    );
    /* Match the visible icon edge without moving or shrinking its hit area. */
    --composer-inset: calc(
      (var(--icon-button-target) - var(--icon-button-glyph)) / 2
    );
  }
</style>
