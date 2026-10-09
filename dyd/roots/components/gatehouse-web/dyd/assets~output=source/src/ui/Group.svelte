<script lang="ts">
  import type { Attributes } from "./types"
  let {
    as = "div",
    grow = false,
    children,
    ...rest
  }: Attributes & {
    as?:
      | "div"
      | "span"
      | "section"
      | "article"
      | "header"
      | "footer"
      | "nav"
      | "li"
      | "dt"
      | "dd"
    grow?: boolean
  } = $props()
</script>

<svelte:element this={as} class={grow ? "min-w-0 flex-1" : "min-w-0"} {...rest}
  >{@render children?.()}</svelte:element
>
