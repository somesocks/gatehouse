<script lang="ts">
  import { onDestroy } from "svelte"
  import { createLoadState } from "../../../utils/load-state.svelte"
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createSystemGrant } from "../../../app/system"
  import PrincipalPicker from "../principals/PrincipalPicker.svelte"
  const runtime = useRuntime()
  const mutation = createLoadState()
  onDestroy(mutation.stop)
  const saving = $derived(mutation.state.pending)
  let principal = $state(""),
    error = $state("")
  async function create() {
    if (!principal || saving) return
    const request = mutation.begin()
    error = ""
    try {
      const response = await createSystemGrant(principal, request.signal)
      if (!request.current()) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (response.status === 409)
        error =
          "This principal already has a system grant. Restore it from the grant directory if it was revoked."
      else if (!response.ok) error = "System access could not be granted."
      else runtime.navigate("/app/system/grants", true)
    } catch {
      if (request.current()) error = "System access could not be granted."
    } finally {
      request.finish()
    }
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "grants")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "System grants", href: "/app/system/grants" },
    { label: "Add manager" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Add system manager"
      description="Grant an existing principal permission to manage global Gatehouse state."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
      ><UI.FormSection title="Principal"
        ><PrincipalPicker bind:value={principal} /></UI.FormSection
      ><UI.FormSection title="Role"
        ><UI.Badge>System manager</UI.Badge><UI.Text tone="muted" size="small"
          >This grants system administration access. Workspace access is
          assigned separately.</UI.Text
        ></UI.FormSection
      >{#if error}<UI.Notice variant="destructive" role="alert"
          >{error}</UI.Notice
        >{/if}<UI.FormActions
        ><UI.RouterLink variant="outline" href="/app/system/grants"
          >Cancel</UI.RouterLink
        ><UI.Button type="submit" disabled={!principal || saving}
          >{saving ? "Granting…" : "Grant system access"}</UI.Button
        ></UI.FormActions
      ></UI.Form
    >
  </UI.AdminContent>
</UI.AppFrame>
