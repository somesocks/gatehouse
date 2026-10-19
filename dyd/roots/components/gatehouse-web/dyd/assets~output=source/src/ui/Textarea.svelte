<script lang="ts">
  import { Textarea } from "../lib/vendor/shadcn/textarea"
  import type { ComponentProps } from "svelte"
  let {
    value = $bindable(),
    ref = $bindable(),
    mode = "default",
    ...rest
  }: Omit<ComponentProps<typeof Textarea>, "class" | "style"> & {
    mode?: "default" | "inline" | "composer"
  } = $props()
</script>

<Textarea
  bind:value
  bind:ref={
    () => ref ?? null,
    (value) => {
      ref = value ?? undefined
    }
  }
  class={mode === "inline"
    ? "h-8 min-h-8 max-h-40 py-1 text-sm leading-5 resize-none"
    : mode === "composer"
      ? "field-sizing-fixed h-11 min-h-11 max-h-11 resize-none overflow-hidden whitespace-pre rounded-none border-0 bg-transparent px-0 py-2.5 text-base leading-6 shadow-none focus-visible:border-transparent focus-visible:ring-0 disabled:bg-transparent group-data-[expanded]/composer:field-sizing-content group-data-[expanded]/composer:h-auto group-data-[expanded]/composer:min-h-[min(6rem,20dvh)] group-data-[expanded]/composer:max-h-[min(12rem,35dvh)] group-data-[expanded]/composer:overflow-y-auto group-data-[expanded]/composer:whitespace-pre-wrap group-data-[expanded]/composer:py-1.5 md:field-sizing-content md:h-auto md:min-h-24 md:max-h-48 md:overflow-y-auto md:whitespace-pre-wrap md:py-1.5 md:text-base dark:bg-transparent dark:disabled:bg-transparent"
      : undefined}
  {...rest}
/>
