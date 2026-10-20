# Workspace grant role administration

Under **System → Workspaces → [workspace] → Principals**, each enabled direct
grant has an **Edit role** action. Select Member, Contributor, or Manager and
save. The operation edits that specific grant; it preserves its `wgr_…` ID,
workspace, principal, and enabled state. Other direct workspace grants still
apply. Group membership never assigns workspace admission or roles.

The principal roster remains grouped by principal. Its API response includes
both the existing `roles` summary and a `grants` array with each enabled direct
grant's ID, workspace, principal, role, enabled status, and revision.

## API

`PATCH /api/v1/system/workspaces/{workspace}/grants/{grant}` requires
`system.manage` and a body such as:

```json
{"role":"contributor","expected_revision":3}
```

The response is the updated principal grant, including its role, enabled status,
and revision. Retired historical group grants cannot be edited or re-enabled and
return `404` through this endpoint.

The endpoint also accepts `enabled` to revoke or restore a single grant, with or
without a role change. Omitted fields are preserved. Removing the last enabled
workspace grant suspends all chat/project access, even through retained resource
grants or group memberships; see [Group administration](group-administration.md).

Actual role changes atomically increment the revision and append a
`workspace_grant.update` activity event. Saving the same role with the current
revision leaves the grant and activity unchanged.

- Invalid roles or missing/nonpositive revisions return `400`.
- Missing grants, or grants under a different workspace, return `404`.
- Stale revisions return `409`; reload the grant before retrying.
- If another grant for the same recipient already has the requested role,
  including a disabled grant, the update returns `409`. Grants are not merged.

## Configuration reconciliation

Configuration matches workspace grants by workspace, principal, and role.
Changing a configured grant's role in the panel does not update its configuration
declaration. A subsequent reconciliation can recreate a grant for the old role.
Keep the configuration declaration in sync when editing configuration-managed
grants.
