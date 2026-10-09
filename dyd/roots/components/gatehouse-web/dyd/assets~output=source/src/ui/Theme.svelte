<script lang="ts">
  import { themeTokens, workspaceTheme } from "./theme.svelte"
  let { workspaceID }: { workspaceID?: string } = $props()
  $effect(() => {
    const theme = workspaceTheme(workspaceID)
    const preference = window.matchMedia("(prefers-color-scheme: dark)")
    const element = document.documentElement
    function apply() {
      const dark =
        theme.mode === "dark" || (theme.mode !== "light" && preference.matches)
      element.classList.toggle("dark", dark)
      element.classList.toggle("scheme-dark", dark)
      element.classList.toggle("scheme-light", !dark)
      const values = dark ? theme.dark : theme.light
      for (const token of themeTokens) {
        element.style.removeProperty(`--${token}`)
        if (values?.[token] !== undefined)
          element.style.setProperty(`--${token}`, values[token])
      }
      if (workspaceID) element.dataset.workspace = workspaceID
      else delete element.dataset.workspace
    }
    apply()
    preference.addEventListener("change", apply)
    return () => {
      preference.removeEventListener("change", apply)
      for (const token of themeTokens)
        element.style.removeProperty(`--${token}`)
      element.classList.remove("dark")
      element.classList.remove("scheme-dark", "scheme-light")
      delete element.dataset.workspace
    }
  })
</script>
