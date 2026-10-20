# Workspace group administration

Sysadmins manage groups under **System → Workspaces → [workspace] → Groups**.
This area requires `system.manage`; workspace manager status alone does not
grant access. The workspace-facing Groups page continues to show the user's
enabled memberships.

The groups directory supports name/alias search, pagination, and enabled or
disabled filtering. Create a group with a workspace-unique alias and optional
display name. Edit its alias, display name, or availability from its detail page.
Edits retain the group's `grp_…` ID and therefore its memberships and grants.

## Members

Add already-admitted workspace principals using the workspace-scoped picker.
An enabled direct workspace grant is required to add or restore a human
membership. A membership has no role;
roles belong to grants. **Remove membership** disables that membership while
retaining its identity. Use **Enable membership** to restore it. Adding an
already-present membership, including a disabled one, returns a conflict.

## Workspace admission

Workspace membership and roles are assigned directly to principals. Groups
organize people and bots within a workspace; they never admit people or assign
workspace roles. A direct workspace grant is required for all chat and project
access, including resources with retained direct or group grants. Removing the
last enabled workspace grant suspends that access without deleting resource
grants or group memberships. Restoring admission makes retained paths effective
again.
Issued upload/download capabilities carry their principal and workspace, and
the storage proxy rechecks admission on each transfer request. Background replies
and generated chat titles also revalidate the requesting human before publishing.

Migration V063 preserves currently effective group-derived workspace roles as
direct principal grants, deduplicating overlapping paths and preserving existing
direct grant identities. Disabled access paths do not acquire admission. Legacy
group workspace grants are disabled and immutable; they remain solely because
activity history references their identities. New group workspace grants and
restoration of retired grants are prohibited by the database and administration.
The default root bootstrap also preserves prior revocation or role downgrades;
its new direct Manager default cannot silently restore old authority.

Disabling a group suspends all access inherited through it, including access to
projects and chats. Re-enabling it restores access through enabled memberships
and grants. Other direct or group-derived permissions continue to apply.

## Bots

The **Bots** section lists workspace bot bindings assigned to the group, with
search, pagination, and enabled/disabled membership filters. Add a bot using the
workspace-scoped bot picker. A bot can belong to multiple groups, and several
bots backed by the same model can have different memberships.

**Remove membership** disables the association; **Enable membership** restores
it. Membership and bot availability have separate enabled states. Membership
changes use independent revisions, and no-op updates retain the current revision.
Adding an existing association, including a disabled one, returns a conflict.
Bots from another workspace cannot be assigned to the group.

## API

All paths below start with `/api/v1/system/workspaces/{workspace}`:

- `GET /groups?name=…&status=enabled|disabled&limit=50&cursor=…` returns `groups`
  and optional `next_cursor`. Omit `status` to include disabled groups.
- `POST /groups` accepts `alias`, optional `name`, and optional `enabled`.
- `GET /groups/{group}` returns the group, including `revision`.
- `PATCH /groups/{group}` accepts any combination of `alias`, `name`, `enabled`,
  plus `expected_revision`. An empty name clears the display name.
- `GET /groups/{group}/members` supports the same search/filter/pagination
  parameters and returns `members` and optional `next_cursor`.
- `POST /groups/{group}/members` accepts `{"principal":"prn_…"}`.
- `PATCH /groups/{group}/members/{principal}` accepts `enabled` and
  `expected_revision`.
- `GET /groups/{group}/agents` supports the same search/filter/pagination
  parameters, with bot-binding ID cursors, and returns `agents` and optional
  `next_cursor`. Each entry includes `agent`, `alias`, optional `label`,
  `agent_enabled`, `enabled`, and `revision`.
- `POST /groups/{group}/agents` accepts `{"agent":"wag_…"}`.
- `PATCH /groups/{group}/agents/{agent}` accepts `enabled` and
  `expected_revision`, returning `204` on success.
- `GET /groups/{group}/member-options` searches enabled, directly admitted
  workspace principals and returns `principals` and optional `next_cursor`.
- Group workspace-grant endpoints have been removed. Workspace role changes
  are performed through direct principal grants.

Groups and membership changes use revisions, as grants already do. Changes and
activity recording are atomic. Stale revisions and duplicate aliases,
memberships, or grant roles return `409`; invalid input returns `400`; resources
under another workspace return `404`. Creation returns `201`; membership updates
return `204`; other updates return the updated record. No-op saves leave
revisions unchanged. Access-change activity is published to affected principals
so their clients can notice revocation after losing workspace access.

## Configuration reconciliation

Groups and memberships use the same **higher revision wins** policy as grants.
Each config declaration has an optional positive `revision`, defaulting to `1`:

```yaml
workspaces:
  - alias: engineering
    grants:
      - principal: alice
        role: contributor
    groups:
      - alias: developers
        name: Developers
        revision: 3
        members:
          - principal: alice
            revision: 2
            enabled: true
        agents:
          - agent: engineering-assistant
            revision: 2
            enabled: true
```

Missing records are created at their configured revision. Existing records are
updated only when the config revision is strictly higher than their stored
revision; that configured revision becomes the stored revision. Equal or lower
config revisions cannot overwrite admin edits. An actual admin edit increments
the stored revision by one. Group metadata and individual memberships have
independent revisions: raising a group revision does not force membership edits.
To deliberately apply config over an admin change, choose a revision above the
current stored value. Undeclared records are retained.

Existing groups and memberships start at revision `1` when upgrading. Renaming a
configured group requires updating its workspace/alias declaration too;
otherwise reconciliation can recreate the old alias. Grant reconciliation keeps
its existing revision-based behavior.

Bot membership declarations identify the bot by its workspace-local binding
alias. Their revisions follow the same higher-revision-wins policy, independently
of group metadata and human memberships. They reconcile after workspace bot
bindings, so groups, bots, and memberships can be created together on a fresh
deployment. Undeclared bot memberships are retained. Mutations and reconciliation
atomically record `group_agent.create` / `group_agent.update` activity with both
group and bot identities and notifications for the group's human members.

Membership aliases can refer to bots created through administration as well as
configuration, including disabled bots. An unresolved workspace, group, or bot
fails reconciliation with an error naming all three aliases and the missing
resource. Validation happens inside the migration transaction before membership
or activity writes.

PostgreSQL group reconciliation resolves aliases and IDs inside its execution
transaction. It briefly blocks writes to the group and membership tables while
resolving identities and applying changes; ordinary reads remain available.
Memberships and activity records therefore use the actual group identity even
if an admin created or renamed a group after the migration SQL was prepared.

Human memberships reconcile after direct workspace grants (R016 after R007).
The default root principal is admitted directly. Default configuration does not
synthesize a root group or membership; groups are created explicitly. Workspace grants
must name a `principal`; the former `group` recipient is no longer accepted.
When upgrading an existing configuration, remove group-targeted workspace-grant
declarations; V063 preserves the access already effective in the database. Use
explicit principal grants for subsequent configuration-managed role changes.
