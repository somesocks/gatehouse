<script lang="ts">
  import type { ComponentProps } from "svelte"
  import Button from "./Button.svelte"
  import Spinner from "./Spinner.svelte"
  let {
    pending = false,
    disabled = false,
    children,
    ...rest
  }: ComponentProps<typeof Button> & {
    pending?: boolean
  } = $props()
</script>

<Button {...rest} disabled={disabled || pending} aria-busy={pending}>
  <span class="relative inline-flex items-center justify-center">
    <span class={pending ? "opacity-0" : undefined}>{@render children?.()}</span
    >
    {#if pending}<span class="absolute inset-0 flex items-center justify-center"
        ><Spinner /></span
      >{/if}
  </span>
</Button>
