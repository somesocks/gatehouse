<script lang="ts">
  let {
    id,
    value = $bindable(""),
    required = false,
  }: { id: string; value?: string; required?: boolean } = $props()
  let error = $state("")
  function validate(event: Event) {
    const input = event.currentTarget as HTMLTextAreaElement
    try {
      if (input.value.trim()) JSON.parse(input.value)
      error = ""
    } catch {
      error = "Enter valid JSON."
    }
    input.setCustomValidity(error)
  }
</script>

<textarea
  {id}
  {required}
  rows="6"
  bind:value
  oninput={validate}
  aria-invalid={error ? true : undefined}
  aria-describedby={error ? `${id}-error` : undefined}
  class="border-input bg-background focus-visible:ring-ring w-full rounded-md border px-3 py-2 font-mono text-sm focus-visible:ring-2"
></textarea>
{#if error}<p id={`${id}-error`} class="text-destructive text-sm" role="alert">
    {error}
  </p>{/if}
