<script lang="ts">
  import { PanelLeft } from "@lucide/svelte"
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let { children: _children }: { children?: Snippet } = $props()
  const pageState = useSidebarPage()
</script>

<button
  aria-haspopup="dialog"
  aria-label="Open navigation"
  class="sidebar-page-trigger icon inline"
  type="button"
  onclick={() => pageState.drawer?.showModal()}
><PanelLeft size={18} strokeWidth={2} aria-hidden="true" /></button>
