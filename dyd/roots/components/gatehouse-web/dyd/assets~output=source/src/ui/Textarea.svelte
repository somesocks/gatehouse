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
      ? "min-h-24 max-h-48 resize-none overflow-y-auto rounded-none border-0 bg-transparent px-0 py-1.5 text-base leading-6 shadow-none focus-visible:border-transparent focus-visible:ring-0 disabled:bg-transparent md:text-base dark:bg-transparent dark:disabled:bg-transparent"
      : undefined}
  {...rest}
/>
