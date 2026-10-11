<script lang="ts">
  let {
    columns,
    message = "No results.",
  }: { columns: number; message?: string } = $props()
</script>

<tr
  ><td
    colspan={columns}
    class="text-muted-foreground px-6 py-12 text-center text-sm">{message}</td
  ></tr
>
