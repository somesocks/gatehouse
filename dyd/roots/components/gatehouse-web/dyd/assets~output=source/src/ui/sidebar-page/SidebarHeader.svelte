<script lang="ts">
  import { Header } from "../../lib/vendor/shadcn/sidebar"
  import Heading from "../Heading.svelte"
  let { title = "Gatehouse" }: { title?: string } = $props()
</script>

<Header><Heading level={3}>{title}</Heading></Header>
