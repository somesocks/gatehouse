<script lang="ts">
  import * as Alert from "../lib/vendor/shadcn/alert"
  import type { Attributes } from "./types"
  let {
    variant = "default",
    role = variant === "destructive" ? "alert" : "status",
    children,
    ...rest
  }: Attributes & { variant?: "default" | "destructive" } = $props()
</script>

<Alert.Root {variant} {role} {...rest}
  ><Alert.Description>{@render children?.()}</Alert.Description></Alert.Root
>
